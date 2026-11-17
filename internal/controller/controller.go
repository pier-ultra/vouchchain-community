package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"vouchchain/internal/entity"
	"vouchchain/pkg/agent"
	"vouchchain/pkg/cryptoutil"
	"vouchchain/pkg/fileparse"
	"vouchchain/pkg/ws"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func Health(r *ghttp.Request) {
	r.Response.WriteJson(g.Map{"status": "ok"})
}

// ---------- model_config（Core 共用内核：AI/ML 接入配置） ----------

func ModelConfigList(r *ghttp.Request) {
	list := make([]entity.ModelConfig, 0)
	if err := g.DB().Model("model_config").Order("id asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// 密钥脱敏：密文/明文一律不直接下发；同时告知"当前进程能否解开"，
	// 避免重启漏带 APP_AES_KEY 时页面显示"已加密存储"、AI 却静默失效。
	for i := range list {
		_, derr := cryptoutil.Decrypt(list[i].ApiKey)
		list[i].KeyUsable = list[i].ApiKey != "" && derr == nil
		list[i].ApiKey = cryptoutil.Mask(list[i].ApiKey)
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ModelConfigCreate(r *ghttp.Request) {
	var req entity.ModelConfig
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// API Key 加密入库（AES-256-GCM），读时解密；密文存 model_config.api_key
	apiKey := req.ApiKey
	if apiKey != "" {
		enc, err := cryptoutil.Encrypt(apiKey)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		apiKey = enc
	}
	now := time.Now()
	data := g.Map{
		"name":         req.Name,
		"category":     req.Category,
		"provider":     req.Provider,
		"base_url":     req.BaseUrl,
		"model_name":   req.ModelName,
		"api_key":      apiKey,
		"extra_config": req.ExtraConfig,
		"enabled":      req.Enabled,
		"created_at":   now,
		"updated_at":   now,
	}
	if _, err := g.DB().Model("model_config").Data(data).Insert(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "created"})
}

// ---------- contract（产品① 合同线核心实体） ----------
// 对应 v13 P0：AC6.1 合同台账（筛选/搜索/分页）；新增/编辑/删除为台账基础操作

// ContractDetail 合同详情（按 id）；对象级隔离守卫在路由层拦截越权访问。
func ContractDetail(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	row, err := g.DB().Model("contract").Fields("*").Where("id", id).One()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "合同不存在"})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": row.Map()})
}

func ContractList(r *ghttp.Request) {
	status := r.Get("status").String()
	keyword := r.Get("keyword").String()
	page := r.Get("page").Int()
	if page < 1 {
		page = 1
	}
	pageSize := r.Get("pageSize").Int()
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	m := scopeFilter(g.DB().Model("contract"), r)
	if status != "" {
		m = m.Where("status", status)
	}
	if keyword != "" {
		m = m.Where("title LIKE ? OR counter_party LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	total, err := m.Count()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]entity.Contract, 0)
	if err := m.Order("id desc").Page(page, pageSize).Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"list":     list,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}})
}

func ContractCreate(r *ghttp.Request) {
	var req entity.Contract
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// AG4.4 实时合规校验：拦截级规则直接返回 409，告警级留痕后放行
	if policyGuard(r, "contract", 0, req.Title, map[string]interface{}{
		"title": req.Title, "counterParty": req.CounterParty, "amount": req.Amount,
		"currency": req.Currency, "status": req.Status,
	}) {
		return
	}
	now := time.Now()
	data := g.Map{
		"title":           req.Title,
		"counter_party":   req.CounterParty,
		"amount":          req.Amount,
		"currency":        req.Currency,
		"sign_date":       req.SignDate,
		"status":          req.Status,
		"legal_entity_id": scopeEntity(r),
		"owner":           r.GetCtxVar("user").String(),
		"created_at":      now,
		"updated_at":      now,
	}
	if _, err := g.DB().Model("contract").Data(data).Insert(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "created"})
}

func ContractUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	var req entity.Contract
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	data := g.Map{
		"title":         req.Title,
		"counter_party": req.CounterParty,
		"amount":        req.Amount,
		"currency":      req.Currency,
		"sign_date":     req.SignDate,
		"status":        req.Status,
		"updated_at":    time.Now(),
	}
	if _, err := g.DB().Model("contract").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "updated"})
}

func ContractDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	if _, err := g.DB().Model("contract").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "deleted"})
}

