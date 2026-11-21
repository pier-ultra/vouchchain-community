package controller

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// TodoList 待办中心聚合入口：按业务域聚合待处理事项，供首页展示
func TodoList(r *ghttp.Request) {
	username := r.GetCtxVar("user").String()
	if username == "" {
		username = r.Get("username").String()
	}
	if username == "" {
		username = "admin"
	}

	list := make([]g.Map, 0)

	// ---------- 待验真发票 ----------
	invRows, _ := g.DB().Model("invoice").
		Where("verify_status <> ? OR verify_status = ?", "verified", "").All()
	pendingVerify := 0
	for _, inv := range invRows {
		if inv["verify_status"].String() != "verified" {
			pendingVerify++
		}
	}
	if pendingVerify > 0 {
		list = append(list, g.Map{
			"id":     "invoice-verify",
			"title":  "待验真发票",
			"desc":   "有 " + itoa(pendingVerify) + " 张发票尚未验真，请尽快完成",
			"type":   "invoice",
			"status": "pending",
			"count":  pendingVerify,
			"url":    "/invoice-ledger",
			"icon":   "📄",
		})
	}

	// ---------- 未匹配发票（匹配异常） ----------
	ledgerRows, _ := g.DB().Model("invoice").Fields("id, match_status").All()
	mismatch := 0
	for _, lr := range ledgerRows {
		if lr["match_status"].String() == "mismatch" || lr["match_status"].String() == "unverified" {
			mismatch++
		}
	}
	if mismatch > 0 {
		list = append(list, g.Map{
			"id":     "invoice-match",
			"title":  "待匹配发票",
			"desc":   "有 " + itoa(mismatch) + " 张发票匹配状态异常，需核对合同关联",
			"type":   "invoice",
			"status": "warning",
			"count":  mismatch,
			"url":    "/invoice-ledger",
			"icon":   "📄",
		})
	}

	// ---------- 待签署合同 ----------
	contractRows, _ := g.DB().Model("contract").Where("status", "draft").All()
	draftContracts := len(contractRows)
	if draftContracts > 0 {
		list = append(list, g.Map{
			"id":     "contract-draft",
			"title":  "待签署合同",
			"desc":   "有 " + itoa(draftContracts) + " 份合同处于起草/待签署状态",
			"type":   "contract",
			"status": "pending",
			"count":  draftContracts,
			"url":    "/contracts",
			"icon":   "📑",
		})
	}

	// ---------- 待会计确认的分录草稿 ----------
	draftRows, _ := g.DB().Model("journal_draft").Where("status", "draft").All()
	draftJournals := len(draftRows)
	if draftJournals > 0 {
		list = append(list, g.Map{
			"id":     "journal-draft",
			"title":  "待会计确认分录",
			"desc":   "有 " + itoa(draftJournals) + " 笔 AI 分录草稿需会计复核入账",
			"type":   "journal",
			"status": "pending",
			"count":  draftJournals,
			"url":    "/accounting",
			"icon":   "📒",
		})
	}

	// ---------- 待付款计划 ----------
	payableRows, _ := g.DB().Model("payment_plan").Where("status", "planned").All()
	pendingPay := len(payableRows)
	if pendingPay > 0 {
		list = append(list, g.Map{
			"id":     "payable-pay",
			"title":  "待付款计划",
			"desc":   "有 " + itoa(pendingPay) + " 条付款计划需登记打款",
			"type":   "payable",
			"status": "urgent",
			"count":  pendingPay,
			"url":    "/accounting",
			"icon":   "💰",
		})
	}

	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"list": list, "total": len(list)}})
}

// itoa 转换 int 为 string（避免 fmt 导入开销）
func itoa(n int) string {
	var buf [12]byte
	i := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
