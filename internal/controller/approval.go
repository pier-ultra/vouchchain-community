package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
	"vouchchain/pkg/approval"
	"vouchchain/pkg/ws"
)

// ---------- 审批流（AC6.3 配置 / AC6.4 动作）+ 消息中心 ----------
// 引擎为纯逻辑（pkg/approval，单元测试覆盖或签/会签/驳回/越权），
// 本文件只做存储适配与通知编排——引擎可独立抽出开源。

// notify 站内通知（消息中心）：入库 + WS 实时广播
func notify(username, ntype, title, content string) {
	if username == "" {
		return
	}
	g.DB().Model("notification").Data(g.Map{
		"username": username, "type": ntype, "title": title, "content": content,
		"is_read": false, "created_at": time.Now(),
	}).Insert()
	ws.Broadcast(ws.Event{Type: "notification", Data: g.Map{"username": username, "title": title, "content": content}})
}

func parseNodes(raw string) []approval.NodeSpec {
	var nodes []approval.NodeSpec
	_ = json.Unmarshal([]byte(raw), &nodes)
	return nodes
}

// ApprovalTemplateCreate 创建审批模板（AC6.3）
func ApprovalTemplateCreate(r *ghttp.Request) {
	var req struct {
		Name  string              `json:"name"`
		Nodes []approval.NodeSpec `json:"nodes"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Name == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "模板名必填"})
		return
	}
	if _, _, err := approval.Expand(req.Nodes); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	nj, _ := json.Marshal(req.Nodes)
	now := time.Now()
	res, err := g.DB().Model("approval_template").Data(g.Map{
		"name": req.Name, "nodes": string(nj), "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "approval.template_create", "approval_template", id, g.Map{"name": req.Name})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func ApprovalTemplateList(r *ghttp.Request) {
	list, err := g.DB().Model("approval_template").Order("id desc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ApprovalInstanceCreate 发起审批（AC6.4）：按模板创建实例，通知首节点全部审批人
func ApprovalInstanceCreate(r *ghttp.Request) {
	var req struct {
		TargetType string         `json:"targetType"`
		TargetId   int64          `json:"targetId"`
		TemplateId int64          `json:"templateId"`
		Context    map[string]any `json:"context"` // AG3.2 条件分支的业务上下文（金额/相对方/风险等级…）
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.TargetId <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "targetId 必填"})
		return
	}
	if req.TargetType == "" {
		req.TargetType = "contract_file"
	}
	var specs []approval.NodeSpec
	if req.TemplateId > 0 {
		tpl, err := g.DB().Model("approval_template").Where("id", req.TemplateId).One()
		if err != nil || tpl.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "模板不存在"})
			return
		}
		specs = parseNodes(tpl["nodes"].String())
	}
	states, seq, err := approval.Expand(specs)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	ctx := buildApprovalContext(r, req.TargetType, req.TargetId, req.Context)
	states, seq, instStatus, skips, err := approval.SkipInactive(states, seq, ctx)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "条件分支求值失败：" + err.Error()})
		return
	}
	ctxJson, _ := json.Marshal(ctx)
	now := time.Now()
	creator := r.GetCtxVar("user").String()
	res, err := g.DB().Model("approval_instance").Data(g.Map{
		"target_type": req.TargetType, "target_id": req.TargetId,
		"template_id": req.TemplateId, "status": instStatus,
		"current_seq": seq, "created_by": creator, "context": string(ctxJson),
		"created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
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
	approvalEvent(instId, 0, creator, "create", "", "", "", ctx)
	for _, sk := range skips {
		approvalEvent(instId, sk.Seq, "system", "skip", "", "", sk.Reason, g.Map{"node": sk.Name})
	}
	if instStatus != approval.InstanceRunning {
		state := "approved"
		if instStatus == approval.InstanceRejected {
			state = "rejected"
		}
		g.DB().Model("contract_file").Data(g.Map{
			"approval_state": state, "updated_at": now,
		}).Where("id", req.TargetId).Update()
	} else {
		g.DB().Model("contract_file").Data(g.Map{
			"approval_state": approval.InstanceRunning, "updated_at": now,
		}).Where("id", req.TargetId).Update()
	}
	// 消息中心：只通知跳过条件分支后真正待办的首节点审批人
	seen := map[string]bool{}
	for _, s := range states {
		if s.Seq == seq && s.Status == approval.NodePending && !seen[s.Approver] {
			seen[s.Approver] = true
			notify(s.Approver, "approval", "新审批任务", fmt.Sprintf("合同 #%d 的「%s」待你审批", req.TargetId, s.Name))
		}
	}
	audit(r, "approval.create", "approval_instance", instId, g.Map{
		"targetId": req.TargetId, "templateId": req.TemplateId, "status": instStatus, "context": ctx,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"id": instId, "status": instStatus, "currentSeq": seq, "skipped": len(skips),
	}})
}

// ApprovalInstanceList 审批实例列表（可按 target/status/我的待办过滤）
func ApprovalInstanceList(r *ghttp.Request) {
	targetId := r.Get("targetId").Int64()
	status := r.Get("status").String()
	approver := r.Get("approver").String() // 我的待办：当前节点含我的实例

	m := g.DB().Model("approval_instance")
	if targetId > 0 {
		m = m.Where("target_id", targetId)
	}
	if status != "" {
		m = m.Where("status", status)
	}
	if approver != "" {
		m = m.Where("id IN (SELECT instance_id FROM approval_node WHERE seq = approval_instance.current_seq AND approver = ? AND status = 'pending')", approver)
	}
	list, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ApprovalInstanceDetail 实例详情（含节点行）
func ApprovalInstanceDetail(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	inst, err := g.DB().Model("approval_instance").Where("id", id).One()
	if err != nil || inst.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例不存在"})
		return
	}
	nodes, err := g.DB().Model("approval_node").Where("instance_id", id).Order("seq asc, id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"instance": inst, "nodes": nodes}})
}

// ApprovalAct 审批动作（AC6.4：approve/reject，引擎校验越权/重复/会签或签规则）
func ApprovalAct(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	var req approval.Action
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	inst, err := g.DB().Model("approval_instance").Where("id", id).One()
	if err != nil || inst.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例不存在"})
		return
	}
	if inst["status"].String() != approval.InstanceRunning {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例已结束（" + inst["status"].String() + "）"})
		return
	}
	nodeRows, err := g.DB().Model("approval_node").Where("instance_id", id).Order("seq asc, id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	states := nodeStates(nodeRows)
	currentSeq := inst["current_seq"].Int()
	newStates, instStatus, newSeq, err := approval.Apply(states, currentSeq, req)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	ctx := approvalContext(inst)
	skips := []approval.SkipRecord{}
	// 节点推进后继续求值条件分支：可能连跳几级甚至直接通过
	if instStatus == approval.InstanceRunning && newSeq > currentSeq {
		newStates, newSeq, instStatus, skips, err = approval.SkipInactive(newStates, newSeq, ctx)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "条件分支求值失败：" + err.Error()})
			return
		}
	}
	now := time.Now()
	saveNodeStates(id, newStates, now)
	g.DB().Model("approval_instance").Data(g.Map{
		"status": instStatus, "current_seq": newSeq, "updated_at": now,
	}).Where("id", id).Update()

	// 合同审批状态联动 + 消息通知
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
	if req.Action == "reject" {
		verb = "已驳回"
	}
	approvalEvent(id, currentSeq, req.Actor, req.Action, "", "", req.Comment,
		g.Map{"node": nodeOf(newStates, currentSeq), "result": instStatus})
	for _, sk := range skips {
		approvalEvent(id, sk.Seq, "system", "skip", "", "", sk.Reason, g.Map{"node": sk.Name})
	}
	notify(inst["created_by"].String(), "approval",
		fmt.Sprintf("审批%s（合同 #%d）", verb, targetId),
		fmt.Sprintf("%s 在「%s」节点%s： %s", req.Actor, nodeOf(newStates, currentSeq), verb, req.Comment))
	if instStatus == approval.InstanceRunning && newSeq > currentSeq {
		seen := map[string]bool{}
		for _, s := range newStates {
			if s.Seq == newSeq && s.Status == approval.NodePending && !seen[s.Approver] {
				seen[s.Approver] = true
				notify(s.Approver, "approval", "新审批任务", fmt.Sprintf("合同 #%d 的「%s」待你审批", targetId, s.Name))
			}
		}
	}
	audit(r, "approval.act", "approval_instance", id, g.Map{
		"actor": req.Actor, "action": req.Action, "comment": req.Comment, "result": instStatus,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"status": instStatus, "currentSeq": newSeq}})
}

// ---------- AG3.2/AG3.4/AG3.6 条件分支、加签、转交、废止与留痕 ----------

func nodeStates(rows gdb.Result) []approval.NodeState {
	out := make([]approval.NodeState, 0, len(rows))
	for _, row := range rows {
		out = append(out, approval.NodeState{
			Seq: row["seq"].Int(), Name: row["name"].String(), Approver: row["approver"].String(),
			Mode: row["mode"].String(), Status: row["status"].String(), Comment: row["comment"].String(),
			When: row["when_expr"].String(), Origin: row["origin"].String(),
		})
	}
	return out
}

func approvalContext(inst gdb.Record) map[string]any {
	ctx := map[string]any{}
	_ = json.Unmarshal([]byte(inst["context"].String()), &ctx)
	return ctx
}

func saveNodeStates(instId int64, states []approval.NodeState, now time.Time) {
	for _, s := range states {
		g.DB().Model("approval_node").Data(g.Map{
			"status": s.Status, "comment": s.Comment, "mode": s.Mode, "acted_at": now,
		}).Where("instance_id", instId).Where("seq", s.Seq).Where("approver", s.Approver).Update()
	}
}

func approvalEvent(instId int64, seq int, actor, action, from, to, comment string, detail any) {
	dj, _ := json.Marshal(detail)
	g.DB().Model("approval_event").Data(g.Map{
		"instance_id": instId, "seq": seq, "actor": actor, "action": action,
		"from_user": from, "to_user": to, "comment": comment,
		"detail": string(dj), "created_at": time.Now(),
	}).Insert()
}

// buildApprovalContext 条件分支的业务上下文：默认值 → 业务表实值 → 发起请求显式覆盖。
// 给默认值是为了模板条件引用了尚未落库的字段时不至于让流程发起失败。
func buildApprovalContext(r *ghttp.Request, targetType string, targetId int64, override map[string]any) map[string]any {
	ctx := map[string]any{
		"amount": 0.0, "counterParty": "", "status": "", "riskLevel": "low",
		"projectCode": "", "legalEntityId": 0, "targetType": targetType, "targetId": targetId,
		"username": r.GetCtxVar("user").String(),
	}
	if row, err := g.DB().Model("contract").Where("id", targetId).One(); err == nil && !row.IsEmpty() {
		ctx["amount"] = row["amount"].Float64()
		ctx["counterParty"] = row["counter_party"].String()
		ctx["status"] = row["status"].String()
		ctx["legalEntityId"] = row["legal_entity_id"].Int64()
		ctx["projectCode"] = row["project_code"].String()
	}
	if row, err := g.DB().Model("contract_file").Where("id", targetId).One(); err == nil && !row.IsEmpty() {
		ctx["filename"] = row["filename"].String()
		if ctx["projectCode"] == "" {
			ctx["projectCode"] = row["project_code"].String()
		}
	}
	for k, v := range override {
		ctx[k] = v
	}
	return ctx
}

func runningApproval(r *ghttp.Request) (int64, gdb.Record, []approval.NodeState, int, bool) {
	id := r.Get("id").Int64()
	inst, err := g.DB().Model("approval_instance").Where("id", id).One()
	if err != nil || inst.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例不存在"})
		return 0, nil, nil, 0, false
	}
	if inst["status"].String() != approval.InstanceRunning {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例已结束（" + inst["status"].String() + "）"})
		return 0, nil, nil, 0, false
	}
	rows, err := g.DB().Model("approval_node").Where("instance_id", id).Order("seq asc, id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return 0, nil, nil, 0, false
	}
	return id, inst, nodeStates(rows), inst["current_seq"].Int(), true
}

// ApprovalAddSign 当前节点加签（AG3.4）：插入审批人，默认把该级转为会签
func ApprovalAddSign(r *ghttp.Request) {
	id, inst, states, seq, ok := runningApproval(r)
	if !ok {
		return
	}
	var req struct {
		Actor   string `json:"actor"`
		To      string `json:"to"`
		Mode    string `json:"mode"`
		Comment string `json:"comment"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	newStates, err := approval.AddSign(states, seq, req.Actor, req.To, req.Mode, req.Comment)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	now := time.Now()
	mode := req.Mode
	if mode != approval.ModeOr {
		mode = approval.ModeAnd
	}
	// 现有行改会签；新审批人单独插一行
	g.DB().Model("approval_node").Data(g.Map{"mode": mode}).Where("instance_id", id).Where("seq", seq).Update()
	g.DB().Model("approval_node").Data(g.Map{
		"instance_id": id, "seq": seq, "name": nodeOf(newStates, seq),
		"approver": req.To, "mode": mode, "status": approval.NodePending,
		"comment": req.Comment, "origin": "addsign", "when_expr": "", "created_at": now,
	}).Insert()
	approvalEvent(id, seq, req.Actor, "addsign", req.Actor, req.To, req.Comment, g.Map{"mode": mode})
	notify(req.To, "approval", "你被加签为审批人",
		fmt.Sprintf("合同 #%d 的「%s」节点需要你审批（%s 加签）", inst["target_id"].Int64(), nodeOf(newStates, seq), req.Actor))
	audit(r, "approval.addsign", "approval_instance", id, g.Map{"actor": req.Actor, "to": req.To, "mode": mode})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"seq": seq, "mode": mode}})
}

