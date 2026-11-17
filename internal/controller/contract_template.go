package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/contract_template"
)

// A6 合同模板库 + 变量套打：
//   GET  /api/contract-templates?keyword=&category=   模板列表（含 varKeys）
//   POST /api/contract-templates                      新建
//   PUT  /api/contract-templates/{id}                 更新
//   DELETE /api/contract-templates/{id}               删除
//   POST /api/contract-templates/render               变量套打（渲染正文，返回缺失必填变量）

func ContractTemplateList(r *ghttp.Request) {
	rows, err := contract_template.List(context.Background(), r.Get("keyword").String(), r.Get("category").String(), r.Get("limit").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func ContractTemplateCreate(r *ghttp.Request) {
	var vars []contract_template.VarDecl
	_ = r.Parse(&vars)
	id, err := contract_template.Create(context.Background(),
		r.Get("name").String(), r.Get("category").String(), r.Get("content").String(), r.Get("remark").String(), vars)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "contract_template.create", "contract_template", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func ContractTemplateUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var vars []contract_template.VarDecl
	_ = r.Parse(&vars)
	if err := contract_template.Update(context.Background(), id,
		r.Get("name").String(), r.Get("category").String(), r.Get("content").String(), r.Get("remark").String(), vars); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "contract_template.update", "contract_template", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func ContractTemplateDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if err := contract_template.Delete(context.Background(), id); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "contract_template.delete", "contract_template", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// ContractTemplateRender 变量套打：body {templateId, values:{key:value}} → {content, missing}
func ContractTemplateRender(r *ghttp.Request) {
	id := r.Get("templateId").Int64()
	row, err := contract_template.Get(context.Background(), id)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	values := r.Get("values").Map()
	content := fmt.Sprint(row["content"])
	vars := []contract_template.VarDecl{}
	_ = json.Unmarshal([]byte(fmt.Sprint(row["variables"])), &vars)
	valuesStr := map[string]string{}
	for k, v := range values {
		valuesStr[k] = fmt.Sprint(v)
	}
	rendered, missing := contract_template.Render(content, valuesStr, vars)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"content": rendered, "missing": missing}})
}
