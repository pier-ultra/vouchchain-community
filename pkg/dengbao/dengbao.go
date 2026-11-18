package dengbao

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/xuri/excelize/v2"

	"vouchchain/pkg/dbutil"
)

// 等保证据链（企业侧，v23 R-C，P2 定位）：
// 被测企业平时攒证据、测评时拿得出来。复用既有 audit_log（五要素），只补「留存策略 + 到期归档 + 按等保口径导出」。
// 明确不做：测评执行（JSecProbe 的领域）、安全设备纳管（SIEM 的领域）、密评密码改造。

// ---------- R-C2：留存期限口径（纯函数，可单测） ----------

// RetentionFloorDays 等保 2.0（GB/T 22239-2019 安全审计）审计日志留存下限：不少于 6 个月。
const RetentionFloorDays = 180

// RetentionCompliant 配置/现状是否达到等保下限
func RetentionCompliant(retentionDays int) bool { return retentionDays >= RetentionFloorDays }

// ValidateRetention 拒绝低于等保下限的留存配置（宁可挡住，不给「配短」的口子）
func ValidateRetention(retentionDays int) error {
	if retentionDays < RetentionFloorDays {
		return fmt.Errorf("留存期限 %d 天低于等保下限 %d 天（GB/T 22239-2019 安全审计要求不少于 6 个月）",
			retentionDays, RetentionFloorDays)
	}
	return nil
}

// Cutoff 到期归档截止点：早于该时刻的日志才可归档
func Cutoff(now time.Time, retentionDays int) time.Time {
	return now.AddDate(0, 0, -retentionDays)
}

// CoverageOK 证据链连续性：现存最早一条日志距今是否仍覆盖留存期（早于该期=证据断了）
func CoverageOK(earliest, now time.Time, retentionDays int) bool {
	if earliest.IsZero() || now.IsZero() {
		return false
	}
	return now.Sub(earliest) >= time.Duration(retentionDays)*24*time.Hour
}

// ---------- R-C3/R-C4：证据块分类口径（纯函数，单一事实源） ----------

// Block 测评证据块（GB/T 22239 常见取证分类）
type Block string

const (
	BlockAuth   Block = "身份鉴别"
	BlockAccess Block = "访问控制"
	BlockAudit  Block = "安全审计" // 安全审计块=全量审计日志（等保要求覆盖所有用户操作）
)

// 动作前缀口径：SQL 过滤与 Go 分类共用这一份，避免「两处阈值口径漂移」
var (
	authPrefixes   = []string{"auth.", "context.switch", "scim.user."}
	accessPrefixes = []string{"rbac.", "isolation.policy", "compliance.rule", "scim.role_map", "seal.approval_rule"}
	permPrefixes   = []string{"rbac.", "scim.user.", "scim.role_map"}
)

func matchAny(action string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(action, p) {
			return true
		}
	}
	return false
}

// IsPermissionChange R-C3：是否权限/身份变更动作（角色授权、绑定、账户处置）
func IsPermissionChange(action string) bool { return matchAny(action, permPrefixes) }

// ClassifyBlock R-C4：动作归属的测评证据块；其余动作归安全审计（全量块）
func ClassifyBlock(action string) Block {
	if matchAny(action, authPrefixes) {
		return BlockAuth
	}
	if matchAny(action, accessPrefixes) {
		return BlockAccess
	}
	return BlockAudit
}

// BlockWhere R-C4：证据块的 SQL 过滤片段（安全审计返回空串=不过滤）
func BlockWhere(b Block) string {
	switch b {
	case BlockAuth:
		return prefixRegexSQL(authPrefixes)
	case BlockAccess:
		return prefixRegexSQL(accessPrefixes)
	case BlockAudit:
		return ""
	}
	return ""
}

// PermissionWhere R-C3：权限变更证据链的 SQL 过滤片段
func PermissionWhere() string { return prefixRegexSQL(permPrefixes) }

// prefixRegexSQL 把前缀列表拼成 WHERE 过滤片段（按数据库方言）：
//   - PostgreSQL / openGauss / KingBase：正则锚定 ^（`action ~ '^(rbac\.|...)'`）
//   - SQLite / MySQL / DM：无 ~ 正则算子（MySQL 的 ~ 是按位异或；达梦 Oracle 系用 REGEXP_LIKE），统一 LIKE 前缀匹配
func prefixRegexSQL(prefixes []string) string {
	if dbutil.IsSQLite() || dbutil.IsMySQL() || dbutil.IsDM() {
		return PrefixLikeSQL(prefixes)
	}
	return PrefixRegexSQL(prefixes)
}

