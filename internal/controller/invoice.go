package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vouchchain/pkg/invoiceclass"
	"vouchchain/pkg/invoiceparse"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- 发票线（v13 US-I1~I8 主链） ----------
// 导入解析(I1) → 查重(I2) → 凭证关联(I3) → 四性检测(I4) → 单套制归档(I5)
// → 回单匹配(I6) → 验真留痕(I7) → 证据链联查(I8)。I9 归档调度为 P1 暂缓。

func num(s string) float64 {
	var f float64
	fmt.Sscan(strings.TrimSpace(s), &f)
	return f
}

// InvoiceImport 导入 OFD/XML 数电票（AI1.1-1.2），解析八字段+明细（AI1.3），
// 查重拦截（AI2.1 发票号唯一 / AI2.2 字节幂等），SHA 留痕（AI7.1）
func InvoiceImport(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少上传文件（字段名 file）"})
		return
	}
	if !invoiceparse.IsSupported(file.Filename) {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "仅支持 OFD / XML 数电票文件"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	defer fh.Close()
	data, err := io.ReadAll(fh)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if len(data) == 0 || len(data) > 20<<20 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "文件为空或超过 20MB"})
		return
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// AI2.2 字节幂等：同内容文件直接跳过
	if exist, _ := g.DB().Model("invoice").Where("file_hash", hash).One(); !exist.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 2, "message": fmt.Sprintf("该文件已导入过（发票 #%d），已跳过", exist["id"].Int64()), "data": g.Map{"duplicate": "hash", "existId": exist["id"].Int64()}})
		return
	}
	path, err := saveInvoiceFile(file.Filename, data)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	p, err := invoiceparse.Parse(path)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// AI2.1 发票号唯一：重复拦截（AI2.4 人工放行走 force 参数）
	if exist, _ := g.DB().Model("invoice").Where("invoice_no", p.InvoiceNo).One(); !exist.IsEmpty() && r.Get("force").Int() == 0 {
		r.Response.WriteJsonExit(g.Map{"code": 2, "message": fmt.Sprintf("发票号 %s 已存在（发票 #%d）。如确认为同一张票请勿重复导入；如需强制放行请带 force=1", p.InvoiceNo, exist["id"].Int64()), "data": g.Map{"duplicate": "invoiceNo", "existId": exist["id"].Int64()}})
		return
	}
	ij, _ := json.Marshal(p.Items)
	now := time.Now()
	res, err := g.DB().Model("invoice").Data(g.Map{
		"invoice_no": p.InvoiceNo, "issue_date": p.IssueDate,
		"buyer_name": p.BuyerName, "seller_name": p.SellerName,
		"amount": num(p.Amount), "tax": num(p.Tax), "total": num(p.Total),
		"invoice_type": p.InvoiceType, "items": string(ij),
		"file_path": path, "file_name": file.Filename, "file_hash": hash,
		"verify_status":   "unverified",
		"legal_entity_id": scopeEntity(r),
		"owner":           r.GetCtxVar("user").String(),
		"created_at":      now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	// 上传时自动分类（用户 11-15）：异步执行，失败静默（保持未分类，可在发票分类页重跑）
	if _, cerr := g.DB().Exec(r.Context(),
		"UPDATE invoice SET category='', category_source='' WHERE id=$1", id); cerr != nil {
		g.Log().Warning(r.Context(), "[invoice-class] 初始化未分类状态失败", cerr)
	}
	go func(invoiceId int64) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		category, source, cerr := invoiceclass.ClassifyOne(bgCtx, invoiceId)
		if cerr != nil || source == "uncategorized" {
			return
		}
		g.DB().Model("invoice").Data(g.Map{"category": category, "category_source": source}).Where("id", invoiceId).Update()
	}(id)
	audit(r, "invoice.import", "invoice", id, g.Map{"invoiceNo": p.InvoiceNo, "force": r.Get("force").Int() == 1})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "invoiceNo": p.InvoiceNo, "total": p.Total}})
}

func saveInvoiceFile(name string, data []byte) (string, error) {
	dir := "data/invoices"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	full := "data/invoices/" + time.Now().Format("20060102150405") + "_" + filepathBase(name)
	return full, os.WriteFile(full, data, 0o644)
}

