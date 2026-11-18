package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
)

// dbType 返回当前数据库类型（sqlite / pgsql / mysql ...），用于 SQL 方言适配。
// 优先读配置 Type；Type 为空时从 link 前缀推断（GoFrame 允许只写 link，由驱动推断类型）。
func dbType() string {
	cfg := g.DB().GetConfig()
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t != "" {
		return t
	}
	link := strings.ToLower(strings.TrimSpace(cfg.Link))
	for _, prefix := range []string{"sqlite", "pgsql", "postgres", "mysql", "mssql", "dm", "oracle"} {
		if strings.HasPrefix(link, prefix) {
			return prefix
		}
	}
	return ""
}

// 以下正则用于 sqlite 方言化：把 pg 专有 DDL 收敛成 sqlite 可执行的等价形式。
var (
	// pg 匿名函数块（DO $$ ... $$;）—— sqlite 无 PL/pgSQL，整体删除。
	// 本项目唯一用处是「放宽 vector 列类型」，sqlite 下 embedding 是 TEXT 列，无需该操作。
	reDOBlock = regexp.MustCompile(`(?is)DO\s+\$\$.*?\$\$\s*;`)
	// pgvector 扩展启用语句 —— sqlite 无 pgvector，删除（列类型已降为 TEXT）。
	reCreateExt = regexp.MustCompile(`(?is)CREATE\s+EXTENSION\s+IF\s+NOT\s+EXISTS\s+[a-zA-Z0-9_]+;`)
	// 序列主键：sqlite 下 INTEGER PRIMARY KEY 即 rowid 别名，自动自增，无需 AUTOINCREMENT。
	reBigSerial = regexp.MustCompile(`(?i)\bBIGSERIAL\b`)
	reSerial    = regexp.MustCompile(`(?i)\bSERIAL\b`)
	// ALTER TABLE ADD COLUMN 的 IF NOT EXISTS 是 pg 语法，sqlite 不支持；
	// 迁移由 schema_migrations 记录只执行一次，sqlite 下裸 ADD COLUMN 即可。
	reAddColIfNotExists = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b`)
	// pgvector：向量列类型 vector(1024) 与 ivfflat/hnsw 索引是 pg 专有，sqlite 无对应能力。
	// 向量列降为 TEXT（存 JSON 文本，代码侧按字符串读写）；向量索引整条删除（sqlite 不做近似检索）。
	reVectorType  = regexp.MustCompile(`(?i)\bvector\(\s*\d+\s*\)`)
	reVectorIndex = regexp.MustCompile(`(?is)CREATE\s+INDEX[^\n;]*USING\s+(?:ivfflat|hnsw)[^\n;]*;`)
	// ALTER TABLE <t> ADD COLUMN <c>：SQLite 无 IF NOT EXISTS，用"列已存在则跳过"补齐 pg 的幂等语义。
	reAlterAddCol = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+([a-zA-Z_]\w*)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z_]\w*)`)
	// jsonb 列：sqlite 以 TEXT 存储 JSON 文本（本项目业务代码均以字符串读写，无 jsonb 操作符依赖）。
	reJSONB = regexp.MustCompile(`(?i)\bJSONB\b`)
)