// PrefixRegexSQL 前缀列表 → PostgreSQL 正则过滤片段（^ 锚定，避免 'xrbac.' 误命中）
func PrefixRegexSQL(prefixes []string) string {
	parts := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		parts = append(parts, regexp.QuoteMeta(p))
	}
	return "action ~ '^(" + strings.Join(parts, "|") + ")'"
}

// PrefixLikeSQL 前缀列表 → SQLite LIKE 前缀过滤片段
func PrefixLikeSQL(prefixes []string) string {
	conds := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		conds = append(conds, "action LIKE '"+strings.ReplaceAll(p, "'", "''")+"%'")
	}
	return "(" + strings.Join(conds, " OR ") + ")"
}

// ---------- R-C9：证据包 ----------

// EvidencePackName 证据包文件名（带生成时刻，便于留档比对）
func EvidencePackName(now time.Time) string {
	return "dengbao-evidence-" + now.Format("20060102-150405") + ".zip"
}

// ManifestEntry 清单条目（zip 内 manifest.xlsx 的一行）
type ManifestEntry struct {
	Name        string
	Block       string
	Rows        int
	GeneratedAt time.Time
}

// EvidenceFile 打包内的一个 Excel 明细
type EvidenceFile struct {
	Name string
	CSV  []byte
}

// ---------- 统一的审计证据查询（live 表 + 归档表合并，时间正序=证据链顺序） ----------

// auditEvidence R-C3/R-C4 共用取证原语：同一把尺子查在库与已归档两类日志。
// cond 只允许传代码内拼好的列条件（值一律走绑定参数）。
func auditEvidence(ctx context.Context, cond string, args []any, username, since string, limit int) ([]gdb.Record, error) {
	if limit <= 0 {
		limit = 5000
	}
	var conds []string
	all := append([]any{}, args...)
	if cond != "" {
		conds = append(conds, cond)
	}
	if username != "" {
		conds = append(conds, "username = ?")
		all = append(all, username)
	}
	if since != "" {
		conds = append(conds, "created_at >= ?")
		all = append(all, since)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	all = append(all, limit)
	// 注意：audit_log.detail 是 jsonb，audit_log_archive.detail 是 TEXT，UNION 两侧必须同型
	// → 统一 ::text 再合并（下游 recordRows 一律按 String() 取数，文本化不影响口径）。
	// sqlite/mysql/dm 下 detail 为 TEXT/JSON/CLOB，无需 cast（无 ::text 语法）。
	detailExpr := "detail::text"
	if dbutil.IsSQLite() || dbutil.IsMySQL() || dbutil.IsDM() {
		detailExpr = "detail"
	}
	sql := `SELECT 'live' AS scope, id, username, action, target_type, target_id, ` + detailExpr + ` AS detail, ip, created_at
	      FROM audit_log` + where + `
	      UNION ALL
	      SELECT 'archive' AS scope, id, username, action, target_type, target_id, ` + detailExpr + ` AS detail, ip, created_at
	      FROM audit_log_archive` + where + `
	      ORDER BY created_at ASC, id ASC
	      LIMIT ?`
	return g.DB().GetAll(ctx, sql, all...)
}

// evidenceColumn 证据列口径：Label 供测评机构直接读，Column 供取数
type evidenceColumn struct {
	Label  string
	Column string
}

// recordRows 把查询记录按列口径摊平成二维字符串（列名取数、标签供展示）
func recordRows(rows []gdb.Record, columns []evidenceColumn) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		rec := make([]string, 0, len(columns))
		for _, c := range columns {
			v := r[c.Column]
			if v == nil || v.IsNil() {
				rec = append(rec, "")
				continue
			}
			rec = append(rec, v.String())
		}
		out = append(out, rec)
	}
	return out
}

// columnsHeader 列口径 → CSV 表头
func columnsHeader(columns []evidenceColumn) []string {
	header := make([]string, 0, len(columns))
	for _, c := range columns {
		header = append(header, c.Label)
	}
	return header
}

// packXLSX 把表头+数据行渲染为单表 xlsx 字节（证据包 zip 内文件用）。
func packXLSX(sheet string, header []string, rows [][]string) []byte {
	if rows == nil {
		rows = [][]string{}
	}
	data, _ := generateWorkbook(sheet, header, rows)
	return data
}

// renderRecords 记录集 → 证据表头+数据行（列口径由 columns 指定）
func renderRecords(columns []evidenceColumn, rows []gdb.Record) [][]string {
	return recordRows(rows, columns)
}

// evidenceColumns 审计证据统一列口径（R-C3/R-C4 共用；标签供测评机构直接读，列名供取数）
var evidenceColumns = []evidenceColumn{
	{"证据来源", "scope"}, {"日志ID", "id"}, {"操作人", "username"}, {"动作", "action"},
	{"对象类型", "target_type"}, {"对象ID", "target_id"}, {"详情", "detail"},
	{"来源IP", "ip"}, {"时间", "created_at"},
}

