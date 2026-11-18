package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"

	"vouchchain/pkg/dbutil"
)

// dbType 返回当前数据库方言标识（sqlite / pgsql / opengauss / kingbase / dm / mysql ...）。
// 优先读配置 Type；Type 为空时从 link 前缀推断。PG 家族连上后按 version() 细分为 pg/opengauss/kingbase。
func dbType() string {
	cfg := g.DB().GetConfig()
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t == "" {
		link := strings.ToLower(strings.TrimSpace(cfg.Link))
		for _, prefix := range []string{"sqlite", "pgsql", "postgres", "mysql", "mssql", "dm", "oracle"} {
			if strings.HasPrefix(link, prefix) {
				t = prefix
				break
			}
		}
	}
	if t == "opengauss" || t == "kingbase" {
		return t
	}
	if t == "pgsql" || t == "postgres" {
		if dbutil.IsOpenGauss() {
			return "opengauss"
		}
		if dbutil.IsKingBase() {
			return "kingbase"
		}
		return "pgsql"
	}
	return t
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
	reMySQLPartialIndex = regexp.MustCompile(`(?is)^(CREATE\s+(UNIQUE\s+)?INDEX\s+[\s\S]+?)\s+WHERE\s+[\s\S]+?$`)
	// MySQL 保留字作列名需加反引号：trigger（触发器）在 archive_schedule_run.trigger 使用。
	reMySQLReservedCol = regexp.MustCompile(`(?im)^([ \t]*)(trigger|groups)(\s+(?:VARCHAR|INT|BIGINT|TEXT|BOOLEAN|TIMESTAMP|JSON))`)
	// MySQL 无 pgvector 索引：DROP INDEX IF EXISTS <向量索引> 在 MySQL 下语法不同（需带 ON table），整条删除。
	reMySQLDropVectorIndex = regexp.MustCompile(`(?is)\bDROP\s+INDEX\s+IF\s+EXISTS\s+(idx_kb_clause_embedding|idx_\w+_(?:ivfflat|hnsw))\s*;?`)
	// 纯注释语句（-- 开头整行）：DO 块/向量索引删除后可能只剩注释，无需执行。
	reMySQLCommentLine = regexp.MustCompile(`(?m)^\s*--[^\n]*$`)
)

// openGauss 5.x 方言差异（PG 内核但裁剪/改造了若干语法）：
//   - 无 pgvector：CREATE EXTENSION vector / vector(N) 列 / ivfflat-hnsw 索引 / ::vector cast 全部不可用；
//   - 无 ON CONFLICT：upsert 只能走 ON DUPLICATE KEY UPDATE（MySQL 兼容语法，openGauss 支持）；
//   - 无 ADD COLUMN IF NOT EXISTS：需按列存在性检查后裸 ADD COLUMN；
//   - DO $$ 匿名块、BIGSERIAL、JSONB、部分索引、正则 ~ 均支持，与 PG 一致。
var (
	// ON CONFLICT (cols) DO NOTHING → ON DUPLICATE KEY UPDATE <非键列>=VALUES(<非键列>)。
	reOGOnConflictNothing = regexp.MustCompile(`(?is)^([\s\S]*?)\s+ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+NOTHING\s*$`)
	// ON CONFLICT (cols) DO UPDATE SET → ON DUPLICATE KEY UPDATE；EXCLUDED.x → VALUES(x)。
	reOGOnConflictUpdate = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\([^)]+\)\s+DO\s+UPDATE\s+SET`)
	reOGExcluded         = regexp.MustCompile(`(?i)\bEXCLUDED\.([a-zA-Z_]\w*)`)
	// openGauss 列存在性检查：information_schema 的 table_schema 需按 current_schema() 取，不能用 current_database()。
	reOGAddColIFNE = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b`)
	// INSERT INTO <table>(cols) VALUES (...)：提取列名列表，挑选非冲突列做 no-op。
	reOGInsertCols = regexp.MustCompile(`(?is)^INSERT\s+INTO\s+([a-zA-Z_]\w*)\s*\(([^)]+)\)\s+VALUES\s+([\s\S]+)$`)
)

