package approval

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// 轻量审批引擎（纯逻辑，零外部依赖——按可抽取为独立开源库的标准编写）。
//
// 模型：审批实例 = 有序节点序列；节点 = 多审批人 + 或签(or)/会签(and)
// 状态机：
//   - 节点逐级推进（seq 1→N）
//   - 或签：当前 seq 任一审批人 approve → 节点过，进下一 seq
//   - 会签：当前 seq 全部审批人 approve → 节点过，进下一 seq
//   - 任一审批人 reject → 实例整体 rejected，流程终止
//   - 最后一个 seq 过 → 实例 approved
//
// 引擎不关心存储与身份来源（DB/HTTP 由调用方适配），输入输出皆为纯数据。

const (
	ModeOr  = "or"
	ModeAnd = "and"

	NodePending  = "pending"
	NodeApproved = "approved"
	NodeRejected = "rejected"
	NodeSkipped  = "skipped" // AG3.2 条件分支：条件不成立的节点整级跳过
	NodeRevoked  = "revoked" // AG3.4 废止：运行中的流程被发起人终止

	InstanceRunning  = "running"
	InstanceApproved = "approved"
	InstanceRejected = "rejected"
	InstanceRevoked  = "revoked"
)

// Condition AG3.2 条件分支：按业务上下文决定该节点是否启用。
// Field 支持业务上下文字段（amount / counterParty / status / riskLevel / projectCode ...）。
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq / ne / gt / gte / lt / lte / contains / in
	Value string `json:"value"`
}

func (c Condition) String() string {
	if strings.TrimSpace(c.Field) == "" {
		return ""
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// NodeSpec 模板节点定义
type NodeSpec struct {
	Name      string     `json:"name"`
	Approvers []string   `json:"approvers"` // 节点审批人列表（≥1）
	Mode      string     `json:"mode"`      // or / and
	When      *Condition `json:"when,omitempty"`
}

// NodeState 实例节点运行态（每审批人一行）
type NodeState struct {
	Seq      int    `json:"seq"`
	Name     string `json:"name"`
	Approver string `json:"approver"`
	Mode     string `json:"mode"`
	Status   string `json:"status"` // pending / approved / rejected / skipped / revoked
	Comment  string `json:"comment"`
	When     string `json:"when"`   // Condition JSON（空=无条件）
	Origin   string `json:"origin"` // template / addsign / transfer
}

// Action 一次审批动作
type Action struct {
	Actor   string `json:"actor"`   // 以谁的身份审批（须为当前节点审批人之一）
	Action  string `json:"action"`  // approve / reject
	Comment string `json:"comment"` // 意见（驳回建议必填，前端强制）
}

// Expand 按模板展开实例节点行
func Expand(specs []NodeSpec) ([]NodeState, int, error) {
	if len(specs) == 0 {
		return nil, 0, errors.New("审批流程至少需要一个节点")
	}
	var states []NodeState
	for i, s := range specs {
		if len(s.Approvers) == 0 {
			return nil, 0, fmt.Errorf("第 %d 个节点未配置审批人", i+1)
		}
		mode := s.Mode
		if mode != ModeOr && mode != ModeAnd {
			mode = ModeOr
		}
		when := ""
		if s.When != nil && strings.TrimSpace(s.When.Field) != "" {
			when = s.When.String()
		}
		for _, a := range s.Approvers {
			if strings.TrimSpace(a) == "" {
				return nil, 0, fmt.Errorf("第 %d 个节点存在空审批人", i+1)
			}
			states = append(states, NodeState{
				Seq: i + 1, Name: s.Name, Approver: strings.TrimSpace(a), Mode: mode,
				Status: NodePending, When: when, Origin: "template",
			})
		}
	}
	return states, 1, nil
}

// Apply 对运行中的实例施加一个动作，返回新状态集与实例状态、当前 seq。
// 输入 states 应为实例全部节点行（按 seq 升序），currentSeq 为实例当前节点序号。
func Apply(states []NodeState, currentSeq int, act Action) ([]NodeState, string, int, error) {
	if act.Actor == "" {
		return nil, "", 0, errors.New("缺少审批人身份")
	}
	if act.Action != "approve" && act.Action != "reject" {
		return nil, "", 0, errors.New("action 取值非法")
	}
	if act.Action == "reject" && strings.TrimSpace(act.Comment) == "" {
		return nil, "", 0, errors.New("驳回必须填写意见")
	}

	// 找当前节点中该审批人的 pending 行
	idx := -1
	for i, s := range states {
		if s.Seq == currentSeq && s.Approver == act.Actor && s.Status == NodePending {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, "", 0, fmt.Errorf("当前节点(%d)不存在待你审批的任务（或已审批/非当前节点）", currentSeq)
	}

	out := make([]NodeState, len(states))
	copy(out, states)
	if act.Action == "approve" {
		out[idx].Status = NodeApproved
	} else {
		out[idx].Status = NodeRejected
		out[idx].Comment = strings.TrimSpace(act.Comment)
	}

	newStatus := InstanceRunning
	newSeq := currentSeq

	if act.Action == "reject" {
		return out, InstanceRejected, newSeq, nil
	}

	// 判定当前节点是否完成
	mode := out[idx].Mode
	seqRows := rowsOfSeq(out, currentSeq)
	done := false
	if mode == ModeOr {
		for _, s := range seqRows {
			if s.Status == NodeApproved {
				done = true
			}
		}
	} else {
		done = true
		for _, s := range seqRows {
			if s.Status != NodeApproved {
				done = false
			}
		}
	}
	if !done {
		return out, newStatus, newSeq, nil
	}

	// 节点完成 → 下一节点或实例通过
	maxSeq := 0
	for _, s := range out {
		if s.Seq > maxSeq {
			maxSeq = s.Seq
		}
	}
	if currentSeq >= maxSeq {
		return out, InstanceApproved, newSeq, nil
	}
	newSeq = currentSeq + 1
	return out, newStatus, newSeq, nil
}

func rowsOfSeq(states []NodeState, seq int) []NodeState {
	var out []NodeState
	for _, s := range states {
		if s.Seq == seq {
			out = append(out, s)
		}
	}
	return out
}
