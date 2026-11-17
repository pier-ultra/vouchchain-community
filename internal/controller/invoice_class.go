package controller

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/invoiceclass"
)

// 发票自动分类接口（规则配置 + 一键执行 + 统计）：
//   GET    /api/invoice-class/rules          规则列表
//   POST   /api/invoice-class/rules          新增/更新（带 id 即更新）
//   DELETE /api/invoice-class/rules/{id}     删除
//   POST   /api/invoice-class/run            执行分类 {onlyEmpty:bool}
//   GET    /api/invoice-class/stats          各分类计数

func InvoiceClassRuleList(r *ghttp.Request) {
	rows, err := g.DB().Model("invoice_class_rule").Order("priority asc, id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, g.Map{
			"id": row["id"].Int64(), "name": row["name"].String(), "category": row["category"].String(),
			"field": row["field"].String(), "keywords": row["keywords"].String(),
			"priority": row["priority"].Int(), "enabled": row["enabled"].Bool(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func InvoiceClassRuleSave(r *ghttp.Request) {
	var req struct {
		ID       int64  `json:"id"`
		Name     string `json:"name" v:"required"`
		Category string `json:"category" v:"required"`
		Field    string `json:"field"`
		Keywords string `json:"keywords" v:"required"`
		Priority int    `json:"priority"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Field == "" {
		req.Field = "any"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	data := g.Map{
		"name": req.Name, "category": req.Category, "field": req.Field,
		"keywords": req.Keywords, "priority": req.Priority, "enabled": enabled,
	}
	var id int64
	if req.ID > 0 {
		_, err := g.DB().Model("invoice_class_rule").Data(data).Where("id", req.ID).Update()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id = req.ID
	} else {
		res, err := g.DB().Model("invoice_class_rule").Data(data).Insert()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id, _ = res.LastInsertId()
	}
	audit(r, "invoice_class.rule_save", "invoice_class_rule", id, g.Map{"name": req.Name, "category": req.Category})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func InvoiceClassRuleDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	g.DB().Model("invoice_class_rule").Where("id", id).Delete()
	audit(r, "invoice_class.rule_delete", "invoice_class_rule", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// InvoiceClassRun 执行分类（AI 兜底可能较慢，single 阈值内同步返回）
func InvoiceClassRun(r *ghttp.Request) {
	var req struct {
		OnlyEmpty bool `json:"onlyEmpty"`
	}
	_ = r.Parse(&req)
	total, ruleHit, aiHit, skipped, err := invoiceclass.RunAll(r.Context(), req.OnlyEmpty)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "invoice_class.run", "invoice", 0, g.Map{
		"total": total, "rule": ruleHit, "ai": aiHit, "skipped": skipped, "onlyEmpty": req.OnlyEmpty,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"total": total, "ruleHit": ruleHit, "aiHit": aiHit, "skipped": skipped,
	}})
}

func InvoiceClassStats(r *ghttp.Request) {
	stats, err := invoiceclass.Stats(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": stats})
}