// ApprovalTransfer 转交（AG3.4）：当前待办任务改派他人，原审批人不再可审
func ApprovalTransfer(r *ghttp.Request) {
	id, inst, states, seq, ok := runningApproval(r)
	if !ok {
		return
	}
	var req struct {
		Actor   string `json:"actor"`
		To      string `json:"to"`
		Comment string `json:"comment"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if _, err := approval.Transfer(states, seq, req.Actor, req.To, req.Comment); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if _, err := g.DB().Model("approval_node").Data(g.Map{
		"approver": req.To, "origin": "transfer",
		"comment": fmt.Sprintf("由 %s 转交：%s", req.Actor, req.Comment),
	}).Where("instance_id", id).Where("seq", seq).Where("approver", req.Actor).Where("status", approval.NodePending).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	approvalEvent(id, seq, req.Actor, "transfer", req.Actor, req.To, req.Comment, nil)
	notify(req.To, "approval", "转交给你的审批任务",
		fmt.Sprintf("合同 #%d 的「%s」节点由 %s 转交给你", inst["target_id"].Int64(), nodeOf(states, seq), req.Actor))
	audit(r, "approval.transfer", "approval_instance", id, g.Map{"actor": req.Actor, "to": req.To})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"seq": seq, "approver": req.To}})
}

// ApprovalCancel 废止（AG3.4）：终止运行中流程，只有发起人或管理员可操作
func ApprovalCancel(r *ghttp.Request) {
	id := r.Get("id").Int64()
	inst, err := g.DB().Model("approval_instance").Where("id", id).One()
	if err != nil || inst.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "实例不存在"})
		return
	}
	if inst["status"].String() != approval.InstanceRunning {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "只有进行中的流程可以废止"})
		return
	}
	var req struct {
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	username := r.GetCtxVar("user").String()
	if req.Actor == "" {
		req.Actor = username
	}
	if username != inst["created_by"].String() && middleware.CurrentRole(r) != "admin" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "只有流程发起人或管理员可以废止"})
		return
	}
	rows, _ := g.DB().Model("approval_node").Where("instance_id", id).Order("seq asc, id asc").All()
	states := nodeStates(rows)
	if _, err := approval.Cancel(states, req.Reason); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	now := time.Now()
	g.DB().Model("approval_node").Data(g.Map{
		"status": approval.NodeRevoked, "comment": "流程废止：" + req.Reason, "acted_at": now,
	}).Where("instance_id", id).Where("status", approval.NodePending).Update()
	g.DB().Model("approval_instance").Data(g.Map{
		"status": approval.InstanceRevoked, "revoked_by": req.Actor, "revoked_at": now, "updated_at": now,
	}).Where("id", id).Update()
	if inst["target_type"].String() == "contract_file" {
		g.DB().Model("contract_file").Data(g.Map{
			"approval_state": "revoked", "updated_at": now,
		}).Where("id", inst["target_id"].Int64()).Update()
	}
	approvalEvent(id, inst["current_seq"].Int(), req.Actor, "revoke", "", "", req.Reason, nil)
	notify(inst["created_by"].String(), "approval", "流程已废止",
		fmt.Sprintf("合同 #%d 的审批流程被 %s 废止：%s", inst["target_id"].Int64(), req.Actor, req.Reason))
	audit(r, "approval.revoke", "approval_instance", id, g.Map{"actor": req.Actor, "reason": req.Reason})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"status": approval.InstanceRevoked}})
}

// ApprovalEvents AG3.6 全程留痕
func ApprovalEvents(r *ghttp.Request) {
	id := r.Get("id").Int64()
	rows, err := g.DB().Model("approval_event").Where("instance_id", id).Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func nodeOf(states []approval.NodeState, seq int) string {
	for _, s := range states {
		if s.Seq == seq {
			return s.Name
		}
	}
	return ""
}

// ---------- 消息中心 ----------

func NotificationList(r *ghttp.Request) {
	username := r.GetCtxVar("user").String()
	list, err := g.DB().Model("notification").Where("username", username).Order("id desc").Limit(100).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	unread, _ := g.DB().Model("notification").Where("username", username).Where("is_read", false).Count()
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"list": list, "unread": unread}})
}

func NotificationRead(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id > 0 {
		g.DB().Model("notification").Data(g.Map{"is_read": true}).Where("id", id).Update()
	}
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func NotificationReadAll(r *ghttp.Request) {
	username := r.GetCtxVar("user").String()
	g.DB().Model("notification").Data(g.Map{"is_read": true}).Where("username", username).Update()
	r.Response.WriteJsonExit(g.Map{"code": 0})
}
