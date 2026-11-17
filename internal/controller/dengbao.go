package controller

import (
	"archive/zip"
	"bytes"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/dengbao"
)

// 等保证据链（企业侧，v23 R-C）接口：
//   GET    /api/dengbao/policy              R-C2 留存策略
//   PUT    /api/dengbao/policy               R-C2 配置留存期限（低于等保下限拒绝）
//   GET    /api/dengbao/retention/status     R-C2 留存现状诊断（是否达标 + 证据链连续性）
//   POST   /api/dengbao/retention/archive    R-C2 到期归档（dryRun 试算）
//   GET    /api/dengbao/perm-chain           R-C3 权限变更证据链
//   GET    /api/dengbao/perm-chain/export    R-C3 按等保口径导出 CSV
//   GET    /api/dengbao/evidence/blocks      R-C4 三块证据条数统计
//   GET    /api/dengbao/evidence/export      R-C4 单块证据导出
//   GET    /api/dengbao/evidence/pack        R-C9 证据包 zip + 清单
//   GET/POST /api/dengbao/assets             R-C1 等保资产台账
//   GET    /api/dengbao/assets/stats         R-C1 台账计数
//   GET/POST /api/dengbao/findings           R-C5 整改跟踪
//   POST   /api/dengbao/findings/{id}/verify|reopen|close   R-C5 复测/退回/关闭
//   GET    /api/dengbao/findings/{id}/log    R-C5 整改时间线
//   POST   /api/dengbao/backups              R-C8 备份记录登记
//   GET    /api/dengbao/backups              R-C8 备份记录清单
//   GET    /api/dengbao/backups/stats        R-C8 近 N 天备份执行统计

// ---------- R-C2：留存策略与到期归档 ----------

// DengbaoPolicyGet R-C2：读取留存期限配置
func DengbaoPolicyGet(r *ghttp.Request) {
	p, err := dengbao.GetPolicy(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"retentionDays": p.RetentionDays, "level": p.Level,
		"updatedBy": p.UpdatedBy, "floorDays": dengbao.RetentionFloorDays,
		"retentionOK": dengbao.RetentionCompliant(p.RetentionDays),
	}})
}