// assetColumns R-C1 资产台账列口径
var assetColumns = []evidenceColumn{
	{"ID", "id"}, {"资产名称", "name"}, {"类别", "category"}, {"拟保级别", "level"},
	{"重要程度", "critical"}, {"责任人", "owner"}, {"部门", "department"},
	{"部署地址", "addr"}, {"状态", "status"}, {"登记时间", "created_at"},
}

// findingColumns R-C5 整改跟踪列口径
var findingColumns = []evidenceColumn{
	{"ID", "id"}, {"控制点/测评项", "item"}, {"级别", "level"}, {"状态", "status"},
	{"整改责任人", "owner"}, {"复测人", "verifier"}, {"复测时间", "verified_at"},
	{"整改期限", "due_date"}, {"登记时间", "created_at"},
}

// backupColumns R-C8 备份记录列口径
var backupColumns = []evidenceColumn{
	{"ID", "id"}, {"来源", "source"}, {"工具", "tool"}, {"备份文件", "file_name"},
	{"大小(字节)", "size_bytes"}, {"校验和", "checksum"}, {"状态", "status"},
	{"开始时间", "started_at"}, {"完成时间", "finished_at"}, {"登记时间", "created_at"},
}

// ---------- R-C2：留存策略 ----------

// Policy 留存策略（app 级单行）
type Policy struct {
	RetentionDays int
	Level         string
	UpdatedBy     string
	UpdatedAt     time.Time
}

// GetPolicy 读留存策略；无记录时按等保下限兜底（未初始化实例也不能低于下限）
func GetPolicy(ctx context.Context) (Policy, error) {
	row, err := g.DB().Model("dengbao_policy").Where("id", 1).One()
	if err != nil {
		return Policy{}, err
	}
	if row.IsEmpty() {
		return Policy{RetentionDays: RetentionFloorDays, Level: "2"}, nil
	}
	return Policy{
		RetentionDays: row["retention_days"].Int(),
		Level:         row["level"].String(),
		UpdatedBy:     row["updated_by"].String(),
		UpdatedAt:     row["updated_at"].Time(),
	}, nil
}

// SavePolicy 配置留存期限（低于等保下限直接拒绝）+ 拟保级别
func SavePolicy(ctx context.Context, retentionDays int, level, by string) (Policy, error) {
	if err := ValidateRetention(retentionDays); err != nil {
		return Policy{}, err
	}
	if level == "" {
		level = "2"
	}
	now := time.Now()
	if _, err := g.DB().Model("dengbao_policy").Data(g.Map{
		"retention_days": retentionDays, "level": level, "updated_by": by, "updated_at": now,
	}).Where("id", 1).Update(); err != nil {
		return Policy{}, err
	}
	// 种子行缺失时补一条（迁移已保证存在，这里兜底手工建库的场景）
	// pg/sqlite 用 ON CONFLICT DO NOTHING；MySQL 用 ON DUPLICATE KEY UPDATE id=id（no-op）；
	// openGauss 无 ON CONFLICT 且禁止更新主键，改用非键列自赋值做 no-op。
	conflict := "ON CONFLICT (id) DO NOTHING"
	if dbutil.IsMySQL() {
		conflict = "ON DUPLICATE KEY UPDATE id = id"
	} else if dbutil.IsOpenGauss() {
		conflict = "ON DUPLICATE KEY UPDATE retention_days = dengbao_policy.retention_days"
	}
	// 达梦无 ON CONFLICT；seed 已由迁移保证，跳过此运行时兜底插入。
	if !dbutil.IsDM() {
		// 金仓 Oracle 兼容模式下 level 是保留字，列名必须双引号；其他库裸写。
		levelCol := "level"
		if dbutil.IsKingBase() {
			levelCol = `"level"`
		}
		_, _ = g.DB().Exec(ctx, `INSERT INTO dengbao_policy (id, retention_days, `+levelCol+`, updated_by, updated_at)
			VALUES (1, ?, ?, ?, ?) `+conflict, retentionDays, level, by, now)
	}
	return GetPolicy(ctx)
}

// earliestLogTime 现存最早一条审计日志（在库 + 归档一起算，证据链连续性口径）
func earliestLogTime(ctx context.Context) time.Time {
	v, err := g.DB().GetValue(ctx, `SELECT MIN(created_at) FROM (
		SELECT MIN(created_at) AS created_at FROM audit_log
		UNION ALL
		SELECT MIN(created_at) FROM audit_log_archive) t`)
	if err != nil || v == nil || v.IsNil() {
		return time.Time{}
	}
	return v.Time()
}

