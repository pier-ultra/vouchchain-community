package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/agent"
	"vouchchain/pkg/expense"
)

// 费控三件套接口：
//   POST   /api/expense-splits          创建切片（硬校验合计；软校验预算/差标以 warnings 返回）
//   GET    /api/expense-splits?invoiceId= 切片列表
//   DELETE /api/expense-splits/{id}     删除切片
//   GET    /api/budgets/usage           预算列表+使用情况
//   POST   /api/budgets                 设置预算
//   DELETE /api/budgets/{id}            删除预算
//   POST   /api/travel-standards        设置差标
//   GET    /api/travel-standards        差标列表
//   DELETE /api/travel-standards/{id}   删除差标
//   POST   /api/travel-check            单发票超标检查

func ExpenseSplitCreate(r *ghttp.Request) {
	var in expense.CreateSplitInput
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	in.Creator = r.GetCtxVar("user").String()
	id, warnings, err := expense.CreateSplit(r.Context(), in)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "expense.split_create", "invoice", in.InvoiceID, g.Map{"amount": in.Amount, "type": in.ExpenseType, "bearer": in.Bearer, "warnings": warnings})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "warnings": warnings}})
}

func ExpenseSplitList(r *ghttp.Request) {
	invoiceID := r.Get("invoiceId").Int64()
	if invoiceID <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "invoiceId 必填"})
		return
	}
	rows, err := g.DB().Model("expense_split").Where("invoice_id", invoiceID).Order("slice_no asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	sum := 0.0
	for _, row := range rows {
		a := row["amount"].Float64()
		sum += a
		out = append(out, g.Map{
			"id": row["id"].Int64(), "invoiceId": row["invoice_id"].Int64(), "sliceNo": row["slice_no"].Int(),
			"amount": a, "expenseType": row["expense_type"].String(), "deptId": row["dept_id"].Int64(),
			"bearer": row["bearer"].String(), "remark": row["remark"].String(),
			"creator": row["creator"].String(), "createdAt": row["created_at"].String(),
		})
	}
	// 发票总额供前端做剩余可切
	tv, _ := g.DB().Model("invoice").Where("id", invoiceID).Value("total")
	if tv == nil {
		tv, _ = g.DB().Model("invoice").Where("id", invoiceID).Value("amount")
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"list": out, "sum": sum, "invoiceTotal": tv.Float64(),
	}})
}

func ExpenseSplitDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	g.DB().Model("expense_split").Where("id", id).Delete()
	audit(r, "expense.split_delete", "expense_split", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func BudgetUsageList(r *ghttp.Request) {
	list, err := expense.BudgetUsages(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func BudgetSave(r *ghttp.Request) {
	var req struct {
		ID          int64   `json:"id"`
		DeptID      int64   `json:"deptId"`
		Period      string  `json:"period" v:"required"`
		Category    string  `json:"category" v:"required"`
		AmountLimit float64 `json:"amountLimit" v:"required"`
		Remark      string  `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	data := g.Map{
		"dept_id": req.DeptID, "period": req.Period, "category": req.Category,
		"amount_limit": req.AmountLimit, "remark": req.Remark, "creator": r.GetCtxVar("user").String(),
	}
	var id int64
	if req.ID > 0 {
		_, err := g.DB().Model("budget").Data(data).Where("id", req.ID).Update()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id = req.ID
	} else {
		res, err := g.DB().Model("budget").Data(data).OnConflict("dept_id", "period", "category").Insert()
		if err != nil {
			// 并发/重复：提示幂等
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该部门+月份+分类已有预算，请编辑而非新增"})
			return
		}
		id, _ = res.LastInsertId()
	}
	audit(r, "expense.budget_save", "budget", id, g.Map{"dept": req.DeptID, "period": req.Period, "category": req.Category, "limit": req.AmountLimit})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func BudgetDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	g.DB().Model("budget").Where("id", id).Delete()
	audit(r, "expense.budget_delete", "budget", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func TravelStandardList(r *ghttp.Request) {
	rows, err := g.DB().Model("travel_standard").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, g.Map{
			"id": row["id"].Int64(), "category": row["category"].String(),
			"limitAmount": row["limit_amount"].Float64(), "note": row["note"].String(),
			"enabled": row["enabled"].Bool(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func TravelStandardSave(r *ghttp.Request) {
	var req struct {
		ID          int64   `json:"id"`
		Category    string  `json:"category" v:"required"`
		LimitAmount float64 `json:"limitAmount" v:"required"`
		Note        string  `json:"note"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	data := g.Map{
		"category": req.Category, "limit_amount": req.LimitAmount,
		"note": req.Note, "enabled": enabled, "creator": r.GetCtxVar("user").String(),
	}
	var id int64
	if req.ID > 0 {
		if _, err := g.DB().Model("travel_standard").Data(data).Where("id", req.ID).Update(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id = req.ID
	} else {
		res, err := g.DB().Model("travel_standard").Data(data).Insert()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		id, _ = res.LastInsertId()
	}
	audit(r, "expense.standard_save", "travel_standard", id, g.Map{"category": req.Category, "limit": req.LimitAmount})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func TravelStandardDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	g.DB().Model("travel_standard").Where("id", id).Delete()
	audit(r, "expense.standard_delete", "travel_standard", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// TravelCheck 单发票差标检查（差旅类票）
func TravelCheck(r *ghttp.Request) {
	var req struct {
		InvoiceID int64 `json:"invoiceId"`
	}
	if err := r.Parse(&req); err != nil || req.InvoiceID <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "invoiceId 必填"})
		return
	}
	warnings, err := expense.CheckInvoice(r.Context(), req.InvoiceID)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "expense.travel_check", "invoice", req.InvoiceID, g.Map{"warnings": warnings})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"warnings": warnings, "checkedAt": time.Now().Format("2006-01-02 15:04:05")}})
}

