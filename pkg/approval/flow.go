package approval

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// AG3.2 条件分支 / AG3.4 转交·加签·废止：仍然全部是纯逻辑，存储适配在 controller 层。

func asNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func asText(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(v)
	}
}

// Match 判定条件是否成立；无条件（Field 为空）恒为 true。
func (c Condition) Match(ctx map[string]any) (bool, error) {
	if strings.TrimSpace(c.Field) == "" {
		return true, nil
	}
	left, exists := ctx[c.Field]
	if !exists {
		return false, fmt.Errorf("业务上下文缺少字段 %s", c.Field)
	}
	op := strings.ToLower(strings.TrimSpace(c.Op))
	if op == "" {
		op = "eq"
	}
	switch op {
	case "eq":
		return strings.EqualFold(asText(left), c.Value), nil
	case "ne":
		return !strings.EqualFold(asText(left), c.Value), nil
	case "contains":
		return strings.Contains(asText(left), c.Value), nil
	case "in":
		for _, v := range strings.Split(c.Value, ",") {
			if strings.EqualFold(strings.TrimSpace(v), asText(left)) {
				return true, nil
			}
		}
		return false, nil
	case "gt", "gte", "lt", "lte":
		l, ok1 := asNumber(left)
		r, ok2 := asNumber(c.Value)
		if !ok1 || !ok2 {
			return false, fmt.Errorf("字段 %s 与阈值 %q 无法做数值比较", c.Field, c.Value)
		}
		switch op {
		case "gt":
			return l > r, nil
		case "gte":
			return l >= r, nil
		case "lt":
			return l < r, nil
		default:
			return l <= r, nil
		}
	default:
		return false, fmt.Errorf("不支持的条件操作符 %s", c.Op)
	}
}

// SkipRecord 供调用方写流程留痕（AG3.6）
type SkipRecord struct {
	Seq    int
	Name   string
	Reason string
}

// SkipInactive 从当前 seq 起，把条件不成立的整级节点标为 skipped 并自动跳到下一级。
// 发起时与每次节点推进后都要调用一次；若所有剩余节点都被跳过，实例直接 approved。
func SkipInactive(states []NodeState, seq int, ctx map[string]any) ([]NodeState, int, string, []SkipRecord, error) {
	out := make([]NodeState, len(states))
	copy(out, states)
	maxSeq := 0
	for _, s := range out {
		if s.Seq > maxSeq {
			maxSeq = s.Seq
		}
	}
	records := []SkipRecord{}
	for seq <= maxSeq {
		rows := rowsOfSeq(out, seq)
		if len(rows) == 0 {
			seq++
			continue
		}
		// 已有非 pending 行说明该级正在处理或已处理完，不回退跳过
		active := false
		for _, s := range rows {
			if s.Status != NodePending {
				active = true
			}
		}
		if active {
			break
		}
		cond := rows[0].When
		if cond == "" {
			break
		}
		var c Condition
		if err := json.Unmarshal([]byte(cond), &c); err != nil {
			return nil, 0, "", nil, fmt.Errorf("第 %d 级条件解析失败：%w", seq, err)
		}
		ok, err := c.Match(ctx)
		if err != nil {
			return nil, 0, "", nil, err
		}
		if ok {
			break
		}
		reason := fmt.Sprintf("条件 %s %s %s 不成立，自动跳过", c.Field, c.Op, c.Value)
		for i := range out {
			if out[i].Seq == seq && out[i].Status == NodePending {
				out[i].Status = NodeSkipped
				out[i].Comment = reason
			}
		}
		records = append(records, SkipRecord{Seq: seq, Name: rows[0].Name, Reason: reason})
		seq++
	}
	if seq > maxSeq {
		return out, seq, InstanceApproved, records, nil
	}
	return out, seq, InstanceRunning, records, nil
}

// AddSign 当前节点加签：由该节点待办审批人发起，插入一名新审批人。
// 默认把该级改为会签（加的人必须同意）；显式 Mode=or 则保持或签。
func AddSign(states []NodeState, seq int, actor, to, mode, comment string) ([]NodeState, error) {
	to = strings.TrimSpace(to)
	if actor == "" || to == "" {
		return nil, errors.New("加签人与被加签人都必填")
	}
	rows := rowsOfSeq(states, seq)
	actorPending := false
	for _, s := range rows {
		if s.Approver == actor && s.Status == NodePending {
			actorPending = true
		}
		if s.Approver == to && s.Status != NodeSkipped {
			return nil, fmt.Errorf("%s 已是本节点审批人，不能重复加签", to)
		}
	}
	if !actorPending {
		return nil, fmt.Errorf("%s 不是当前节点待办审批人，无权加签", actor)
	}
	if mode != ModeOr {
		mode = ModeAnd
	}
	out := make([]NodeState, 0, len(states)+1)
	last := -1
	for i, s := range states {
		out = append(out, s)
		if s.Seq == seq {
			last = i
			if mode == ModeAnd {
				out[len(out)-1].Mode = ModeAnd
			}
		}
	}
	node := NodeState{Seq: seq, Name: rows[0].Name, Approver: to, Mode: mode,
		Status: NodePending, Comment: comment, Origin: "addsign"}
	// 插入到本 seq 最后一行之后，保持按 seq 升序
	out = append(out, NodeState{})
	copy(out[last+2:], out[last+1:])
	out[last+1] = node
	return out, nil
}

// Transfer 转交：把当前节点某位待办审批人的任务转给他人（原行改归属，留痕在事件表）。
func Transfer(states []NodeState, seq int, actor, to, comment string) ([]NodeState, error) {
	to = strings.TrimSpace(to)
	if actor == "" || to == "" || actor == to {
		return nil, errors.New("转交人与接收人必填且不能相同")
	}
	out := make([]NodeState, len(states))
	copy(out, states)
	hit := false
	for i, s := range out {
		if s.Seq != seq {
			continue
		}
		if s.Approver == to && s.Status != NodeSkipped {
			return nil, fmt.Errorf("%s 已是本节点审批人，不能转交给 TA", to)
		}
		if s.Approver == actor && s.Status == NodePending {
			hit = true
			out[i].Approver = to
			out[i].Origin = "transfer"
			out[i].Comment = fmt.Sprintf("由 %s 转交：%s", actor, comment)
		}
	}
	if !hit {
		return nil, fmt.Errorf("%s 在当前节点没有待办任务，无法转交", actor)
	}
	return out, nil
}

// Cancel 废止：终止运行中的流程，全部待办节点置 revoked；已审批节点保持原状留痕。
func Cancel(states []NodeState, reason string) ([]NodeState, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("废止必须填写原因")
	}
	out := make([]NodeState, len(states))
	copy(out, states)
	for i := range out {
		if out[i].Status == NodePending {
			out[i].Status = NodeRevoked
			out[i].Comment = "流程废止：" + strings.TrimSpace(reason)
		}
	}
	return out, nil
}
