package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"vouchchain/pkg/approval"
)

// AI 对话工具注册表（对话即操作）：模型通过 tool calls 调这些函数查询/操作系统。
// 读操作直接执行；写操作（发起审批/审批动作）以对话用户身份落库并全程审计留痕
//（audit_log.action 带 ai_chat 前缀区分），审批事件、站内通知与手工操作走同一套表。

type ChatActor struct {
	Username     string
	Role         string
	Source       string // 工具调出来源：空=网页对话 ai_chat；mcp=外部 MCP 客户端（审计区分）
	SkillsPrompt string // Agent Skill 注入（按用户消息触发词匹配，pkg/skills.MatchPrompt 生成）
}

type StepTrace struct {
	Tool    string `json:"tool"`
	Args    string `json:"args"`
	Summary string `json:"summary"`
	OK      bool   `json:"ok"`
}

type ChatTool struct {
	Name   string
	Desc   string
	Params map[string]*schema.ParameterInfo
	Fn     func(ctx context.Context, args map[string]any, actor ChatActor) (string, error)
}

// ---------- args 取值 helper（LLM 产出的 JSON 数字恒为 float64） ----------

func argStr(args map[string]any, key string) string {
	if v, ok := args[key]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func argInt(args map[string]any, key string) int64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case string:
			var id int64
			_, _ = fmt.Sscanf(strings.TrimSpace(n), "%d", &id)
			return id
		}
	}
	return 0
}

func argFloat(args map[string]any, key string) float64 {
	if v, ok := args[key]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return 0
}

// ---------- 审计留痕（与 controller.audit 同表同字段，ai_chat 前缀区分来源） ----------

func auditAI(actor ChatActor, action, targetType string, targetId int64, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	src := actor.Source
	if src == "" {
		src = "ai_chat"
	}
	detail["source"] = src
	dj, _ := json.Marshal(detail)
	g.DB().Model("audit_log").Data(g.Map{
		"ip":          "ai-chat",
		"username":    actor.Username,
		"action":      action,
		"target_type": targetType,
		"target_id":   targetId,
		"detail":      string(dj),
		"created_at":  time.Now(),
	}).Insert()
}

// notifyAI 站内通知（直插 notification 表；不广播 WebSocket，实时推送下次打开可见，诚实取舍）
func notifyAI(username, ntype, title, content string) {
	if username == "" {
		return
	}
	g.DB().Model("notification").Data(g.Map{
		"username": username, "type": ntype, "title": title, "content": content,
		"is_read": false, "created_at": time.Now(),
	}).Insert()
}

// ---------- 查询类工具 ----------

func toolQueryContracts(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	m := g.DB().Model("contract").Order("id desc").Limit(8)
	if kw := argStr(args, "keyword"); kw != "" {
		m = m.Where("title LIKE ? OR counter_party LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	if cp := argStr(args, "counterParty"); cp != "" {
		m = m.Where("counter_party LIKE ?", "%"+cp+"%")
	}
	if st := argStr(args, "status"); st != "" {
		m = m.Where("status", st)
	}
	rows, err := m.All()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "未找到符合条件的合同。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共列出 %d 份合同：\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&b, "#%d《%s》相对方=%s 金额=%.2f%s 状态=%s 签署=%s\n",
			r["id"].Int64(), r["title"].String(), r["counter_party"].String(),
			r["amount"].Float64(), r["currency"].String(), r["status"].String(), r["sign_date"].String())
	}
	return b.String(), nil
}