// RetentionStatus 留存现状诊断（期限是否达标 + 证据链是否覆盖留存期 + 在库/归档条数）
func RetentionStatus(ctx context.Context) (map[string]any, error) {
	p, err := GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	earliest := earliestLogTime(ctx)
	live, _ := g.DB().GetValue(ctx, "SELECT COUNT(*) FROM audit_log")
	arch, _ := g.DB().GetValue(ctx, "SELECT COUNT(*) FROM audit_log_archive")
	coverage := 0.0
	if !earliest.IsZero() {
		coverage = float64(now.Sub(earliest).Hours()) / 24
	}
	out := map[string]any{
		"retentionDays": p.RetentionDays,
		"floorDays":     RetentionFloorDays,
		"retentionOK":   RetentionCompliant(p.RetentionDays),
		"earliestAt":    "",
		"coverageDays":  coverage,
		"coverageOK":    CoverageOK(earliest, now, p.RetentionDays),
		"liveCount":     0,
		"archiveCount":  0,
	}
	if !earliest.IsZero() {
		out["earliestAt"] = earliest.Format("2006-01-02 15:04:05")
	}
	if live != nil && !live.IsNil() {
		out["liveCount"] = live.Int64()
	}
	if arch != nil && !arch.IsNil() {
		out["archiveCount"] = arch.Int64()
	}
	return out, nil
}

// ArchiveExpired 到期归档：早于截止点的审计日志搬到 audit_log_archive（搬不是删，总留存期不变）。
// dryRun=true 只数条数不搬。返回（处理条数, 截止点, err）。
func ArchiveExpired(ctx context.Context, dryRun bool) (int64, time.Time, error) {
	p, err := GetPolicy(ctx)
	if err != nil {
		return 0, time.Time{}, err
	}
	cut := Cutoff(time.Now(), p.RetentionDays)
	if dryRun {
		n, e := g.DB().GetValue(ctx, "SELECT COUNT(*) FROM audit_log WHERE created_at < ?", cut)
		if e != nil {
			return 0, cut, e
		}
		return n.Int64(), cut, nil
	}
	detailExpr := "detail::text"
	if dbutil.IsSQLite() || dbutil.IsMySQL() || dbutil.IsDM() {
		detailExpr = "detail"
	}
	// pg/sqlite 用 ON CONFLICT DO NOTHING；MySQL 用 ON DUPLICATE KEY UPDATE id=id（no-op）；
	// openGauss 禁止更新主键，用非键列自赋值做 no-op。
	conflictClause := "ON CONFLICT (id) DO NOTHING"
	if dbutil.IsMySQL() {
		conflictClause = "ON DUPLICATE KEY UPDATE id = id"
	} else if dbutil.IsOpenGauss() {
		conflictClause = "ON DUPLICATE KEY UPDATE username = audit_log_archive.username"
	}
	if dbutil.IsDM() {
		// 达梦无 INSERT...SELECT ON CONFLICT：先删后插（归档表不存在唯一约束冲突时直接插入）
		if _, err := g.DB().Exec(ctx, `
			INSERT INTO audit_log_archive (id, username, action, target_type, target_id, detail, ip, created_at, archived_at)
			SELECT id, COALESCE(username, ''), action, target_type, target_id, COALESCE(`+detailExpr+`, ''), COALESCE(ip, ''), created_at, CURRENT_TIMESTAMP
			FROM audit_log WHERE created_at < ?`, cut); err != nil {
			return 0, cut, err
		}
		res, err := g.DB().Exec(ctx, "DELETE FROM audit_log WHERE created_at < ?", cut)
		if err != nil {
			return 0, cut, err
		}
		n, _ := res.RowsAffected()
		return n, cut, nil
	}
	if _, err := g.DB().Exec(ctx, `
		INSERT INTO audit_log_archive (id, username, action, target_type, target_id, detail, ip, created_at, archived_at)
		SELECT id, COALESCE(username, ''), action, target_type, target_id, COALESCE(`+detailExpr+`, ''), COALESCE(ip, ''), created_at, CURRENT_TIMESTAMP
		FROM audit_log WHERE created_at < ?
		`+conflictClause, cut); err != nil {
		return 0, cut, err
	}
	res, err := g.DB().Exec(ctx, "DELETE FROM audit_log WHERE created_at < ?", cut)
	if err != nil {
		return 0, cut, err
	}
	n, _ := res.RowsAffected()
	return n, cut, nil
}

// ---------- R-C3：权限变更证据链 ----------

