package revrec

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// 收入确认 P1（v23 R-A6 增强 / R-A7 / R-A11）。
// 三条线口径：确认（权责发生，价税合计）/ 结算（出结算单）/ 收款（实际收钱）。
// R-A6 差额口径：差额 = 已确认 − 已结算；正 = 合同资产（干得多结算得少），负 = 合同负债（先结算/干得少）。

// LedgerRow2 合同结算台账（R-A6 增强版）
type LedgerRow2 struct {
	ContractFileID int64   `json:"contractFileId"`
	Title          string  `json:"title"`
	ContractAmount float64 `json:"contractAmount"`
	Confirmed      float64 `json:"confirmed"`   // 已确认收入（价税合计）
	Settled        float64 `json:"settled"`     // 已结算（出结算单）
	Received       float64 `json:"received"`    // 已收款
	Balance        float64 `json:"balance"`     // 差额 = 确认 − 结算
	BalanceType    string  `json:"balanceType"` // asset 合同资产 / liability 合同负债 / even 平衡
	Unreceived     float64 `json:"unreceived"`  // 确认 − 收款（应收）
}

// ClassifyBalance R-A6 纯函数：确认与结算的差额分类。正=合同资产，负=合同负债，零=平衡。
func ClassifyBalance(confirmed, settled float64) (string, float64) {
	b := round2(confirmed - settled)
	switch {
	case b > 0.005:
		return "asset", b
	case b < -0.005:
		return "liability", -b
	default:
		return "even", 0
	}
}

// SettlementAdd 登记结算（出结算单）。与回款（revrec_receipt）分开：结算是"该收多少"，收款是"实收多少"。
func SettlementAdd(ctx context.Context, cfID, oblID int64, amount float64, settledAt, remark, creator string) (int64, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("结算金额必须大于 0")
	}
	if _, err := time.Parse("2006-01-02", settledAt); err != nil {
		return 0, fmt.Errorf("结算日期格式需 YYYY-MM-DD")
	}
	res, err := g.DB().Model("revrec_settlement").Data(g.Map{
		"contract_file_id": cfID, "obligation_id": oblID, "amount": amount,
		"settled_at": settledAt, "remark": remark, "creator": creator, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SettlementList 结算记录列表（可按合同过滤）
func SettlementList(ctx context.Context, cfID int64) ([]g.Map, error) {
	m := g.DB().Model("revrec_settlement")
	if cfID > 0 {
		m = m.Where("contract_file_id", cfID)
	}
	rows, err := m.Order("settled_at desc, id desc").Limit(200).All()
	if err != nil {
		return nil, err
	}
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Map())
	}
	return out, nil
}