// ---------- contract file 上传与 AI 抽取流水线（v13 AC1.1-1.6 + 任务中心） ----------
// 流水线（ETL）：parse 文本解析 → extract AI条款抽取 → vectorize 向量化（知识库阶段接入，占位）
// 状态机：pending（待处理）→ extracting（流水线执行中）→ done / failed
// 进度经 WebSocket（pkg/ws）实时广播，任务中心免刷新可见

const uploadDir = "data/uploads"

// pipeline 总进度权重：parse 30% + extract 70%；vectorize 接入后改为 30/50/20
func defaultPipeline() []entity.PipelineStage {
	return []entity.PipelineStage{
		{Key: "parse", Label: "文本解析", Status: "planned", Progress: 0},
		{Key: "extract", Label: "AI 条款抽取", Status: "planned", Progress: 0},
		{Key: "vectorize", Label: "向量化入库", Status: "planned", Progress: 0, Msg: "知识库阶段接入"},
	}
}

type taskNotifier struct {
	fileId int64
}

// save 持久化流水线快照并广播 WS（节流由调用方控制）
func (n taskNotifier) save(status string, stages []entity.PipelineStage, overall int) {
	pj, _ := json.Marshal(stages)
	curStage := ""
	for _, s := range stages {
		if s.Status == "running" {
			curStage = s.Key
		}
	}
	if curStage == "" {
		curStage = "parse"
	}
	errMsg := ""
	if status == "failed" {
		for _, s := range stages {
			if s.Status == "failed" {
				errMsg = s.Msg
			}
		}
	}
	g.DB().Model("contract_file").Data(g.Map{
		"status": status, "stage": curStage, "progress": overall,
		"pipeline": pj, "error_msg": errMsg, "updated_at": time.Now(),
	}).Where("id", n.fileId).Update()
	ws.Broadcast(ws.Event{Type: "task", Data: g.Map{
		"id": n.fileId, "status": status, "stage": curStage,
		"progress": overall, "pipeline": stages, "errorMsg": errMsg,
	}})
}

func (n taskNotifier) setStage(stages []entity.PipelineStage, key, status string, progress int, msg string) {
	for i := range stages {
		if stages[i].Key == key {
			stages[i].Status = status
			stages[i].Progress = progress
			if msg != "" {
				stages[i].Msg = msg
			}
		}
	}
}

// overallProgress 按 parse 30 / extract 70 权重合成总进度（vectorize 未启用不占权重）
func overallProgress(stages []entity.PipelineStage) int {
	m := map[string]int{}
	for _, s := range stages {
		m[s.Key] = s.Progress
	}
	return m["parse"]*3/10 + m["extract"]*7/10
}

func ContractFileUpload(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少上传文件（字段名 file）"})
		return
	}
	// AC1.8 抽取模板（非必选）：general/purchase/sales/lease
	template := r.Get("template").String()
	switch template {
	case "purchase", "sales", "lease":
	case "", "general":
		template = "general"
	default:
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "template 取值非法"})
		return
	}
	// AC6.2 版本历史：versionOf 指向组内任一文件，新上传作为其下一版本
	versionOf := r.Get("versionOf").Int64()
	groupId, versionNo := int64(0), 1
	if versionOf > 0 {
		base, err := g.DB().Model("contract_file").Where("id", versionOf).One()
		if err != nil || base.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "versionOf 指向的文件不存在"})
			return
		}
		groupId = base["group_id"].Int64()
		if groupId == 0 {
			groupId = base["id"].Int64()
		}
		cnt, err := g.DB().Model("contract_file").
			Where("COALESCE(NULLIF(group_id,0), id) = ?", groupId).Count()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		versionNo = cnt + 1
	}
	// AC1.2 格式校验：非 PDF/DOCX 拒绝，不进入抽取流程
	if !fileparse.IsSupported(file.Filename) {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "仅支持 PDF / DOCX 格式，请重新选择文件"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取上传文件失败: " + err.Error()})
		return
	}
	defer fh.Close()
	data, err := io.ReadAll(fh)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取上传文件失败: " + err.Error()})
		return
	}
	const maxSize = 20 << 20 // 20MB
	if len(data) == 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "文件内容为空"})
		return
	}
	if len(data) > maxSize {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "文件超过 20MB 上限"})
		return
	}
	path, err := fileparse.SaveUpload(uploadDir, file.Filename, data)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "保存文件失败: " + err.Error()})
		return
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	now := time.Now()
	pipeline := defaultPipeline()
	pj, _ := json.Marshal(pipeline)
	res, err := g.DB().Model("contract_file").Data(g.Map{
		"filename": file.Filename, "path": path, "file_hash": sum,
		"status": "pending", "error_msg": "", "stage": "", "progress": 0,
		"pipeline": pj, "template": template,
		"group_id": groupId, "version_no": versionNo,
		"legal_entity_id": scopeEntity(r),
		"created_at":      now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "contract.upload", "contract_file", id, g.Map{
		"filename": file.Filename, "size": len(data), "template": template,
		"groupId": groupId, "versionNo": versionNo,
	})
	// 异步触发流水线，接口立即返回（AC1.3 状态机，任务中心实时可见）
	go runExtraction(id)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "filename": file.Filename, "status": "pending"}})
}

