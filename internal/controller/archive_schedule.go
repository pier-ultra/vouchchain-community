package controller

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/glog"
	"vouchchain/pkg/schedcore"
	"vouchchain/pkg/ws"
)

// ---------- US-I9 定期自动归档调度（AI9.1–AI9.4，P1） ----------
// 设计口径：AI5 的归档动作抽成 archiveInvoiceNow，手动归档与自动调度共用同一份实现；
// 调度按「账期」触发（周/月/季结束后第 N 天凌晨），只归档上一账期及更早的票，
// 门槛不满足的不静默跳过，而是记为失败项并进入人工处理队列。

// archiveInvoiceNow AI5.1–AI5.4 归档动作：源件 + 元数据 + 四性报告打 zip，缺件拒绝。
// 返回归档包路径；失败原因可读，供调度日志与异常告警直接使用。
func archiveInvoiceNow(id int64) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("发票 id 非法")
	}
	inv, err := g.DB().Model("invoice").Where("id", id).One()
	if err != nil || inv.IsEmpty() {
		return "", fmt.Errorf("发票 #%d 不存在", id)
	}
	if inv["archive_status"].String() == "archived" {
		return "", fmt.Errorf("发票 #%d 已归档，勿重复操作", id)
	}
	if rep := inv["integrity_report"].String(); rep == "" || rep == "[]" {
		return "", fmt.Errorf("缺件：请先完成四性检测（检测报告随档案留存）")
	}
	src, err := os.ReadFile(inv["file_path"].String())
	if err != nil {
		return "", fmt.Errorf("电子源件读取失败: %w", err)
	}
	meta, _ := json.MarshalIndent(g.Map{
		"invoice_no": inv["invoice_no"].String(), "issue_date": inv["issue_date"].String(),
		"buyer": inv["buyer_name"].String(), "seller": inv["seller_name"].String(),
		"amount": inv["amount"].Float64(), "tax": inv["tax"].Float64(), "total": inv["total"].Float64(),
		"file_hash": inv["file_hash"].String(), "voucher_id": inv["voucher_id"].Int64(),
		"verify_status":   inv["verify_status"].String(),
		"legal_entity_id": inv["legal_entity_id"].Int64(),
		"archived_at":     time.Now().Format(time.RFC3339),
	}, "", "  ")

	if err := os.MkdirAll("data/archives", 0o755); err != nil {
		return "", err
	}
	archivePath := fmt.Sprintf("data/archives/invoice-%d-%s.zip", id, time.Now().Format("20060102150405"))
	f, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)
	wSrc, _ := zw.Create("source_" + inv["file_name"].String())
	_, _ = wSrc.Write(src)
	wMeta, _ := zw.Create("metadata.json")
	_, _ = wMeta.Write(meta)
	wRep, _ := zw.Create("integrity_report.json")
	_, _ = wRep.Write([]byte(inv["integrity_report"].String()))
	if err := zw.Close(); err != nil {
		f.Close()
		return "", fmt.Errorf("归档包封口失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("归档包落盘失败: %w", err)
	}
	if _, err := g.DB().Model("invoice").Data(g.Map{
		"archive_status": "archived", "archive_path": archivePath, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		return "", fmt.Errorf("归档状态回写失败: %w", err)
	}
	return archivePath, nil
}

// auditSystem 后台调度没有 HTTP 请求上下文，按五要素补一条系统审计
func auditSystem(action, targetType string, targetId int64, detail g.Map) {
	if detail == nil {
		detail = g.Map{}
	}
	dj, _ := json.Marshal(detail)
	_, _ = g.DB().Model("audit_log").Data(g.Map{
		"username": "system", "action": action, "target_type": targetType,
		"target_id": targetId, "detail": string(dj), "ip": "scheduler", "created_at": time.Now(),
	}).Insert()
}

// systemAlert 归档异常告警：进消息中心 + WebSocket 实时角标（devMode 用户 dev / 生产 admin 各一条）
func systemAlert(title, content string) {
	for _, u := range []string{"dev", "admin"} {
		notify(u, "archive_failure", title, content)
	}
	ws.Broadcast(ws.Event{Type: "notification", Data: g.Map{"title": title, "content": content}})
}

// ---------- AI9.1 调度配置 ----------

type scheduleReq struct {
	Id              int64  `json:"id"`
	Name            string `json:"name"`
	PeriodType      string `json:"periodType"`
	DueDays         int    `json:"dueDays"`
	ScopeType       string `json:"scopeType"`
	ScopeId         int64  `json:"scopeId"`
	RequireVerified *bool  `json:"requireVerified"`
	RequireVoucher  *bool  `json:"requireVoucher"`
	RequirePaid     *bool  `json:"requirePaid"`
	Enabled         *bool  `json:"enabled"`
}

func scheduleData(req scheduleReq) (g.Map, error) {
	period, err := schedcore.NormalizePeriod(req.PeriodType)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("调度名称必填")
	}
	if req.DueDays < 1 || req.DueDays > 28 {
		return nil, fmt.Errorf("dueDays 需在 1–28 之间")
	}
	if req.ScopeType == "" {
		req.ScopeType = "all"
	}
	if req.ScopeType != "all" && req.ScopeType != "legal_entity" {
		return nil, fmt.Errorf("scopeType 只能是 all / legal_entity")
	}
	if req.ScopeType == "legal_entity" {
		if req.ScopeId <= 0 {
			return nil, fmt.Errorf("按主体归档时必须指定 scopeId")
		}
		row, err := g.DB().Model("legal_entity").Where("id", req.ScopeId).One()
		if err != nil || row.IsEmpty() {
			return nil, fmt.Errorf("法人主体 #%d 不存在", req.ScopeId)
		}
	} else {
		req.ScopeId = 0
	}
	boolVal := func(p *bool, def bool) bool {
		if p == nil {
			return def
		}
		return *p
	}
	return g.Map{
		"name": strings.TrimSpace(req.Name), "period_type": period, "due_days": req.DueDays,
		"scope_type": req.ScopeType, "scope_id": req.ScopeId,
		"require_verified": boolVal(req.RequireVerified, true),
		"require_voucher":  boolVal(req.RequireVoucher, true),
		"require_paid":     boolVal(req.RequirePaid, false),
		"enabled":          boolVal(req.Enabled, true),
		"updated_at":       time.Now(),
	}, nil
}