// 人大金仓 KingBase V8（PG 内核）：语法最接近原生 PG——
//   - 支持 BIGSERIAL / JSONB / ON CONFLICT / ADD COLUMN IF NOT EXISTS / 部分索引 / 正则 ~ / DO 块；
//   - 唯一实测差异：无 pgvector（CREATE EXTENSION vector / vector(N) / ::vector 不可用）。
// 因此 dialectize 只需裁剪 pgvector 相关，其余原样走 PG 方言。

// 达梦 DM8（Oracle 系方言，GoFrame dm 驱动）实测差异：
//   - 无 JSONB（→ TEXT/CLOB）、无 BIGSERIAL（→ BIGINT IDENTITY(1,1)）、无 vector、无 DO $$、无 CREATE EXTENSION；
//   - ADD COLUMN 不支持 IF NOT EXISTS（裸 ADD + 执行层 USER_TAB_COLUMNS 列检查）；
//   - CREATE [UNIQUE] INDEX IF NOT EXISTS 支持；但部分索引 WHERE 不支持（去掉 WHERE）；
//   - 无 ON CONFLICT / ON DUPLICATE KEY：upsert 只能 MERGE INTO；
//   - 列存在性查 USER_TAB_COLUMNS（无 information_schema）。
var (
	reDMJSONBDefault = regexp.MustCompile(`(?i)\bJSONB\s+(?:NOT\s+NULL\s+)?DEFAULT\s+'[^']*'`)
	reDMJSONB        = regexp.MustCompile(`(?i)\bJSONB\b`)
	reDMBigSerial    = regexp.MustCompile(`(?i)\bBIGSERIAL\b`)
	reDMSerial       = regexp.MustCompile(`(?i)\bSERIAL\b`)
	reDMAddColIFNE   = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b`)
	// 达梦无 BOOLEAN 类型（BIT 替代），无 TRUE/FALSE 字面量（1/0）。
	reDMBoolean = regexp.MustCompile(`(?i)\bBOOLEAN\b`)
	// 达梦保留字作列名需双引号：comment / level 等。
	reDMReservedCol = regexp.MustCompile(`(?im)^([ \t]*)(comment|level|context|source|owner|type|name|status|mode|key|value|trigger|groups|desc|when|current|table|index|order|group|user|password|date|time|year|month|day|domain|item|result|block|parameter|session|resource|start|end|check|asc|desc|null|from|to|as|is|and|or|not|set|values|into|unique|primary|foreign|references|default|constraint|cascade|restrict|action|grant|revoke|role)(\s+(?:TEXT|CLOB|VARCHAR|INT|BIGINT|BOOLEAN|BIT|TIMESTAMP|DECIMAL|NUMERIC))`)
	// ALTER ADD COLUMN 的保留字列名（不受行首锚点约束）。
	reDMReservedWord = regexp.MustCompile(`(?i)^(comment|level|context|source|owner|type|name|status|mode|key|value|trigger|groups|desc|when|current|table|index|order|group|user|password|date|time|year|month|day|domain|item|result|block|parameter|session|resource|start|end|check|asc|desc|null|from|to|as|is|and|or|not|set|values|into|unique|primary|foreign|references|default|constraint|cascade|restrict|action|grant|revoke|role)$`)
	reDMAddReserved  = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+(comment|level|context|source|owner|type|name|status|mode|key|value|trigger|groups|desc|when|current|table|index|order|group|user|password|date|time|year|month|day|domain|item|result|block|parameter|session|resource|start|end|check|asc|desc|null|from|to|as|is|and|or|not|set|values|into|unique|primary|foreign|references|default|constraint|cascade|restrict|action|grant|revoke|role)\b`)
	// CREATE INDEX ... ON table(cols) 的列名括号（达梦保留字需引号）。
	reDMIndexCols    = regexp.MustCompile(`(?i)\bON\s+[a-zA-Z_]\w*\s*\(([^)]+)\)`)
	reDMDefaultTF    = regexp.MustCompile(`(?i)\bDEFAULT\s+(TRUE|FALSE)\b`)
	reDMPartialIndex = regexp.MustCompile(`(?is)^(CREATE\s+(UNIQUE\s+)?INDEX\s+[\s\S]+?)\s+WHERE\s+[\s\S]+?$`)
	// ON CONFLICT (...) DO UPDATE SET ... 需整段改写为 MERGE INTO，单独处理（见 dmRewriteUpsert）。
	reDMOnConflictUpdate = regexp.MustCompile(`(?is)^([\s\S]*?)\s+ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+UPDATE\s+SET\s+([\s\S]+)$`)
	// DO NOTHING 形式：INSERT INTO t(cols) VALUES (...) ON CONFLICT (keys) DO NOTHING。
	reDMOnConflictNothing = regexp.MustCompile(`(?is)^([\s\S]*?)\s+ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+NOTHING\s*$`)
	reDMInsertCols        = regexp.MustCompile(`(?is)^INSERT\s+INTO\s+([a-zA-Z_]\w*)\s*\(([^)]+)\)\s+VALUES\s*\(([\s\S]+)\)\s*$`)

	// 金仓 Oracle 兼容模式下 LEVEL/COMMENT/CONTEXT/TRIGGER/DOMAIN/GROUPS 是保留字，
	// 出现在 INSERT 列清单 / ON CONFLICT 列清单 / SET 左侧 / 列定义行首时必须双引号。
	reKBReserved     = regexp.MustCompile(`(?i)\b(level|comment|context|trigger|domain|groups)\b`)
	reKBInsertCols   = regexp.MustCompile(`(?is)(INSERT\s+INTO\s+[a-zA-Z_]\w*\s*\()([^)]+)(\))`)
	reKBConflictCols = regexp.MustCompile(`(?is)(ON\s+CONFLICT\s*\()([^)]+)(\))`)
	reKBSetReserved  = regexp.MustCompile(`(?i)(\bSET\s+)(level|comment|context|trigger|domain|groups)\b`)
	reKBDefReserved  = regexp.MustCompile(`(?im)^([ \t]*)(level|comment|context|trigger|domain|groups)(\s+(?:TEXT|CLOB|VARCHAR|INT|BIGINT|BOOLEAN|BIT|TIMESTAMP|DECIMAL|NUMERIC))`)
)