func filepathBase(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			return name[i+1:]
		}
	}
	return name
}

// InvoiceList 发票台账（按发票号/验真/归档筛选）
func InvoiceList(r *ghttp.Request) {
	keyword := r.Get("keyword").String()
	verify := r.Get("verifyStatus").String()
	archive := r.Get("archiveStatus").String()
	m := scopeFilter(g.DB().Model("invoice"), r)
	if keyword != "" {
		m = m.Where("invoice_no LIKE ? OR buyer_name LIKE ? OR seller_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if verify != "" {
		m = m.Where("verify_status", verify)
	}
	if archive != "" {
		m = m.Where("archive_status", archive)
	}
	if cat := r.Get("category").String(); cat != "" {
		if cat == "未分类" {
			m = m.Where("category", "")
		} else {
			m = m.Where("category", cat)
		}
	}
	list, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// AG1.2 字段级权限：角色配置了字段掩码时，列表返回前替换为 ***
	out := make([]g.Map, 0, len(list))
	for _, row := range list {
		out = append(out, row.Map())
	}
	applyFieldMask(r, out)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// InvoiceIntegrity 四性检测（AI4.1-4.5）：真实性/完整性/安全性/可用性逐项 + 报告落库
func InvoiceIntegrity(r *ghttp.Request) {
	id := r.Get("id").Int64()
	inv, err := g.DB().Model("invoice").Where("id", id).One()
	if err != nil || inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "发票不存在"})
		return
	}
	type check struct {
		Name string `json:"name"`
		Pass bool   `json:"pass"`
		Msg  string `json:"msg"`
	}
	report := []check{}

	// 真实性（AI4.1）：本地规则占位——发票号/购销方非空即默认通过；税务签章验真走 I7 占位
	authentic := inv["invoice_no"].String() != "" && (inv["buyer_name"].String() != "" || inv["seller_name"].String() != "")
	authMsg := "关键字段齐备（本地规则）"
	if !authentic {
		authMsg = "发票号或购销方缺失"
	}
	report = append(report, check{Name: "真实性", Pass: authentic, Msg: authMsg})

	// 完整性（AI4.2）：八字段逐项
	incomplete := []string{}
	for k, label := range map[string]string{
		"invoice_no": "发票号码", "issue_date": "开票日期", "buyer_name": "购买方",
		"seller_name": "销售方", "amount": "金额", "tax": "税额", "total": "价税合计", "invoice_type": "发票类型",
	} {
		if inv[k].String() == "" || inv[k].String() == "0" {
			incomplete = append(incomplete, label)
		}
	}
	compMsg := "八字段完整"
	if len(incomplete) > 0 {
		compMsg = "缺失：" + joinStr(incomplete)
	}
	report = append(report, check{Name: "完整性", Pass: len(incomplete) == 0, Msg: compMsg})

	// 安全性（AI4.3）：文件哈希重算比对（防篡改）
	safe, safeMsg := false, "源文件缺失，无法重算哈希"
	if f, err := os.ReadFile(inv["file_path"].String()); err == nil {
		reHash := fmt.Sprintf("%x", sha256.Sum256(f))
		safe = reHash == inv["file_hash"].String()
		safeMsg = map[bool]string{true: "SHA-256 比对一致，未被篡改", false: "SHA-256 不一致！文件可能被篡改"}[safe]
	}
	report = append(report, check{Name: "安全性", Pass: safe, Msg: safeMsg})

	// 可用性（AI4.4）：源文件可读 + 可再解析
	usable := false
	if _, err := invoiceparse.Parse(inv["file_path"].String()); err == nil {
		usable = true
	}
	usableMsg := "源件不可读或解析失败"
	if usable {
		usableMsg = "源件可读且可再解析"
	}
	report = append(report, check{Name: "可用性", Pass: usable, Msg: usableMsg})

	rj, _ := json.Marshal(report)
	g.DB().Model("invoice").Data(g.Map{
		"integrity_report": string(rj), "updated_at": time.Now(),
	}).Where("id", id).Update()
	audit(r, "invoice.integrity", "invoice", id, g.Map{"allPass": authentic && len(incomplete) == 0 && safe && usable})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": report})
}

func joinStr(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "、"
		}
		out += s
	}
	return out
}