// PermissionChain 权限/身份变更证据链（在库+归档合并，时间正序）
func PermissionChain(ctx context.Context, username, since string, limit int) ([]gdb.Record, error) {
	return auditEvidence(ctx, PermissionWhere(), nil, username, since, limit)
}

// EvidenceBlocks R-C4：按测评证据块统计取证条数（三块 + 权限变更块）
type EvidenceBlock struct {
	Block Block
	Count int64
}

// BlockStats R-C4：三块证据包的取证条数（安全审计块=全量）
func BlockStats(ctx context.Context, username, since string) ([]EvidenceBlock, error) {
	out := make([]EvidenceBlock, 0, 3)
	for _, b := range []Block{BlockAuth, BlockAccess, BlockAudit} {
		rows, err := auditEvidence(ctx, BlockWhere(b), nil, username, since, 5000)
		if err != nil {
			return nil, err
		}
		out = append(out, EvidenceBlock{Block: b, Count: int64(len(rows))})
	}
	return out, nil
}

// BlockRows R-C4：取某一块证据的明细（在库+归档合并，时间正序）
func BlockRows(ctx context.Context, b Block, username, since string, limit int) ([]gdb.Record, error) {
	return auditEvidence(ctx, BlockWhere(b), nil, username, since, limit)
}

// EvidenceCSV R-C3/R-C4 共用：审计证据 CSV（列口径统一，含 BOM）
// EvidenceTable 证据列口径的表头+数据行双通道（Excel 导出复用）。
func EvidenceTable(rows []gdb.Record) ([]string, [][]string) {
	return columnsHeader(evidenceColumns), recordRows(rows, evidenceColumns)
}

// TotalRows 证据包内明细总行数（清单自述用；清单本身不计入）
func (p Pack) TotalRows() int {
	n := 0
	for _, m := range p.Manifest {
		if m.Name == "清单.xlsx" {
			continue
		}
		n += m.Rows
	}
	return n
}

// ---------- R-C1：等保资产台账 ----------

// Asset 资产台账输入
type Asset struct {
	ID         int64
	Name       string
	Category   string // application/system/device
	Level      string // 1/2/3
	Critical   string // critical/normal
	Owner      string
	Department string
	Addr       string
	Remark     string
	Status     string // active/retired
}