// DengbaoPolicySave R-C2：配置审计日志留存期限（等保下限 180 天由代码层挡住）
func DengbaoPolicySave(r *ghttp.Request) {
	var req struct {
		RetentionDays int    `json:"retentionDays" v:"required"`
		Level         string `json:"level"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	by := r.GetCtxVar("user").String()
	p, err := dengbao.SavePolicy(r.Context(), req.RetentionDays, req.Level, by)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.policy.save", "dengbao_policy", 1,
		g.Map{"retentionDays": req.RetentionDays, "level": req.Level, "operator": by})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"retentionDays": p.RetentionDays, "level": p.Level,
	}})
}

// DengbaoRetentionStatus R-C2：留存现状诊断（测评前自查：期限达标吗、证据链连续吗）
func DengbaoRetentionStatus(r *ghttp.Request) {
	out, err := dengbao.RetentionStatus(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// DengbaoArchiveExpired R-C2：到期归档（dryRun=true 只数条数不动数据）
func DengbaoArchiveExpired(r *ghttp.Request) {
	var req struct {
		DryRun bool `json:"dryRun"`
	}
	_ = r.Parse(&req)
	n, cut, err := dengbao.ArchiveExpired(r.Context(), req.DryRun)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.archive", "audit_log", 0, g.Map{
		"dryRun": req.DryRun, "moved": n, "cutoff": cut.Format("2006-01-02 15:04:05"),
		"operator": r.GetCtxVar("user").String(),
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"moved":  n,
		"dryRun": req.DryRun,
		"cutoff": cut.Format("2006-01-02 15:04:05"),
	}})
}

// ---------- R-C3：权限变更证据链 ----------

// DengbaoPermChain R-C3：角色授权/绑定/账户处置的证据链（在库 + 已归档合并）
func DengbaoPermChain(r *ghttp.Request) {
	rows, err := dengbao.PermissionChain(r.Context(), r.Get("username").String(), r.Get("since").String(), 2000)
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

// DengbaoPermChainExport R-C3：权限变更证据链按等保口径导出 Excel（测评机构常见取证口径）
func DengbaoPermChainExport(r *ghttp.Request) {
	rows, err := dengbao.PermissionChain(r.Context(), r.Get("username").String(), r.Get("since").String(), 5000)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	headers, data := dengbao.EvidenceTable(rows)
	if err := sendXLSX(r, "permission-chain.xlsx", "权限变更证据链", headers, data); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.perm_export", "audit_log", 0, g.Map{"rows": len(rows)})
}

// ---------- R-C4：访问审计导出（测评格式） ----------

// DengbaoEvidenceBlocks R-C4：身份鉴别/访问控制/安全审计三块证据条数统计
func DengbaoEvidenceBlocks(r *ghttp.Request) {
	blocks, err := dengbao.BlockStats(r.Context(), r.Get("username").String(), r.Get("since").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(blocks))
	for _, b := range blocks {
		list = append(list, g.Map{"block": string(b.Block), "count": b.Count})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// DengbaoEvidenceExport R-C4：单块证据导出（block=身份鉴别|访问控制|安全审计，缺省安全审计=全量）
func DengbaoEvidenceExport(r *ghttp.Request) {
	block := dengbao.Block(r.Get("block").String())
	if block == "" {
		block = dengbao.BlockAudit
	}
	rows, err := dengbao.BlockRows(r.Context(), block, "", r.Get("since").String(), 5000)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	name := "evidence-" + evidenceFileSlug(block) + ".xlsx"
	headers, data := dengbao.EvidenceTable(rows)
	if err := sendXLSX(r, name, string(block), headers, data); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.evidence_export", "audit_log", 0, g.Map{"block": string(block), "rows": len(rows)})
}

// evidenceFileSlug 证据块 → 文件名 slug（避免中文文件名在部分客户端/邮件网关下乱码）
func evidenceFileSlug(b dengbao.Block) string {
	switch b {
	case dengbao.BlockAuth:
		return "auth"
	case dengbao.BlockAccess:
		return "access"
	default:
		return "audit"
	}
}

// ---------- R-C9：证据包导出 ----------

// DengbaoEvidencePack R-C9：按测评机构常见要求打包（zip + 包内清单）
func DengbaoEvidencePack(r *ghttp.Request) {
	pack, err := dengbao.EvidencePack(r.Context(), r.Get("since").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	name := dengbao.EvidencePackName(time.Now())
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range pack.Files {
		w, e := zw.Create(f.Name)
		if e != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "打包失败：" + e.Error()})
			return
		}
		if _, e := w.Write(f.CSV); e != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "写入失败：" + e.Error()})
			return
		}
	}
	if e := zw.Close(); e != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "打包失败：" + e.Error()})
		return
	}
	r.Response.Header().Set("Content-Type", "application/zip")
	r.Response.Header().Set("Content-Disposition", "attachment; filename="+name)
	r.Response.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	r.Response.Write(buf.Bytes())
	audit(r, "dengbao.evidence_pack", "dengbao", 0, g.Map{
		"files": len(pack.Files), "rows": pack.TotalRows(), "operator": r.GetCtxVar("user").String(),
	})
}

// ---------- R-C1：等保资产台账 ----------

func DengbaoAssetSave(r *ghttp.Request) {
	var req struct {
		Id         int64  `json:"id"`
		Name       string `json:"name" v:"required"`
		Category   string `json:"category"`
		Level      string `json:"level"`
		Critical   string `json:"critical"`
		Owner      string `json:"owner"`
		Department string `json:"department"`
		Addr       string `json:"addr"`
		Remark     string `json:"remark"`
		Status     string `json:"status"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	by := r.GetCtxVar("user").String()
	id, err := dengbao.AssetSave(r.Context(), dengbao.Asset{
		ID: req.Id, Name: req.Name, Category: req.Category, Level: req.Level,
		Critical: req.Critical, Owner: req.Owner, Department: req.Department,
		Addr: req.Addr, Remark: req.Remark, Status: req.Status,
	}, by)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	action := "dengbao.asset.create"
	if req.Id > 0 {
		action = "dengbao.asset.update"
	}
	audit(r, action, "dengbao_asset", id, g.Map{"name": req.Name, "category": req.Category, "status": req.Status})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func DengbaoAssetList(r *ghttp.Request) {
	rows, err := dengbao.AssetList(r.Context(), r.Get("status").String(), r.Get("category").String())
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

func DengbaoAssetStats(r *ghttp.Request) {
	out, err := dengbao.AssetStats(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// ---------- R-C5：整改跟踪 ----------

func DengbaoFindingCreate(r *ghttp.Request) {
	var req struct {
		Item        string `json:"item" v:"required"`
		Description string `json:"description"`
		Level       string `json:"level"`
		Owner       string `json:"owner"`
		DueDate     string `json:"dueDate"`
		AssetID     int64  `json:"assetId"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	due := time.Time{}
	if req.DueDate != "" {
		if t, e := time.ParseInLocation("2006-01-02", req.DueDate, time.Local); e == nil {
			due = t
		}
	}
	by := r.GetCtxVar("user").String()
	id, err := dengbao.FindingCreate(r.Context(), req.Item, req.Description, req.Level, req.Owner, due, req.AssetID, by)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.finding.create", "dengbao_finding", id,
		g.Map{"item": req.Item, "level": req.Level, "owner": req.Owner})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func DengbaoFindingList(r *ghttp.Request) {
	rows, err := dengbao.FindingList(r.Context(), r.Get("status").String(), 500)
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

func DengbaoFindingStats(r *ghttp.Request) {
	out, err := dengbao.FindingStats(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// DengbaoFindingVerify R-C5：复测留痕（复测人不可与整改责任人相同）
func DengbaoFindingVerify(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Note string `json:"note"`
	}
	_ = r.Parse(&req)
	verifier := r.GetCtxVar("user").String()
	if err := dengbao.FindingVerify(r.Context(), id, verifier, req.Note); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.finding.verify", "dengbao_finding", id, g.Map{"verifier": verifier})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func DengbaoFindingReopen(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Note string `json:"note"`
	}
	_ = r.Parse(&req)
	actor := r.GetCtxVar("user").String()
	if err := dengbao.FindingReopen(r.Context(), id, actor, req.Note); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.finding.reopen", "dengbao_finding", id, g.Map{"actor": actor, "note": req.Note})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func DengbaoFindingClose(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Note string `json:"note"`
	}
	_ = r.Parse(&req)
	actor := r.GetCtxVar("user").String()
	if err := dengbao.FindingClose(r.Context(), id, actor, req.Note); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.finding.close", "dengbao_finding", id, g.Map{"actor": actor})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func DengbaoFindingLog(r *ghttp.Request) {
	rows, err := dengbao.FindingLog(r.Context(), r.Get("id").Int64(), 200)
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

// ---------- R-C8：备份记录证据 ----------

func DengbaoBackupAdd(r *ghttp.Request) {
	var req struct {
		Source     string `json:"source"`
		Tool       string `json:"tool"`
		FileName   string `json:"fileName"`
		Size       int64  `json:"sizeBytes"`
		Checksum   string `json:"checksum"`
		Status     string `json:"status"`
		Note       string `json:"note"`
		StartedAt  string `json:"startedAt"`
		FinishedAt string `json:"finishedAt"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, err := dengbao.BackupRecordAdd(r.Context(), req.Source, req.Tool, req.FileName, req.Checksum,
		req.Status, req.Note, r.GetCtxVar("user").String(), req.Size,
		parseEvidenceTime(req.StartedAt), parseEvidenceTime(req.FinishedAt))
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "dengbao.backup.add", "dengbao_backup_record", id,
		g.Map{"source": req.Source, "tool": req.Tool, "status": req.Status, "checksum": req.Checksum})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func DengbaoBackupList(r *ghttp.Request) {
	rows, err := dengbao.BackupRecordList(r.Context(), 200)
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

func DengbaoBackupStats(r *ghttp.Request) {
	days := r.Get("days").Int()
	out, err := dengbao.BackupStats(r.Context(), days)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// ---------- 公用 ----------

// parseEvidenceTime 证据时间解析（容错：解析失败按零值处理，不强塞假时间）
func parseEvidenceTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, e := time.ParseInLocation(layout, s, time.Local); e == nil {
			return t
		}
	}
	return time.Time{}
}
