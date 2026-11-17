// Package mdmatch MDM 实体解析与去重的纯逻辑内核（产品② AG6.1/AG6.2/AG6.5）。
// 不依赖数据库与 Web 框架：匹配字段、权重、阈值均以数据描述，便于单独抽取开源。
package mdmatch

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Candidate 一条主数据记录的匹配视图
type Candidate struct {
	ID         int64
	UID        string
	Code       string
	Name       string
	Status     string
	Attributes map[string]interface{}
}

// Rule 可配置匹配规则（AG6.5）
type Rule struct {
	Fields    []string
	Threshold float64
}

// Match 一对候选的相似度明细
type Match struct {
	Score      float64            `json:"score"`
	FieldScore map[string]float64 `json:"fieldScores"`
	Reasons    []string           `json:"reasons"`
	Duplicate  bool               `json:"duplicate"`
}

var noise = []string{"有限公司", "股份有限公司", "有限责任公司", "有限責任公司", "公司", "集团", "（", "）", "(", ")", " ", "\t", "-", "·", "更名"}

// Normalize 名称归一化：全角转半角、去公司后缀与标点、大小写统一
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		b.WriteRune(unicode.ToLower(r))
	}
	out := b.String()
	for _, n := range noise {
		out = strings.ReplaceAll(out, strings.ToLower(n), "")
	}
	return strings.TrimSpace(out)
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// Similarity 基于字符二元组 Dice 系数的相似度（0–100），对中文名称比编辑距离更稳
func Similarity(a, b string) float64 {
	x, y := Normalize(a), Normalize(b)
	if x == "" || y == "" {
		return 0
	}
	if x == y {
		return 100
	}
	rx, ry := []rune(x), []rune(y)
	if (len(rx) >= 4 && strings.Contains(y, x)) || (len(ry) >= 4 && strings.Contains(x, y)) {
		return 88
	}
	bigrams := func(rs []rune) map[string]int {
		m := map[string]int{}
		if len(rs) < 2 {
			m[string(rs)]++
			return m
		}
		for i := 0; i+1 < len(rs); i++ {
			m[string(rs[i:i+2])]++
		}
		return m
	}
	bx, by := bigrams(rx), bigrams(ry)
	inter, total := 0, 0
	for k, n := range bx {
		total += n
		if m, ok := by[k]; ok {
			if n < m {
				inter += n
			} else {
				inter += m
			}
		}
	}
	for _, n := range by {
		total += n
	}
	if total == 0 {
		return 0
	}
	return math.Round(float64(2*inter)/float64(total)*10000) / 100
}

var fieldLabel = map[string]string{
	"name": "名称", "code": "编码", "taxNo": "统一社会信用代码", "taxno": "统一社会信用代码",
	"address": "地址", "phone": "联系方式", "legalPerson": "法定代表人",
}

func fieldValue(c Candidate, field string) string {
	switch strings.ToLower(field) {
	case "name":
		return c.Name
	case "code":
		return c.Code
	}
	if c.Attributes == nil {
		return ""
	}
	if v, ok := c.Attributes[field]; ok {
		return str(v)
	}
	for k, v := range c.Attributes {
		if strings.EqualFold(k, field) {
			return str(v)
		}
	}
	return ""
}

func weight(field string) float64 {
	switch strings.ToLower(field) {
	case "taxno":
		return 3 // 统一社会信用代码是最强身份信号
	case "name", "code":
		return 2
	default:
		return 1
	}
}

// Compare 按可配置字段与权重计算一对候选的相似度与判重结论（AG6.1/AG6.2）
func Compare(a, b Candidate, rule Rule) Match {
	m := Match{FieldScore: map[string]float64{}, Reasons: []string{}}
	fields := rule.Fields
	if len(fields) == 0 {
		fields = []string{"name", "taxNo"}
	}
	var wsum, ssum float64
	strongHit := ""
	for _, f := range fields {
		va, vb := fieldValue(a, f), fieldValue(b, f)
		s := Similarity(va, vb)
		w := weight(f)
		m.FieldScore[f] = s
		wsum += w
		ssum += s * w
		label := fieldLabel[strings.ToLower(f)]
		if label == "" {
			label = f
		}
		if va != "" && vb != "" {
			m.Reasons = append(m.Reasons, fmt.Sprintf("%s：%q vs %q，相似度 %.1f", label, va, vb, s))
		} else {
			m.Reasons = append(m.Reasons, fmt.Sprintf("%s：至少一方缺失，按 0 计", label))
		}
		// 强身份字段完全一致 → 直接认定为高置信重复
		if (strings.EqualFold(f, "taxNo") || strings.EqualFold(f, "code")) && va != "" && Normalize(va) == Normalize(vb) {
			strongHit = label
		}
	}
	if wsum > 0 {
		m.Score = math.Round(ssum/wsum*100) / 100
	}
	if strongHit != "" && m.Score < 92 {
		m.Score = 92
		m.Reasons = append(m.Reasons, strongHit+"完全一致，按强身份信号提升至 92 分")
	}
	m.Duplicate = m.Score >= rule.Threshold
	sort.SliceStable(m.Reasons, func(i, j int) bool { return len(m.Reasons[i]) < len(m.Reasons[j]) })
	return m
}

// SuggestSurvivor 合并建议：优先保留状态有效、属性更完整、名称更完整的记录作为权威版本（AG6.2/AG6.3）
func SuggestSurvivor(a, b Candidate) (Candidate, Candidate, string) {
	score := func(c Candidate) int {
		n := len(c.Attributes)*10 + len([]rune(c.Name))
		if c.Status == "active" {
			n += 1000
		}
		if c.Code != "" {
			n += 20
		}
		return n
	}
	if score(a) >= score(b) {
		return a, b, fmt.Sprintf("保留 #%d（%s，稳定 ID %s）作为权威记录，#%d 归并；其稳定 ID 写入映射表，历史关联关系自动重定向",
			a.ID, a.Name, a.UID, b.ID)
	}
	return b, a, fmt.Sprintf("保留 #%d（%s，稳定 ID %s）作为权威记录，#%d 归并；其稳定 ID 写入映射表，历史关联关系自动重定向",
		b.ID, b.Name, b.UID, a.ID)
}