// ArchiveScheduleList 调度配置列表（附下次执行时间）
func ArchiveScheduleList(r *ghttp.Request) {
	rows, err := g.DB().Model("archive_schedule").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ArchiveScheduleCreate 新建调度（AI9.1）：按账期 / 主体设置周期与范围
func ArchiveScheduleCreate(r *ghttp.Request) {
	var req scheduleReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	data, err := scheduleData(req)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	period := data["period_type"].(string)
	data["created_at"] = time.Now()
	data["next_run_at"] = schedcore.NextRun(period, req.DueDays, time.Now())
	res, err := g.DB().Model("archive_schedule").Data(data).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "archive.schedule.create", "archive_schedule", id, data)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "nextRunAt": data["next_run_at"]}})
}

// ArchiveScheduleUpdate 修改调度配置
func ArchiveScheduleUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req scheduleReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	old, err := g.DB().Model("archive_schedule").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "调度配置不存在"})
		return
	}
	data, err := scheduleData(req)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// 周期/账期变了就重算下次执行时间，否则保持原计划
	if data["period_type"] != old["period_type"].String() || req.DueDays != old["due_days"].Int() {
		data["next_run_at"] = schedcore.NextRun(data["period_type"].(string), req.DueDays, time.Now())
	}
	if _, err := g.DB().Model("archive_schedule").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "archive.schedule.update", "archive_schedule", id, data)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func ArchiveScheduleDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if _, err := g.DB().Model("archive_schedule").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "archive.schedule.delete", "archive_schedule", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// ArchiveScheduleToggle 启停（停用后不再被扫描）
func ArchiveScheduleToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("archive_schedule").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "调度配置不存在"})
		return
	}
	next := !row["enabled"].Bool()
	_, err = g.DB().Model("archive_schedule").Data(g.Map{"enabled": next, "updated_at": time.Now()}).Where("id", id).Update()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "archive.schedule.toggle", "archive_schedule", id, g.Map{"enabled": next})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"enabled": next}})
}

// ---------- AI9.2 自动执行 ----------