// runExtraction 后台执行流水线：parse → extract（分块流式）→ done / failed
func runExtraction(id int64) {
	n := taskNotifier{fileId: id}
	stages := defaultPipeline()
	n.setStage(stages, "parse", "running", 5, "")
	n.save("extracting", stages, overallProgress(stages))

	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		n.setStage(stages, "parse", "failed", 0, "任务记录不存在")
		n.save("failed", stages, 0)
		return
	}

	// ---- 阶段一：文本解析 ----
	text, err := fileparse.ExtractText(rec["path"].String())
	if err != nil {
		n.setStage(stages, "parse", "failed", 0, err.Error())
		n.save("failed", stages, overallProgress(stages))
		return
	}
	chunks := agent.SplitChunks(text)
	// AC1.5 原文定位：解析全文落库（条款 quote 可回原文高亮定位）
	g.DB().Model("contract_file").Data(g.Map{"text_content": text, "updated_at": time.Now()}).Where("id", id).Update()
	n.setStage(stages, "parse", "done", 100, fmt.Sprintf("共 %d 字，切分为 %d 块", len([]rune(text)), len(chunks)))
	n.setStage(stages, "extract", "running", 0, fmt.Sprintf("0/%d 块", len(chunks)))
	n.save("extracting", stages, overallProgress(stages))

	// ---- 阶段二：AI 条款抽取（分块流式，按模板侧重 AC1.8） ----
	clauses, err := agent.ExtractClauses(context.Background(), text, rec["template"].String(), func(done, total int, frac float64, msg string) {
		stageP := 0
		if total > 0 {
			stageP = int((float64(done) + frac) / float64(total) * 100)
			if stageP > 98 {
				stageP = 98
			}
		}
		n.setStage(stages, "extract", "running", stageP, msg)
		n.save("extracting", stages, overallProgress(stages))
	})
	if err != nil {
		n.setStage(stages, "extract", "failed", 0, err.Error())
		n.save("failed", stages, overallProgress(stages))
		return
	}

	// ---- 落库（重试幂等：先清旧结果） ----
	g.DB().Model("contract_clause").Where("file_id", id).Delete()
	now := time.Now()
	for _, c := range clauses {
		g.DB().Model("contract_clause").Data(g.Map{
			"file_id": id, "type": c.Type, "summary": c.Summary,
			"quote": c.Quote, "risk": c.Risk,
			"review_status": "unconfirmed", "review_remark": "", "created_at": now,
		}).Insert()
	}
	n.setStage(stages, "extract", "done", 100, fmt.Sprintf("抽取 %d 条条款", len(clauses)))
	n.save("done", stages, 100)
}

func ContractFileList(r *ghttp.Request) {
	list := make([]entity.ContractFile, 0)
	if err := g.DB().Model("contract_file").Order("id desc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// path 为服务端内部路径，不下发
	for i := range list {
		list[i].Path = ""
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ContractFileClauses(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	list := make([]entity.ContractClause, 0)
	if err := g.DB().Model("contract_clause").Where("file_id", id).Order("id asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ContractFileRetry(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	g.DB().Model("contract_file").Data(g.Map{
		"status": "pending", "error_msg": "", "updated_at": time.Now(),
	}).Where("id", id).Update()
	audit(r, "contract.retry", "contract_file", id, nil)
	go runExtraction(id)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "retriggered"})
}

// WsTasks 任务中心 WebSocket：进度实时推送（推送中心见 pkg/ws）
func WsTasks(r *ghttp.Request) {
	ws.HandlerTask(r)
}

