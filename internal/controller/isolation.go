package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vouchchain/internal/middleware"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- 产品② US-G5 强隔离策略管理面（AG5.1/AG5.2/AG5.4/AG5.5） ----------
// 策略本体与执行逻辑在 pkg/isolate + middleware/isolation.go；这里只做配置 CRUD、审计与查询。

type isoPolicyRow struct {
	Id              int64    `json:"id"`
	Name            string   `json:"name"`
	Dimension       string   `json:"dimension"`
	SubjectType     string   `json:"subjectType"`
	Subject         string   `json:"subject"`
	AllowedValues   []string `json:"allowedValues"`
	IncludeChildren bool     `json:"includeChildren"`
	Enabled         bool     `json:"enabled"`
	Priority        int      `json:"priority"`
	CreatedBy       string   `json:"createdBy"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

func IsoDimensions(r *ghttp.Request) {
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": []g.Map{
		{"value": "entity", "label": "法人主体", "valueLabel": "主体 ID（0=未归属历史数据）", "numeric": true, "children": true},
		{"value": "dept", "label": "组织部门", "valueLabel": "组织节点 ID", "numeric": true, "children": true},
		{"value": "project", "label": "项目", "valueLabel": "项目编码（*=不限）", "numeric": false, "children": false},
	}})
}

func IsoPolicyList(r *ghttp.Request) {
	rows, err := g.DB().Model("isolation_policy").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]isoPolicyRow, 0, len(rows))
	for _, row := range rows {
		vals := []string{}
		_ = json.Unmarshal([]byte(row["allowed_values"].String()), &vals)
		list = append(list, isoPolicyRow{
			Id: row["id"].Int64(), Name: row["name"].String(), Dimension: row["dimension"].String(),
			SubjectType: row["subject_type"].String(), Subject: row["subject"].String(),
			AllowedValues: vals, IncludeChildren: row["include_children"].Bool(),
			Enabled: row["enabled"].Bool(), Priority: row["priority"].Int(),
			CreatedBy: row["created_by"].String(), CreatedAt: row["created_at"].String(),
			UpdatedAt: row["updated_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func parseIsoPolicy(r *ghttp.Request) (g.Map, error) {
	var req struct {
		Name            string   `json:"name"`
		Dimension       string   `json:"dimension"`
		SubjectType     string   `json:"subjectType"`
		Subject         string   `json:"subject"`
		AllowedValues   []string `json:"allowedValues"`
		IncludeChildren bool     `json:"includeChildren"`
		Enabled         bool     `json:"enabled"`
		Priority        int      `json:"priority"`
	}
	if err := r.Parse(&req); err != nil {
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Name == "" {
		return nil, fmt.Errorf("策略名必填")
	}
	if req.Dimension != "entity" && req.Dimension != "dept" && req.Dimension != "project" {
		return nil, fmt.Errorf("隔离维度只能是 entity / dept / project")
	}
	if req.SubjectType != "user" && req.SubjectType != "role" && req.SubjectType != "all" {
		return nil, fmt.Errorf("授权对象只能是 user / role / all")
	}
	if req.SubjectType != "all" && req.Subject == "" {
		return nil, fmt.Errorf("按用户/角色授权时必须填写对象（用户名或角色码）")
	}
	clean := []string{}
	for _, v := range req.AllowedValues {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if req.Dimension != "project" && v != "*" {
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				return nil, fmt.Errorf("%s 维度的允许值必须是数字 ID（或 *）：%s", req.Dimension, v)
			}
		}
		clean = append(clean, v)
	}
	b, _ := json.Marshal(clean)
	return g.Map{
		"name": req.Name, "dimension": req.Dimension, "subject_type": req.SubjectType,
		"subject": req.Subject, "allowed_values": string(b),
		"include_children": req.IncludeChildren, "enabled": req.Enabled,
		"priority": req.Priority, "updated_at": time.Now(),
	}, nil
}

func IsoPolicySave(r *ghttp.Request) {
	data, err := parseIsoPolicy(r)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id := r.Get("id").Int64()
	action := "isolation.policy_create"
	username := r.GetCtxVar("user").String()
	if id > 0 {
		exist, _ := g.DB().Model("isolation_policy").Where("id", id).One()
		if exist.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "策略不存在"})
			return
		}
		if _, err := g.DB().Model("isolation_policy").Data(data).Where("id", id).Update(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		action = "isolation.policy_update"
	} else {
		data["created_by"] = username
		data["created_at"] = time.Now()
		res, err := g.DB().Model("isolation_policy").Data(data).Insert()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id, _ = res.LastInsertId()
	}
	middleware.InvalidateIsolationCache()
	// AG5.5 策略变更写审计轨迹（含完整快照，便于回溯"谁把范围放宽了"）
	audit(r, action, "isolation_policy", id, g.Map{
		"name": data["name"], "dimension": data["dimension"], "subjectType": data["subject_type"],
		"subject": data["subject"], "allowedValues": data["allowed_values"],
		"includeChildren": data["include_children"], "enabled": data["enabled"],
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func IsoPolicyDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	row, _ := g.DB().Model("isolation_policy").Where("id", id).One()
	if row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "策略不存在"})
		return
	}
	if _, err := g.DB().Model("isolation_policy").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	middleware.InvalidateIsolationCache()
	audit(r, "isolation.policy_delete", "isolation_policy", id, g.Map{"snapshot": row.Map()})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func IsoPolicyToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("isolation_policy").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "策略不存在"})
		return
	}
	next := !row["enabled"].Bool()
	if _, err := g.DB().Model("isolation_policy").Data(g.Map{
		"enabled": next, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	middleware.InvalidateIsolationCache()
	audit(r, "isolation.policy_toggle", "isolation_policy", id, g.Map{"enabled": next, "name": row["name"].String()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"enabled": next}})
}

func IsoEffective(r *ghttp.Request) {
	d := middleware.IsolationFor(r)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"username": r.GetCtxVar("user").String(), "role": middleware.CurrentRole(r),
		"enforced": d.Enforced, "allowed": d.Allowed, "entityIds": d.EntityIDs,
		"deptIds": d.DeptIDs, "projects": d.Projects, "matched": d.Matched,
		"overrides": d.Overrides, "sourceLabels": d.SourceLabels,
	}})
}

func IsoViolationList(r *ghttp.Request) {
	m := g.DB().Model("isolation_violation")
	if status := r.Get("status").String(); status != "" {
		m = m.Where("status", status)
	}
	rows, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func IsoViolationHandle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Status != "resolved" && req.Status != "waived" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "status 只能是 resolved / waived"})
		return
	}
	if _, err := g.DB().Model("isolation_violation").Data(g.Map{
		"status": req.Status, "handled_by": r.GetCtxVar("user").String(),
		"handled_note": req.Note, "handled_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "isolation.violation_handle", "isolation_violation", id, g.Map{"status": req.Status, "note": req.Note})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func IsoSummary(r *ghttp.Request) {
	total, _ := g.DB().Model("isolation_policy").Count()
	enabled, _ := g.DB().Model("isolation_policy").Where("enabled", true).Count()
	violations, _ := g.DB().Model("isolation_violation").Count()
	open, _ := g.DB().Model("isolation_violation").Where("status", "open").Count()
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"total": total, "enabled": enabled, "violations": violations, "open": open,
	}})
}
