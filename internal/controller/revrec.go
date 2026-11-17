package controller

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"time"

	"vouchchain/pkg/revrec"
)

// 收入确认接口（v23 R-A）：
//   PUT    /api/revrec/mode/{obligationId}   义务挂确认方式/比例/税率/预计成本
//   POST   /api/revrec/suggest               生成确认建议 {obligationId, accumCost?}（时点法/时段法自动分流）
//   POST   /api/revrec/confirm/{id}          人审确认（→journal_draft 走既有确认链）
//   GET    /api/revrec/list?obligationId=    确认记录列表
//   GET    /api/revrec/ledger                合同结算台账（确认/回款/差额）
//   POST   /api/revrec/receipt               回款登记
//   GET    /api/revrec/receipts?contractFileId= 回款列表

func RevRecSetMode(r *ghttp.Request) {
	id := r.Get("obligationId").Int64()
	var req struct {
		Mode        string  `json:"mode" v:"required"`
		ShareRatio  float64 `json:"shareRatio"`
		TaxRate     float64 `json:"taxRate"`
		PlannedCost float64 `json:"plannedCost"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := revrec.SetConfirmMode(r.Context(), id, req.Mode, req.ShareRatio, req.TaxRate, req.PlannedCost); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "revrec.set_mode", "obligation", id, g.Map{"mode": req.Mode, "ratio": req.ShareRatio})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func RevRecSuggest(r *ghttp.Request) {
	var req struct {
		ObligationID int64   `json:"obligationId" v:"required"`
		AccumCost    float64 `json:"accumCost"` // 时段法：本期累计成本
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	creator := r.GetCtxVar("user").String()
	var id int64
	var warnings []string
	var err error
	// 按义务的 confirm_mode 分流
	obl, _ := g.DB().Model("obligation").Where("id", req.ObligationID).One()
	if obl.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "义务不存在"})
		return
	}
	if obl["confirm_mode"].String() == "time" {
		id, warnings, err = revrec.SuggestByProgress(r.Context(), req.ObligationID, req.AccumCost, creator)
	} else {
		id, warnings, err = revrec.SuggestOnPointComplete(r.Context(), req.ObligationID, creator)
	}
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "revrec.suggest", "obligation", req.ObligationID, g.Map{"warnings": warnings})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "warnings": warnings}})
}

func RevRecConfirm(r *ghttp.Request) {
	id := r.Get("id").Int64()
	draftID, err := revrec.Confirm(r.Context(), id, r.GetCtxVar("user").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "revrec.confirm", "revrec_confirm", id, g.Map{"draftId": draftID})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"draftId": draftID}})
}

func RevRecList(r *ghttp.Request) {
	m := g.DB().Model("revrec_confirm")
	if v := r.Get("obligationId").Int64(); v > 0 {
		m = m.Where("obligation_id", v)
	}
	rows, err := m.Order("id desc").Limit(100).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, g.Map{
			"id": row["id"].Int64(), "obligationId": row["obligation_id"].Int64(),
			"period": row["period"].String(), "amount": row["amount"].Float64(),
			"tax": row["tax"].Float64(), "total": row["total"].Float64(),
			"cost":     row["cost"].Float64(),
			"progress": row["progress"].Float64(), "source": row["source"].String(),
			"status": row["status"].String(), "draftId": row["draft_id"].Int64(),
			"creator": row["creator"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func RevRecLedger(r *ghttp.Request) {
	list, err := revrec.Ledgers(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func RevRecReceiptAdd(r *ghttp.Request) {
	var req struct {
		ContractFileID int64   `json:"contractFileId"`
		ObligationID   int64   `json:"obligationId"`
		Amount         float64 `json:"amount" v:"required"`
		ReceivedAt     string  `json:"receivedAt" v:"required"`
		Remark         string  `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	res, err := g.DB().Model("revrec_receipt").Data(g.Map{
		"contract_file_id": req.ContractFileID, "obligation_id": req.ObligationID,
		"amount": req.Amount, "received_at": req.ReceivedAt, "remark": req.Remark,
		"creator": r.GetCtxVar("user").String(), "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "revrec.receipt", "contract_file", req.ContractFileID, g.Map{"amount": req.Amount})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func RevRecReceiptList(r *ghttp.Request) {
	m := g.DB().Model("revrec_receipt")
	if v := r.Get("contractFileId").Int64(); v > 0 {
		m = m.Where("contract_file_id", v)
	}
	rows, err := m.Order("received_at desc").Limit(100).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Map())
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}