// kbQuoteCols 给 INSERT INTO t(...) / ON CONFLICT (...) 中的保留字列名加双引号。
func kbQuoteCols(match string) string {
	l := strings.Index(match, "(")
	r := strings.LastIndex(match, ")")
	if l < 0 || r < l {
		return match
	}
	return match[:l+1] + reKBReserved.ReplaceAllString(match[l+1:r], `"$1"`) + match[r:]
}

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
// pgsql/kingbase 走原样（保留 DO 块 / CREATE EXTENSION / BIGSERIAL / JSONB；kingbase 仅裁 pgvector）；
// sqlite 收敛 pg 专有语法；mysql/openGauss/dm 各自收敛为对应方言。
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
	case "kingbase":
		// 仅裁 pgvector：DO 块/扩展/向量索引/向量列；其余 PG 语法原样支持。
		sql = reDOBlock.ReplaceAllString(sql, "")
		sql = reCreateExt.ReplaceAllString(sql, "")
		sql = reVectorIndex.ReplaceAllString(sql, "")
		sql = reVectorType.ReplaceAllString(sql, "TEXT")
		// Oracle 兼容模式保留字：INSERT/ON CONFLICT 列清单、SET 左侧、列定义行首加双引号。
		sql = reKBInsertCols.ReplaceAllStringFunc(sql, kbQuoteCols)
		sql = reKBConflictCols.ReplaceAllStringFunc(sql, kbQuoteCols)
		sql = reKBSetReserved.ReplaceAllString(sql, `${1}"${2}"`)
		sql = reKBDefReserved.ReplaceAllString(sql, `${1}"${2}"${3}`)
	case "opengauss":
		// 无 pgvector：与 sqlite/mysql 同样降级（列型 TEXT、删向量索引/扩展/DO 放宽块）。
		sql = reDOBlock.ReplaceAllString(sql, "")
		sql = reCreateExt.ReplaceAllString(sql, "")
		sql = reVectorIndex.ReplaceAllString(sql, "")
		sql = reVectorType.ReplaceAllString(sql, "TEXT")
		// 无 ADD COLUMN IF NOT EXISTS → 裸 ADD COLUMN（执行层用 ogHasColumn 补齐幂等）。
		sql = reOGAddColIFNE.ReplaceAllString(sql, "ADD COLUMN")
		// upsert：DO UPDATE SET → ON DUPLICATE KEY UPDATE，EXCLUDED.x → VALUES(x)。
		sql = reOGOnConflictUpdate.ReplaceAllString(sql, "ON DUPLICATE KEY UPDATE")
		sql = reOGExcluded.ReplaceAllString(sql, "VALUES(${1})")
		// DO NOTHING → ON DUPLICATE KEY UPDATE <非键列>=VALUES(<非键列>)（逐语句处理，需解析列）。
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
	case "dm":
		// 达梦（Oracle 系）：裁剪 pg 专有；BIGSERIAL→IDENTITY、JSONB→TEXT、upsert→MERGE INTO。
		sql = reDOBlock.ReplaceAllString(sql, "")
		sql = reCreateExt.ReplaceAllString(sql, "")
		sql = reVectorIndex.ReplaceAllString(sql, "")
		sql = reVectorType.ReplaceAllString(sql, "TEXT")
		sql = reDMJSONBDefault.ReplaceAllString(sql, "TEXT")
		sql = reDMJSONB.ReplaceAllString(sql, "TEXT")
		sql = reDMBigSerial.ReplaceAllString(sql, "BIGINT IDENTITY(1,1)")
		sql = reDMSerial.ReplaceAllString(sql, "INT IDENTITY(1,1)")
		sql = reDMAddColIFNE.ReplaceAllString(sql, "ADD COLUMN")
		// BOOLEAN → BIT；DEFAULT TRUE/FALSE → 1/0（达梦无布尔类型与字面量）。
		sql = reDMDefaultTF.ReplaceAllStringFunc(sql, func(m string) string {
			if strings.Contains(strings.ToLower(m), "true") {
				return "DEFAULT 1"
			}
			return "DEFAULT 0"
		})
		// INSERT VALUES 里的 TRUE/FALSE 字面量（达梦 BIT 列用 1/0）；跳过单引号字符串内部。
		sql = dmReplaceBoolLiterals(sql)
		sql = reDMBoolean.ReplaceAllString(sql, "BIT")
		sql = reDMReservedCol.ReplaceAllString(sql, `${1}"${2}"${3}`)
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
	if dialect == "opengauss" {
		filtered := stmts[:0]
		for _, st := range stmts {
			// 去掉行注释后为空的空语句跳过（DO 块/向量扩展删除后仅剩注释）。
			if strings.TrimSpace(reMySQLCommentLine.ReplaceAllString(st, "")) == "" {
				continue
			}
			rewritten := ogRewriteConflictNothing(st)
			if rewritten != "" {
				filtered = append(filtered, rewritten)
			}
		}
		stmts = filtered
	}
	if dialect == "dm" {
		filtered := stmts[:0]
		for _, st := range stmts {
			if strings.TrimSpace(reMySQLCommentLine.ReplaceAllString(st, "")) == "" {
				continue
			}
			rewritten := dmRewrite(st)
			if rewritten != "" {
				filtered = append(filtered, rewritten)
			}
		}
		stmts = filtered
	}
	return stmts
}

