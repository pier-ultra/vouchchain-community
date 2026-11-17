package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// AI 分录草稿（P2P 闭环缺的第 3 格）：发票要素 + 科目映射 → 借贷分录建议。
// 硬规则：AI 只出「建议」，必须会计确认才写正式凭证；不生成总账、不碰报表报税。
// 科目只能从 account_mapping 里选，模型编造科目/借贷不平 → 一律回退规则兜底，绝不把脏数据入库。
// 商业对标：汇联易/每刻的「ERP 凭证」模块单独收费；我们社区版免费内嵌。

type JournalFacts struct {
	InvoiceNo   string  `json:"invoiceNo"`
	InvoiceType string  `json:"invoiceType"`
	SellerName  string  `json:"sellerName"`
	BuyerName   string  `json:"buyerName"`
	Category    string  `json:"category"` // 费用类型，对应科目映射表
	Items       string  `json:"items"`    // 发票明细行（JSON 字符串）
	Amount      float64 `json:"amount"`   // 不含税金额
	Tax         float64 `json:"tax"`
	Total       float64 `json:"total"`
}

type AccountSubject struct {
	Category    string `json:"category"`
	SubjectCode string `json:"subjectCode"`
	SubjectName string `json:"subjectName"`
	Direction   string `json:"direction"`
}

type JournalEntry struct {
	Summary     string  `json:"summary"`
	SubjectCode string  `json:"subjectCode"`
	SubjectName string  `json:"subjectName"`
	Debit       float64 `json:"debit"`
	Credit      float64 `json:"credit"`
}

type JournalSuggestion struct {
	Entries []JournalEntry `json:"entries"`
	Reason  string         `json:"reason"`
	Source  string         `json:"source"` // llm / rule / rule_fallback
}

const journalSystemPrompt = `你是中国企业会计准则下的资深应付会计。根据进项发票要素和给定的科目映射表，给出记账凭证的分录建议。
只输出 JSON，不要输出任何其他文字：
{"entries":[{"summary":"摘要","subjectCode":"科目编码","subjectName":"科目名称","debit":借方金额,"credit":贷方金额}],"reason":"判断依据一句话"}
规则：
1. 标准进项采购分录：借 费用/存货科目（不含税金额）、借 进项税额（税额）、贷 应付账款（价税合计）。税额为 0 时只做两行。
2. 每一行的 subjectCode/subjectName 必须逐字来自给定的科目映射表，禁止编造或改写科目；同一行 debit 与 credit 只能有一个大于 0。
3. 借方合计必须严格等于贷方合计（保留两位小数），金额取自发票要素，禁止自行估算。
4. summary 用中文简述业务，含销方名称或发票号；reason 说明为什么选这个费用科目。`

// Balanced 借贷平衡校验（controller 人工编辑分录后同样调用）
func Balanced(entries []JournalEntry) bool {
	if len(entries) < 2 {
		return false
	}
	var d, c float64
	for _, e := range entries {
		if e.Debit < 0 || e.Credit < 0 || (e.Debit > 0 && e.Credit > 0) {
			return false
		}
		d += e.Debit
		c += e.Credit
	}
	return math.Abs(d-c) < 0.01 && d > 0
}

// RuleJournal 规则兜底（无密钥 / 模型不可用 / 模型输出不合规时都能出草稿）
func RuleJournal(f JournalFacts, subjects []AccountSubject) *JournalSuggestion {
	pick := func(category, direction string) AccountSubject {
		for _, s := range subjects {
			if s.Category == category && s.Direction == direction {
				return s
			}
		}
		for _, s := range subjects {
			if s.Direction == direction {
				return s
			}
		}
		return AccountSubject{Category: category, SubjectCode: "待补", SubjectName: category, Direction: direction}
	}
	expense := pick(f.Category, "debit")
	if expense.SubjectCode == "待补" || f.Category == "" {
		expense = pick("服务采购", "debit")
	}
	tax := pick("进项税额", "debit")
	payable := pick("应付账款", "credit")

	amount := f.Amount
	// 解析出的金额与价税合计不一致时，以价税合计为准倒挤不含税额，并在 reason 里说清楚
	if math.Abs(amount+f.Tax-f.Total) > 0.01 {
		amount = math.Round((f.Total-f.Tax)*100) / 100
	}
	summary := fmt.Sprintf("采购 %s 发票 %s", f.SellerName, f.InvoiceNo)
	entries := []JournalEntry{
		{Summary: summary, SubjectCode: expense.SubjectCode, SubjectName: expense.SubjectName, Debit: amount, Credit: 0},
	}
	if f.Tax > 0 {
		entries = append(entries, JournalEntry{
			Summary: summary, SubjectCode: tax.SubjectCode, SubjectName: tax.SubjectName, Debit: f.Tax, Credit: 0,
		})
	}
	entries = append(entries, JournalEntry{
		Summary: summary, SubjectCode: payable.SubjectCode, SubjectName: payable.SubjectName, Debit: 0, Credit: f.Total,
	})
	return &JournalSuggestion{
		Entries: entries,
		Reason:  fmt.Sprintf("规则分录：按「%s」映射 %s %s，进项税额单独借记，价税合计挂应付；金额以发票要素为准。", f.Category, expense.SubjectCode, expense.SubjectName),
		Source:  "rule",
	}
}

// SuggestJournal 优先 LLM 建议，失败/不合规自动回退规则（AI 不可用不让流程断）
func SuggestJournal(ctx context.Context, f JournalFacts, subjects []AccountSubject, useAI bool) (*JournalSuggestion, error) {
	rule := RuleJournal(f, subjects)
	if !useAI {
		return rule, nil
	}
	apiKey, baseUrl, model := resolveLLMConfig()
	if apiKey == "" {
		rule.Reason += "（未配置大模型密钥，使用规则兜底）"
		return rule, nil
	}
	chatModel, err := openaiNewChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		rule.Reason += "（模型初始化失败，使用规则兜底：" + err.Error() + "）"
		return rule, nil
	}
	sj, _ := json.Marshal(subjects)
	fj, _ := json.Marshal(f)
	content, err := generateWithRetry(ctx, chatModel, []*schema.Message{
		{Role: schema.System, Content: journalSystemPrompt},
		{Role: schema.User, Content: "【科目映射表】\n" + string(sj) + "\n\n【发票要素】\n" + string(fj)},
	})
	if err != nil {
		rule.Reason += "（模型调用失败，使用规则兜底：" + err.Error() + "）"
		rule.Source = "rule_fallback"
		return rule, nil
	}
	out, err := parseJournalJSON(content)
	if err != nil || !Balanced(out.Entries) || !subjectsAllowed(out.Entries, subjects) {
		rule.Reason += "（模型输出不合规或科目越界/借贷不平，已回退规则分录）"
		rule.Source = "rule_fallback"
		return rule, nil
	}
	out.Source = "llm"
	return out, nil
}

func subjectsAllowed(entries []JournalEntry, subjects []AccountSubject) bool {
	allowed := map[string]bool{}
	for _, s := range subjects {
		allowed[s.SubjectCode] = true
	}
	for _, e := range entries {
		if e.SubjectCode == "" || !allowed[e.SubjectCode] {
			return false
		}
	}
	return true
}

func parseJournalJSON(s string) (*JournalSuggestion, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("模型输出中未找到 JSON")
	}
	var out JournalSuggestion
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("解析分录建议失败: %w", err)
	}
	return &out, nil
}