// ContractFileCompleteReview 复核完成门槛（AC2.6）：全部条款确认/疑义后方可置"复核完成"，
// 进入下一环节（义务分派等）。存在待复核条目时拒绝并返回数量。
func ContractFileCompleteReview(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "任务不存在"})
		return
	}
	if rec["status"].String() != "done" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "抽取尚未完成，无法复核"})
		return
	}
	unconfirmed, err := g.DB().Model("contract_clause").
		Where("file_id", id).Where("review_status", "unconfirmed").Count()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if unconfirmed > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("还有 %d 条条款待复核（确认或标疑义后方可完成）", unconfirmed)})
		return
	}
	if _, err := g.DB().Model("contract_file").Data(g.Map{
		"review_state": "complete", "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "contract.review_complete", "contract_file", id, nil)
	ws.Broadcast(ws.Event{Type: "task", Data: g.Map{"id": id, "reviewState": "complete"}})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "review completed"})
}

// ---------- 义务清单（v13 AC3.1-AC3.6） ----------

// ObligationGenerate 从「复核完成」合同生成义务清单（AC3.1，LLM 单次调用）
func ObligationGenerate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "任务不存在"})
		return
	}
	if rec["status"].String() != "done" || rec["review_state"].String() != "complete" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "请先完成抽取与全部条款复核，再生成义务清单"})
		return
	}
	clauses := make([]entity.ContractClause, 0)
	if err := g.DB().Model("contract_clause").Where("file_id", id).Order("id asc").Scan(&clauses); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if len(clauses) == 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该合同无已抽取条款"})
		return
	}
	input := make([]agent.ExtractedClause, 0, len(clauses))
	for _, c := range clauses {
		input = append(input, agent.ExtractedClause{Type: c.Type, Summary: c.Summary, Quote: c.Quote, Risk: c.Risk})
	}
	obs, err := agent.ExtractObligations(context.Background(), input)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// 重新生成幂等：先清旧清单
	g.DB().Model("obligation").Where("file_id", id).Delete()
	now := time.Now()
	for _, o := range obs {
		g.DB().Model("obligation").Data(g.Map{
			"file_id": id, "type": o.Type, "description": o.Description,
			"due_date": o.DueDate, "source_quote": o.SourceQuote,
			"status": "pending", "created_at": now, "updated_at": now,
		}).Insert()
	}
	audit(r, "obligation.generate", "contract_file", id, g.Map{"count": len(obs)})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"count": len(obs)}})
}