func toolGetContract(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	id := argInt(args, "contractId")
	if id <= 0 {
		return "", errors.New("contractId 必填")
	}
	row, err := g.DB().Model("contract").Where("id", id).One()
	if err != nil {
		return "", err
	}
	if row.IsEmpty() {
		return "合同不存在。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "合同 #%d《%s》：相对方=%s 金额=%.2f%s 状态=%s 签署日=%s 项目=%s 负责人=%s\n",
		row["id"].Int64(), row["title"].String(), row["counter_party"].String(),
		row["amount"].Float64(), row["currency"].String(), row["status"].String(),
		row["sign_date"].String(), row["project_code"].String(), row["owner"].String())
	// 关联合同文件与审批状态（按相对方关联，参数化查询）
	files, _ := g.DB().Model("contract_file").Where("counter_party", row["counter_party"].String()).Fields("id,filename,status,approval_state").Limit(5).All()
	if len(files) > 0 {
		b.WriteString("该相对方的合同文件：")
		for _, f := range files {
			fmt.Fprintf(&b, "文件#%d《%s》状态=%s 审批=%s；", f["id"].Int64(), f["filename"].String(), f["status"].String(), f["approval_state"].String())
		}
		b.WriteString("\n")
		ids := []int64{}
		for _, f := range files {
			ids = append(ids, f["id"].Int64())
		}
		obs, _ := g.DB().Model("obligation").Where("file_id IN (?)", ids).Fields("type,description,due_date,status").Limit(6).All()
		if len(obs) > 0 {
			b.WriteString("义务：")
			for _, o := range obs {
				fmt.Fprintf(&b, "[%s|%s|%s] %s；", o["status"].String(), o["type"].String(), o["due_date"].String(), trunc(o["description"].String(), 40))
			}
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

func toolGetPartyProfile(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	name := argStr(args, "name")
	if name == "" {
		return "", errors.New("name 必填（相对方名称，支持模糊）")
	}
	rows, err := g.DB().Model("contract").Fields("DISTINCT counter_party").Where("counter_party LIKE ?", "%"+name+"%").Limit(5).All()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "内部合同库中找不到该相对方（画像仅基于内部合同数据，不含外部工商/司法数据）。", nil
	}
	var b strings.Builder
	for _, r := range rows {
		cp := r["counter_party"].String()
		contracts, _ := g.DB().Model("contract").Where("counter_party", cp).All()
		var total float64
		lastSign := ""
		for _, c := range contracts {
			total += c["amount"].Float64()
			if d := c["sign_date"].String(); d > lastSign {
				lastSign = d
			}
		}
		// 义务按该相对方名下合同文件统计（参数化，避免拼接）
		fileRows, _ := g.DB().Model("contract_file").Fields("id").Where("counter_party", cp).Limit(50).All()
		ids := []int64{}
		for _, f := range fileRows {
			ids = append(ids, f["id"].Int64())
		}
		obsDone, obsAll, obsLate := 0, 0, 0
		if len(ids) > 0 {
			obsAll, _ = g.DB().Model("obligation").Where("file_id IN (?)", ids).Count()
			obsDone, _ = g.DB().Model("obligation").Where("file_id IN (?)", ids).Where("status", "done").Count()
			obsLate, _ = g.DB().Model("obligation").Where("file_id IN (?)", ids).Where("status", "overdue").Count()
		}
		fmt.Fprintf(&b, "相对方「%s」：合同 %d 份、累计金额 %.2f、义务完成 %d/%d（逾期 %d）、最近签署 %s。\n",
			cp, len(contracts), total, obsDone, obsAll, obsLate, lastSign)
	}
	b.WriteString("（数据仅来自内部合同库，不含外部工商/司法数据）")
	return b.String(), nil
}

func toolQueryInvoices(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	m := g.DB().Model("invoice").Order("id desc").Limit(8)
	if kw := argStr(args, "keyword"); kw != "" {
		m = m.Where("invoice_no LIKE ? OR seller_name LIKE ? OR buyer_name LIKE ?", "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	if st := argStr(args, "verifyStatus"); st != "" {
		m = m.Where("verify_status", st)
	}
	rows, err := m.All()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "未找到符合条件的发票。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共列出 %d 张发票：\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&b, "#%d 发票号=%s 销方=%s 价税合计=%.2f 验真=%s\n",
			r["id"].Int64(), r["invoice_no"].String(), r["seller_name"].String(),
			r["total"].Float64(), r["verify_status"].String())
	}
	return b.String(), nil
}

func toolQueryObligations(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	m := g.DB().Model("obligation").Order("id desc").Limit(8)
	if st := argStr(args, "status"); st != "" {
		m = m.Where("status", st)
	}
	if kw := argStr(args, "keyword"); kw != "" {
		m = m.Where("description LIKE ?", "%"+kw+"%")
	}
	rows, err := m.All()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "未找到符合条件的义务。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共列出 %d 条义务：\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&b, "#%d [状态=%s 类型=%s 到期=%s] %s（合同文件#%d）\n",
			r["id"].Int64(), r["status"].String(), r["type"].String(), r["due_date"].String(),
			trunc(r["description"].String(), 50), r["file_id"].Int64())
	}
	return b.String(), nil
}

func toolQueryApprovals(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	m := g.DB().Model("approval_instance").Order("id desc").Limit(8)
	if st := argStr(args, "status"); st != "" {
		m = m.Where("status", st)
	}
	if mine, _ := args["mine"].(bool); mine {
		m = m.Where("created_by", actor.Username)
	}
	rows, err := m.All()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "没有符合条件的审批实例。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共列出 %d 条审批：\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&b, "#%d 对象=%s#%d 状态=%s 当前节点=%d 发起人=%s\n",
			r["id"].Int64(), r["target_type"].String(), r["target_id"].Int64(),
			r["status"].String(), r["current_seq"].Int(), r["created_by"].String())
	}
	return b.String(), nil
}