// 以下正则用于 mysql 方言化：把 pg/sqlite 通用脚本收敛为 MySQL 可执行形式。
var (
	// 自增主键：pg 的 BIGSERIAL/SERIAL → MySQL AUTO_INCREMENT。
	reMySQLBigSerial = regexp.MustCompile(`(?i)\bBIGSERIAL\b`)
	reMySQLSerial    = regexp.MustCompile(`(?i)\bSERIAL\b`)
	// MySQL 的 JSON 列不允许字面量 DEFAULT（8.0.13 前），收敛为可空 JSON（业务读取走 .String()，NULL→"" 已兜底）。
	reMySQLJSONBDefault = regexp.MustCompile(`(?i)\bJSONB\s+(?:NOT\s+NULL\s+)?DEFAULT\s+'[^']*'`)
	reMySQLJSONB        = regexp.MustCompile(`(?i)\bJSONB\b`)
	// MySQL 的 TEXT 列不允许字面量 DEFAULT，收敛为可空 TEXT。
	// NOT NULL 可有可无（如 comment TEXT DEFAULT ''），都要去掉 DEFAULT。
	reMySQLTextDefault = regexp.MustCompile(`(?i)\bTEXT\s+(?:NOT\s+NULL\s+)?DEFAULT\s+'[^']*'`)
	// MySQL 8.0 的 CREATE [UNIQUE] INDEX 不支持 IF NOT EXISTS（幂等由 schema_migrations 保证）。
	reMySQLCreateIndexIFNE = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\b`)
	// MySQL 无 ON CONFLICT，seed INSERT 收敛为 ON DUPLICATE KEY UPDATE <冲突列>=<冲突列>（no-op）。
	reMySQLOnConflict = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+NOTHING`)
	// PG upsert 的 DO UPDATE SET 形式 → MySQL ON DUPLICATE KEY UPDATE；EXCLUDED.col → VALUES(col)。
	reMySQLOnConflictUpdate = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\([^)]+\)\s+DO\s+UPDATE\s+SET`)
	reMySQLExcluded         = regexp.MustCompile(`(?i)\bEXCLUDED\.([a-zA-Z_]\w*)`)
	// MySQL 8.0 的 ALTER TABLE ADD COLUMN 不支持 IF NOT EXISTS，执行层用 information_schema 列存在性检查补齐。
	reMySQLAddColIFNE = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b`)
	// MySQL 不支持部分索引（CREATE INDEX ... WHERE cond）：去掉 WHERE 后缀退化为全列索引。
	// 涉及列（invoice_no/code/external_id 等）业务上唯一性针对有值记录，空值场景极少；语义略宽松但可用。
	reMySQLPartialIndex = regexp.MustCompile(`(?is)^(CREATE\s+(UNIQUE\s+)?INDEX\s+[\s\S]+?)\s+WHERE\s+[\s\S]+?$`)
	// MySQL 保留字作列名需加反引号：trigger（触发器）在 archive_schedule_run.trigger 使用。
	reMySQLReservedCol = regexp.MustCompile(`(?im)^([ \t]*)(trigger|groups)(\s+(?:VARCHAR|INT|BIGINT|TEXT|BOOLEAN|TIMESTAMP|JSON))`)
	// MySQL 无 pgvector 索引：DROP INDEX IF EXISTS <向量索引> 在 MySQL 下语法不同（需带 ON table），
	// 且 MySQL 方言下这些索引本就不存在，整条删除。
	reMySQLDropVectorIndex = regexp.MustCompile(`(?is)\bDROP\s+INDEX\s+IF\s+EXISTS\s+(idx_kb_clause_embedding|idx_\w+_(?:ivfflat|hnsw))\s*;?`)
	// 纯注释语句（-- 开头整行）：DO 块/向量索引删除后可能只剩注释，无需执行。
	reMySQLCommentLine = regexp.MustCompile(`(?m)^\s*--[^\n]*$`)
)

// splitStatements 按分号把脚本拆成可独立执行的语句，正确跳过单引号字符串（含 两个连续单引号 转义）
// 与 $$ ... $$ 美元引用块（pg 函数体/字符串）内的分号，避免误切。
func splitStatements(sql string) []string {
	var stmts []string
	var cur strings.Builder
	i, n := 0, len(sql)
	for i < n {
		c := sql[i]
		switch c {
		case '\'':
			cur.WriteByte(c)
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						cur.WriteString("''")
						i += 2
						continue
					}
					cur.WriteByte('\'')
					i++
					break
				}
				cur.WriteByte(sql[i])
				i++
			}
		case '$':
			if i+1 < n && sql[i+1] == '$' {
				cur.WriteString("$$")
				i += 2
				if end := strings.Index(sql[i:], "$$"); end < 0 {
					cur.WriteString(sql[i:])
					i = n
				} else {
					cur.WriteString(sql[i : i+end+2])
					i += end + 2
				}
			} else {
				cur.WriteByte(c)
				i++
			}
		case ';':
			if s := strings.TrimSpace(cur.String()); s != "" {
				stmts = append(stmts, s)
			}
			cur.Reset()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	if tail := strings.TrimSpace(cur.String()); tail != "" {
		stmts = append(stmts, tail)
	}
	return stmts
}