// ObligationList 义务看板（AC3.7：按人/按合同/按状态；逾期与临期为计算字段）
func ObligationList(r *ghttp.Request) {
	owner := r.Get("owner").String()
	status := r.Get("status").String()
	fileId := r.Get("fileId").Int64()

	m := g.DB().Model("obligation")
	if owner != "" {
		m = m.Where("owner", owner)
	}
	if status != "" {
		m = m.Where("status", status)
	}
	if fileId > 0 {
		m = m.Where("file_id", fileId)
	}
	list := make([]entity.Obligation, 0)
	if err := m.Order("id asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	today := time.Now().Format("2006-01-02")
	soon := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	for i := range list {
		d := list[i].DueDate
		if list[i].Status == "pending" && d != "" {
			if d < today {
				list[i].Overdue = true
			} else if d <= soon {
				list[i].DueSoon = true
			}
		}
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ClauseBatchReview 批量复核（AC2.4：批量确认 / 批量标记疑义）
func ClauseBatchReview(r *ghttp.Request) {
	var req struct {
		Ids          []int64 `json:"ids"`
		ReviewStatus string  `json:"reviewStatus"` // confirmed / doubt / unconfirmed
		ReviewRemark string  `json:"reviewRemark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	switch req.ReviewStatus {
	case "confirmed", "doubt", "unconfirmed":
	default:
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "reviewStatus 取值非法"})
		return
	}
	if len(req.Ids) == 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "ids 不能为空"})
		return
	}
	data := g.Map{"review_status": req.ReviewStatus, "updated_at": time.Now()}
	if req.ReviewRemark != "" {
		data["review_remark"] = req.ReviewRemark
	}
	res, err := g.DB().Model("contract_clause").Data(data).WhereIn("id", req.Ids).Update()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	audit(r, "clause.batch_review", "contract_clause", 0, g.Map{"ids": req.Ids, "reviewStatus": req.ReviewStatus, "count": n})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"count": n}})
}

// ObligationEvidenceUpload 证据附件上传（AC3.6：完成义务时上传证据，留痕）
func ObligationEvidenceUpload(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	if rec, err := g.DB().Model("obligation").Where("id", id).One(); err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "义务不存在"})
		return
	}
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少上传文件（字段名 file）"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取附件失败: " + err.Error()})
		return
	}
	defer fh.Close()
	data, err := io.ReadAll(fh)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取附件失败: " + err.Error()})
		return
	}
	const maxSize = 20 << 20
	if len(data) == 0 || len(data) > maxSize {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "附件为空或超过 20MB"})
		return
	}
	path, err := fileparse.SaveUpload(uploadDir+"/evidence", file.Filename, data)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "保存附件失败: " + err.Error()})
		return
	}
	if _, err := g.DB().Model("obligation").Data(g.Map{
		"evidence_path": path, "evidence_name": file.Filename, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "obligation.evidence", "obligation", id, g.Map{"filename": file.Filename, "size": len(data)})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"evidenceName": file.Filename}})
}

// ObligationEvidenceDownload 证据附件下载
func ObligationEvidenceDownload(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteStatus(400, "id required")
		return
	}
	rec, err := g.DB().Model("obligation").Where("id", id).One()
	if err != nil || rec.IsEmpty() || rec["evidence_path"].String() == "" {
		r.Response.WriteStatus(404, "no evidence")
		return
	}
	data, err := os.ReadFile(rec["evidence_path"].String())
	if err != nil {
		r.Response.WriteStatus(404, "evidence file missing")
		return
	}
	name := rec["evidence_name"].String()
	if name == "" {
		name = "evidence"
	}
	r.Response.Header().Set("Content-Disposition", "attachment; filename="+url.PathEscape(name))
	r.Response.Write(data)
}

// ObligationUpdate 分派负责人/截止日（AC3.2/3.3）、完成反馈（AC3.6），全量留痕（AC6.5）
func ObligationUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	var req struct {
		Owner   *string `json:"owner"`
		DueDate *string `json:"dueDate"`
		Status  *string `json:"status"` // pending / done
		Remark  *string `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "pending", "done":
		default:
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "status 取值非法"})
			return
		}
	}
	data := g.Map{"updated_at": time.Now()}
	if req.Owner != nil {
		data["owner"] = *req.Owner
	}
	if req.DueDate != nil {
		data["due_date"] = *req.DueDate
	}
	if req.Remark != nil {
		data["remark"] = *req.Remark
	}
	if req.Status != nil {
		data["status"] = *req.Status
		if *req.Status == "done" {
			now := time.Now()
			data["completed_at"] = now
		} else {
			data["completed_at"] = nil
		}
	}
	if _, err := g.DB().Model("obligation").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	detail := g.Map{}
	for k, v := range data {
		if k != "updated_at" {
			detail[k] = v
		}
	}
	audit(r, "obligation.update", "obligation", id, detail)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "updated"})
}

// ---------- 抽取结果导出（v13 AC1.7：JSON / Excel） ----------