// ---------- 写操作工具（对话用户身份 + 审计留痕） ----------

func toolStartContractApproval(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	fileId := argInt(args, "contractFileId")
	if fileId <= 0 {
		return "", errors.New("contractFileId 必填（先查合同文件 id）")
	}
	file, err := g.DB().Model("contract_file").Where("id", fileId).One()
	if err != nil {
		return "", err
	}
	if file.IsEmpty() {
		return "合同文件不存在。", nil
	}
	if file["approval_state"].String() == "running" {
		return "该合同文件已有进行中的审批，不能重复发起。", nil
	}
	// 选模板：templateName 模糊 → 第一个
	tplName := argStr(args, "templateName")
	var tpl gdb.Record
	if tplName != "" {
		tpl, err = g.DB().Model("approval_template").Where("name LIKE ?", "%"+tplName+"%").Order("id asc").One()
	} else {
		tpl, err = g.DB().Model("approval_template").Order("id asc").One()
	}
	if err != nil || tpl.IsEmpty() {
		return "找不到审批模板（请先在审批中心建模板）。", nil
	}
	var specs []approval.NodeSpec
	_ = json.Unmarshal([]byte(tpl["nodes"].String()), &specs)
	states, seq, err := approval.Expand(specs)
	if err != nil {
		return "", err
	}
	// 条件分支上下文（与 controller.buildApprovalContext 同口径）
	actx := map[string]any{
		"amount": 0.0, "counterParty": "", "status": "", "riskLevel": "low",
		"projectCode": "", "legalEntityId": 0, "targetType": "contract_file", "targetId": fileId,
		"username": actor.Username, "filename": file["filename"].String(),
	}
	if file["project_code"].String() != "" {
		actx["projectCode"] = file["project_code"].String()
	}
	if c, err := g.DB().Model("contract").Where("id", fileId).One(); err == nil && !c.IsEmpty() {
		actx["amount"] = c["amount"].Float64()
		actx["counterParty"] = c["counter_party"].String()
		actx["status"] = c["status"].String()
		actx["legalEntityId"] = c["legal_entity_id"].Int64()
		if c["project_code"].String() != "" {
			actx["projectCode"] = c["project_code"].String()
		}
	}
	states, seq, instStatus, skips, err := approval.SkipInactive(states, seq, actx)
	if err != nil {
		return "", fmt.Errorf("条件分支求值失败：%w", err)
	}
	ctxJson, _ := json.Marshal(actx)
	now := time.Now()
	res, err := g.DB().Model("approval_instance").Data(g.Map{
		"target_type": "contract_file", "target_id": fileId,
		"template_id": tpl["id"].Int64(), "status": instStatus,
		"current_seq": seq, "created_by": actor.Username, "context": string(ctxJson),
		"created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		return "", err
	}
	instId, _ := res.LastInsertId()
	for _, s := range states {
		g.DB().Model("approval_node").Data(g.Map{
			"instance_id": instId, "seq": s.Seq, "name": s.Name,
			"approver": s.Approver, "mode": s.Mode, "status": s.Status,
			"comment": s.Comment, "when_expr": s.When, "origin": s.Origin,
			"created_at": now,
		}).Insert()
	}
	evtCtx, _ := json.Marshal(actx)
	g.DB().Model("approval_event").Data(g.Map{
		"instance_id": instId, "seq": 0, "actor": actor.Username, "action": "create",
		"from_user": "", "to_user": "", "comment": "AI 对话发起", "detail": string(evtCtx),
		"created_at": now,
	}).Insert()
	state := "running"
	if instStatus != approval.InstanceRunning {
		state = "approved"
		if instStatus == approval.InstanceRejected || instStatus == approval.InstanceRevoked {
			state = "rejected"
		}
	}
	g.DB().Model("contract_file").Data(g.Map{
		"approval_state": state, "updated_at": now,
	}).Where("id", fileId).Update()
	seen := map[string]bool{}
	for _, s := range states {
		if s.Seq == seq && s.Status == approval.NodePending && !seen[s.Approver] {
			seen[s.Approver] = true
			notifyAI(s.Approver, "approval", "新审批任务", fmt.Sprintf("合同文件 #%d 的「%s」待你审批（AI 对话发起）", fileId, s.Name))
		}
	}
	auditAI(actor, "ai_chat.approval.create", "approval_instance", instId, map[string]any{
		"targetId": fileId, "templateId": tpl["id"].Int64(), "status": instStatus, "skipped": len(skips),
	})
	return fmt.Sprintf("已为合同文件 #%d 发起审批（模板「%s」，实例 #%d，状态 %s，条件跳过 %d 个节点），首节点待办已通知。", fileId, tpl["name"].String(), instId, instStatus, len(skips)), nil
}

func toolActApproval(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	instId := argInt(args, "instanceId")
	action := argStr(args, "action")
	if instId <= 0 || (action != "approve" && action != "reject") {
		return "", errors.New("instanceId 必填；action 只能是 approve 或 reject")
	}
	inst, err := g.DB().Model("approval_instance").Where("id", instId).One()
	if err != nil {
		return "", err
	}
	if inst.IsEmpty() {
		return "审批实例不存在。", nil
	}
	if inst["status"].String() != approval.InstanceRunning {
		return "实例已结束（" + inst["status"].String() + "）。", nil
	}
	nodeRows, err := g.DB().Model("approval_node").Where("instance_id", instId).Order("seq asc, id asc").All()
	if err != nil {
		return "", err
	}
	states := []approval.NodeState{}
	for _, row := range nodeRows {
		states = append(states, approval.NodeState{
			Seq: row["seq"].Int(), Name: row["name"].String(), Approver: row["approver"].String(),
			Mode: row["mode"].String(), Status: row["status"].String(), Comment: row["comment"].String(),
			When: row["when_expr"].String(), Origin: row["origin"].String(),
		})
	}
	currentSeq := inst["current_seq"].Int()
	comment := argStr(args, "comment")
	if action == "reject" && comment == "" {
		comment = "AI 对话驳回"
	}
	newStates, instStatus, newSeq, err := approval.Apply(states, currentSeq, approval.Action{
		Actor: actor.Username, Action: action, Comment: comment,
	})
	if err != nil {
		return "审批动作失败：" + err.Error(), nil
	}
	actx := map[string]any{}
	_ = json.Unmarshal([]byte(inst["context"].String()), &actx)
	skips := []approval.SkipRecord{}
	if instStatus == approval.InstanceRunning && newSeq > currentSeq {
		newStates, newSeq, instStatus, skips, err = approval.SkipInactive(newStates, newSeq, actx)
		if err != nil {
			return "", fmt.Errorf("条件分支求值失败：%w", err)
		}
	}
	now := time.Now()
	for _, s := range newStates {
		g.DB().Model("approval_node").Data(g.Map{
			"status": s.Status, "comment": s.Comment, "mode": s.Mode, "acted_at": now,
		}).Where("instance_id", instId).Where("seq", s.Seq).Where("approver", s.Approver).Update()
	}
	g.DB().Model("approval_instance").Data(g.Map{
		"status": instStatus, "current_seq": newSeq, "updated_at": now,
	}).Where("id", instId).Update()
	targetId := inst["target_id"].Int64()
	if inst["target_type"].String() == "contract_file" && instStatus != approval.InstanceRunning {
		state := "approved"
		if instStatus == approval.InstanceRejected || instStatus == approval.InstanceRevoked {
			state = "rejected"
		}
		g.DB().Model("contract_file").Data(g.Map{
			"approval_state": state, "updated_at": now,
		}).Where("id", targetId).Update()
	}
	verb := "已通过"
	if action == "reject" {
		verb = "已驳回"
	}
	curNode := ""
	for _, s := range newStates {
		if s.Seq == currentSeq {
			curNode = s.Name
		}
	}
	evtDetail, _ := json.Marshal(map[string]any{"node": curNode, "result": instStatus, "source": "ai_chat"})
	g.DB().Model("approval_event").Data(g.Map{
		"instance_id": instId, "seq": currentSeq, "actor": actor.Username, "action": action,
		"from_user": "", "to_user": "", "comment": comment, "detail": string(evtDetail),
		"created_at": now,
	}).Insert()
	for _, sk := range skips {
		g.DB().Model("approval_event").Data(g.Map{
			"instance_id": instId, "seq": sk.Seq, "actor": "system", "action": "skip",
			"from_user": "", "to_user": "", "comment": sk.Reason,
			"detail": fmt.Sprintf(`{"node":%q}`, sk.Name), "created_at": now,
		}).Insert()
	}
	notifyAI(inst["created_by"].String(), "approval",
		fmt.Sprintf("审批%s（合同文件 #%d）", verb, targetId),
		fmt.Sprintf("%s 在「%s」节点%s（AI 对话操作）： %s", actor.Username, curNode, verb, comment))
	auditAI(actor, "ai_chat.approval.act", "approval_instance", instId, map[string]any{
		"targetId": targetId, "action": action, "comment": comment, "result": instStatus,
	})
	return fmt.Sprintf("审批 #%d %s（节点「%s」，当前状态 %s）。", instId, verb, curNode, instStatus), nil
}

func trunc(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