// dialectize 按方言处理迁移脚本，返回可逐条执行的语句列表。
// pgsql 走原样（保留 DO 块 / CREATE EXTENSION / BIGSERIAL / JSONB）；
// sqlite 收敛上述 pg 专有语法；mysql 收敛为 MySQL 方言（AUTO_INCREMENT / JSON / 无 IF NOT EXISTS / ON DUPLICATE KEY）。
// 迁移脚本本身已是多方言通用的语法（CURRENT_TIMESTAMP、单引号字符串）。
func dialectize(sql string, dialect string) []string {
	switch dialect {
	case "sqlite":
		sql = reDOBlock.ReplaceAllString(sql, "")
		sql = reCreateExt.ReplaceAllString(sql, "")
		sql = reBigSerial.ReplaceAllString(sql, "INTEGER")
		sql = reSerial.ReplaceAllString(sql, "INTEGER")
		sql = reJSONB.ReplaceAllString(sql, "TEXT")
		sql = reAddColIfNotExists.ReplaceAllString(sql, "ADD COLUMN")
		sql = reVectorIndex.ReplaceAllString(sql, "") // 先删向量索引，避免下面把列型改成 TEXT 后残留 pg 索引语法
		sql = reVectorType.ReplaceAllString(sql, "TEXT")
	case "mysql":
		sql = reDOBlock.ReplaceAllString(sql, "")
		sql = reCreateExt.ReplaceAllString(sql, "")
		sql = reVectorIndex.ReplaceAllString(sql, "")
		sql = reVectorType.ReplaceAllString(sql, "TEXT")
		// 先处理带 DEFAULT 的 JSONB（避免先替换列型后残留 DEFAULT 导致 JSON 列 DEFAULT 语法错误）
		sql = reMySQLJSONBDefault.ReplaceAllString(sql, "JSON")
		sql = reMySQLJSONB.ReplaceAllString(sql, "JSON")
		sql = reMySQLTextDefault.ReplaceAllString(sql, "TEXT")
		sql = reMySQLBigSerial.ReplaceAllString(sql, "BIGINT AUTO_INCREMENT")
		sql = reMySQLSerial.ReplaceAllString(sql, "INT AUTO_INCREMENT")
		sql = reMySQLCreateIndexIFNE.ReplaceAllString(sql, "CREATE ${1}INDEX")
		sql = reMySQLAddColIFNE.ReplaceAllString(sql, "ADD COLUMN")
		sql = reMySQLReservedCol.ReplaceAllString(sql, "${1}`${2}`${3}")
		sql = reMySQLOnConflict.ReplaceAllStringFunc(sql, func(m string) string {
			cols := reMySQLOnConflict.FindStringSubmatch(m)[1]
			var assigns []string
			for _, c := range strings.Split(cols, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					assigns = append(assigns, c+" = "+c)
				}
			}
			return "ON DUPLICATE KEY UPDATE " + strings.Join(assigns, ", ")
		})
		// DO UPDATE SET 形式：先替换引导词，再把 EXCLUDED.col 转为 VALUES(col)
		sql = reMySQLOnConflictUpdate.ReplaceAllString(sql, "ON DUPLICATE KEY UPDATE")
		sql = reMySQLExcluded.ReplaceAllString(sql, "VALUES(${1})")
	}
	stmts := splitStatements(sql)
	if dialect == "mysql" {
		// 部分索引必须逐语句处理（CREATE INDEX ... WHERE 的 WHERE 在语句级后缀）。
		filtered := stmts[:0]
		for _, st := range stmts {
			st = reMySQLDropVectorIndex.ReplaceAllString(st, "")
			// 去掉行注释后若为空语句（DO 块/DROP 索引删除后仅剩注释），跳过。
			withoutComments := reMySQLCommentLine.ReplaceAllString(st, "")
			if strings.TrimSpace(withoutComments) == "" {
				continue
			}
			filtered = append(filtered, reMySQLPartialIndex.ReplaceAllString(st, "${1}"))
		}
		stmts = filtered
	}
	return stmts
}

