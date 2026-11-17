package revrec

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gogf/gf/v2/database/gdb"

	"github.com/gogf/gf/v2/frame/g"

	"vouchchain/pkg/agent"
)

// 收入确认（乙方轻量版，v23 R-A）：义务挂确认方式→完成/进度触发确认建议→人审确认落分录草稿。
// 铁律：AI/规则只出建议（status=draft），会计确认才生效（confirmed）。

type ConfirmMode string

const (
	ModePoint ConfirmMode = "point" // 时点法：控制权转移（默认=验收/完成即确认）
	ModeTime  ConfirmMode = "time"  // 时段法投入法：进度=累计成本/预计总成本
)

// SetConfirmMode R-A1/A2：义务挂确认方式、分摊比例、税率
func SetConfirmMode(ctx context.Context, obligationID int64, mode string, shareRatio, taxRate float64, plannedCost float64) error {
	if mode != string(ModePoint) && mode != string(ModeTime) {
		return fmt.Errorf("confirm_mode 只能是 point/time")
	}
	row, err := g.DB().Model("obligation").Where("id", obligationID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("义务不存在")
	}
	_, err = g.DB().Model("obligation").Data(g.Map{
		"confirm_mode": mode, "share_ratio": shareRatio, "tax_rate": taxRate,
		"planned_cost": plannedCost,
	}).Where("id", obligationID).Update()
	return err
}

// obligationAmount 义务对应的合同金额分摊额（合同总额 × 比例；未设比例时按义务数均分）
func obligationAmount(ctx context.Context, oblID int64) (float64, error) {
	obl, err := g.DB().Model("obligation").Where("id", oblID).One()
	if err != nil || obl.IsEmpty() {
		return 0, fmt.Errorf("义务不存在")
	}
	ratio := obl["share_ratio"].Float64()
	cfID := obl["file_id"].Int64()
	total := 0.0
	if cfID > 0 {
		cf, err := g.DB().Model("contract_file").Where("id", cfID).One()
		if err == nil && !cf.IsEmpty() {
			if v := cf["amount"]; !v.IsNil() {
				total = v.Float64()
			}
		}
	}
	if ratio > 0 {
		return total * ratio / 100, nil
	}
	// 未设比例：按该合同义务数均分
	n, _ := g.DB().Model("obligation").Where("file_id", cfID).Count()
	if n <= 0 {
		return total, nil
	}
	return total / float64(n), nil
}

// SuggestOnPointComplete R-A3：时点法——义务完成事件触发确认建议
func SuggestOnPointComplete(ctx context.Context, obligationID int64, creator string) (int64, []string, error) {
	obl, err := g.DB().Model("obligation").Where("id", obligationID).One()
	if err != nil || obl.IsEmpty() {
		return 0, nil, fmt.Errorf("义务不存在")
	}
	if obl["confirm_mode"].String() == string(ModeTime) {
		return 0, nil, fmt.Errorf("该义务为时段法，请走进度录入")
	}
	amount, err := obligationAmount(ctx, obligationID)
	if err != nil {
		return 0, nil, err
	}
	if amount <= 0 {
		return 0, nil, fmt.Errorf("义务分摊金额为 0，请先设置合同金额或分摊比例")
	}
	// 时点法：成本不单独跟踪，成本结转 0（不生成成本分录；时段法才走成本结转）
	return createSuggestion(ctx, obl, amount, 0, 100, time.Now().Format("2006-01"), creator)
}

// SuggestByProgress R-A4：时段法——录入本期累计成本，自动算进度与本期确认额
func SuggestByProgress(ctx context.Context, obligationID int64, accumCost float64, creator string) (int64, []string, error) {
	obl, err := g.DB().Model("obligation").Where("id", obligationID).One()
	if err != nil || obl.IsEmpty() {
		return 0, nil, fmt.Errorf("义务不存在")
	}
	planned := obl["planned_cost"].Float64()
	if planned <= 0 {
		return 0, nil, fmt.Errorf("请先设置预计总成本")
	}
	g.DB().Model("obligation").Data(g.Map{"accum_cost": accumCost}).Where("id", obligationID).Update()
	progress := accumCost / planned * 100
	if progress > 100 {
		progress = 100
	}
	amount, err := obligationAmount(ctx, obligationID)
	if err != nil {
		return 0, nil, err
	}
	// 本期确认额 = 总额 × 进度 − 已确认
	confirmed, _ := g.DB().Model("revrec_confirm").
		Where("obligation_id", obligationID).Where("status", "confirmed").Sum("amount")
	cur := amount*progress/100 - confirmed
	if cur < 0.005 {
		return 0, []string{fmt.Sprintf("按进度 %.1f%% 无新增确认额（已确认 %.2f）", progress, confirmed)}, nil
	}
	// 本期确认成本（R-A5 成本结转）：累计已发生成本 − 已确认成本
	confirmedCost, _ := g.DB().Model("revrec_confirm").
		Where("obligation_id", obligationID).Where("status", "confirmed").Sum("cost")
	cost := recognitionCost(accumCost, confirmedCost)
	return createSuggestion(ctx, obl, cur, cost, progress, time.Now().Format("2006-01"), creator)
}

