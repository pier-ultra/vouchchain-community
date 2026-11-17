package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/agent"
)

// ---------- P2P 闭环补格：③ 应付台账/付款计划 + ④ AI 分录草稿 ----------
// 链路：合同义务 → 收票验真 → AI 分录建议 → 会计确认入账 → 挂应付 → 按期打款
//       → 回单销账 → 单套制归档。总账本体/报表/报税不碰（用户 11-14 拍板红线）。

func parseBizDate(s string) time.Time {
	for _, layout := range []string{"2006-01-02", "2006/01/02", "20060102", "2006年1月2日", "2006-1-2"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Now()
}

// PayableGenerate 从已入库发票批量生成应付计划（默认只收验真通过的票；同一发票幂等不重复建）
func PayableGenerate(r *ghttp.Request) {
	var req struct {
		DueDate      string `json:"dueDate"`      // 指定统一到期日（空则按账期天数推算）
		Days         int    `json:"days"`         // 账期天数，默认 30
		OnlyVerified bool   `json:"onlyVerified"` // 默认 true：未验真不入付款计划
	}
	_ = r.Parse(&req)
	if req.Days <= 0 {
		req.Days = 30
	}
	if req.DueDate == "" && !r.Get("onlyVerified").IsEmpty() {
		req.OnlyVerified = r.Get("onlyVerified").Bool()
	} else if r.Get("onlyVerified").IsEmpty() {
		req.OnlyVerified = true
	}
	invs, err := scopeFilter(g.DB().Model("invoice"), r).Order("id asc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	created, skipped := 0, 0
	for _, inv := range invs {
		id := inv["id"].Int64()
		if req.OnlyVerified && inv["verify_status"].String() != "verified" {
			skipped++
			continue
		}
		exist, _ := g.DB().Model("payment_plan").Where("invoice_id", id).One()
		if !exist.IsEmpty() {
			skipped++
			continue
		}
		due := req.DueDate
		if due == "" {
			due = parseBizDate(inv["issue_date"].String()).AddDate(0, 0, req.Days).Format("2006-01-02")
		}
		_, err := g.DB().Model("payment_plan").Data(g.Map{
			"invoice_id": id, "payee": inv["seller_name"].String(),
			"amount": inv["total"].Float64(), "due_date": due,
			"status": "planned", "created_at": time.Now(), "updated_at": time.Now(),
			"legal_entity_id": inv["legal_entity_id"].Int64(),
		}).Insert()
		if err != nil {
			skipped++
			continue
		}
		created++
	}
	audit(r, "payable.generate", "payment_plan", 0, g.Map{"created": created, "skipped": skipped, "days": req.Days})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"created": created, "skipped": skipped}})
}

// PayableList 应付台账 + 顶部汇总（待付/逾期/已付），附发票号与验真状态
func PayableList(r *ghttp.Request) {
	m := scopeFilter(g.DB().Model("payment_plan"), r)
	if st := r.Get("status").String(); st != "" {
		m = m.Where("status", st)
	}
	list, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	rows := make([]g.Map, 0, len(list))
	var plannedSum, paidSum float64
	overdue := 0
	today := time.Now().Format("2006-01-02")
	for _, p := range list {
		inv, _ := g.DB().Model("invoice").Where("id", p["invoice_id"].Int64()).One()
		row := g.Map{
			"id": p["id"].Int64(), "invoiceId": p["invoice_id"].Int64(),
			"invoiceNo": inv["invoice_no"].String(), "sellerName": inv["seller_name"].String(),
			"verifyStatus": inv["verify_status"].String(),
			"payee":        p["payee"].String(), "amount": p["amount"].Float64(),
			"dueDate": p["due_date"].String(), "status": p["status"].String(),
			"paidDate": p["paid_date"].String(), "receiptId": p["receipt_id"].Int64(),
			"remark": p["remark"].String(),
		}
		overdueFlag := p["status"].String() == "planned" && p["due_date"].String() != "" && p["due_date"].String() < today
		row["overdue"] = overdueFlag
		if overdueFlag {
			overdue++
		}
		switch p["status"].String() {
		case "planned":
			plannedSum += p["amount"].Float64()
		case "paid":
			paidSum += p["amount"].Float64()
		}
		rows = append(rows, row)
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"list":    rows,
		"summary": g.Map{"plannedAmount": plannedSum, "paidAmount": paidSum, "overdueCount": overdue},
	}})
}

