package invoiceclass

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"

	"vouchchain/pkg/agent"
)

// 发票自动分类：规则优先（关键词匹配，可配置），未命中走 AI 兜底（LLM 从固定类别清单选）。
// 用户 11-15 需求：点一点配置规则 → 一键分类 → 按分类查询查看。

// DefaultCategories 固定类别清单（AI 只能从中选，防止自由发挥）
var DefaultCategories = []string{
	"设备采购", "咨询服务", "软件服务", "办公用品", "差旅交通", "餐饮招待", "广告宣传", "其他",
}

// Rule 一条分类规则
type Rule struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Field    string `json:"field"` // any/seller/items
	Keywords string `json:"keywords"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// SeedRulesIfEmpty 空表时写入种子规则（常见费用类目，用户可在页面改）
func SeedRulesIfEmpty(ctx context.Context) (int, error) {
	n, err := g.DB().Model("invoice_class_rule").Count()
	if err != nil || n > 0 {
		return 0, err
	}
	seeds := []Rule{
		{Name: "差旅交通", Category: "差旅交通", Field: "any", Keywords: "机票,火车,高铁,打车,滴滴,出租车,客运,交通,酒店,住宿,差旅", Priority: 10},
		{Name: "餐饮招待", Category: "餐饮招待", Field: "any", Keywords: "餐饮,餐费,食堂,宴请,招待餐", Priority: 20},
		{Name: "办公用品", Category: "办公用品", Field: "any", Keywords: "办公用品,文具,耗材,打印,纸张,办公家具", Priority: 30},
		{Name: "软件服务", Category: "软件服务", Field: "any", Keywords: "软件,系统,云服务,SaaS,平台服务,订阅,年费,信息技术", Priority: 40},
		{Name: "咨询服务", Category: "咨询服务", Field: "any", Keywords: "咨询,顾问,审计,法律,律师,会计服务", Priority: 50},
		{Name: "广告宣传", Category: "广告宣传", Field: "any", Keywords: "广告,宣传,推广,设计,营销", Priority: 60},
		{Name: "设备采购", Category: "设备采购", Field: "any", Keywords: "设备,机械,自动化,仪器,硬件,装置,生产线", Priority: 70},
	}
	for _, s := range seeds {
		if _, err := g.DB().Model("invoice_class_rule").Data(g.Map{
			"name": s.Name, "category": s.Category, "field": s.Field,
			"keywords": s.Keywords, "priority": s.Priority, "enabled": true,
			"created_at": time.Now(),
		}).Insert(); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// loadRules 按优先级取启用规则
func loadRules(ctx context.Context) []Rule {
	rows, err := g.DB().Model("invoice_class_rule").Where("enabled", true).Order("priority asc, id asc").All()
	if err != nil {
		return nil
	}
	out := make([]Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, Rule{
			ID: r["id"].Int64(), Name: r["name"].String(), Category: r["category"].String(),
			Field: r["field"].String(), Keywords: r["keywords"].String(),
			Priority: r["priority"].Int(), Enabled: r["enabled"].Bool(),
		})
	}
	return out
}

// matchRule 规则命中判断：field 决定扫哪些字段
func matchRule(rule Rule, seller, items string) bool {
	kws := strings.Split(rule.Keywords, ",")
	switch rule.Field {
	case "seller":
		return containsAny(seller, kws)
	case "items":
		return containsAny(items, kws)
	default: // any：开票方 + 商品 + 购方全扫
		return containsAny(seller, kws) || containsAny(items, kws)
	}
}

func containsAny(text string, keywords []string) bool {
	if text == "" {
		return false
	}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// ClassifyOne 单张发票分类：规则 → AI 兜底 → uncategorized
// 返回 (分类, 来源, error)；来源 rule/ai/uncategorized
func ClassifyOne(ctx context.Context, id int64) (string, string, error) {
	row, err := g.DB().Model("invoice").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return "", "", fmt.Errorf("发票不存在")
	}
	seller := row["seller_name"].String()
	items := row["items"].String()
	invoiceType := row["invoice_type"].String()
	amount := row["amount"].String()

	// 1) 规则优先
	rules := loadRules(ctx)
	for _, rule := range rules {
		if matchRule(rule, seller, items) {
			return rule.Category, "rule", nil
		}
	}
	// 2) AI 兜底（LLM 不可用时不强编，标 uncategorized）
	category, err := aiClassify(ctx, seller, items, invoiceType, amount)
	if err != nil {
		glog.Warningf(ctx, "[invoice-class] #%d AI 分类失败（%v），保持未分类", id, err)
		return "", "uncategorized", nil
	}
	return category, "ai", nil
}

// aiClassify 让 LLM 从固定清单里选（sensenova/硅基流动均可，复用 agent 的模型解析）
func aiClassify(ctx context.Context, seller, items, invoiceType, amount string) (string, error) {
	apiKey, baseUrl, model := agent.ResolveLLMConfig()
	if apiKey == "" {
		return "", fmt.Errorf("未配置大模型")
	}
	chatModel, err := agent.NewOpenAIChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		return "", err
	}
	prompt := fmt.Sprintf(
		"你是发票分类助手。请只输出一个分类名称，不要任何其他文字。\n候选分类：%s\n\n发票信息：\n类型：%s\n开票方：%s\n商品/服务：%s\n金额：%s\n\n输出候选分类中最合适的一个：",
		strings.Join(DefaultCategories, "/"), invoiceType, seller, truncStr(items, 500), amount)
	content, err := agent.GenerateOnce(ctx, chatModel, prompt)
	if err != nil {
		return "", err
	}
	ans := strings.TrimSpace(content)
	for _, c := range DefaultCategories {
		if strings.Contains(ans, c) {
			return c, nil
		}
	}
	return "其他", nil
}

// RunAll 批量分类。onlyEmpty=true 只处理未分类的；返回统计
func RunAll(ctx context.Context, onlyEmpty bool) (total, ruleHit, aiHit, skipped int, err error) {
	m := g.DB().Model("invoice").Where("1=1")
	if onlyEmpty {
		m = m.Where("category", "")
	}
	rows, err := m.All()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	for _, r := range rows {
		id := r["id"].Int64()
		category, source, cerr := ClassifyOne(ctx, id)
		if cerr != nil {
			skipped++
			continue
		}
		if source == "uncategorized" {
			// 保持空，让下次规则扩充后重跑
			skipped++
			continue
		}
		g.DB().Model("invoice").Data(g.Map{
			"category": category, "category_source": source, "updated_at": time.Now(),
		}).Where("id", id).Update()
		total++
		if source == "rule" {
			ruleHit++
		} else {
			aiHit++
		}
	}
	return total, ruleHit, aiHit, skipped, nil
}

// Stats 各分类计数（页面统计条用）
func Stats(ctx context.Context) (g.Map, error) {
	rows, err := g.DB().GetAll(ctx,
		"SELECT COALESCE(NULLIF(category,''),'未分类') AS cat, COUNT(*) AS cnt FROM invoice GROUP BY 1 ORDER BY cnt DESC")
	if err != nil {
		return nil, err
	}
	out := g.Map{}
	for _, r := range rows {
		out[r["cat"].String()] = r["cnt"].Int()
	}
	return out, nil
}

func truncStr(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