// StartArchiveScheduler 后台扫描：每 60s 检查到期调度；用「原子推进 next_run_at」抢执行权，
// 多实例部署时也不会重复归档。启动 10s 后先扫一次，避免启动期迁移/DB 未就绪。
func StartArchiveScheduler() {
	go func() {
		time.Sleep(10 * time.Second)
		scanDueSchedules()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanDueSchedules()
		}
	}()
}

func scanDueSchedules() {
	now := time.Now()
	rows, err := g.DB().Model("archive_schedule").
		Where("enabled", true).Where("next_run_at <= ?", now).All()
	if err != nil {
		glog.Warningf(context.Background(), "[archive-sched] 扫描到期调度失败: %v", err)
		return
	}
	for _, row := range rows {
		id := row["id"].Int64()
		next := schedcore.NextRun(row["period_type"].String(), row["due_days"].Int(), now)
		res, err := g.DB().Model("archive_schedule").
			Where("id", id).Where("next_run_at <= ?", now).
			Data(g.Map{"next_run_at": next}).Update()
		if err != nil {
			glog.Warningf(context.Background(), "[archive-sched] 抢锁失败 #%d: %v", id, err)
			continue
		}
		affected, _ := res.RowsAffected()
		if affected != 1 {
			continue // 已被其它实例抢走
		}
		if _, err := runArchiveSchedule(id, "auto"); err != nil {
			glog.Warningf(context.Background(), "[archive-sched] 调度 #%d 执行异常: %v", id, err)
		}
	}
}

// runArchiveSchedule 执行一次调度（手动与自动共用），返回运行 id
func runArchiveSchedule(scheduleId int64, trigger string) (int64, error) {
	s, err := g.DB().Model("archive_schedule").Where("id", scheduleId).One()
	if err != nil || s.IsEmpty() {
		return 0, fmt.Errorf("调度配置 #%d 不存在", scheduleId)
	}
	period := s["period_type"].String()
	cutoff := schedcore.Cutoff(period, time.Now())
	res, err := g.DB().Model("archive_schedule_run").Data(g.Map{
		"schedule_id": scheduleId, "trigger": trigger, "status": "running",
		"cutoff_date": cutoff, "started_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	runId, _ := res.LastInsertId()

	m := g.DB().Model("invoice").
		Where("issue_date <= ? AND COALESCE(issue_date,'') <> ''", cutoff).
		Where("COALESCE(archive_status,'') <> ?", "archived")
	if s["scope_type"].String() == "legal_entity" {
		m = m.Where("legal_entity_id", s["scope_id"].Int64())
	}
	if s["require_verified"].Bool() {
		m = m.Where("verify_status", "verified")
	}
	if s["require_voucher"].Bool() {
		m = m.Where("voucher_id > 0")
	}
	if s["require_paid"].Bool() {
		m = m.Where("id IN (SELECT invoice_id FROM payment_plan WHERE status = 'paid')")
	}
	invs, err := m.Order("id asc").Limit(500).All()
	if err != nil {
		_, _ = g.DB().Model("archive_schedule_run").Data(g.Map{
			"status": "failed", "message": err.Error(), "finished_at": time.Now(),
		}).Where("id", runId).Update()
		return runId, err
	}

	success, failed, skipped := 0, 0, 0
	for _, inv := range invs {
		invId := inv["id"].Int64()
		invNo := inv["invoice_no"].String()
		path, aerr := archiveInvoiceNow(invId)
		if aerr == nil {
			success++
			_, _ = g.DB().Model("archive_schedule_item").Data(g.Map{
				"run_id": runId, "invoice_id": invId, "invoice_no": invNo,
				"result": "ok", "archive_path": path, "created_at": time.Now(),
			}).Insert()
			auditSystem("invoice.archive", "invoice", invId, g.Map{
				"archivePath": path, "trigger": "schedule", "runId": runId, "scheduleId": scheduleId,
			})
			continue
		}
		failed++
		_, _ = g.DB().Model("archive_schedule_item").Data(g.Map{
			"run_id": runId, "invoice_id": invId, "invoice_no": invNo,
			"result": "failed", "message": aerr.Error(), "created_at": time.Now(),
		}).Insert()
		// AI9.4 同票已有待处理异常不重复堆积
		exist, _ := g.DB().Model("archive_exception").
			Where("invoice_id", invId).Where("status", "pending").One()
		if exist.IsEmpty() {
			_, _ = g.DB().Model("archive_exception").Data(g.Map{
				"run_id": runId, "invoice_id": invId, "invoice_no": invNo,
				"reason": aerr.Error(), "status": "pending", "created_at": time.Now(),
			}).Insert()
			systemAlert("归档失败待处理",
				fmt.Sprintf("发票 %s（#%d）自动归档失败：%s。请到「归档调度 - 异常待办」人工处理。", invNo, invId, aerr.Error()))
		}
	}

	msg := fmt.Sprintf("账期 %s 截止 %s，命中 %d 张：成功 %d / 失败 %d / 跳过 %d", period, cutoff, len(invs), success, failed, skipped)
	if failed > 0 {
		msg += "；失败项已进入人工处理队列并告警"
	}
	_, _ = g.DB().Model("archive_schedule_run").Data(g.Map{
		"status": "done", "total": len(invs), "success": success, "failed": failed,
		"skipped": skipped, "message": msg, "finished_at": time.Now(),
	}).Where("id", runId).Update()
	_, _ = g.DB().Model("archive_schedule").Data(g.Map{
		"last_run_at": time.Now(), "updated_at": time.Now(),
	}).Where("id", scheduleId).Update()
	auditSystem("archive.schedule.run", "archive_schedule", scheduleId, g.Map{
		"runId": runId, "trigger": trigger, "total": len(invs), "success": success, "failed": failed, "cutoff": cutoff,
	})
	if failed > 0 {
		systemAlert("归档调度完成（有失败项）", msg)
	}
	return runId, nil
}

// ArchiveScheduleRunNow 手动立即执行（同一执行函数，便于验证）
func ArchiveScheduleRunNow(r *ghttp.Request) {
	id := r.Get("id").Int64()
	runId, err := runArchiveSchedule(id, "manual")
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "archive.schedule.run", "archive_schedule", id, g.Map{"runId": runId, "trigger": "manual"})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"runId": runId}})
}