// PayableUpdate 改账期/金额/状态（会计手工调整留痕）
func PayableUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		DueDate string   `json:"dueDate"`
		Amount  *float64 `json:"amount"`
		Status  string   `json:"status"`
		Remark  string   `json:"remark"`
	}
	if err := r.Parse(&req); err != nil || id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id 与参数必填"})
		return
	}
	data := g.Map{"updated_at": time.Now()}
	if req.DueDate != "" {
		data["due_date"] = req.DueDate
	}
	if req.Amount != nil {
		data["amount"] = *req.Amount
	}
	if req.Status != "" {
		if req.Status != "planned" && req.Status != "paid" && req.Status != "cancelled" {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "status 仅支持 planned/paid/cancelled"})
			return
		}
		data["status"] = req.Status
	}
	data["remark"] = req.Remark
	if _, err := g.DB().Model("payment_plan").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "payable.update", "payment_plan", id, g.Map{"dueDate": req.DueDate, "status": req.Status})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// PayablePay 登记打款：计划置 paid + 自动写银行回单并匹配该发票（证据链回单层闭合）
func PayablePay(r *ghttp.Request) {
	id := r.Get("id").Int64()
	plan, err := g.DB().Model("payment_plan").Where("id", id).One()
	if err != nil || plan.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "付款计划不存在"})
		return
	}
	if plan["status"].String() == "paid" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该计划已登记打款，勿重复操作"})
		return
	}
	var req struct {
		PaidDate string `json:"paidDate"`
		Payer    string `json:"payer"`
		Remark   string `json:"remark"`
	}
	_ = r.Parse(&req)
	if req.PaidDate == "" {
		req.PaidDate = time.Now().Format("2006-01-02")
	}
	inv, _ := g.DB().Model("invoice").Where("id", plan["invoice_id"].Int64()).One()
	payer := req.Payer
	if payer == "" {
		payer = inv["buyer_name"].String()
	}
	res, err := g.DB().Model("bank_receipt").Data(g.Map{
		"payer": payer, "payee": plan["payee"].String(), "amount": plan["amount"].Float64(),
		"receipt_date": req.PaidDate, "matched_invoice_id": plan["invoice_id"].Int64(),
		"match_rule": "付款计划登记打款自动关联（发票 " + inv["invoice_no"].String() + "）",
		"remark":     req.Remark, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	receiptId, _ := res.LastInsertId()
	g.DB().Model("payment_plan").Data(g.Map{
		"status": "paid", "paid_date": req.PaidDate, "receipt_id": receiptId,
		"remark": req.Remark, "updated_at": time.Now(),
	}).Where("id", id).Update()
	audit(r, "payable.pay", "payment_plan", id, g.Map{"receiptId": receiptId, "amount": plan["amount"].Float64()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"receiptId": receiptId, "status": "paid"}})
}

// ---------- 科目映射 ----------

func AccountMappingList(r *ghttp.Request) {
	list, err := g.DB().Model("account_mapping").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func AccountMappingSave(r *ghttp.Request) {
	var req struct {
		Category    string `json:"category"`
		SubjectCode string `json:"subjectCode"`
		SubjectName string `json:"subjectName"`
		Direction   string `json:"direction"`
	}
	if err := r.Parse(&req); err != nil || req.Category == "" || req.SubjectCode == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "category / subjectCode 必填"})
		return
	}
	if req.Direction == "" {
		req.Direction = "debit"
	}
	exist, _ := g.DB().Model("account_mapping").Where("category", req.Category).Where("direction", req.Direction).One()
	data := g.Map{"subject_code": req.SubjectCode, "subject_name": req.SubjectName}
	var err error
	if exist.IsEmpty() {
		data["category"] = req.Category
		data["direction"] = req.Direction
		data["created_at"] = time.Now()
		_, err = g.DB().Model("account_mapping").Data(data).Insert()
	} else {
		_, err = g.DB().Model("account_mapping").Data(data).Where("id", exist["id"].Int64()).Update()
	}
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "account_mapping.save", "account_mapping", exist["id"].Int64(), g.Map{"category": req.Category, "subjectCode": req.SubjectCode})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// ---------- AI 分录草稿 ----------

