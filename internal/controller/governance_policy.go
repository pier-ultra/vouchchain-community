package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"vouchchain/pkg/policycore"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- 产品② US-G4 合规策略引擎（AG4.1–AG4.6） ----------
// 规则库（定义/启停/版本/说明） + 纯逻辑执行内核 + 业务写入点实时校验 + 违规记录与人工处理。
// 判定内核在 pkg/policycore，零 DB 依赖，可单独抽取开源。

func loadComplianceRules(objectType string) []policycore.Rule {
	rows, err := g.DB().Model("compliance_rule").
		Where("object_type", objectType).Where("enabled", true).Order("id asc").All()
	if err != nil {
		return nil
	}
	rules := make([]policycore.Rule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, policycore.Rule{
			Code: row["code"].String(), Name: row["name"].String(),
			RuleType: row["rule_type"].String(), ObjectType: row["object_type"].String(),
			Field: row["field"].String(), Op: row["op"].String(), Value: row["rule_value"].String(),
			Condition: row["condition_expr"].String(),
			Severity:  row["severity"].String(), Enabled: true, Version: row["version"].Int(),
		})
	}
	return rules
}

// evaluateRecord 返回全部规则的判定明细（便于手动试跑与前端展示）
func evaluateRecord(objectType string, record map[string]interface{}) ([]policycore.Result, []policycore.Result) {
	rules := loadComplianceRules(objectType)
	all := make([]policycore.Result, 0, len(rules))
	failed := make([]policycore.Result, 0)
	for _, rule := range rules {
		if !policycore.MatchCondition(rule.Condition, record) {
			continue // AG4.3 规则未绑定到当前对象状态/子类型
		}
		res := policycore.Evaluate(rule, record)
		all = append(all, res)
		if !res.Passed {
			failed = append(failed, res)
		}
	}
	return all, failed
}

// policyGuard 业务写入点的统一入口：block 级违规直接写 409 并返回 true，warn 级留痕后放行
func policyGuard(r *ghttp.Request, objectType string, objectId int64, label string, record map[string]interface{}) bool {
	failed := policyCheck(r, objectType, objectId, label, record)
	if policycore.HasBlock(failed) {
		policyBlockResponse(r, failed)
		return true
	}
	return false
}

// policyCheck 业务写入点实时校验（AG4.4）：违规留痕（AG4.5），由调用方按 HasBlock 决定是否拦截
func policyCheck(r *ghttp.Request, objectType string, objectId int64, label string, record map[string]interface{}) []policycore.Result {
	_, failed := evaluateRecord(objectType, record)
	for _, f := range failed {
		ruleId, _ := g.DB().Model("compliance_rule").Where("code", f.RuleCode).Value("id")
		_, _ = g.DB().Model("compliance_violation").Data(g.Map{
			"rule_id": ruleId.Int64(), "rule_code": f.RuleCode, "rule_name": f.RuleName,
			"object_type": objectType, "object_id": objectId, "object_label": label,
			"field": f.Field, "message": f.Message, "severity": f.Severity,
			"status": "open", "owner": r.GetCtxVar("user").String(), "created_at": time.Now(),
		}).Insert()
		if f.Severity == "block" {
			audit(r, "compliance.block", objectType, objectId, g.Map{"rule": f.RuleCode, "message": f.Message})
		} else {
			audit(r, "compliance.warn", objectType, objectId, g.Map{"rule": f.RuleCode, "message": f.Message})
		}
	}
	return failed
}

// policyBlockResponse 拦截级违规统一返回（HTTP 409，携带可展示的违规明细）
func policyBlockResponse(r *ghttp.Request, failed []policycore.Result) {
	r.Response.WriteHeader(409) // 只设状态码，不往 body 写状态文本，保证响应仍是合法 JSON
	r.Response.WriteJsonExit(g.Map{
		"code":    409,
		"message": "合规策略拦截：" + failed[0].Message,
		"data":    failed,
	})
}

func ComplianceRuleList(r *ghttp.Request) {
	rows, err := g.DB().Model("compliance_rule").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

type complianceRuleReq struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	RuleType    string `json:"ruleType"`
	ObjectType  string `json:"objectType"`
	Field       string `json:"field"`
	Op          string `json:"op"`
	RuleValue   string `json:"ruleValue"`
	Condition   string `json:"conditionExpr"`
	Severity    string `json:"severity"`
	Doc         string `json:"doc"`
	Enabled     *bool  `json:"enabled"`
}

func (q complianceRuleReq) validate() error {
	if strings.TrimSpace(q.Code) == "" || strings.TrimSpace(q.Name) == "" {
		return fmt.Errorf("规则编码与名称必填")
	}
	switch q.RuleType {
	case "required", "format", "relation", "threshold":
	default:
		return fmt.Errorf("规则类型必须是 required / format / relation / threshold")
	}
	if q.Severity != "block" && q.Severity != "warn" {
		return fmt.Errorf("处置方式必须是 block（拦截）或 warn（告警）")
	}
	if strings.TrimSpace(q.Field) == "" {
		return fmt.Errorf("校验字段必填")
	}
	if q.RuleType == "format" && q.RuleValue == "" {
		return fmt.Errorf("格式规则必须给正则表达式")
	}
	if q.RuleType == "threshold" && q.RuleValue == "" {
		return fmt.Errorf("阈值规则必须给阈值")
	}
	return nil
}

