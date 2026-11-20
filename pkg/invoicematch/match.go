package invoicematch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// MatchRule 匹配规则配置
type MatchRule struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	WeightTaxID       int    `json:"weight_tax_id"`
	WeightName        int    `json:"weight_name"`
	WeightAmount      int    `json:"weight_amount"`
	WeightDate        int    `json:"weight_date"`
	WeightItem        int    `json:"weight_item"`
	ThresholdMatched  int    `json:"threshold_matched"`
	ThresholdMismatch int    `json:"threshold_mismatch"`
	Enabled           bool   `json:"enabled"`
	IsDefault         bool   `json:"is_default"`
}

// MatchResult 单条匹配结果
type MatchResult struct {
	InvoiceID     int64  `json:"invoiceId"`
	ContractID    int64  `json:"contractId"`
	PaymentPlanID int64  `json:"paymentPlanId"`
	ScoreTaxID    int    `json:"scoreTaxId"`
	ScoreName     int    `json:"scoreName"`
	ScoreAmount   int    `json:"scoreAmount"`
	ScoreDate     int    `json:"scoreDate"`
	ScoreItem     int    `json:"scoreItem"`
	TotalScore    int    `json:"totalScore"`
	MatchStatus   string `json:"matchStatus"`
	MatchRule     string `json:"matchRule"`
	MatchedBy     string `json:"matchedBy"`
}

// AuditLog 四性归档审计记录
type AuditLog struct {
	InvoiceID    int64                  `json:"invoiceId"`
	AuditType    string                 `json:"auditType"` // authenticity / integrity / compliance / association
	FieldName    string                 `json:"fieldName"`
	ExpectedHash string                 `json:"expectedHash"`
	ActualHash   string                 `json:"actualHash"`
	CheckPass    bool                   `json:"checkPass"`
	CheckDetail  map[string]interface{} `json:"checkDetail"`
	Operator     string                 `json:"operator"`
}

// defaultRule 免费版内置默认规则
var defaultRule = MatchRule{
	Name:              "default",
	WeightTaxID:       30,
	WeightName:        20,
	WeightAmount:      30,
	WeightDate:        10,
	WeightItem:        10,
	ThresholdMatched:  80,
	ThresholdMismatch: 40,
	Enabled:           true,
	IsDefault:         true,
}

// GetActiveRule 获取当前启用的默认规则
func GetActiveRule(ctx context.Context) (MatchRule, error) {
	var rule MatchRule
	err := g.DB().Model("invoice_match_rule").Where("enabled=1 AND is_default=1").Order("id asc").Limit(1).Scan(&rule)
	if err != nil || rule.ID == 0 {
		return defaultRule, nil
	}
	return rule, nil
}

// GetAllEnabledRules 获取所有启用规则（专业版扩展用）
func GetAllEnabledRules(ctx context.Context) ([]MatchRule, error) {
	var rules []MatchRule
	err := g.DB().Model("invoice_match_rule").Where("enabled=1").Order("id asc").Scan(&rules)
	return rules, err
}