// ---------- AI9.3 执行日志 ----------

func ArchiveScheduleRunList(r *ghttp.Request) {
	m := g.DB().Model("archive_schedule_run")
	if v := r.Get("scheduleId").Int64(); v > 0 {
		m = m.Where("schedule_id", v)
	}
	rows, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func ArchiveScheduleRunItems(r *ghttp.Request) {
	runId := r.Get("id").Int64()
	rows, err := g.DB().Model("archive_schedule_item").Where("run_id", runId).Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ---------- AI9.4 异常告警与人工处理 ----------

func ArchiveExceptionList(r *ghttp.Request) {
	m := g.DB().Model("archive_exception")
	if v := r.Get("status").String(); v != "" {
		m = m.Where("status", v)
	}
	rows, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ArchiveExceptionHandle 人工处理：retry=补齐缺件后重试归档；ignore=登记说明后忽略
func ArchiveExceptionHandle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Action string `json:"action"` // retry / ignore
		Note   string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	row, err := g.DB().Model("archive_exception").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "异常记录不存在"})
		return
	}
	if row["status"].String() != "pending" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该异常已处理"})
		return
	}
	if req.Action != "retry" && req.Action != "ignore" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "action 只能是 retry / ignore"})
		return
	}
	result := "resolved"
	note := strings.TrimSpace(req.Note)
	if req.Action == "retry" {
		path, aerr := archiveInvoiceNow(row["invoice_id"].Int64())
		if aerr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "重试归档仍失败：" + aerr.Error() + "（异常保持待处理）"})
			return
		}
		note = "重试归档成功：" + path + "；" + note
		auditSystem("invoice.archive", "invoice", row["invoice_id"].Int64(), g.Map{
			"archivePath": path, "trigger": "exception_retry", "exceptionId": id,
		})
	} else {
		if note == "" {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "忽略异常必须填写处理说明"})
			return
		}
		result = "ignored"
	}
	_, err = g.DB().Model("archive_exception").Data(g.Map{
		"status": result, "handled_by": r.GetCtxVar("user").String(),
		"handled_note": note, "handled_at": time.Now(),
	}).Where("id", id).Update()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "archive.exception.handle", "archive_exception", id, g.Map{"action": req.Action, "result": result, "note": note})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}