// sqliteHasColumn 用 SQLite 的 pragma_table_info 表值函数查某表是否已有该列
// （仅 SQLite 分支调用；PostgreSQL 走原生的 ADD COLUMN IF NOT EXISTS）。
func sqliteHasColumn(ctx context.Context, table, col string) bool {
	rows, err := g.DB().GetAll(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return false
	}
	for _, r := range rows {
		if r["name"].String() == col {
			return true
		}
	}
	return false
}

// mysqlHasColumn 用 information_schema 查某表是否已有该列（MySQL 8.0 的 ADD COLUMN 不支持 IF NOT EXISTS，
// 用列存在性检查补齐 pg 的幂等语义；MariaDB/TiDB/OceanBase 的 information_schema 结构一致）。
func mysqlHasColumn(ctx context.Context, table, col string) bool {
	rows, err := g.DB().GetAll(ctx,
		"SELECT COUNT(*) AS n FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?",
		table, col)
	if err != nil {
		return false
	}
	return len(rows) > 0 && rows[0]["n"].Int64() > 0
}

// RunMigrations 启动期自动迁移：按 migrations/*.sql 文件名顺序执行，schema_migrations 表记录已执行文件。
// 所有迁移必须写成幂等（CREATE ... IF NOT EXISTS / ON CONFLICT DO NOTHING），单二进制自托管分发时
// 用户不需要手工 docker cp 执行 SQL——这是「当收费版做好」的基本盘。
// 兼容 PostgreSQL 与 SQLite：方言化 + 按分号拆分逐条执行（不依赖驱动层多语句支持）。
func RunMigrations() error {
	// 总超时：SSH 隧道半死时 DB 请求会挂住而不是报错，启动时不能被迁移拖死。
	// 120s：经 SSH 隧道跑 27+ 个迁移 + 建新表，20s 不够（隧道单请求延迟大），放宽到 120s。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	isSQLite := dbType() == "sqlite"
	isMySQL := dbType() == "mysql"
	dialect := dbType()
	if _, err := g.DB().Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		file VARCHAR(200) PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}
	entries, err := os.ReadDir("migrations")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		done, qerr := g.DB().Model("schema_migrations").Where("file", name).One()
		if qerr != nil {
			return fmt.Errorf("check migration %s: %w", name, qerr)
		}
		if !done.IsEmpty() {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join("migrations", name))
		if rerr != nil {
			return rerr
		}
		for idx, stmt := range dialectize(string(body), dialect) {
			// SQLite 的 ALTER TABLE ADD COLUMN 无 IF NOT EXISTS：列已存在则跳过，等价于 pg 的幂等语义。
			if isSQLite {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && sqliteHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			// MySQL 8.0 的 ADD COLUMN 同样无 IF NOT EXISTS：用 information_schema 列存在性检查补齐。
			if isMySQL {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && mysqlHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			if _, xerr := g.DB().Exec(ctx, stmt); xerr != nil {
				return fmt.Errorf("apply migration %s (stmt #%d): %w", name, idx+1, xerr)
			}
		}
		if _, ierr := g.DB().Model("schema_migrations").Data(g.Map{"file": name}).Insert(); ierr != nil {
			return fmt.Errorf("record migration %s: %w", name, ierr)
		}
		glog.Infof(ctx, "[migrate] applied %s", name)
	}
	return nil
}