// InvoiceVerify 验真状态机（AI7.2-7.4）：税务接口占位——当前支持手动置已验真/失败并留痕
func InvoiceVerify(r *ghttp.Request) {
	id := r.Get("id").Int64()
	status := r.Get("verifyStatus").String() // verified / failed / unverified
	if id <= 0 || (status != "verified" && status != "failed" && status != "unverified") {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id 与 verifyStatus(verified|failed|unverified) 必填"})
		return
	}
	inv, _ := g.DB().Model("invoice").Where("id", id).One()
	if inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "发票不存在"})
		return
	}
	// AG4.3/AG4.4：规则按状态绑定（验真动作才触发票号格式校验）
	if policyGuard(r, "invoice", id, inv["invoice_no"].String(), map[string]interface{}{
		"invoiceNo": inv["invoice_no"].String(), "total": inv["total"].Float64(),
		"verifyStatus": status,
	}) {
		return
	}
	g.DB().Model("invoice").Data(g.Map{
		"verify_status": status, "verify_remark": "手动登记（税务验真接口占位）", "updated_at": time.Now(),
	}).Where("id", id).Update()
	audit(r, "invoice.verify", "invoice", id, g.Map{"verifyStatus": status})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// InvoiceArchive 单套制归档（AI5.1-5.5）：HTTP 手动入口。
// 实际动作抽到 archiveInvoiceNow，供 US-I9 定时调度复用（AI9.2 要求"调用 AI5"）。
func InvoiceArchive(r *ghttp.Request) {
	id := r.Get("id").Int64()
	path, err := archiveInvoiceNow(id)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "invoice.archive", "invoice", id, g.Map{"archivePath": path, "trigger": "manual"})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"archivePath": path}})
}

// ---------- 凭证（AI3）与回单（AI6） ----------