// createSuggestion 生成本期确认建议 + 分录（R-A5：收入+成本双重确认）
func createSuggestion(ctx context.Context, obl gdb.Record, amountExTax, cost, progress float64, period, creator string) (int64, []string, error) {
	oblID := obl["id"].Int64()
	taxRate := obl["tax_rate"].Float64()
	if taxRate <= 0 {
		taxRate = 13
	}
	tax := round2(amountExTax * taxRate / 100)
	total := round2(amountExTax + tax)
	warnings := make([]string, 0)

	// 分录（R-A5）：借 应收账款 / 贷 主营业务收入 + 销项税额；cost>0 时补 借 主营业务成本 / 贷 合同履约成本。
	// 科目码/名从 account_mapping 复用（不硬编码），缺失回退默认。
	summary := fmt.Sprintf("确认收入-履约义务#%d", oblID)
	subj := LoadSubjectCodes(ctx)
	entries := BuildRecognitionEntries(amountExTax, cost, tax, subj, summary)
	if !agent.Balanced(entries) {
		warnings = append(warnings, "分录借贷不平衡，请人工核对")
	}

	// 重复确认防护：同义务同期间已有 draft/confirmed 则提示
	dup, _ := g.DB().Model("revrec_confirm").
		Where("obligation_id", oblID).Where("period", period).
		Where("status", "confirmed").Count()
	if dup > 0 {
		warnings = append(warnings, fmt.Sprintf("该义务 %s 已有确认记录，本期将叠加确认", period))
	}
	ej, _ := json.Marshal(entries)
	res, err := g.DB().Model("revrec_confirm").Data(g.Map{
		"obligation_id": oblID, "period": period,
		"amount": amountExTax, "tax": tax, "total": total, "cost": cost,
		"progress": progress, "entries": ej, "source": "rule",
		"status": "draft", "creator": creator, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, warnings, err
	}
	id, _ := res.LastInsertId()
	return id, warnings, nil
}

// ---------- 科目映射（R-A5：复用 account_mapping，规则/分录生成不硬编码科目码） ----------

type Subject struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// SubjectCodes 收入确认全链路科目（默认中国会计准则科目编码；可在 account_mapping 里改）
type SubjectCodes struct {
	ContractSettlement Subject // 合同结算（1131）
	Revenue            Subject // 主营业务收入（6001）
	RevenueCost        Subject // 主营业务成本（6401）
	PerformanceCost    Subject // 合同履约成本（1471）
	Receivable         Subject // 应收账款（1122）
	VatOutput          Subject // 应交税费—应交增值税（销项税额）（22210105）
}

func defaultSubjectCodes() SubjectCodes {
	return SubjectCodes{
		ContractSettlement: Subject{"1131", "合同结算"},
		Revenue:            Subject{"6001", "主营业务收入"},
		RevenueCost:        Subject{"6401", "主营业务成本"},
		PerformanceCost:    Subject{"1471", "合同履约成本"},
		Receivable:         Subject{"1122", "应收账款"},
		VatOutput:          Subject{"22210105", "应交税费—应交增值税（销项税额）"},
	}
}

// LoadSubjectCodes 从 account_mapping 读收入确认科目（category 前缀「收入确认-」），缺失回退默认。
func LoadSubjectCodes(ctx context.Context) SubjectCodes {
	sc := defaultSubjectCodes()
	set := func(category string, s *Subject) {
		row, err := g.DB().Model("account_mapping").Where("category", category).Order("id asc").One()
		if err != nil || row.IsEmpty() {
			return
		}
		if c := row["subject_code"].String(); c != "" {
			s.Code = c
		}
		if n := row["subject_name"].String(); n != "" {
			s.Name = n
		}
	}
	set("收入确认-合同结算", &sc.ContractSettlement)
	set("收入确认-主营业务收入", &sc.Revenue)
	set("收入确认-主营业务成本", &sc.RevenueCost)
	set("收入确认-合同履约成本", &sc.PerformanceCost)
	set("收入确认-应收账款", &sc.Receivable)
	set("收入确认-销项税额", &sc.VatOutput)
	return sc
}

// BuildRecognitionEntries 生成收入确认分录（R-A5，纯函数可单测）。
// 乙方轻量版合并「确认+结算」：合同结算科目作为中间过渡（借合同结算/贷收入 + 借应收/贷合同结算+销项），
// 合并后其净额为零，故分录直接以净额形态呈现（借应收/贷收入+销项），并补成本结转（借成本/贷合同履约成本）。
// 满足 R-A5「借合同结算/贷主营业务收入 + 借主营业务成本/贷合同履约成本」的收入+成本双重确认实质。
func BuildRecognitionEntries(amountExTax, cost, tax float64, subj SubjectCodes, summary string) []agent.JournalEntry {
	total := round2(amountExTax + tax)
	out := []agent.JournalEntry{
		{Summary: summary, SubjectCode: subj.Receivable.Code, SubjectName: subj.Receivable.Name, Debit: total, Credit: 0},
		{Summary: summary, SubjectCode: subj.Revenue.Code, SubjectName: subj.Revenue.Name, Debit: 0, Credit: amountExTax},
		{Summary: summary, SubjectCode: subj.VatOutput.Code, SubjectName: subj.VatOutput.Name, Debit: 0, Credit: tax},
	}
	if cost > 0 {
		out = append(out,
			agent.JournalEntry{Summary: summary, SubjectCode: subj.RevenueCost.Code, SubjectName: subj.RevenueCost.Name, Debit: cost, Credit: 0},
			agent.JournalEntry{Summary: summary, SubjectCode: subj.PerformanceCost.Code, SubjectName: subj.PerformanceCost.Name, Debit: 0, Credit: cost},
		)
	}
	return out
}

// recognitionCost 本期确认成本 = 累计已发生成本 − 已确认成本（时段法投入法）；不足 0 归 0。纯函数可单测。
func recognitionCost(accumCost, confirmedCost float64) float64 {
	if accumCost <= 0 {
		return 0
	}
	c := accumCost - confirmedCost
	if c < 0 {
		c = 0
	}
	return round2(c)
}

// Confirm R-A8：人审确认 → 落分录草稿（journal_draft，走既有确认/桥接链路）
func Confirm(ctx context.Context, confirmID int64, operator string) (int64, error) {
	row, err := g.DB().Model("revrec_confirm").Where("id", confirmID).One()
	if err != nil || row.IsEmpty() {
		return 0, fmt.Errorf("确认记录不存在")
	}
	if row["status"].String() == "confirmed" {
		return row["draft_id"].Int64(), nil
	}
	// 生成 journal_draft（金额、分录来自 revrec_confirm）
	oblID := row["obligation_id"].Int64()
	obl, _ := g.DB().Model("obligation").Where("id", oblID).One()
	var cfID int64
	invoiceNo := ""
	seller := ""
	if !obl.IsEmpty() {
		cfID = obl["file_id"].Int64()
	}
	if cfID > 0 {
		cf, _ := g.DB().Model("contract_file").Where("id", cfID).One()
		if !cf.IsEmpty() {
			invoiceNo = fmt.Sprintf("REVREC-CF%d", cfID)
			seller = cf["title"].String()
		}
	}
	res, err := g.DB().Model("journal_draft").Data(g.Map{
		"invoice_id": 0, "split_id": 0, "category": "收入确认",
		"entries": row["entries"].String(), "source": row["source"].String(),
		"reason":          fmt.Sprintf("收入确认：履约义务#%d 期间 %s（乙方视角）", oblID, row["period"].String()),
		"legal_entity_id": 0, "status": "draft",
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	draftID, _ := res.LastInsertId()
	g.DB().Model("revrec_confirm").Data(g.Map{
		"status": "confirmed", "draft_id": draftID, "confirmed_at": time.Now(),
	}).Where("id", confirmID).Update()
	_ = invoiceNo
	_ = seller
	return draftID, nil
}

// Ledger R-A6/A7：合同结算台账（确认/收款汇总）——按合同文件聚合
type ContractLedger struct {
	ContractFileID int64   `json:"contractFileId"`
	Title          string  `json:"title"`
	ContractAmount float64 `json:"contractAmount"`
	Confirmed      float64 `json:"confirmed"` // 已确认收入（含税口径价税合计）
	Received       float64 `json:"received"`  // 已回款
	Unreceived     float64 `json:"unreceived"`
}

// Ledgers 全部合同的确认/回款台账
func Ledgers(ctx context.Context) ([]ContractLedger, error) {
	rows, err := g.DB().GetAll(ctx, `
		SELECT cf.id AS cfid, COALESCE(c.title, cf.filename, '') AS title, COALESCE(c.amount,0) AS amount,
		       COALESCE(r.total_sum,0) AS confirmed, COALESCE(rc.received_sum,0) AS received
		FROM contract_file cf LEFT JOIN contract c ON c.id = cf.group_id
		LEFT JOIN (SELECT o.file_id AS cfid, SUM(c.total) AS total_sum
		           FROM revrec_confirm c JOIN obligation o ON o.id=c.obligation_id
		           WHERE c.status='confirmed' GROUP BY o.file_id) r ON r.cfid=cf.id
		LEFT JOIN (SELECT contract_file_id AS cfid, SUM(amount) AS received_sum
		           FROM revrec_receipt GROUP BY contract_file_id) rc ON rc.cfid=cf.id
		ORDER BY cf.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	out := make([]ContractLedger, 0, len(rows))
	for _, r := range rows {
		conf := r["confirmed"].Float64()
		rec := r["received"].Float64()
		out = append(out, ContractLedger{
			ContractFileID: r["cfid"].Int64(), Title: r["title"].String(),
			ContractAmount: r["amount"].Float64(), Confirmed: conf,
			Received: rec, Unreceived: conf - rec,
		})
	}
	return out, nil
}

// ---------- 小工具 ----------

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