func ContractFileExport(r *ghttp.Request) {
	id := r.Get("id").Int64()
	format := strings.ToLower(r.Get("format").String())
	if id <= 0 || (format != "json" && format != "xlsx") {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id 与 format(json|xlsx) 必填"})
		return
	}
	list := make([]entity.ContractClause, 0)
	if err := g.DB().Model("contract_clause").Where("file_id", id).Order("id asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// Excel 明细（v23 口径：导出一律 Excel，不再出 CSV）
	if format == "xlsx" {
		reviewMap := map[string]string{"unconfirmed": "待复核", "confirmed": "已确认", "doubt": "有疑义"}
		riskMap := map[string]string{"high": "高", "medium": "中", "low": "低", "none": "无"}
		headers := []string{"类型", "条款要点", "原文引用", "风险", "复核状态", "复核备注"}
		rows := make([][]string, 0, len(list))
		for _, c := range list {
			rows = append(rows, []string{c.Type, c.Summary, c.Quote,
				riskMap[c.Risk], reviewMap[c.ReviewStatus], c.ReviewRemark})
		}
		if err := sendXLSX(r, fmt.Sprintf("clauses-%d.xlsx", id), "抽取条款", headers, rows); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		return
	}
	body, _ := json.MarshalIndent(list, "", "  ")
	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.Response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=clauses-%d.%s", id, format))
	r.Response.Write(body)
}

// ---------- AI 生成合同（用户 11-14 拍板第一能力；产出自动进流水线） ----------

func ContractGenerate(r *ghttp.Request) {
	var p agent.GenerateParams
	p.ContractType = r.Get("contractType").String()
	p.PartyA = r.Get("partyA").String()
	p.PartyB = r.Get("partyB").String()
	p.Subject = r.Get("subject").String()
	p.Amount = r.Get("amount").String()
	p.Term = r.Get("term").String()
	p.ExtraNotes = r.Get("notes").String()
	if strings.TrimSpace(p.PartyA) == "" && strings.TrimSpace(p.PartyB) == "" && strings.TrimSpace(p.ExtraNotes) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "请至少填写一方主体或一段要点描述"})
		return
	}
	// 可选参考文件（为空则仅凭要素生成）
	if file := r.GetUploadFile("file"); file != nil {
		if !fileparse.IsSupported(file.Filename) {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "参考文件仅支持 PDF/DOCX/TXT"})
			return
		}
		fh, err := file.Open()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取参考文件失败: " + err.Error()})
			return
		}
		defer fh.Close()
		tmp, err := fileparse.SaveUpload(uploadDir+"/ref", file.Filename, mustReadAll(fh))
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "保存参考文件失败: " + err.Error()})
			return
		}
		text, err := fileparse.ExtractText(tmp)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "解析参考文件失败: " + err.Error()})
			return
		}
		p.ReferenceText = text
	}
	text, err := agent.GenerateContract(context.Background(), p)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// 生成结果存 .txt 进同一流水线（抽取→复核→审批→签署）
	path, err := fileparse.SaveUpload(uploadDir+"/generated", fmt.Sprintf("AI生成-%s-%s.txt", p.ContractType, time.Now().Format("20060102150405")), []byte(text))
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "保存生成合同失败: " + err.Error()})
		return
	}
	now := time.Now()
	res, err := g.DB().Model("contract_file").Data(g.Map{
		"filename": "AI生成-" + orDash(p.ContractType) + "-" + time.Now().Format("20060102150405") + ".txt",
		"path":     path, "file_hash": fmt.Sprintf("%x", sha256.Sum256([]byte(text))),
		"status": "pending", "stage": "", "progress": 0, "pipeline": defaultPipelineJSON(),
		"template": "general", "text_content": text,
		"legal_entity_id": scopeEntity(r),
		"created_at":      now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "contract.generate", "contract_file", id, g.Map{
		"contractType": p.ContractType, "partyA": p.PartyA, "partyB": p.PartyB, "hasRef": p.ReferenceText != "",
	})
	go runExtraction(id)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "status": "pending"}})
}

func mustReadAll(fh io.Reader) []byte {
	data, _ := io.ReadAll(fh)
	return data
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "合同"
	}
	return s
}

// defaultPipelineJSON 默认流水线 JSON（生成合同入库用）
func defaultPipelineJSON() string {
	pj, _ := json.Marshal(defaultPipeline())
	return string(pj)
}

// ---------- AC1.5 原文定位 / AC4 redline / AC5 自然语言查询 / AC7 电子签适配 ----------

// ContractFileVersions 版本历史（AC6.2）：返回同一版本组的全部文件（按版本号升序）
func ContractFileVersions(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "任务不存在"})
		return
	}
	groupId := rec["group_id"].Int64()
	if groupId == 0 {
		groupId = id
	}
	list := make([]entity.ContractFile, 0)
	if err := g.DB().Model("contract_file").
		Where("COALESCE(NULLIF(group_id,0), id) = ?", groupId).
		Order("version_no asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	for i := range list {
		list[i].Path = ""
		list[i].TextContent = ""
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ContractFileText 合同解析全文（AC1.5：条款 quote 在全文中定位高亮）
func ContractFileText(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "任务不存在"})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"id": id, "filename": rec["filename"].String(), "text": ensureText(rec),
	}})
}

// ensureText 懒补全文：旧任务缺 text_content 时从原始上传文件重解析并回填（自愈历史数据）
func ensureText(rec gdb.Record) string {
	if t := rec["text_content"].String(); t != "" {
		return t
	}
	path := rec["path"].String()
	if path == "" {
		return ""
	}
	text, err := fileparse.ExtractText(path)
	if err != nil {
		return ""
	}
	g.DB().Model("contract_file").Data(g.Map{
		"text_content": text, "updated_at": time.Now(),
	}).Where("id", rec["id"].Int64()).Update()
	return text
}