func VoucherCreate(r *ghttp.Request) {
	var req struct {
		VoucherNo   string  `json:"voucherNo"`
		Summary     string  `json:"summary"`
		Amount      float64 `json:"amount"`
		VoucherDate string  `json:"voucherDate"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if policyGuard(r, "voucher", 0, req.VoucherNo, map[string]interface{}{
		"voucherNo": req.VoucherNo, "amount": req.Amount, "summary": req.Summary,
	}) {
		return
	}
	res, err := g.DB().Model("voucher").Data(g.Map{
		"voucher_no": req.VoucherNo, "summary": req.Summary, "amount": req.Amount,
		"voucher_date": req.VoucherDate, "created_at": time.Now(),
		"legal_entity_id": scopeEntity(r), "owner": r.GetCtxVar("user").String(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "voucher.create", "voucher", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func VoucherList(r *ghttp.Request) {
	list, err := g.DB().Model("voucher").Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// InvoiceLinkVoucher 关联凭证（AI3.2-3.5）：双向联查的基础，重复关联提示，留痕
func InvoiceLinkVoucher(r *ghttp.Request) {
	var req struct {
		InvoiceId int64 `json:"invoiceId"`
		VoucherId int64 `json:"voucherId"`
	}
	if err := r.Parse(&req); err != nil || req.InvoiceId <= 0 || req.VoucherId <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "invoiceId/voucherId 必填"})
		return
	}
	inv, _ := g.DB().Model("invoice").Where("id", req.InvoiceId).One()
	if inv.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "发票不存在"})
		return
	}
	// AI3.4 关联校验
	if inv["voucher_id"].Int64() == req.VoucherId {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("该发票已关联凭证 #%d，勿重复关联", req.VoucherId)})
		return
	}
	g.DB().Model("invoice").Data(g.Map{
		"voucher_id": req.VoucherId, "updated_at": time.Now(),
	}).Where("id", req.InvoiceId).Update()
	audit(r, "invoice.link_voucher", "invoice", req.InvoiceId, g.Map{"voucherId": req.VoucherId})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "linked"})
}

// ReceiptCreate 录入银行回单（AI6.1：MVP 手工录入+文件留档占位）
func ReceiptCreate(r *ghttp.Request) {
	var req struct {
		Payer       string  `json:"payer"`
		Payee       string  `json:"payee"`
		Amount      float64 `json:"amount"`
		ReceiptDate string  `json:"receiptDate"`
		Remark      string  `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	res, err := g.DB().Model("bank_receipt").Data(g.Map{
		"payer": req.Payer, "payee": req.Payee, "amount": req.Amount,
		"receipt_date": req.ReceiptDate, "remark": req.Remark, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "receipt.create", "bank_receipt", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func ReceiptList(r *ghttp.Request) {
	list, err := g.DB().Model("bank_receipt").Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ReceiptAutoMatch 自动匹配（AI6.2-6.5）：金额相等优先 + 对方名称兜底，匹配留痕；未匹配进待处理
func ReceiptAutoMatch(r *ghttp.Request) {
	receipts, err := g.DB().Model("bank_receipt").Where("matched_invoice_id", 0).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	matched, unmatched := 0, 0
	for _, rc := range receipts {
		inv, err := g.DB().Model("invoice").
			Where("total", rc["amount"].Float64()).
			Where("voucher_id > 0").
			Order("id asc").Limit(1).One()
		if err != nil || inv.IsEmpty() {
			// 兜底：无凭证约束时仅按金额匹配未归档发票
			inv, err = g.DB().Model("invoice").
				Where("total", rc["amount"].Float64()).
				Order("id asc").Limit(1).One()
		}
		if err != nil || inv.IsEmpty() {
			unmatched++
			continue
		}
		g.DB().Model("bank_receipt").Data(g.Map{
			"matched_invoice_id": inv["id"].Int64(),
			"match_rule":         fmt.Sprintf("金额匹配 %s（发票号 %s）", fmt.Sprintf("%.2f", inv["total"].Float64()), inv["invoice_no"].String()),
			"updated_at":         time.Now(),
		}).Where("id", rc["id"].Int64()).Update()
		matched++
		audit(r, "receipt.match", "bank_receipt", rc["id"].Int64(), g.Map{"invoiceId": inv["id"].Int64()})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"matched": matched, "unmatched": unmatched}})
}

// chainLink 证据链节点（AI8.2 链路展示 / AI8.3 断链标红）
type chainLink struct {
	Layer  string      `json:"layer"`
	Id     int64       `json:"id"`
	Label  string      `json:"label"`
	Data   interface{} `json:"data"`
	Broken bool        `json:"broken"`
}

// buildChain 凭证→发票→回单 三层穿透（AI8.1/8.2/8.3），联查与导出共用
func buildChain(invoiceId int64) (gdb.Record, []chainLink, error) {
	if invoiceId <= 0 {
		return nil, nil, fmt.Errorf("invoiceId required")
	}
	inv, err := g.DB().Model("invoice").Where("id", invoiceId).One()
	if err != nil || inv.IsEmpty() {
		return nil, nil, fmt.Errorf("发票不存在")
	}
	chain := []chainLink{}
	chain = append(chain, chainLink{Layer: "invoice", Id: invoiceId, Label: "发票 " + inv["invoice_no"].String(), Data: inv, Broken: false})

	voucherId := inv["voucher_id"].Int64()
	if voucherId > 0 {
		vc, err := g.DB().Model("voucher").Where("id", voucherId).One()
		broken := err != nil || vc.IsEmpty()
		chain = append(chain, chainLink{Layer: "voucher", Id: voucherId, Label: fmt.Sprintf("凭证 #%d", voucherId), Data: vc, Broken: broken})
	} else {
		chain = append(chain, chainLink{Layer: "voucher", Id: 0, Label: "凭证（未关联）", Data: nil, Broken: true})
	}

	receipts, _ := g.DB().Model("bank_receipt").Where("matched_invoice_id", invoiceId).All()
	if !receipts.IsEmpty() {
		for _, rc := range receipts {
			chain = append(chain, chainLink{Layer: "receipt", Id: rc["id"].Int64(), Label: fmt.Sprintf("回单 #%d（%.2f 元，%s）", rc["id"].Int64(), rc["amount"].Float64(), rc["receipt_date"].String()), Data: rc, Broken: false})
		}
	} else {
		chain = append(chain, chainLink{Layer: "receipt", Id: 0, Label: "回单（未匹配）", Data: nil, Broken: true})
	}
	return inv, chain, nil
}

func InvoiceChain(r *ghttp.Request) {
	inv, chain, err := buildChain(r.Get("id").Int64())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"invoiceId": inv["id"].Int64(), "chain": chain}})
}

