package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/approval"
	"vouchchain/pkg/seal"
)

// 用印管理接口（v23 R-B）：
//   POST   /api/seals                  印章登记
//   GET    /api/seals                  印章台账
//   PUT    /api/seals/{id}/status      印章状态变更（停用/销毁/恢复）
//   POST   /api/seal-applies           用印申请
//   GET    /api/seal-applies?status=   申请/台账列表
//   POST   /api/seal-applies/{id}/approve  审批
//   POST   /api/seal-applies/{id}/stamp    用印登记（七字段）
//   GET    /api/seal-risks             风险预警扫描

func SealCreate(r *ghttp.Request) {
	var req struct {
		Name   string `json:"name" v:"required"`
		Type   string `json:"type"`
		Keeper string `json:"keeper"`
		Remark string `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Type == "" {
		req.Type = "contract"
	}
	res, err := g.DB().Model("seal").Data(g.Map{
		"name": req.Name, "type": req.Type, "keeper": req.Keeper,
		"status": "active", "remark": req.Remark,
		"creator": r.GetCtxVar("user").String(), "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "seal.create", "seal", id, g.Map{"name": req.Name, "type": req.Type})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func SealList(r *ghttp.Request) {
	rows, err := g.DB().Model("seal").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Map())
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func SealStatus(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Status string `json:"status" v:"required"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	g.DB().Model("seal").Data(g.Map{"status": req.Status}).Where("id", id).Update()
	audit(r, "seal.status", "seal", id, g.Map{"status": req.Status})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func SealApplyCreate(r *ghttp.Request) {
	var req struct {
		SealID  int64  `json:"sealId" v:"required"`
		BizType string `json:"bizType"`
		BizID   int64  `json:"bizId"`
		FileID  int64  `json:"fileId"`
		Copies  int    `json:"copies"`
		Reason  string `json:"reason" v:"required"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Copies <= 0 {
		req.Copies = 1
	}
	if req.BizType == "" {
		req.BizType = "contract"
	}
	creator := r.GetCtxVar("user").String()
	id, warnings, err := seal.CreateApply(r.Context(), req.SealID, req.BizType, req.BizID, req.FileID, req.Copies, req.Reason, creator)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// R-B3：按印章类型解析审批模板；配置了 → 启动审批实例并挂接（未配置回退简化审批）
	instID := int64(0)
	if sealType := sealTypeOf(r, req.SealID); sealType != "" {
		if tplID := seal.ResolveApprovalTemplate(r.Context(), sealType); tplID > 0 {
			if iid, e := sealStartApproval(r, id, sealType, tplID); e == nil {
				instID = iid
				_ = seal.LinkApprovalInstance(r.Context(), id, instID)
			} else {
				warnings = append(warnings, "审批实例创建失败（已回退简化审批）："+e.Error())
			}
		}
	}
	audit(r, "seal.apply", "seal_apply", id, g.Map{"sealId": req.SealID, "warnings": warnings, "approvalInstanceId": instID})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "warnings": warnings, "approvalInstanceId": instID}})
}

// sealTypeOf 取印章类型（R-B3 按类型配置审批模板）
func sealTypeOf(r *ghttp.Request, sealID int64) string {
	row, err := g.DB().Model("seal").Where("id", sealID).One()
	if err != nil || row.IsEmpty() {
		return ""
	}
	return row["type"].String()
}

func SealApplyList(r *ghttp.Request) {
	m := g.DB().Model("seal_apply")
	if v := r.Get("status").String(); v != "" {
		m = m.Where("status", v)
	}
	if v := r.Get("sealId").Int64(); v > 0 {
		m = m.Where("seal_id", v)
	}
	rows, err := m.Order("id desc").Limit(100).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Map())
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func SealApplyApprove(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Approve bool   `json:"approve"`
		Comment string `json:"comment"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	approver := r.GetCtxVar("user").String()
	// R-B3：已挂审批实例 → 走审批引擎（复用，支持多级/或签/会签/条件分支）；未挂 → 回退简化审批
	if instID := seal.ApplyApprovalInstanceID(r.Context(), id); instID > 0 {
		if err := sealEngineAct(r, id, instID, approver, req.Approve, req.Comment); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		r.Response.WriteJsonExit(g.Map{"code": 0})
		return
	}
	if err := seal.Approve(r.Context(), id, approver, req.Approve, req.Comment); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.approve", "seal_apply", id, g.Map{"approve": req.Approve, "approver": approver})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// sealStartApproval R-B3：为用印申请创建审批实例（target_type=seal_apply），按印章类型模板展开
func sealStartApproval(r *ghttp.Request, applyID int64, sealType string, templateID int64) (int64, error) {
	tpl, err := g.DB().Model("approval_template").Where("id", templateID).One()
	if err != nil || tpl.IsEmpty() {
		return 0, fmt.Errorf("审批模板不存在")
	}
	specs := parseNodes(tpl["nodes"].String())
	states, seq, err := approval.Expand(specs)
	if err != nil {
		return 0, err
	}
	creator := r.GetCtxVar("user").String()
	ctx := map[string]any{
		"amount": 0.0, "counterParty": "", "status": "pending", "riskLevel": "low",
		"projectCode": "", "legalEntityId": 0,
		"targetType": "seal_apply", "targetId": applyID, "sealType": sealType, "username": creator,
	}
	states, seq, instStatus, skips, err := approval.SkipInactive(states, seq, ctx)
	if err != nil {
		return 0, fmt.Errorf("条件分支求值失败：%w", err)
	}
	ctxJson, _ := json.Marshal(ctx)
	now := time.Now()
	res, err := g.DB().Model("approval_instance").Data(g.Map{
		"target_type": "seal_apply", "target_id": applyID, "template_id": templateID,
		"status": instStatus, "current_seq": seq, "created_by": creator,
		"context": string(ctxJson), "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		return 0, err
	}
	instID, _ := res.LastInsertId()
	for _, s := range states {
		_, _ = g.DB().Model("approval_node").Data(g.Map{
			"instance_id": instID, "seq": s.Seq, "name": s.Name, "approver": s.Approver,
			"mode": s.Mode, "status": s.Status, "comment": s.Comment,
			"when_expr": s.When, "origin": s.Origin, "created_at": now,
		}).Insert()
	}
	approvalEvent(instID, 0, creator, "create", "", "", "", ctx)
	for _, sk := range skips {
		approvalEvent(instID, sk.Seq, "system", "skip", "", "", sk.Reason, g.Map{"node": sk.Name})
	}
	seen := map[string]bool{}
	for _, s := range states {
		if s.Seq == seq && s.Status == approval.NodePending && !seen[s.Approver] {
			seen[s.Approver] = true
			notify(s.Approver, "approval", "新审批任务", fmt.Sprintf("用印申请 #%d 的「%s」待你审批", applyID, s.Name))
		}
	}
	audit(r, "approval.create", "approval_instance", instID, g.Map{
		"targetType": "seal_apply", "targetId": applyID, "templateId": templateID, "status": instStatus,
	})
	return instID, nil
}

// sealEngineAct R-B3：对用印申请的审批实例执行一次审批动作；实例终态回写 seal_apply.status
func sealEngineAct(r *ghttp.Request, applyID, instID int64, actor string, approve bool, comment string) error {
	inst, err := g.DB().Model("approval_instance").Where("id", instID).One()
	if err != nil || inst.IsEmpty() {
		return fmt.Errorf("审批实例不存在")
	}
	if inst["status"].String() != approval.InstanceRunning {
		return fmt.Errorf("审批已结束（%s）", inst["status"].String())
	}
	nodeRows, err := g.DB().Model("approval_node").Where("instance_id", instID).Order("seq asc, id asc").All()
	if err != nil {
		return err
	}
	states := nodeStates(nodeRows)
	currentSeq := inst["current_seq"].Int()
	action := approval.Action{Actor: actor, Action: "approve", Comment: comment}
	if !approve {
		action.Action = "reject"
	}
	newStates, instStatus, newSeq, err := approval.Apply(states, currentSeq, action)
	if err != nil {
		return err
	}
	ctx := approvalContext(inst)
	skips := []approval.SkipRecord{}
	if instStatus == approval.InstanceRunning && newSeq > currentSeq {
		newStates, newSeq, instStatus, skips, err = approval.SkipInactive(newStates, newSeq, ctx)
		if err != nil {
			return fmt.Errorf("条件分支求值失败：%w", err)
		}
	}
	now := time.Now()
	saveNodeStates(instID, newStates, now)
	_, _ = g.DB().Model("approval_instance").Data(g.Map{
		"status": instStatus, "current_seq": newSeq, "updated_at": now,
	}).Where("id", instID).Update()

	// 终态回写用印申请状态（approved→可登记用印；rejected→留痕）
	switch instStatus {
	case approval.InstanceApproved:
		_ = seal.MarkApproved(r.Context(), applyID, actor)
	case approval.InstanceRejected:
		_ = seal.MarkRejected(r.Context(), applyID, actor, comment)
	case approval.InstanceRevoked:
		_ = seal.MarkRejected(r.Context(), applyID, actor, comment)
	}
	verb := "已通过"
	if action.Action == "reject" {
		verb = "已驳回"
	}
	approvalEvent(instID, currentSeq, actor, action.Action, "", "", comment,
		g.Map{"node": nodeOf(newStates, currentSeq), "result": instStatus})
	for _, sk := range skips {
		approvalEvent(instID, sk.Seq, "system", "skip", "", "", sk.Reason, g.Map{"node": sk.Name})
	}
	notify(inst["created_by"].String(), "approval",
		fmt.Sprintf("用印审批%s（申请 #%d）", verb, applyID),
		fmt.Sprintf("%s 在「%s」节点%s： %s", actor, nodeOf(newStates, currentSeq), verb, comment))
	if instStatus == approval.InstanceRunning && newSeq > currentSeq {
		seen := map[string]bool{}
		for _, s := range newStates {
			if s.Seq == newSeq && s.Status == approval.NodePending && !seen[s.Approver] {
				seen[s.Approver] = true
				notify(s.Approver, "approval", "新审批任务", fmt.Sprintf("用印申请 #%d 的「%s」待你审批", applyID, s.Name))
			}
		}
	}
	audit(r, "seal.approve", "seal_apply", applyID, g.Map{
		"approvalInstanceId": instID, "approve": approve, "approver": actor, "result": instStatus,
	})
	return nil
}

func SealApplyStamp(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Operator string `json:"operator"`
		Device   string `json:"device"`
	}
	_ = r.Parse(&req)
	operator := req.Operator
	if operator == "" {
		operator = r.GetCtxVar("user").String()
	}
	device := req.Device
	if device == "" {
		device = r.GetClientIp()
	}
	applyID, err := seal.Stamp(r.Context(), id, operator, device)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.stamp", "seal_apply", applyID, g.Map{"operator": operator, "device": device})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": applyID}})
}

func SealRisks(r *ghttp.Request) {
	list, err := seal.RiskScan(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// SealApprovalRuleSave R-B3：按印章类型配置审批模板（templateId<=0 清除规则，回退简化审批）
func SealApprovalRuleSave(r *ghttp.Request) {
	var req struct {
		SealType string `json:"sealType" v:"required"`
		Template int64  `json:"templateId"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Template > 0 {
		tpl, err := g.DB().Model("approval_template").Where("id", req.Template).One()
		if err != nil || tpl.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "审批模板不存在"})
			return
		}
	}
	if err := seal.RegisterApprovalRule(r.Context(), req.SealType, req.Template, r.GetCtxVar("user").String()); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.approval_rule", "seal_approval_rule", 0, g.Map{"sealType": req.SealType, "templateId": req.Template})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// SealApprovalRuleList R-B3：印章类型 → 审批模板 配置清单
func SealApprovalRuleList(r *ghttp.Request) {
	rows, err := g.DB().Model("seal_approval_rule").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Map())
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// SealTransfer R-B10：印章移交保管人（留痕）
func SealTransfer(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Keeper string `json:"keeper" v:"required"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := seal.TransferKeeper(r.Context(), id, req.Keeper); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.transfer", "seal", id, g.Map{"keeper": req.Keeper, "operator": r.GetCtxVar("user").String()})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// SealApplyExport R-B6：用印台账导出 Excel（.xlsx）
func SealApplyExport(r *ghttp.Request) {
	rows, err := g.DB().GetAll(r.Context(), `
		SELECT a.id, s.name AS seal_name, s.type AS seal_type, a.biz_type, a.biz_id, a.copies,
		       a.reason, a.status, a.creator, a.approver, a.operator, a.stamped_at, a.device,
		       a.file_hash, a.approval_instance_id, a.created_at
		FROM seal_apply a LEFT JOIN seal s ON s.id = a.seal_id
		ORDER BY a.id DESC LIMIT 5000`)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	headers := []string{"申请ID", "印章", "印章类型", "业务对象", "业务ID", "份数", "事由", "状态",
		"申请人", "审批人", "操作人", "用印时间", "设备/来源IP", "文件哈希", "审批实例", "创建时间"}
	data := make([][]string, 0, len(rows))
	for _, row := range rows {
		data = append(data, []string{
			row["id"].String(), row["seal_name"].String(), row["seal_type"].String(),
			row["biz_type"].String(), row["biz_id"].String(), row["copies"].String(),
			row["reason"].String(), row["status"].String(), row["creator"].String(),
			row["approver"].String(), row["operator"].String(), row["stamped_at"].String(),
			row["device"].String(), row["file_hash"].String(), row["approval_instance_id"].String(),
			row["created_at"].String(),
		})
	}
	audit(r, "seal.export", "seal_apply", 0, g.Map{"rows": len(rows)})
	if err := sendXLSX(r, "seal-apply.xlsx", "用印台账", headers, data); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
}