// ogRewriteConflictNothing 把 openGauss 的 INSERT ... ON CONFLICT (cols) DO NOTHING
// 改写为 ON DUPLICATE KEY UPDATE <非键列>=VALUES(<非键列>)。
// openGauss 不允许 UPDATE 主键/唯一键，所以从 INSERT 列里挑一个不在冲突列集合中的列做 no-op；
// 找不到（极端：INSERT 列全是键）时返回空串由调用方跳过——seed 已存在即幂等，跳过不影响语义。
func ogRewriteConflictNothing(stmt string) string {
	// 剥掉行注释：迁移脚本常在 INSERT 前留 -- 注释，锚点 ^INSERT 需从真实语句开始。
	stmt = reMySQLCommentLine.ReplaceAllString(stmt, "")
	stmt = strings.TrimSpace(stmt)
	m := reOGOnConflictNothing.FindStringSubmatch(stmt)
	if m == nil {
		return stmt
	}
	insertPart, conflictCols := m[1], m[2]
	keySet := make(map[string]bool)
	for _, c := range strings.Split(conflictCols, ",") {
		keySet[strings.ToLower(strings.TrimSpace(c))] = true
	}
	im := reOGInsertCols.FindStringSubmatch(insertPart)
	if im == nil {
		return "" // 无法解析列（如多 Values 的复杂形式），跳过该 seed
	}
	for _, c := range strings.Split(im[2], ",") {
		col := strings.TrimSpace(c)
		if col != "" && !keySet[strings.ToLower(col)] {
			return insertPart + " ON DUPLICATE KEY UPDATE " + col + " = VALUES(" + col + ")"
		}
	}
	return ""
}

