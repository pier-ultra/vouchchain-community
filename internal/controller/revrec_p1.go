package controller

import (
	"strconv"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/revrec"
)

// 收入确认 P1 接口（v23 R-A6 增强 / R-A7 / R-A11 / R-A14）：
//   POST   /api/revrec/settlement        结算登记（出结算单）
//   GET    /api/revrec/settlements?contractFileId=  结算记录列表
//   GET    /api/revrec/ledger/v2           合同结算台账（确认/结算/收款三线 + 合同资产/负债）
//   GET    /api/revrec/ar-cr-diff         应收实收差异表（应收带账龄 / 预收）
//   GET    /api/revrec/monthly            收入确认月度视图（确认/结算/收款三条线）
//   GET    /api/revrec/export             审计口径 Excel 导出（台账/确认明细/差异/月度四张表）

func RevRecSettlementAdd(r *ghttp.Request) {
	var req struct {
		ContractFileID int64   `json:"contractFileId"`
		ObligationID   int64   `json:"obligationId"`
		Amount         float64 `json:"amount" v:"required"`
		SettledAt      string  `json:"settledAt" v:"required"`
		Remark         string  `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, err := revrec.SettlementAdd(r.Context(), req.ContractFileID, req.ObligationID, req.Amount, req.SettledAt, req.Remark, r.GetCtxVar("user").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "revrec.settlement", "contract_file", req.ContractFileID, g.Map{"amount": req.Amount, "settledAt": req.SettledAt})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func RevRecSettlementList(r *ghttp.Request) {
	list, err := revrec.SettlementList(r.Context(), r.Get("contractFileId").Int64())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func RevRecLedgerV2(r *ghttp.Request) {
	list, err := revrec.Ledgers2(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func RevRecArCrDiff(r *ghttp.Request) {
	list, err := revrec.ArCrDiff(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func RevRecMonthly(r *ghttp.Request) {
	list, err := revrec.MonthlyView(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// RevRecExport R-A14 审计口径 Excel 导出（一个 Excel 四张表，对账/审计一次拿全）：
// 合同结算台账 / 确认明细 / 应收预收差异 / 月度三线。
// 口径：一律 Excel 导出，不再出 CSV（用户口径）。
func RevRecExport(r *ghttp.Request) {
	ctx := r.Context()
	ledgers, err := revrec.Ledgers2(ctx)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	balName := map[string]string{"asset": "合同资产", "liability": "合同负债", "even": "平衡"}
	s1h := []string{"合同ID", "合同名称", "合同金额", "已确认(价税合计)", "已结算", "已收款", "差额", "差额性质", "未收(应收)"}
	s1 := make([][]string, 0, len(ledgers))
	for _, l := range ledgers {
		s1 = append(s1, []string{
			strconv.FormatInt(l.ContractFileID, 10), l.Title, money(l.ContractAmount),
			money(l.Confirmed), money(l.Settled), money(l.Received),
			money(l.Balance), balName[l.BalanceType], money(l.Unreceived),
		})
	}

	// 确认明细：逐笔确认记录（审计追溯用）；合同文件ID 由 obligation 表关联取
	confRows, err := g.DB().GetAll(ctx, `
		SELECT c.id, COALESCE(o.file_id,0) AS file_id, c.obligation_id, c.period, c.amount, c.tax,
		       c.total, c.cost, c.progress, c.source, c.status, c.draft_id, c.creator, c.confirmed_at
		FROM revrec_confirm c LEFT JOIN obligation o ON o.id=c.obligation_id
		ORDER BY c.id DESC LIMIT 2000`)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	s2h := []string{"确认ID", "合同文件ID", "义务ID", "期间", "不含税", "税额", "价税合计", "成本", "进度", "来源", "状态", "凭证草稿", "创建人", "确认时间"}
	s2 := make([][]string, 0, len(confRows))
	for _, row := range confRows {
		s2 = append(s2, []string{
			strconv.FormatInt(row["id"].Int64(), 10),
			strconv.FormatInt(row["file_id"].Int64(), 10),
			strconv.FormatInt(row["obligation_id"].Int64(), 10),
			row["period"].String(), money(row["amount"].Float64()), money(row["tax"].Float64()),
			money(row["total"].Float64()), money(row["cost"].Float64()),
			strconv.FormatFloat(row["progress"].Float64(), 'f', 4, 64),
			row["source"].String(), row["status"].String(),
			strconv.FormatInt(row["draft_id"].Int64(), 10), row["creator"].String(),
			row["confirmed_at"].Time().Format("2006-01-02 15:04:05"),
		})
	}

	// 应收/预收差异（含应收账龄）
	acrs, err := revrec.ArCrDiff(ctx)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	s3h := []string{"合同ID", "合同名称", "已确认", "已收款", "应收(确认未收)", "预收(收款未确认)", "应收账龄(天)"}
	s3 := make([][]string, 0, len(acrs))
	for _, a := range acrs {
		s3 = append(s3, []string{
			strconv.FormatInt(a.ContractFileID, 10), a.Title, money(a.Confirmed), money(a.Received),
			money(a.Receivable), money(a.Advance), strconv.Itoa(a.AgingDays),
		})
	}

	// 月度三线
	months, err := revrec.MonthlyView(ctx)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	s4h := []string{"期间", "已确认", "已结算", "已收款"}
	s4 := make([][]string, 0, len(months))
	for _, m := range months {
		s4 = append(s4, []string{m.Period, money(m.Confirmed), money(m.Settled), money(m.Received)})
	}

	name := "revrec-" + time.Now().Format("20060102-150405") + ".xlsx"
	if err := sendXLSXSheets(r, name, []xlsxSheet{
		{Name: "合同结算台账", Headers: s1h, Rows: s1},
		{Name: "确认明细", Headers: s2h, Rows: s2},
		{Name: "应收预收差异", Headers: s3h, Rows: s3},
		{Name: "月度三线", Headers: s4h, Rows: s4},
	}); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "revrec.export", "revrec_confirm", 0, g.Map{
		"ledgerRows": len(s1), "confirmRows": len(s2), "arcrRows": len(s3),
		"monthRows": len(s4), "operator": r.GetCtxVar("user").String(),
	})
}

// money 导出金额统一两位小数字符串（审计口径可比对）。
func money(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