func loadSubjects() ([]agent.AccountSubject, error) {
	rows, err := g.DB().Model("account_mapping").Order("id asc").All()
	if err != nil {
		return nil, err
	}
	out := make([]agent.AccountSubject, 0, len(rows))
	for _, row := range rows {
		out = append(out, agent.AccountSubject{
			Category: row["category"].String(), SubjectCode: row["subject_code"].String(),
			SubjectName: row["subject_name"].String(), Direction: row["direction"].String(),
		})
	}
	return out, nil
}

// JournalDraftCreate 发票 → 分录建议（AI 优先，失败自动回退规则）；同发票未确认草稿覆盖更新
func JournalDraftCreate(r *ghttp.Request) {
	var req struct {
		InvoiceId int64  `json:"invoiceId"`
		Category  string `json:"category"`
		Mode      string `json:"mode"` // ai（默认） / rule
	}
	if err := r.Parse(&req); err != nil || req.InvoiceId <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "invoiceId 必填"})
		return
	}
	inv, err := g.DB().Model("invoice").Where("id", req.InvoiceId).One()
	if err != nil || inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "发票不存在"})
		return
	}
	if req.Category == "" {
		req.Category = "服务采购"
	}
	subjects, err := loadSubjects()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	facts := agent.JournalFacts{
		InvoiceNo: inv["invoice_no"].String(), InvoiceType: inv["invoice_type"].String(),
		SellerName: inv["seller_name"].String(), BuyerName: inv["buyer_name"].String(),
		Category: req.Category, Items: inv["items"].String(),
		Amount: inv["amount"].Float64(), Tax: inv["tax"].Float64(), Total: inv["total"].Float64(),
	}
	sug, err := agent.SuggestJournal(r.Context(), facts, subjects, req.Mode != "rule")
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	ej, _ := json.Marshal(sug.Entries)
	exist, _ := g.DB().Model("journal_draft").
		Where("invoice_id", req.InvoiceId).Where("status", "draft").Order("id desc").Limit(1).One()
	data := g.Map{
		"category": req.Category, "entries": string(ej), "source": sug.Source,
		"legal_entity_id": inv["legal_entity_id"].Int64(),
		"reason":          sug.Reason, "status": "draft", "updated_at": time.Now(),
	}
	var draftId int64
	if exist.IsEmpty() {
		data["invoice_id"] = req.InvoiceId
		data["created_at"] = time.Now()
		res, ierr := g.DB().Model("journal_draft").Data(data).Insert()
		if ierr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": ierr.Error()})
			return
		}
		draftId, _ = res.LastInsertId()
	} else {
		draftId = exist["id"].Int64()
		if _, uerr := g.DB().Model("journal_draft").Data(data).Where("id", draftId).Update(); uerr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": uerr.Error()})
			return
		}
	}
	audit(r, "journal.draft", "journal_draft", draftId, g.Map{"invoiceId": req.InvoiceId, "source": sug.Source})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"id": draftId, "entries": sug.Entries, "reason": sug.Reason, "source": sug.Source,
	}})
}