// dmRewrite 逐语句处理达梦方言：部分索引去 WHERE；upsert（DO NOTHING / DO UPDATE）改 MERGE INTO。
func dmRewrite(stmt string) string {
	stmt = reMySQLCommentLine.ReplaceAllString(stmt, "")
	stmt = strings.TrimSpace(stmt)
	// 部分索引：去掉 WHERE 后缀（达梦不支持）；索引列名里的保留字加双引号。
	if strings.HasPrefix(strings.ToUpper(stmt), "CREATE INDEX") || strings.HasPrefix(strings.ToUpper(stmt), "CREATE UNIQUE INDEX") {
		stmt = reDMPartialIndex.ReplaceAllString(stmt, "${1}")
		stmt = reDMIndexCols.ReplaceAllStringFunc(stmt, func(m string) string {
			mm := reDMIndexCols.FindStringSubmatch(m)
			if len(mm) < 2 {
				return m
			}
			inner := mm[1]
			var quoted []string
			for _, c := range strings.Split(inner, ",") {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				// 去掉 DESC/ASC 后缀再判断保留字
				word := strings.Fields(c)
				if len(word) > 0 && reDMReservedWord.MatchString(word[0]) {
					word[0] = `"` + word[0] + `"`
					c = strings.Join(word, " ")
				}
				quoted = append(quoted, c)
			}
			return strings.TrimSuffix(m, "("+inner+")") + "(" + strings.Join(quoted, ",") + ")"
		})
	}
	// ALTER TABLE ... ADD COLUMN <保留字>：达梦保留字列名需双引号（行首锚点不覆盖此场景）。
	if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && reDMReservedWord.MatchString(m[2]) {
		stmt = reDMAddReserved.ReplaceAllString(stmt, `ADD COLUMN "$1"`)
	}
	// DO UPDATE SET → MERGE INTO（带 EXCLUDED 已在上层转为 VALUES()）。
	if m := reDMOnConflictUpdate.FindStringSubmatch(stmt); m != nil {
		assigns := reOGExcluded.ReplaceAllString(m[3], "VALUES(${1})")
		return dmBuildMerge(m[1], m[2], assigns)
	}
	// DO NOTHING → MERGE INTO（no-op：命中则更新非键列自身）。
	if m := reDMOnConflictNothing.FindStringSubmatch(stmt); m != nil {
		return dmBuildMergeNoop(m[1], m[2])
	}
	return stmt
}

// dmBuildMerge 把 INSERT ... ON CONFLICT ... DO UPDATE SET ... 改写为 MERGE INTO。
// USING 侧用 INSERT 的 VALUES 构造 SELECT（达梦需 FROM DUAL），ON 用冲突键，命中走 UPDATE，未命中走 INSERT。
func dmBuildMerge(insertPart, conflictCols, assignments string) string {
	im := reDMInsertCols.FindStringSubmatch(insertPart)
	if im == nil {
		return ""
	}
	table, colsRaw, valuesRaw := im[1], im[2], im[3]
	cols := splitCSV(colsRaw)
	vals := splitTopLevel(valuesRaw, ',')
	if len(cols) == 0 || len(cols) != len(vals) {
		return ""
	}
	keys := splitCSV(conflictCols)
	usingParts := make([]string, 0, len(cols))
	for i, c := range cols {
		usingParts = append(usingParts, vals[i]+" AS "+c)
	}
	// VALUES(col) → s.col（MERGE 的 USING 别名）；上层已把 EXCLUDED.col 转成 VALUES(col)。
	assigns := reDMValuesToS.ReplaceAllString(assignments, "s.$1")
	onParts := make([]string, 0, len(keys))
	for _, k := range keys {
		onParts = append(onParts, "t."+k+"=s."+k)
	}
	insertCols := make([]string, 0, len(cols))
	for _, c := range cols {
		if !strings.EqualFold(c, "id") {
			insertCols = append(insertCols, c)
		}
	}
	if len(insertCols) == 0 {
		return ""
	}
	return fmt.Sprintf("MERGE INTO %s t USING (SELECT %s FROM DUAL) s ON (%s) WHEN MATCHED THEN UPDATE SET %s WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		table, strings.Join(usingParts, ", "), strings.Join(onParts, " AND "), assigns,
		strings.Join(insertCols, ","), "s."+strings.Join(insertCols, ",s."))
}

