package contract_template

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// A6 合同模板库 + 变量套打（P1）：
//   - 模板以 {{变量名}} 占位符书写正文，变量清单可先声明（可选，未声明时从正文自动提取）。
//   - Render 只做占位符替换（自研，不引入 text/template，避免误解析 Go 模板语法）。
//   - 与 AI 生成互补：AI 生成是"从零拟稿"，模板套打是"按范式填变量"，后者是法大大/上上签的核心收费点。

var varRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_\p{Han}]+)\s*\}\}`)

// ExtractVars 从模板文本提取 {{变量}} 清单（按首次出现顺序去重）。纯函数。
func ExtractVars(content string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range varRe.FindAllStringSubmatch(content, -1) {
		k := strings.TrimSpace(m[1])
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// VarDecl 变量声明（模板变量清单项）
type VarDecl struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Default  string `json:"default"`
}

// Render 变量套打：把 {{key}} 替换为 values[key]（缺失用 default，仍缺留空）。返回渲染结果 + 缺失必填变量。纯函数。
func Render(content string, values map[string]string, vars []VarDecl) (string, []string) {
	missing := []string{}
	byKey := map[string]VarDecl{}
	for _, v := range vars {
		byKey[v.Key] = v
	}
	out := varRe.ReplaceAllStringFunc(content, func(m string) string {
		k := strings.TrimSpace(varRe.FindStringSubmatch(m)[1])
		if v, ok := values[k]; ok {
			return v
		}
		if d, ok := byKey[k]; ok && d.Default != "" {
			return d.Default
		}
		if d, ok := byKey[k]; ok && d.Required {
			missing = append(missing, k)
		}
		return ""
	})
	return out, missing
}

// Create 新建模板（自动提取变量清单，若未显式声明）
func Create(ctx context.Context, name, category, content, remark string, declared []VarDecl) (int64, error) {
	if name == "" {
		return 0, fmt.Errorf("模板名称不能为空")
	}
	if strings.TrimSpace(content) == "" {
		return 0, fmt.Errorf("模板正文不能为空")
	}
	vars := declared
	if len(vars) == 0 {
		for _, k := range ExtractVars(content) {
			vars = append(vars, VarDecl{Key: k, Label: k, Required: true})
		}
	}
	vj, _ := json.Marshal(vars)
	res, err := g.DB().Model("contract_template").Data(g.Map{
		"name": name, "category": category, "content": content,
		"variables": string(vj), "remark": remark,
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// List 模板列表
func List(ctx context.Context, keyword, category string, limit int) ([]g.Map, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	m := g.DB().Model("contract_template")
	if keyword != "" {
		m = m.Where("name LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if category != "" {
		m = m.Where("category", category)
	}
	rows, err := m.Order("id desc").Limit(limit).All()
	if err != nil {
		return nil, err
	}
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		row := r.Map()
		row["varKeys"] = ExtractVars(r["content"].String())
		out = append(out, row)
	}
	return out, nil
}

// Get 单模板
func Get(ctx context.Context, id int64) (g.Map, error) {
	row, err := g.DB().Model("contract_template").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return nil, fmt.Errorf("模板不存在")
	}
	m := row.Map()
	m["varKeys"] = ExtractVars(row["content"].String())
	return m, nil
}

// Update 更新模板
func Update(ctx context.Context, id int64, name, category, content, remark string, declared []VarDecl) error {
	if _, err := Get(ctx, id); err != nil {
		return err
	}
	vars := declared
	if len(vars) == 0 {
		for _, k := range ExtractVars(content) {
			vars = append(vars, VarDecl{Key: k, Label: k, Required: true})
		}
	}
	vj, _ := json.Marshal(vars)
	_, err := g.DB().Model("contract_template").Data(g.Map{
		"name": name, "category": category, "content": content,
		"variables": string(vj), "remark": remark, "updated_at": time.Now(),
	}).Where("id", id).Update()
	return err
}

// Delete 删除模板
func Delete(ctx context.Context, id int64) error {
	_, err := g.DB().Model("contract_template").Where("id", id).Delete()
	return err
}

// SortVarKeys 变量名稳定排序（前端表单渲染用）
func SortVarKeys(keys []string) []string {
	sort.Strings(keys)
	return keys
}