func NLQuery(r *ghttp.Request) {
	question := strings.TrimSpace(r.Get("question").String())
	if question == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "question required"})
		return
	}
	type clauseRow struct {
		FileId   int64  `json:"fileId"`
		Filename string `json:"filename"`
		Type     string `json:"type"`
		Summary  string `json:"summary"`
		Quote    string `json:"quote"`
	}
	type obRow struct {
		FileId      int64  `json:"fileId"`
		Filename    string `json:"filename"`
		Type        string `json:"type"`
		Description string `json:"description"`
		DueDate     string `json:"dueDate"`
		Status      string `json:"status"`
	}
	clauses := make([]clauseRow, 0)
	rows, err := g.DB().Model("contract_clause AS cc").
		LeftJoin("contract_file cf", "cf.id=cc.file_id").
		Fields("cc.file_id", "cf.filename", "cc.type", "cc.summary", "cc.quote").
		Order("cc.file_id asc, cc.id asc").All()
	if err == nil {
		for _, row := range rows {
			clauses = append(clauses, clauseRow{
				FileId:   row["file_id"].Int64(),
				Filename: row["filename"].String(),
				Type:     row["type"].String(),
				Summary:  row["summary"].String(),
				Quote:    row["quote"].String(),
			})
		}
	}
	obs := make([]obRow, 0)
	orows, err := g.DB().Model("obligation AS ob").
		LeftJoin("contract_file cf", "cf.id=ob.file_id").
		Fields("ob.file_id", "cf.filename", "ob.type", "ob.description", "ob.due_date", "ob.status").
		Order("ob.id asc").All()
	if err == nil {
		for _, row := range orows {
			obs = append(obs, obRow{
				FileId:      row["file_id"].Int64(),
				Filename:    row["filename"].String(),
				Type:        row["type"].String(),
				Description: row["description"].String(),
				DueDate:     row["due_date"].String(),
				Status:      row["status"].String(),
			})
		}
	}
	corpus, _ := json.Marshal(g.Map{"clauses": clauses, "obligations": obs})
	ans, err := agent.AnswerQuery(context.Background(), question, string(corpus))
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "query.ask", "contract_corpus", 0, g.Map{"question": question})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": ans})
}

// ---------- AC7 电子签适配（适配层经 API 对接外部提供方；MVP 内置 mock 演示状态机） ----------

// SignCreate 发起签署（AC7.2）：从合同生成签署任务推送至外部提供方。
// provider=mock 时走本地模拟（created→sent→(查询时转)signed），验证整条适配链路；
// docuseal / 商业 CA 为真实适配位：读取 model_config(category=esign) 的 baseUrl/apiKey 推送。
func SignCreate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	provider := r.Get("provider").String()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	if provider == "" {
		provider = "mock"
	}
	rec, err := g.DB().Model("contract_file").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "合同任务不存在"})
		return
	}
	if rec["status"].String() != "done" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "合同抽取尚未完成，不能发起签署"})
		return
	}
	now := time.Now()
	res, err := g.DB().Model("sign_task").Data(g.Map{
		"file_id": id, "provider": provider, "status": "created",
		"created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	taskId, _ := res.LastInsertId()
	// 推送提供方（mock 直接视为已送达；真实提供方在此调其 REST API，失败置 failed 可重试）
	_ = signPush(taskId, provider)
	// 回读真实状态（真实提供方未配置时 signPush 已置 failed，不谎报 sent）
	st := "sent"
	if task, err := g.DB().Model("sign_task").Where("id", taskId).One(); err == nil && !task.IsEmpty() {
		st = task["status"].String()
	}
	audit(r, "sign.create", "sign_task", taskId, g.Map{"fileId": id, "provider": provider})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"taskId": taskId, "status": st}})
}

// signPush 推送签署任务；mock 置 sent，真实提供方占位（AC7.6 失败写 fail_msg 可重试）
func signPush(taskId int64, provider string) error {
	if provider == "mock" {
		g.DB().Model("sign_task").Data(g.Map{
			"status": "sent", "external_id": fmt.Sprintf("mock-%d", taskId), "updated_at": time.Now(),
		}).Where("id", taskId).Update()
		return nil
	}
	// 真实提供方（DocuSeal/商业CA）：读 model_config(category=esign, provider=?) 配置后调其 API。
	// MVP 阶段未配置真实实例，显式置 failed 并给可读原因（不假装成功）。
	g.DB().Model("sign_task").Data(g.Map{
		"status": "failed", "fail_msg": "提供方 " + provider + " 未配置（需在模型配置页登记 esign 类别的 baseUrl/凭据）",
		"updated_at": time.Now(),
	}).Where("id", taskId).Update()
	return nil
}