// dmBuildMergeNoop 把 INSERT ... ON CONFLICT (keys) DO NOTHING 改写为 MERGE INTO。
// 支持多行 VALUES（UNION ALL 构造 USING），命中则非键列自更新 no-op。
func dmBuildMergeNoop(insertPart, conflictCols string) string {
	im := reDMInsertCols.FindStringSubmatch(insertPart)
	if im == nil {
		return ""
	}
	table, colsRaw, valuesRaw := im[1], im[2], im[3]
	cols := splitCSV(colsRaw)
	rows := splitDMValueRows(valuesRaw)
	if len(cols) == 0 || len(rows) == 0 {
		return ""
	}
	for _, r := range rows {
		if len(r) != len(cols) {
			return ""
		}
	}
	keySet := make(map[string]bool)
	for _, k := range splitCSV(conflictCols) {
		keySet[strings.ToLower(k)] = true
	}
	var noopCol string
	for _, c := range cols {
		if !keySet[strings.ToLower(c)] {
			noopCol = c
			break
		}
	}
	if noopCol == "" {
		return ""
	}
	keys := splitCSV(conflictCols)
	// USING 侧：每行一个 SELECT ... FROM DUAL，多行 UNION ALL。
	var selects []string
	for _, r := range rows {
		parts := make([]string, 0, len(cols))
		for i, c := range cols {
			parts = append(parts, r[i]+" AS "+c)
		}
		selects = append(selects, "SELECT "+strings.Join(parts, ", ")+" FROM DUAL")
	}
	onParts := make([]string, 0, len(keys))
	for _, k := range keys {
		onParts = append(onParts, "t."+k+"=s."+k)
	}
	// 达梦 IDENTITY 自增列（id）不能在 INSERT 显式赋值，去掉（自增即得 seed 的 1）。
	insertCols := make([]string, 0, len(cols))
	for _, c := range cols {
		if !strings.EqualFold(c, "id") {
			insertCols = append(insertCols, c)
		}
	}
	if len(insertCols) == 0 {
		return ""
	}
	return fmt.Sprintf("MERGE INTO %s t USING (%s) s ON (%s) WHEN MATCHED THEN UPDATE SET t.%s=s.%s WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		table, strings.Join(selects, " UNION ALL "), strings.Join(onParts, " AND "), noopCol, noopCol,
		strings.Join(insertCols, ","), "s."+strings.Join(insertCols, ",s."))
}

// splitDMValueRows 把 INSERT 的 VALUES 部分拆成 [][]string。
// 注意入参是 reDMInsertCols 的第 3 组：外层括号已被剥掉，格式为 `v1,v2),(v3,v4),...(vn,vm`。
func splitDMValueRows(s string) [][]string {
	// 补回首尾括号，再按括号深度拆行。
	wrapped := "(" + s + ")"
	var rows [][]string
	depth := 0
	inQuote := false
	rowStart := -1
	for i := 0; i < len(wrapped); i++ {
		c := wrapped[i]
		if c == '\'' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch c {
		case '(':
			depth++
			if depth == 1 {
				rowStart = i + 1
			}
		case ')':
			depth--
			if depth == 0 && rowStart >= 0 {
				rows = append(rows, splitTopLevel(wrapped[rowStart:i], ','))
				rowStart = -1
			}
		}
	}
	return rows
}

// dmReplaceBoolLiterals 把 SQL 中引号外的 TRUE/FALSE 字面量替换为 1/0（达梦无布尔字面量）。
// 单引号字符串内的文本（如 seed 的技能描述含 "TRUE"）保持原样。
func dmReplaceBoolLiterals(sql string) string {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(sql); {
		c := sql[i]
		if c == '\'' {
			inQuote = !inQuote
			b.WriteByte(c)
			i++
			continue
		}
		if !inQuote && (c == 'T' || c == 't' || c == 'F' || c == 'f') {
			for _, kw := range []string{"TRUE", "FALSE"} {
				if len(sql)-i >= len(kw) && strings.EqualFold(sql[i:i+len(kw)], kw) {
					before := i == 0 || !isWordByte(sql[i-1])
					after := i+len(kw) == len(sql) || !isWordByte(sql[i+len(kw)])
					if before && after {
						if kw == "TRUE" {
							b.WriteByte('1')
						} else {
							b.WriteByte('0')
						}
						i += len(kw)
						goto next
					}
				}
			}
		}
		b.WriteByte(c)
		i++
	next:
	}
	return b.String()
}

func isWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// reDMValuesToS 把 VALUES(col) 转成 s.col（MERGE 的 USING 别名）。
var reDMValuesToS = regexp.MustCompile(`(?i)\bVALUES\(\s*([a-zA-Z_]\w*)\s*\)`)

// splitCSV 拆分逗号列表并 trim。
func splitCSV(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// splitTopLevel 按分隔符拆分，跳过单引号字符串内的分隔符与括号嵌套。
func splitTopLevel(s string, sep byte) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			inQuote = !inQuote
			cur.WriteByte(c)
			continue
		}
		if inQuote {
			cur.WriteByte(c)
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteByte(c)
	}
	if v := strings.TrimSpace(cur.String()); v != "" {
		out = append(out, v)
	}
	return out
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

// ogHasColumn 用 information_schema 查某表是否已有该列（openGauss 无 ADD COLUMN IF NOT EXISTS，
// 用列存在性检查补齐幂等语义；table_schema 必须是 current_schema()，不是 current_database()）。
func ogHasColumn(ctx context.Context, table, col string) bool {
	rows, err := g.DB().GetAll(ctx,
		"SELECT COUNT(*) AS n FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?",
		table, col)
	if err != nil {
		return false
	}
	return len(rows) > 0 && rows[0]["n"].Int64() > 0
}

// dmHasColumn 用达梦 USER_TAB_COLUMNS 查某表是否已有该列（达梦无 ADD COLUMN IF NOT EXISTS）。
func dmHasColumn(ctx context.Context, table, col string) bool {
	rows, err := g.DB().GetAll(ctx,
		"SELECT COUNT(*) AS n FROM USER_TAB_COLUMNS WHERE UPPER(table_name)=UPPER(?) AND UPPER(column_name)=UPPER(?)",
		table, col)
	if err != nil {
		return false
	}
	return len(rows) > 0 && rows[0]["n"].Int64() > 0
}

// RunMigrations 启动期自动迁移：按 migrations/*.sql 文件名顺序执行，schema_migrations 表记录已执行文件。
// mfs 为迁移脚本来源（main.go 内嵌的 migrationsFS，或开发态自定义 fs.FS）；裸二进制单文件分发无需旁置磁盘目录。
func RunMigrations(mfs fs.FS) error {
	// 总超时：SSH 隧道半死时 DB 请求会挂住而不是报错，启动时不能被迁移拖死。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	dialect := dbType()
	isSQLite := dialect == "sqlite"
	isMySQL := dialect == "mysql"
	isOG := dialect == "opengauss"
	isDM := dialect == "dm"
	if _, err := g.DB().Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		file VARCHAR(200) PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(mfs, "migrations")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
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
		body, rerr := fs.ReadFile(mfs, path.Join("migrations", name))
		if rerr != nil {
			return rerr
		}
		for idx, stmt := range dialectize(string(body), dialect) {
			// SQLite / openGauss / DM 的 ADD COLUMN 无 IF NOT EXISTS：列已存在则跳过。
			if isSQLite {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && sqliteHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			if isMySQL {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && mysqlHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			if isOG {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && ogHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			if isDM {
				if m := reAlterAddCol.FindStringSubmatch(stmt); m != nil && dmHasColumn(ctx, m[1], m[2]) {
					continue
				}
			}
			if _, xerr := g.DB().Exec(ctx, stmt); xerr != nil {
				// 达梦幂等差异：同列重复建索引（不同索引名）报 -3236「此列列表已索引」。
				// PG/SQLite/MySQL 允许，达梦拒绝；已索引即目标达成，跳过。
				if isDM && strings.Contains(xerr.Error(), "此列列表已索引") {
					continue
				}
				return fmt.Errorf("apply migration %s (stmt #%d): %w [SQL: %.300s]", name, idx+1, xerr, stmt)
			}
		}
		if _, ierr := g.DB().Model("schema_migrations").Data(g.Map{"file": name}).Insert(); ierr != nil {
			return fmt.Errorf("record migration %s: %w", name, ierr)
		}
		glog.Infof(ctx, "[migrate] applied %s", name)
	}
	return nil
}