// Ledgers2 R-A6：合同结算台账（确认/结算/收款三线 + 合同资产/负债标注）
func Ledgers2(ctx context.Context) ([]LedgerRow2, error) {
	rows, err := g.DB().GetAll(ctx, `
		SELECT cf.id AS cfid, COALESCE(c.title, cf.filename, '') AS title, COALESCE(c.amount,0) AS amount,
		       COALESCE(r.total_sum,0) AS confirmed, COALESCE(s.settled_sum,0) AS settled,
		       COALESCE(rc.received_sum,0) AS received
		FROM contract_file cf LEFT JOIN contract c ON c.id = cf.group_id
		LEFT JOIN (SELECT o.file_id AS cfid, SUM(c.total) AS total_sum
		           FROM revrec_confirm c JOIN obligation o ON o.id=c.obligation_id
		           WHERE c.status='confirmed' GROUP BY o.file_id) r ON r.cfid=cf.id
		LEFT JOIN (SELECT contract_file_id AS cfid, SUM(amount) AS settled_sum
		           FROM revrec_settlement GROUP BY contract_file_id) s ON s.cfid=cf.id
		LEFT JOIN (SELECT contract_file_id AS cfid, SUM(amount) AS received_sum
		           FROM revrec_receipt GROUP BY contract_file_id) rc ON rc.cfid=cf.id
		ORDER BY cf.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	out := make([]LedgerRow2, 0, len(rows))
	for _, r := range rows {
		conf := r["confirmed"].Float64()
		set := r["settled"].Float64()
		rec := r["received"].Float64()
		bt, bal := ClassifyBalance(conf, set)
		out = append(out, LedgerRow2{
			ContractFileID: r["cfid"].Int64(), Title: r["title"].String(),
			ContractAmount: r["amount"].Float64(), Confirmed: conf, Settled: set,
			Received: rec, Balance: bal, BalanceType: bt, Unreceived: round2(conf - rec),
		})
	}
	return out, nil
}

// ArCrRow R-A7：应收实收差异行
type ArCrRow struct {
	ContractFileID int64   `json:"contractFileId"`
	Title          string  `json:"title"`
	Confirmed      float64 `json:"confirmed"`
	Received       float64 `json:"received"`
	Receivable     float64 `json:"receivable"` // 应收：确认 > 收款（已确认未收）
	Advance        float64 `json:"advance"`    // 预收：收款 > 确认（收了未确认）
	AgingDays      int     `json:"agingDays"`  // 应收账龄（最早确认日至今天，仅应收>0 时有意义）
}

// AgingDays 应收账龄（纯函数）：从最早确认日到今天的天数，无确认归 0。
func AgingDays(earliest, now time.Time) int {
	if earliest.IsZero() || now.IsZero() {
		return 0
	}
	d := int(now.Sub(earliest).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// ArCrDiff R-A7：应收（已确认未收，带账龄）vs 预收（已收款未确认）
func ArCrDiff(ctx context.Context) ([]ArCrRow, error) {
	rows, err := g.DB().GetAll(ctx, `
		SELECT cf.id AS cfid, COALESCE(c.title, cf.filename, '') AS title,
		       COALESCE(r.total_sum,0) AS confirmed, COALESCE(rc.received_sum,0) AS received,
		       r.earliest
		FROM contract_file cf LEFT JOIN contract c ON c.id = cf.group_id
		LEFT JOIN (SELECT o.file_id AS cfid, SUM(c.total) AS total_sum, MIN(c.confirmed_at) AS earliest
		           FROM revrec_confirm c JOIN obligation o ON o.id=c.obligation_id
		           WHERE c.status='confirmed' GROUP BY o.file_id) r ON r.cfid=cf.id
		LEFT JOIN (SELECT contract_file_id AS cfid, SUM(amount) AS received_sum
		           FROM revrec_receipt GROUP BY contract_file_id) rc ON rc.cfid=cf.id
		ORDER BY cf.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]ArCrRow, 0, len(rows))
	for _, r := range rows {
		conf := r["confirmed"].Float64()
		rec := r["received"].Float64()
		d := conf - rec
		row := ArCrRow{
			ContractFileID: r["cfid"].Int64(), Title: r["title"].String(),
			Confirmed: conf, Received: rec,
		}
		if d > 0.005 {
			row.Receivable = round2(d)
			if !r["earliest"].Time().IsZero() {
				row.AgingDays = AgingDays(r["earliest"].Time(), now)
			}
		} else if d < -0.005 {
			row.Advance = round2(-d)
		}
		out = append(out, row)
	}
	return out, nil
}

// MonthlyLine R-A11：月度视图一行（确认/结算/收款三条线）
type MonthlyLine struct {
	Period    string  `json:"period"`    // YYYY-MM
	Confirmed float64 `json:"confirmed"` // 已确认收入（价税合计）
	Settled   float64 `json:"settled"`   // 已结算
	Received  float64 `json:"received"`  // 已收款
}

// MonthOf 期间取月（纯函数）：零值返回空串
func MonthOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01")
}

// MonthlyView R-A11：按月看确认/结算/收款三条线（时间正序，便于对照现金流日历）
func MonthlyView(ctx context.Context) ([]MonthlyLine, error) {
	merge := map[string]*MonthlyLine{}
	put := func(period, kind string, amt float64) {
		if period == "" {
			return
		}
		l, ok := merge[period]
		if !ok {
			l = &MonthlyLine{Period: period}
			merge[period] = l
		}
		switch kind {
		case "c":
			l.Confirmed += amt
		case "s":
			l.Settled += amt
		case "r":
			l.Received += amt
		}
	}
	q := []struct {
		sql  string
		kind string
	}{
		{`SELECT period, SUM(total) AS amt FROM revrec_confirm WHERE status='confirmed' GROUP BY period`, "c"},
		{`SELECT to_char(settled_at,'YYYY-MM') AS period, SUM(amount) AS amt FROM revrec_settlement GROUP BY 1`, "s"},
		{`SELECT to_char(received_at,'YYYY-MM') AS period, SUM(amount) AS amt FROM revrec_receipt GROUP BY 1`, "r"},
	}
	for _, item := range q {
		rows, err := g.DB().GetAll(ctx, item.sql)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(r["period"].String(), item.kind, r["amt"].Float64())
		}
	}
	out := make([]MonthlyLine, 0, len(merge))
	for _, l := range merge {
		l.Confirmed = round2(l.Confirmed)
		l.Settled = round2(l.Settled)
		l.Received = round2(l.Received)
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period < out[j].Period })
	return out, nil
}