// SignList 签署任务列表
func SignList(r *ghttp.Request) {
	list := make([]entity.SignTask, 0)
	if err := g.DB().Model("sign_task").Order("id desc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// SignPoll 状态回传（AC7.3：轮询提供方；mock 首次查询即转 signed 模拟对方完成签署）
func SignPoll(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	rec, err := g.DB().Model("sign_task").Where("id", id).One()
	if err != nil || rec.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "签署任务不存在"})
		return
	}
	status := rec["status"].String()
	provider := rec["provider"].String()
	if provider == "mock" && status == "sent" {
		// mock：模拟对方在下一轮查询时完成签署
		g.DB().Model("sign_task").Data(g.Map{"status": "signed", "updated_at": time.Now()}).Where("id", id).Update()
		status = "signed"
	}
	// 真实提供方：此处调其状态接口同步（AC7.3 轮询/回调）
	if status == "signed" {
		// AC7.5 签署留痕：动作与状态写入审计
		audit(r, "sign.signed", "sign_task", id, g.Map{"provider": provider})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "status": status}})
}

// ---------- 条款复核修正（v13 AC2.2/2.3 + AC2.5 留痕） ----------

func ClauseUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	var req struct {
		Summary      *string `json:"summary"`
		Risk         *string `json:"risk"`
		ReviewStatus *string `json:"reviewStatus"` // confirmed / doubt / unconfirmed
		ReviewRemark *string `json:"reviewRemark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.ReviewStatus != nil {
		switch *req.ReviewStatus {
		case "confirmed", "doubt", "unconfirmed":
		default:
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "reviewStatus 取值非法"})
			return
		}
	}
	data := g.Map{"updated_at": time.Now()}
	if req.Summary != nil {
		data["summary"] = *req.Summary
	}
	if req.Risk != nil {
		data["risk"] = *req.Risk
	}
	if req.ReviewStatus != nil {
		data["review_status"] = *req.ReviewStatus
	}
	if req.ReviewRemark != nil {
		data["review_remark"] = *req.ReviewRemark
	}
	if _, err := g.DB().Model("contract_clause").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// AC2.5 复核留痕：谁改了什么
	detail := g.Map{}
	for k, v := range data {
		if k != "updated_at" {
			detail[k] = v
		}
	}
	audit(r, "clause.review", "contract_clause", id, detail)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "updated"})
}

// AuditLogList 审计轨迹查询（AC6.5；MVP 全量倒序）
// auditQuery AK4.4 检索：按时间 / 用户 / 动作类型 / 对象类型过滤（列表与导出共用同一把尺子）
func auditQuery(r *ghttp.Request) *gdb.Model {
	m := g.DB().Model("audit_log")
	if v := r.Get("action").String(); v != "" {
		m = m.Where("action LIKE ?", "%"+v+"%")
	}
	if v := r.Get("targetType").String(); v != "" {
		m = m.Where("target_type", v)
	}
	if v := r.Get("username").String(); v != "" {
		m = m.Where("username LIKE ?", "%"+v+"%")
	}
	if v := r.Get("dateFrom").String(); v != "" {
		m = m.Where("created_at >= ?", v)
	}
	return m
}

func AuditLogList(r *ghttp.Request) {
	list := make([]entity.AuditLog, 0)
	if err := auditQuery(r).Order("id desc").Limit(200).Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// AuditLogExport AK4.5：审计轨迹导出 Excel 供合规调阅（只读追加，不修改审计记录）
func AuditLogExport(r *ghttp.Request) {
	rows, err := auditQuery(r).Order("id desc").Limit(5000).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	headers := []string{"时间", "用户", "动作", "对象类型", "对象ID", "来源IP", "详情"}
	data := make([][]string, 0, len(rows))
	for _, row := range rows {
		data = append(data, []string{
			row["created_at"].String(), row["username"].String(), row["action"].String(),
			row["target_type"].String(), row["target_id"].String(), row["ip"].String(), row["detail"].String(),
		})
	}
	audit(r, "audit.export", "audit_log", 0, g.Map{"rows": len(rows)})
	if err := sendXLSX(r, "audit-log.xlsx", "审计日志", headers, data); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
}

// audit 写审计轨迹（AC6.5：谁+何时+何事；用户当前取 dev 上下文，生产接 Casdoor 后为真实用户）
func audit(r *ghttp.Request, action, targetType string, targetId int64, detail g.Map) {
	if detail == nil {
		detail = g.Map{}
	}
	dj, _ := json.Marshal(detail)
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	g.DB().Model("audit_log").Data(g.Map{
		"ip":          ip,
		"username":    r.GetCtxVar("user").String(),
		"action":      action,
		"target_type": targetType,
		"target_id":   targetId,
		"detail":      string(dj),
		"created_at":  time.Now(),
	}).Insert()
}