// AssetSave R-C1：登记/更新资产（id>0 更新，否则新建）
func AssetSave(ctx context.Context, a Asset, by string) (int64, error) {
	if strings.TrimSpace(a.Name) == "" {
		return 0, fmt.Errorf("资产名称必填")
	}
	if a.Category == "" {
		a.Category = "application"
	}
	if a.Level == "" {
		a.Level = "2"
	}
	if a.Critical == "" {
		a.Critical = "normal"
	}
	if a.Status == "" {
		a.Status = "active"
	}
	data := g.Map{
		"name": a.Name, "category": a.Category, "level": a.Level, "critical": a.Critical,
		"owner": a.Owner, "department": a.Department, "addr": a.Addr,
		"remark": a.Remark, "status": a.Status, "updated_at": time.Now(),
	}
	if a.ID > 0 {
		row, err := g.DB().Model("dengbao_asset").Where("id", a.ID).One()
		if err != nil {
			return 0, err
		}
		if row.IsEmpty() {
			return 0, fmt.Errorf("资产不存在")
		}
		if _, err := g.DB().Model("dengbao_asset").Data(data).Where("id", a.ID).Update(); err != nil {
			return 0, err
		}
		return a.ID, nil
	}
	res, err := g.DB().Model("dengbao_asset").Data(g.Map{
		"name": a.Name, "category": a.Category, "level": a.Level, "critical": a.Critical,
		"owner": a.Owner, "department": a.Department, "addr": a.Addr,
		"remark": a.Remark, "status": a.Status, "creator": by,
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// AssetList R-C1：资产清单（status/category 过滤）
func AssetList(ctx context.Context, status, category string) ([]gdb.Record, error) {
	m := g.DB().Model("dengbao_asset")
	if status != "" {
		m = m.Where("status", status)
	}
	if category != "" {
		m = m.Where("category", category)
	}
	return m.Order("id asc").All()
}

// AssetStats R-C1：按类别/级别/状态计数（资产台账一览，供测评口径核对）
func AssetStats(ctx context.Context) (map[string]any, error) {
	// 金仓 Oracle 兼容模式下 level 是保留字，列名必须双引号；其他库裸写。
	levelCol := "level"
	if dbutil.IsKingBase() {
		levelCol = `"level"`
	}
	rows, err := g.DB().GetAll(ctx, `
		SELECT category, `+levelCol+`, status, COUNT(*) AS n
		FROM dengbao_asset GROUP BY category, `+levelCol+`, status ORDER BY category, `+levelCol+`, status`)
	if err != nil {
		return nil, err
	}
	groups := make([]map[string]any, 0, len(rows))
	out := map[string]any{"total": int64(0), "byGroup": groups}
	total := int64(0)
	for _, r := range rows {
		n := r["n"].Int64()
		total += n
		groups = append(groups, map[string]any{
			"category": r["category"].String(), "level": r["level"].String(),
			"status": r["status"].String(), "count": n,
		})
	}
	out["total"] = total
	out["byGroup"] = groups
	return out, nil
}

// ---------- R-C5：整改跟踪 ----------

// 整改状态机：open → fixing → verified → closed（可 reopen 回 fixing）
const (
	FindingOpen     = "open"
	FindingFixing   = "fixing"
	FindingVerified = "verified"
	FindingClosed   = "closed"
)

func findingLog(ctx context.Context, findingID int64, action, actor, note string) {
	_, _ = g.DB().Model("dengbao_finding_log").Data(g.Map{
		"finding_id": findingID, "action": action, "actor": actor,
		"note": note, "created_at": time.Now(),
	}).Insert()
}

// FindingCreate R-C5：登记测评问题（问题清单 → 整改任务）
func FindingCreate(ctx context.Context, item, description, level, owner string, due time.Time, assetID int64, by string) (int64, error) {
	if strings.TrimSpace(item) == "" {
		return 0, fmt.Errorf("控制点/测评项必填")
	}
	if level == "" {
		level = "medium"
	}
	data := g.Map{
		"item": item, "description": description, "level": level,
		"status": FindingOpen, "owner": owner, "asset_id": assetID,
		"creator": by, "created_at": time.Now(), "updated_at": time.Now(),
	}
	if !due.IsZero() {
		data["due_date"] = due
	}
	res, err := g.DB().Model("dengbao_finding").Data(data).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	findingLog(ctx, id, "create", by, "登记整改任务")
	return id, nil
}

// FindingUpdate R-C5：改整改责任人/期限/描述（状态推进同时留痕）
func FindingUpdate(ctx context.Context, id int64, owner, description, level, status string, due time.Time, actor, note string) error {
	row, err := g.DB().Model("dengbao_finding").Where("id", id).One()
	if err != nil {
		return err
	}
	if row.IsEmpty() {
		return fmt.Errorf("整改任务不存在")
	}
	data := g.Map{"updated_at": time.Now()}
	if owner != "" {
		data["owner"] = owner
	}
	if description != "" {
		data["description"] = description
	}
	if level != "" {
		data["level"] = level
	}
	if status != "" {
		if !findingStatusOK(status) {
			return fmt.Errorf("状态只能是 open/fixing/verified/closed")
		}
		data["status"] = status
	}
	if !due.IsZero() {
		data["due_date"] = due
	}
	if _, err := g.DB().Model("dengbao_finding").Data(data).Where("id", id).Update(); err != nil {
		return err
	}
	act := "update"
	if status != "" {
		act = "status_" + status
	}
	findingLog(ctx, id, act, actor, note)
	return nil
}

// FindingVerify R-C5：复测留痕（复测人 + 结论 + 时间，可独立于整改人）
func FindingVerify(ctx context.Context, id int64, verifier, note string) error {
	row, err := g.DB().Model("dengbao_finding").Where("id", id).One()
	if err != nil {
		return err
	}
	if row.IsEmpty() {
		return fmt.Errorf("整改任务不存在")
	}
	if owner := row["owner"].String(); owner != "" && verifier == owner {
		return fmt.Errorf("复测人不可与整改责任人相同（%s）", owner)
	}
	now := time.Now()
	if _, err := g.DB().Model("dengbao_finding").Data(g.Map{
		"status": FindingVerified, "verifier": verifier, "verify_note": note,
		"verified_at": now, "updated_at": now,
	}).Where("id", id).Update(); err != nil {
		return err
	}
	findingLog(ctx, id, "verify", verifier, note)
	return nil
}

// FindingReopen R-C5：复测不过退回整改中（留痕）
func FindingReopen(ctx context.Context, id int64, actor, note string) error {
	if _, err := g.DB().Model("dengbao_finding").Data(g.Map{
		"status": FindingFixing, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		return err
	}
	findingLog(ctx, id, "reopen", actor, note)
	return nil
}

// FindingClose R-C5：关闭（必须已复测通过）
func FindingClose(ctx context.Context, id int64, actor, note string) error {
	row, err := g.DB().Model("dengbao_finding").Where("id", id).One()
	if err != nil {
		return err
	}
	if row.IsEmpty() {
		return fmt.Errorf("整改任务不存在")
	}
	if row["status"].String() != FindingVerified {
		return fmt.Errorf("仅复测通过（verified）的任务可关闭，当前 %s", row["status"].String())
	}
	if _, err := g.DB().Model("dengbao_finding").Data(g.Map{
		"status": FindingClosed, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		return err
	}
	findingLog(ctx, id, "close", actor, note)
	return nil
}

// FindingList R-C5：整改任务清单
func FindingList(ctx context.Context, status string, limit int) ([]gdb.Record, error) {
	if limit <= 0 {
		limit = 500
	}
	m := g.DB().Model("dengbao_finding")
	if status != "" {
		m = m.Where("status", status)
	}
	return m.Order("id desc").Limit(limit).All()
}

// FindingLog R-C5：整改/复测时间线
func FindingLog(ctx context.Context, findingID int64, limit int) ([]gdb.Record, error) {
	if limit <= 0 {
		limit = 200
	}
	return g.DB().Model("dengbao_finding_log").Where("finding_id", findingID).
		Order("id asc").Limit(limit).All()
}

// FindingStats R-C5：按状态/级别计数（未闭环占比=自查口径的合规缺口）
func FindingStats(ctx context.Context) (map[string]any, error) {
	// 金仓 Oracle 兼容模式下 level 是保留字，列名必须双引号；其他库裸写。
	levelCol := "level"
	if dbutil.IsKingBase() {
		levelCol = `"level"`
	}
	rows, err := g.DB().GetAll(ctx, `
		SELECT status, `+levelCol+`, COUNT(*) AS n FROM dengbao_finding
		GROUP BY status, `+levelCol+` ORDER BY status, `+levelCol+``)
	if err != nil {
		return nil, err
	}
	open := int64(0)
	groups := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		n := r["n"].Int64()
		if r["status"].String() != FindingClosed && r["status"].String() != FindingVerified {
			open += n
		}
		groups = append(groups, map[string]any{
			"status": r["status"].String(), "level": r["level"].String(), "count": n,
		})
	}
	return map[string]any{"open": open, "byGroup": groups}, nil
}

func findingStatusOK(s string) bool {
	switch s {
	case FindingOpen, FindingFixing, FindingVerified, FindingClosed:
		return true
	}
	return false
}

// ---------- R-C8：备份记录证据 ----------

// BackupRecord R-C8：登记一次备份执行记录（DB 备份脚本输出回填）
func BackupRecordAdd(ctx context.Context, source, tool, fileName, checksum, status, note, by string,
	size int64, started, finished time.Time) (int64, error) {
	if source == "" {
		source = "database"
	}
	if status == "" {
		status = "success"
	}
	data := g.Map{
		"source": source, "tool": tool, "file_name": fileName, "size_bytes": size,
		"checksum": checksum, "status": status, "note": note, "creator": by,
		"created_at": time.Now(),
	}
	if !started.IsZero() {
		data["started_at"] = started
	}
	if !finished.IsZero() {
		data["finished_at"] = finished
	}
	res, err := g.DB().Model("dengbao_backup_record").Data(data).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// BackupRecordList R-C8：备份记录清单
func BackupRecordList(ctx context.Context, limit int) ([]gdb.Record, error) {
	if limit <= 0 {
		limit = 200
	}
	return g.DB().Model("dengbao_backup_record").Order("id desc").Limit(limit).All()
}

// BackupStats R-C8：近 N 天备份执行情况（成功/失败次数 + 最近一次成功时间，供测评口径核对）
func BackupStats(ctx context.Context, days int) (map[string]any, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := g.DB().GetAll(ctx, `
		SELECT status, COUNT(*) AS n FROM dengbao_backup_record
		WHERE created_at >= ? GROUP BY status`, since)
	if err != nil {
		return nil, err
	}
	byStatus := make([]map[string]any, 0, len(rows))
	out := map[string]any{"windowDays": days, "byStatus": byStatus, "lastSuccessAt": ""}
	for _, r := range rows {
		byStatus = append(byStatus, map[string]any{
			"status": r["status"].String(), "count": r["n"].Int64(),
		})
	}
	out["byStatus"] = byStatus
	last, err := g.DB().GetValue(ctx, `SELECT MAX(created_at) FROM dengbao_backup_record
		WHERE status='success' AND created_at >= ?`, since)
	if err == nil && last != nil && !last.IsNil() {
		if t := last.Time(); !t.IsZero() {
			out["lastSuccessAt"] = t.Format("2006-01-02 15:04:05")
		}
	}
	return out, nil
}

// ---------- R-C9：证据包（zip + 清单） ----------

// Pack 测评证据包内容：三块证据 + 资产 + 整改 + 备份 + 清单
type Pack struct {
	Files    []EvidenceFile
	Manifest []ManifestEntry
}

// EvidencePack R-C9：按测评机构常见要求打包证据（xlsx 明细 + 清单）
func EvidencePack(ctx context.Context, since string) (Pack, error) {
	p, err := GetPolicy(ctx)
	if err != nil {
		return Pack{}, err
	}
	now := time.Now()
	pack := Pack{}
	add := func(name, block string, columns []evidenceColumn, rows []gdb.Record) {
		data := recordRows(rows, columns)
		pack.Files = append(pack.Files, EvidenceFile{Name: name, CSV: packXLSX(strings.TrimSuffix(name, ".xlsx"), columnsHeader(columns), data)})
		pack.Manifest = append(pack.Manifest, ManifestEntry{
			Name: name, Block: block, Rows: len(data), GeneratedAt: now,
		})
	}
	addRaw := func(name, block string, header []string, rows [][]string) {
		pack.Files = append(pack.Files, EvidenceFile{Name: name, CSV: packXLSX(strings.TrimSuffix(name, ".xlsx"), header, rows)})
		pack.Manifest = append(pack.Manifest, ManifestEntry{
			Name: name, Block: block, Rows: len(rows), GeneratedAt: now,
		})
	}

	// 权限变更证据链（R-C3）
	permRows, err := PermissionChain(ctx, "", since, 5000)
	if err != nil {
		return Pack{}, err
	}
	add("权限变更证据链.xlsx", "权限变更", evidenceColumns, permRows)

	// 三块证据包（R-C4）
	blocks := []struct {
		file string
		b    Block
	}{
		{"身份鉴别证据.xlsx", BlockAuth},
		{"访问控制证据.xlsx", BlockAccess},
		{"安全审计证据.xlsx", BlockAudit},
	}
	for _, b := range blocks {
		rows, err := auditEvidence(ctx, BlockWhere(b.b), nil, "", since, 5000)
		if err != nil {
			return Pack{}, err
		}
		add(b.file, string(b.b), evidenceColumns, rows)
	}

	// 资产台账（R-C1）
	assets, err := AssetList(ctx, "", "")
	if err != nil {
		return Pack{}, err
	}
	add("等保资产台账.xlsx", "资产台账", assetColumns, assets)

	// 整改跟踪（R-C5）
	findings, err := FindingList(ctx, "", 2000)
	if err != nil {
		return Pack{}, err
	}
	add("整改跟踪.xlsx", "整改跟踪", findingColumns, findings)

	// 备份记录（R-C8）
	backups, err := BackupRecordList(ctx, 2000)
	if err != nil {
		return Pack{}, err
	}
	add("备份记录.xlsx", "备份记录", backupColumns, backups)

	// 清单（manifest.xlsx）：留存期限与下限一起写进包内，测评时可直接核对
	sortManifest(pack.Manifest)
	manifestHeader := []string{"文件名", "证据块", "明细行数", "留存期限(天)", "等保下限(天)", "生成时间"}
	manifestRows := make([][]string, 0, len(pack.Manifest))
	for _, m := range pack.Manifest {
		manifestRows = append(manifestRows, []string{
			m.Name, m.Block, fmt.Sprintf("%d", m.Rows),
			fmt.Sprintf("%d", p.RetentionDays), fmt.Sprintf("%d", RetentionFloorDays),
			m.GeneratedAt.Format("2006-01-02 15:04:05"),
		})
	}
	addRaw("清单.xlsx", "清单", manifestHeader, manifestRows)

	return pack, nil
}

// sortManifest 清单按名称稳定排序（同一时刻重复生成时可比对）
func sortManifest(m []ManifestEntry) {
	sort.SliceStable(m, func(i, j int) bool { return m[i].Name < m[j].Name })
}

// generateWorkbook 把二维表渲染为单表 xlsx 字节（证据包 zip 内文件用）。
// 表头一行、首行冻结；无样式（包内取数用，非交付给客户的口径）。
func generateWorkbook(sheet string, headers []string, rows [][]string) ([]byte, error) {
	if sheet == "" {
		sheet = "导出"
	}
	f := excelize.NewFile()
	created, err := f.NewSheet(sheet)
	if err != nil || created == 0 {
		f.SetSheetName("Sheet1", sheet)
	}
	for i, h := range headers {
		col, e := excelize.ColumnNumberToName(i + 1)
		if e != nil {
			continue
		}
		_ = f.SetCellValue(sheet, col+"1", h)
	}
	for ri, row := range rows {
		for ci, v := range row {
			col, e := excelize.ColumnNumberToName(ci + 1)
			if e != nil {
				continue
			}
			_ = f.SetCellValue(sheet, col+strconv.Itoa(ri+2), v)
		}
	}
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