func JournalDraftList(r *ghttp.Request) {
	list, err := g.DB().Model("journal_draft").Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	rows := make([]g.Map, 0, len(list))
	for _, d := range list {
		inv, _ := g.DB().Model("invoice").Where("id", d["invoice_id"].Int64()).One()
		rows = append(rows, g.Map{
			"id": d["id"].Int64(), "invoiceId": d["invoice_id"].Int64(),
			"invoiceNo": inv["invoice_no"].String(), "sellerName": inv["seller_name"].String(),
			"total": inv["total"].Float64(), "category": d["category"].String(),
			"entries": d["entries"].String(), "source": d["source"].String(),
			"reason": d["reason"].String(), "status": d["status"].String(),
			"voucherId": d["voucher_id"].Int64(), "createdAt": d["created_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// JournalDraftUpdate 会计人工改分录（改完仍要过借贷平衡校验）
func JournalDraftUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Entries []agent.JournalEntry `json:"entries"`
	}
	if err := r.Parse(&req); err != nil || id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id 与 entries 必填"})
		return
	}
	if !agent.Balanced(req.Entries) {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "借贷不平衡或分录不合法（至少两行，每行只填一方）"})
		return
	}
	d, _ := g.DB().Model("journal_draft").Where("id", id).One()
	if d.IsEmpty() || d["status"].String() != "draft" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "草稿不存在或已确认，不能修改"})
		return
	}
	ej, _ := json.Marshal(req.Entries)
	if _, err := g.DB().Model("journal_draft").Data(g.Map{
		"entries": string(ej), "reason": d["reason"].String() + "（会计已人工调整）", "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "journal.edit", "journal_draft", id, g.Map{"rows": len(req.Entries)})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// JournalDraftConfirm 会计确认 → 写正式凭证 + 关联发票（证据链凭证层闭合）；未确认前不入库
func JournalDraftConfirm(r *ghttp.Request) {
	id := r.Get("id").Int64()
	d, err := g.DB().Model("journal_draft").Where("id", id).One()
	if err != nil || d.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "分录草稿不存在"})
		return
	}
	if d["status"].String() != "draft" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该草稿已确认，勿重复入账"})
		return
	}
	var entries []agent.JournalEntry
	if err := json.Unmarshal([]byte(d["entries"].String()), &entries); err != nil || !agent.Balanced(entries) {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "分录数据不合法或借贷不平衡，拒绝入账"})
		return
	}
	// 支持非发票来源草稿（如 v23 收入确认 invoice_id=0）：走同一人审确认链路，但不动发票关联。
	invID := d["invoice_id"].Int64()
	hasInv := invID > 0
	inv, _ := g.DB().Model("invoice").Where("id", invID).One()
	if hasInv && inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "关联发票不存在"})
		return
	}
	if hasInv && inv["voucher_id"].Int64() > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("发票已关联凭证 #%d，勿重复入账", inv["voucher_id"].Int64())})
		return
	}
	var debitSum float64
	for _, e := range entries {
		debitSum += e.Debit
	}
	now := time.Now()
	voucherNo := fmt.Sprintf("PZ-%s-%d", now.Format("20060102"), id)
	summary := d["reason"].String() // 非发票来源（收入确认等）用草稿事由做凭证摘要
	if hasInv {
		summary = fmt.Sprintf("%s（发票 %s）", inv["seller_name"].String(), inv["invoice_no"].String())
	}
	var legalEntityID int64
	if hasInv {
		legalEntityID = inv["legal_entity_id"].Int64()
	}
	res, err := g.DB().Model("voucher").Data(g.Map{
		"voucher_no": voucherNo, "summary": summary, "amount": debitSum,
		"voucher_date": now.Format("2006-01-02"), "created_at": now,
		"legal_entity_id": legalEntityID,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	voucherId, _ := res.LastInsertId()
	if hasInv {
		g.DB().Model("invoice").Data(g.Map{"voucher_id": voucherId, "updated_at": now}).Where("id", invID).Update()
	}
	g.DB().Model("journal_draft").Data(g.Map{
		"status": "confirmed", "voucher_id": voucherId,
		"confirmed_by": r.GetCtxVar("user").String(), "updated_at": now,
	}).Where("id", id).Update()
	audit(r, "journal.confirm", "voucher", voucherId, g.Map{"draftId": id, "invoiceId": d["invoice_id"].Int64(), "source": d["source"].String()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"voucherId": voucherId, "voucherNo": voucherNo, "amount": debitSum}})
}