var _ = fmt.Sprintf

// ---------- 切片凭证联动（费控闭环最后一环）----------

// SplitVoucher POST /api/expense-splits/{id}/voucher：按切片生成 AI 分录建议（金额按价税比例拆不含税/税额）
func SplitVoucher(r *ghttp.Request) {
	id := r.Get("id").Int64()
	splitRow, err := g.DB().Model("expense_split").Where("id", id).One()
	if err != nil || splitRow.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "切片不存在"})
		return
	}
	inv, err := g.DB().Model("invoice").Where("id", splitRow["invoice_id"].Int64()).One()
	if err != nil || inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "发票不存在"})
		return
	}
	invTotal := inv["total"].Float64()
	if invTotal <= 0 {
		invTotal = inv["amount"].Float64() + inv["tax"].Float64()
	}
	sliceAmount := splitRow["amount"].Float64()
	// 价税比例拆分（保留两位，尾差进税额避免合计不平）
	ratio := sliceAmount / invTotal
	sAmount := round2(inv["amount"].Float64() * ratio)
	sTax := round2(sliceAmount - sAmount)

	category := splitRow["expense_type"].String()
	if category == "" {
		category = "其他"
	}
	subjects, err := loadSubjects()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	bearer := splitRow["bearer"].String()
	bearerNote := "公司承担"
	if bearer == "personal" {
		bearerNote = "个人垫付（贷方挂个人往来）"
	}
	facts := agent.JournalFacts{
		InvoiceNo:   fmt.Sprintf("%s-切片#%d", inv["invoice_no"].String(), splitRow["slice_no"].Int()),
		InvoiceType: inv["invoice_type"].String(),
		SellerName:  inv["seller_name"].String(),
		BuyerName:   inv["buyer_name"].String(),
		Category:    category,
		Items:       fmt.Sprintf(`[{"ItemName":"报销切片：%s（%s，%s）","Amount":"%.2f"}]`, splitRow["remark"].String(), category, bearerNote, sAmount),
		Amount:      sAmount, Tax: sTax, Total: sliceAmount,
	}
	sug, err := agent.SuggestJournal(r.Context(), facts, subjects, true)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	ej, _ := json.Marshal(sug.Entries)
	// journal_draft 带 split_id（同片重生成时覆盖旧草稿）
	exist, _ := g.DB().Model("journal_draft").Where("split_id", id).Where("status", "draft").Order("id desc").Limit(1).One()
	data := g.Map{
		"invoice_id": inv["id"].Int64(), "split_id": id, "category": category,
		"entries": string(ej), "source": sug.Source, "reason": sug.Reason,
		"legal_entity_id": inv["legal_entity_id"].Int64(),
		"status":          "draft", "updated_at": time.Now(),
	}
	var draftId int64
	if exist.IsEmpty() {
		data["created_at"] = time.Now()
		res, ierr := g.DB().Model("journal_draft").Data(data).Insert()
		if ierr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": ierr.Error()})
			return
		}
		draftId, _ = res.LastInsertId()
	} else {
		draftId = exist["id"].Int64()
		g.DB().Model("journal_draft").Data(data).Where("id", draftId).Update()
	}
	audit(r, "expense.split_voucher", "journal_draft", draftId, g.Map{"splitId": id, "amount": sliceAmount, "source": sug.Source})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"id": draftId, "entries": sug.Entries, "reason": sug.Reason, "source": sug.Source,
		"amount": sliceAmount, "amountExTax": sAmount, "tax": sTax,
	}})
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