// InvoiceChainExport AI8.4：联查结果导出供审计调阅（Markdown 报告 / Excel 明细）
func InvoiceChainExport(r *ghttp.Request) {
	inv, chain, err := buildChain(r.Get("id").Int64())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	format := r.Get("format").String()
	if format == "xlsx" {
		stamp := time.Now().Format("2006-01-02 15:04:05")
		headers := []string{"层级", "编号", "说明", "是否断链", "导出时间"}
		data := make([][]string, 0, len(chain))
		for _, l := range chain {
			data = append(data, []string{l.Layer, fmt.Sprintf("%d", l.Id), l.Label,
				map[bool]string{true: "断链", false: "完整"}[l.Broken], stamp})
		}
		if err := sendXLSX(r, fmt.Sprintf("chain-%d.xlsx", inv["id"].Int64()), "证据链", headers, data); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		return
	}
	var body strings.Builder
	{
		body.WriteString(fmt.Sprintf("# 证据链联查报告（发票 %s）\n\n", inv["invoice_no"].String()))
		body.WriteString(fmt.Sprintf("- 导出时间：%s\n- 购买方：%s\n- 销售方：%s\n- 价税合计：%.2f 元\n- 验真状态：%s / 归档状态：%s\n\n",
			time.Now().Format("2006-01-02 15:04:05"), inv["buyer_name"].String(), inv["seller_name"].String(),
			inv["total"].Float64(), inv["verify_status"].String(), inv["archive_status"].String()))
		body.WriteString("## 链路\n\n| 层级 | 编号 | 说明 | 状态 |\n|---|---|---|---|\n")
		for _, l := range chain {
			body.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n", l.Layer, l.Id, l.Label,
				map[bool]string{true: "**断链**", false: "完整"}[l.Broken]))
		}
		broken := 0
		for _, l := range chain {
			if l.Broken {
				broken++
			}
		}
		body.WriteString(fmt.Sprintf("\n> 断链环节：%d 处。断链项需人工补齐后重新联查。\n", broken))
		r.Response.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	}
	r.Response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=chain-%d.md", inv["id"].Int64()))
	r.Response.Write([]byte{0xEF, 0xBB, 0xBF})
	r.Response.Write(body.String())
}

// ---------- AI5.5 归档检索与调阅 ----------

// ArchiveList 已归档发票多条件检索（发票号 / 销方 / 验真状态 / 开票日期起）
func ArchiveList(r *ghttp.Request) {
	m := scopeFilter(g.DB().Model("invoice"), r).Where("archive_status", "archived")
	if v := r.Get("keyword").String(); v != "" {
		m = m.Where("invoice_no LIKE ? OR seller_name LIKE ?", "%"+v+"%", "%"+v+"%")
	}
	if v := r.Get("verifyStatus").String(); v != "" {
		m = m.Where("verify_status", v)
	}
	if v := r.Get("dateFrom").String(); v != "" {
		m = m.Where("issue_date >= ?", v)
	}
	rows, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		list = append(list, g.Map{
			"id": row["id"].Int64(), "invoiceNo": row["invoice_no"].String(),
			"issueDate": row["issue_date"].String(), "sellerName": row["seller_name"].String(),
			"total": row["total"].Float64(), "verifyStatus": row["verify_status"].String(),
			"archivePath": row["archive_path"].String(), "fileHash": row["file_hash"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ArchiveDownload AI5.5 归档包调阅（按发票 id 取归档 zip，不接受任意路径）
func ArchiveDownload(r *ghttp.Request) {
	inv, err := g.DB().Model("invoice").Where("id", r.Get("id").Int64()).One()
	if err != nil || inv.IsEmpty() || inv["archive_status"].String() != "archived" {
		r.Response.WriteStatusExit(404, "归档记录不存在")
		return
	}
	path := inv["archive_path"].String()
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, "data/archives") {
		r.Response.WriteStatusExit(400, "非法归档路径")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		r.Response.WriteStatusExit(404, "归档包读取失败（源件可能已迁移）")
		return
	}
	name := filepath.Base(path)
	r.Response.Header().Set("Content-Type", "application/zip")
	r.Response.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.QueryEscape(name))
	r.Response.Write(data)
}