// ComplianceRuleCreate AG4.1/AG4.2/AG4.3/AG4.6
func ComplianceRuleCreate(r *ghttp.Request) {
	var req complianceRuleReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	exists, _ := g.DB().Model("compliance_rule").Where("code", req.Code).Count()
	if exists > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "规则编码已存在"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	res, err := g.DB().Model("compliance_rule").Data(g.Map{
		"code": strings.TrimSpace(req.Code), "name": strings.TrimSpace(req.Name), "description": req.Description,
		"rule_type": req.RuleType, "object_type": req.ObjectType, "field": req.Field, "op": req.Op,
		"rule_value": req.RuleValue, "condition_expr": req.Condition, "severity": req.Severity, "enabled": enabled,
		"version": 1, "doc": req.Doc, "created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "compliance.rule.create", "compliance_rule", id, g.Map{"code": req.Code, "ruleType": req.RuleType})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "version": 1}})
}

// ComplianceRuleUpdate AG4.6：编辑升版本，旧快照入历史表
func ComplianceRuleUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("compliance_rule").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "规则不存在"})
		return
	}
	var req complianceRuleReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	version := old["version"].Int() + 1
	snapshot, _ := json.Marshal(old.Map())
	_, _ = g.DB().Model("compliance_rule_history").Data(g.Map{
		"rule_id": id, "version": old["version"].Int(), "snapshot": string(snapshot),
		"changed_by": r.GetCtxVar("user").String(), "changed_at": time.Now(),
	}).Insert()
	_, err = g.DB().Model("compliance_rule").Data(g.Map{
		"name": req.Name, "description": req.Description, "rule_type": req.RuleType,
		"object_type": req.ObjectType, "field": req.Field, "op": req.Op, "rule_value": req.RuleValue,
		"condition_expr": req.Condition, "severity": req.Severity, "doc": req.Doc, "version": version, "updated_at": time.Now(),
	}).Where("id", id).Update()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "compliance.rule.update", "compliance_rule", id, g.Map{"version": version, "code": old["code"].String()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"version": version}})
}

func ComplianceRuleToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("compliance_rule").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "规则不存在"})
		return
	}
	next := !row["enabled"].Bool()
	if _, err := g.DB().Model("compliance_rule").Data(g.Map{"enabled": next, "updated_at": time.Now()}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "compliance.rule.toggle", "compliance_rule", id, g.Map{"enabled": next})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"enabled": next}})
}

func ComplianceRuleHistory(r *ghttp.Request) {
	rows, err := g.DB().Model("compliance_rule_history").Where("rule_id", r.Get("id").Int64()).Order("version desc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func ComplianceViolationList(r *ghttp.Request) {
	m := g.DB().Model("compliance_violation")
	if v := r.Get("status").String(); v != "" {
		m = m.Where("status", v)
	}
	if v := r.Get("severity").String(); v != "" {
		m = m.Where("severity", v)
	}
	if v := r.Get("objectType").String(); v != "" {
		m = m.Where("object_type", v)
	}
	rows, err := m.Order("id desc").Limit(300).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ComplianceViolationHandle AG4.5：违规项处理状态与责任人
func ComplianceViolationHandle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Action != "resolved" && req.Action != "waived" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "action 必须是 resolved / waived"})
		return
	}
	if strings.TrimSpace(req.Note) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "处理说明必填（合规留痕要求）"})
		return
	}
	row, err := g.DB().Model("compliance_violation").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "违规记录不存在"})
		return
	}
	if _, err := g.DB().Model("compliance_violation").Data(g.Map{
		"status": req.Action, "handled_by": r.GetCtxVar("user").String(),
		"handled_note": req.Note, "handled_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "compliance.violation.handle", "compliance_violation", id, g.Map{"action": req.Action, "note": req.Note})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// ComplianceCheck AG4.4 手动试跑：不落库、不拦截，用于验证规则配置是否正确
func ComplianceCheck(r *ghttp.Request) {
	var req struct {
		ObjectType string                 `json:"objectType"`
		Record     map[string]interface{} `json:"record"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	all, failed := evaluateRecord(req.ObjectType, req.Record)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"objectType": req.ObjectType, "evaluated": len(all), "failed": len(failed),
		"blocked": policycore.HasBlock(failed), "results": all,
	}})
}

func ComplianceSummary(r *ghttp.Request) {
	total, _ := g.DB().Model("compliance_rule").Count()
	enabled, _ := g.DB().Model("compliance_rule").Where("enabled", true).Count()
	open, _ := g.DB().Model("compliance_violation").Where("status", "open").Count()
	blocked, _ := g.DB().Model("compliance_violation").Where("severity", "block").Count()
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"total": total, "enabled": enabled, "open": open, "blocked": blocked,
	}})
}