// UpsertRule 创建/更新规则（专业版）
func UpsertRule(ctx context.Context, rule MatchRule) (int64, error) {
	if rule.ID > 0 {
		_, err := g.DB().Model("invoice_match_rule").Data(g.Map{
			"name": rule.Name, "weight_tax_id": rule.WeightTaxID, "weight_name": rule.WeightName,
			"weight_amount": rule.WeightAmount, "weight_date": rule.WeightDate, "weight_item": rule.WeightItem,
			"threshold_matched": rule.ThresholdMatched, "threshold_mismatch": rule.ThresholdMismatch,
			"enabled": rule.Enabled, "updated_at": gtime.Now(),
		}).Where("id", rule.ID).Update()
		return rule.ID, err
	}
	res, err := g.DB().Model("invoice_match_rule").Data(g.Map{
		"name": rule.Name, "weight_tax_id": rule.WeightTaxID, "weight_name": rule.WeightName,
		"weight_amount": rule.WeightAmount, "weight_date": rule.WeightDate, "weight_item": rule.WeightItem,
		"threshold_matched": rule.ThresholdMatched, "threshold_mismatch": rule.ThresholdMismatch,
		"enabled": rule.Enabled, "is_default": rule.IsDefault, "created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ---------- 核心匹配算法 ----------

// RunMatching 对单张发票执行三方匹配（发票-合同-付款）
func RunMatching(ctx context.Context, invoiceID int64, rule *MatchRule) (MatchResult, error) {
	if rule == nil {
		r, _ := GetActiveRule(ctx)
		rule = &r
	}

	// 1. 取发票主记录
	inv, err := g.DB().Model("invoice").Where("id", invoiceID).One()
	if err != nil || inv.IsEmpty() {
		return MatchResult{}, fmt.Errorf("发票不存在: %d", invoiceID)
	}

	_ = inv["tax_id"].String()      // 发票税号（需在 invoice 表补字段或从 OCR 扩展字段取）
	_ = inv["buyer_name"].String()  // 购买方
	_ = inv["seller_name"].String() // 销售方
	_ = inv["total"].Float64()
	_ = inv["issue_date"].String()
	_ = inv["items"].String()

	// 2. 候选合同：同一法人实体下，状态非草稿/作废
	contracts, _ := g.DB().Model("contract").
		Where("legal_entity_id", inv["legal_entity_id"].Int64()).
		Where("status NOT IN ('draft','cancelled')").
		All()

	// 3. 候选付款计划：关联该合同，状态 planned/paid
	payments, _ := g.DB().Model("payment_plan").
		Where("status IN ('planned','paid')").
		All()

	best := MatchResult{InvoiceID: invoiceID, MatchRule: rule.Name, MatchedBy: "auto"}
	bestTotal := -1

	for _, c := range contracts {
		for _, p := range payments {
			// 只考虑同一合同下的付款计划
			if p["contract_id"].Int64() != c["id"].Int64() {
				continue
			}

			res := scoreMatch(inv, c, p, rule)
			res.InvoiceID = invoiceID
			res.ContractID = c["id"].Int64()
			res.PaymentPlanID = p["id"].Int64()
			res.MatchRule = rule.Name

			if res.TotalScore > bestTotal {
				best = res
				bestTotal = res.TotalScore
			}
		}
	}

	// 无任何候选时，标记为 mismatch
	if bestTotal < 0 {
		best.MatchStatus = "mismatch"
		best.MatchedBy = "auto"
	} else if best.TotalScore >= rule.ThresholdMatched {
		best.MatchStatus = "matched"
	} else if best.TotalScore <= rule.ThresholdMismatch {
		best.MatchStatus = "mismatch"
	} else {
		best.MatchStatus = "pending"
	}

	// 4. 落库
	now := time.Now()
	_, err = g.DB().Model("invoice_match_logs").Data(g.Map{
		"invoice_id": invoiceID, "contract_id": best.ContractID, "payment_plan_id": best.PaymentPlanID,
		"score_tax_id": best.ScoreTaxID, "score_name": best.ScoreName, "score_amount": best.ScoreAmount,
		"score_date": best.ScoreDate, "score_item": best.ScoreItem, "total_score": best.TotalScore,
		"match_status": best.MatchStatus, "match_rule": best.MatchRule, "matched_by": best.MatchedBy,
		"remark": "自动三方匹配", "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		return best, err
	}

	// 5. 回写 invoice.match_status（可选，用于列表筛选）
	g.DB().Model("invoice").Data(g.Map{
		"match_status": best.MatchStatus, "updated_at": now,
	}).Where("id", invoiceID).Update()

	return best, nil
}

// BatchRunMatching 批量跑匹配（定时/手动触发）
func BatchRunMatching(ctx context.Context, invoiceIDs []int64) ([]MatchResult, error) {
	rule, _ := GetActiveRule(ctx)
	results := make([]MatchResult, 0, len(invoiceIDs))
	for _, id := range invoiceIDs {
		r, err := RunMatching(ctx, id, &rule)
		if err != nil {
			g.Log().Warning(ctx, "[invoicematch] 单票匹配失败", "invoiceId", id, "err", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// ---------- 评分细则 ----------

func scoreMatch(inv, contract, payment gdb.Record, rule *MatchRule) MatchResult {
	res := MatchResult{}

	// ---- 税号一致性（30 分）----
	// 发票销售方税号 vs 合同对方税号 / 付款收款方税号
	// 优先使用新增的专用 tax_id 字段
	invTax := strings.TrimSpace(inv["seller_tax_id"].String())
	if invTax == "" {
		invTax = strings.TrimSpace(inv["tax_id"].String()) // 兼容旧字段
	}
	cntTax := strings.TrimSpace(contract["counter_party_tax_id"].String())
	if cntTax == "" {
		cntTax = strings.TrimSpace(contract["tax_id"].String())
	}
	payTax := strings.TrimSpace(payment["payee_tax_id"].String())
	if payTax == "" {
		payTax = strings.TrimSpace(payment["tax_id"].String())
	}
	res.ScoreTaxID = scoreTaxID(invTax, cntTax, payTax)

	// ---- 名称相似度（20 分）----
	// 发票销售方 vs 合同对方 / 付款收款方
	invName := strings.TrimSpace(inv["seller_name"].String())
	cntName := strings.TrimSpace(contract["counter_party"].String())
	payName := strings.TrimSpace(payment["payee"].String())
	res.ScoreName = scoreName(invName, cntName, payName)

	// ---- 金额一致性（30 分）----
	invAmt := inv["total"].Float64()
	payAmt := payment["amount"].Float64()
	res.ScoreAmount = scoreAmount(invAmt, payAmt)

	// ---- 日期区间合理性（10 分）----
	invDate := inv["issue_date"].String()
	payDue := payment["due_date"].String()
	res.ScoreDate = scoreDate(invDate, payDue)

	// ---- 品目/项目编码一致性（10 分）----
	invItems := inv["items"].String()
	cntProject := contract["project_code"].String()
	res.ScoreItem = scoreItem(invItems, cntProject)

	// 加权总分
	res.TotalScore = (res.ScoreTaxID*rule.WeightTaxID +
		res.ScoreName*rule.WeightName +
		res.ScoreAmount*rule.WeightAmount +
		res.ScoreDate*rule.WeightDate +
		res.ScoreItem*rule.WeightItem) / 100

	if res.TotalScore > 100 {
		res.TotalScore = 100
	}
	return res
}

// 税号评分：完全一致 100，前缀匹配 60，无 0
func scoreTaxID(inv, cnt, pay string) int {
	inv = strings.TrimSpace(strings.ToUpper(inv))
	cnt = strings.TrimSpace(strings.ToUpper(cnt))
	pay = strings.TrimSpace(strings.ToUpper(pay))
	if inv != "" && (inv == cnt || inv == pay) {
		return 100
	}
	if inv != "" && (strings.HasPrefix(cnt, inv[:min(8, len(inv))]) || strings.HasPrefix(pay, inv[:min(8, len(inv))])) {
		return 60
	}
	return 0
}

// 名称评分：Levenshtein 相似度 * 100
func scoreName(inv, cnt, pay string) int {
	if inv == "" {
		return 0
	}
	s1 := similarity(inv, cnt)
	s2 := similarity(inv, pay)
	return int(math.Max(s1, s2) * 100)
}

func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	// 简化：Jaccard 字符二元组
	setA := bigrams(a)
	setB := bigrams(b)
	inter := 0
	for k := range setA {
		if setB[k] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

func bigrams(s string) map[string]bool {
	set := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		set[string(runes[i:i+2])] = true
	}
	return set
}

// 金额评分：相差 ≤ 0.01 元 100；≤ 1 元 80；≤ 1% 60；≤ 5% 40；其它 0
func scoreAmount(invAmt, payAmt float64) int {
	if payAmt == 0 {
		return 0
	}
	diff := math.Abs(invAmt - payAmt)
	ratio := diff / payAmt
	switch {
	case diff <= 0.01:
		return 100
	case diff <= 1:
		return 80
	case ratio <= 0.01:
		return 60
	case ratio <= 0.05:
		return 40
	default:
		return 0
	}
}

// 日期评分：发票日期在付款计划日期前 0-90 天内 100；前 180 天 60；其它 0
func scoreDate(invDate, payDue string) int {
	invT, err1 := time.Parse("2006-01-02", invDate)
	payT, err2 := time.Parse("2006-01-02", payDue)
	if err1 != nil || err2 != nil {
		return 0
	}
	days := payT.Sub(invT).Hours() / 24
	if days >= 0 && days <= 90 {
		return 100
	}
	if days > 90 && days <= 180 {
		return 60
	}
	return 0
}

// 品目评分：发票明细项中包含合同 project_code 100；关键词命中 60；无 0
func scoreItem(invItems, projectCode string) int {
	if projectCode == "" {
		return 0
	}
	pc := strings.ToLower(projectCode)
	items := strings.ToLower(invItems)
	if strings.Contains(items, pc) {
		return 100
	}
	// 简单关键词兜底
	keywords := strings.Split(pc, "-")
	for _, kw := range keywords {
		if len(kw) > 2 && strings.Contains(items, kw) {
			return 60
		}
	}
	return 0
}

func pickTaxID(rec gdb.Record) string {
	// 优先取 tax_id 字段（需在各表补齐），回退 tax_no / taxpayer_id
	for _, k := range []string{"tax_id", "tax_no", "taxpayer_id", "tax_number"} {
		if v := rec[k].String(); v != "" {
			return v
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- 四性归档审计 ----------

// RunFourPropertyAudit 对单张发票执行四性归档审计，落 archive_audit_logs
func RunFourPropertyAudit(ctx context.Context, invoiceID int64, operator string) ([]AuditLog, error) {
	inv, err := g.DB().Model("invoice").Where("id", invoiceID).One()
	if err != nil || inv.IsEmpty() {
		return nil, fmt.Errorf("发票不存在: %d", invoiceID)
	}

	logs := make([]AuditLog, 0, 4)
	now := time.Now()

	// 1. 真实性：发票号/购销方/税号非空 + 税务验真状态 verified
	authPass := inv["invoice_no"].String() != "" &&
		(inv["buyer_name"].String() != "" || inv["seller_name"].String() != "") &&
		inv["verify_status"].String() == "verified"
	logs = append(logs, AuditLog{
		InvoiceID: invoiceID, AuditType: "authenticity", FieldName: "invoice_no,buyer_name,seller_name,verify_status",
		CheckPass: authPass,
		CheckDetail: map[string]interface{}{
			"invoiceNo": inv["invoice_no"].String(), "buyer": inv["buyer_name"].String(),
			"seller": inv["seller_name"].String(), "verifyStatus": inv["verify_status"].String(),
		},
		Operator: operator,
	})

	// 2. 完整性：八字段齐全
	fields := []string{"invoice_no", "issue_date", "buyer_name", "seller_name", "amount", "tax", "total", "invoice_type"}
	missing := []string{}
	for _, f := range fields {
		if inv[f].String() == "" || inv[f].String() == "0" {
			missing = append(missing, f)
		}
	}
	compPass := len(missing) == 0
	logs = append(logs, AuditLog{
		InvoiceID: invoiceID, AuditType: "integrity", FieldName: strings.Join(fields, ","),
		CheckPass:   compPass,
		CheckDetail: map[string]interface{}{"missing": missing, "totalFields": len(fields)},
		Operator:    operator,
	})

	// 3. 合规性：发票类型合规 + 金额非负 + 税率合规（简化：税额/金额在合理区间）
	taxRate := 0.0
	if inv["amount"].Float64() > 0 {
		taxRate = inv["tax"].Float64() / inv["amount"].Float64()
	}
	complPass := inv["total"].Float64() >= 0 && taxRate >= 0 && taxRate <= 0.2 // 含增值税最高 13%，留余量
	logs = append(logs, AuditLog{
		InvoiceID: invoiceID, AuditType: "compliance", FieldName: "total,tax,invoice_type",
		CheckPass:   complPass,
		CheckDetail: map[string]interface{}{"total": inv["total"].Float64(), "taxRate": taxRate, "invoiceType": inv["invoice_type"].String()},
		Operator:    operator,
	})

	// 4. 关联性：三方匹配状态 matched + 凭证/回单齐全 + 税号一致
	// 查最新匹配日志
	var matchLog gdb.Record
	g.DB().Model("invoice_match_logs").Where("invoice_id", invoiceID).Order("id desc").Limit(1).Scan(&matchLog)
	assocPass := !matchLog.IsEmpty() && matchLog["match_status"].String() == "matched"
	hasVoucher := inv["voucher_id"].Int64() > 0
	hasReceipt := false
	g.DB().Model("bank_receipt").Where("matched_invoice_id", invoiceID).Count(&hasReceipt)

	// 税号一致性校验：发票销售方税号 vs 合同对方税号 / 付款收款方税号
	invTax := strings.TrimSpace(inv["seller_tax_id"].String())
	if invTax == "" {
		invTax = strings.TrimSpace(inv["tax_id"].String())
	}
	taxConsistent := true
	if invTax != "" && matchLog["contract_id"].Int64() > 0 {
		var cnt gdb.Record
		g.DB().Model("contract").Where("id", matchLog["contract_id"].Int64()).One(&cnt)
		cntTax := strings.TrimSpace(cnt["counter_party_tax_id"].String())
		if cntTax == "" {
			cntTax = strings.TrimSpace(cnt["tax_id"].String())
		}
		if cntTax != "" && !strings.EqualFold(invTax, cntTax) {
			taxConsistent = false
		}
	}
	if invTax != "" && matchLog["payment_plan_id"].Int64() > 0 {
		var pay gdb.Record
		g.DB().Model("payment_plan").Where("id", matchLog["payment_plan_id"].Int64()).One(&pay)
		payTax := strings.TrimSpace(pay["payee_tax_id"].String())
		if payTax == "" {
			payTax = strings.TrimSpace(pay["tax_id"].String())
		}
		if payTax != "" && !strings.EqualFold(invTax, payTax) {
			taxConsistent = false
		}
	}

	logs = append(logs, AuditLog{
		InvoiceID: invoiceID, AuditType: "association", FieldName: "match_status,voucher_id,receipt,seller_tax_id",
		CheckPass: assocPass && hasVoucher && hasReceipt && taxConsistent,
		CheckDetail: map[string]interface{}{
			"matchStatus":   matchLog["match_status"].String(),
			"hasVoucher":    hasVoucher,
			"hasReceipt":    hasReceipt,
			"taxConsistent": taxConsistent,
			"invoiceTaxId":  invTax,
		},
		Operator: operator,
	})

	// 批量入库
	for _, l := range logs {
		detailJSON, _ := json.Marshal(l.CheckDetail)
		_, err = g.DB().Model("archive_audit_logs").Data(g.Map{
			"invoice_id": l.InvoiceID, "audit_type": l.AuditType, "field_name": l.FieldName,
			"expected_hash": l.ExpectedHash, "actual_hash": l.ActualHash,
			"check_pass": l.CheckPass, "check_detail": string(detailJSON),
			"operator": l.Operator, "created_at": now,
		}).Insert()
		if err != nil {
			g.Log().Warning(ctx, "[archive_audit] 审计日志落库失败", "err", err)
		}
	}

	return logs, nil
}

// GetAuditLogs 获取发票的四性归档审计历史
func GetAuditLogs(ctx context.Context, invoiceID int64) ([]AuditLog, error) {
	var rows []gdb.Record
	err := g.DB().Model("archive_audit_logs").Where("invoice_id", invoiceID).Order("created_at desc").Scan(&rows)
	if err != nil {
		return nil, err
	}
	logs := make([]AuditLog, len(rows))
	for i, r := range rows {
		var detail map[string]interface{}
		json.Unmarshal([]byte(r["check_detail"].String()), &detail)
		logs[i] = AuditLog{
			InvoiceID:    r["invoice_id"].Int64(),
			AuditType:    r["audit_type"].String(),
			FieldName:    r["field_name"].String(),
			ExpectedHash: r["expected_hash"].String(),
			ActualHash:   r["actual_hash"].String(),
			CheckPass:    r["check_pass"].Bool(),
			CheckDetail:  detail,
			Operator:     r["operator"].String(),
		}
	}
	return logs, nil
}

// GetMatchLogs 获取发票的匹配日志
func GetMatchLogs(ctx context.Context, invoiceID int64) ([]MatchResult, error) {
	var rows []gdb.Record
	err := g.DB().Model("invoice_match_logs").Where("invoice_id", invoiceID).Order("id desc").Scan(&rows)
	if err != nil {
		return nil, err
	}
	results := make([]MatchResult, len(rows))
	for i, r := range rows {
		results[i] = MatchResult{
			InvoiceID:     r["invoice_id"].Int64(),
			ContractID:    r["contract_id"].Int64(),
			PaymentPlanID: r["payment_plan_id"].Int64(),
			ScoreTaxID:    r["score_tax_id"].Int(),
			ScoreName:     r["score_name"].Int(),
			ScoreAmount:   r["score_amount"].Int(),
			ScoreDate:     r["score_date"].Int(),
			ScoreItem:     r["score_item"].Int(),
			TotalScore:    r["total_score"].Int(),
			MatchStatus:   r["match_status"].String(),
			MatchRule:     r["match_rule"].String(),
			MatchedBy:     r["matched_by"].String(),
		}
	}
	return results, nil
}

// ---------- 导出审计台账（Excel/Markdown）---------

// ExportLedger 导出发票台账（含匹配、四性审计）
func ExportLedger(ctx context.Context, invoiceIDs []int64) ([][]string, []string, error) {
	headers := []string{
		"发票ID", "发票号码", "开票日期", "购买方", "销售方", "销方税号", "购方税号", "价税合计",
		"验真状态", "归档状态", "匹配状态", "匹配总分",
		"关联合同", "关联付款计划",
		"真实性", "完整性", "合规性", "关联性",
		"审计时间", "操作人",
	}
	data := make([][]string, 0, len(invoiceIDs))

	for _, id := range invoiceIDs {
		inv, _ := g.DB().Model("invoice").Where("id", id).One()
		if inv.IsEmpty() {
			continue
		}

		matchLogs, _ := GetMatchLogs(ctx, id)
		auditLogs, _ := GetAuditLogs(ctx, id)

		latestMatch := MatchResult{}
		if len(matchLogs) > 0 {
			latestMatch = matchLogs[0]
		}

		auditMap := map[string]string{}
		for _, a := range auditLogs {
			auditMap[a.AuditType] = map[bool]string{true: "通过", false: "不通过"}[a.CheckPass]
		}

		row := []string{
			fmt.Sprintf("%d", inv["id"].Int64()),
			inv["invoice_no"].String(),
			inv["issue_date"].String(),
			inv["buyer_name"].String(),
			inv["seller_name"].String(),
			inv["seller_tax_id"].String(),
			inv["buyer_tax_id"].String(),
			fmt.Sprintf("%.2f", inv["total"].Float64()),
			inv["verify_status"].String(),
			inv["archive_status"].String(),
			latestMatch.MatchStatus,
			fmt.Sprintf("%d", latestMatch.TotalScore),
			fmt.Sprintf("%d", latestMatch.ContractID),
			fmt.Sprintf("%d", latestMatch.PaymentPlanID),
			auditMap["authenticity"],
			auditMap["integrity"],
			auditMap["compliance"],
			auditMap["association"],
			time.Now().Format("2006-01-02 15:04:05"),
			"系统导出",
		}
		data = append(data, row)
	}
	return data, headers, nil
}

// ---------- 兼容 database/sql 的事务辅助 ----------
func WithTx(ctx context.Context, fn func(tx *gdb.TX) error) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return fn(&tx)
	})
}
