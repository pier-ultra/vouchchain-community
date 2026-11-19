package bootstrap

// schema 注释元数据同步：把 migrations/20261119_schema_comments.sql 里的
// COMMENT ON TABLE/COLUMN 语句落到各数据库的信息_schema 层。
// 各方言适配：
//   - pgsql / opengauss：原生执行 COMMENT ON（幂等，重复执行即覆盖，代价极低）；
//   - dm / kingbase：COMMENT ON 语法一致，保留字列名（level/comment/…）加双引号；
//   - mysql（MySQL/TiDB/OceanBase）：不支持 COMMENT ON——表注释转 ALTER TABLE ... COMMENT；
//     字段注释从 information_schema.COLUMNS 读回列定义拼 ALTER TABLE ... MODIFY COLUMN ... COMMENT，
//     已有同注释的列跳过（首跑全量，之后零 DDL）；
//   - sqlite：无注释机制，整体跳过。
// 单条失败只记 WARN 不阻断启动（注释是元数据增强，缺一不致命）。

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
)

const schemaCommentsFile = "20261119_schema_comments.sql"

// reCommentOn 解析 COMMENT ON TABLE t IS '...' / COMMENT ON COLUMN t.c IS '...'。
var reCommentOn = regexp.MustCompile(`(?is)^\s*COMMENT\s+ON\s+(TABLE|COLUMN)\s+([a-zA-Z_][\w]*)(?:\.([a-zA-Z_][\w]*))?\s+IS\s+'((?:[^']|'')*)'\s*;?\s*$`)

// 达梦/金仓（Oracle 兼容模式）保留字列名，COMMENT ON COLUMN 中需双引号。
func quoteCommentIdent(dialect, ident string) string {
	if dialect != "dm" && dialect != "kingbase" {
		return ident
	}
	if reDMReservedWord.MatchString(strings.ToLower(ident)) || reKBReserved.MatchString(strings.ToLower(ident)) {
		return `"` + ident + `"`
	}
	return ident
}

// applySchemaComments 在 RunMigrations 末尾调用：每次启动幂等对齐注释元数据。
func applySchemaComments(ctx context.Context, dialect string, mfs fs.FS) {
	if dialect == "sqlite" {
		return // SQLite 无注释机制
	}
	body, err := fs.ReadFile(mfs, path.Join("migrations", schemaCommentsFile))
	if err != nil {
		return // 内嵌包里没有该文件（旧版本），静默跳过
	}
	var applied, failed int
	for _, st := range splitStatements(string(body)) {
		st = strings.TrimSpace(st)
		if !reCommentOn.MatchString(st) {
			continue
		}
		m := reCommentOn.FindStringSubmatch(st)
		kind, table, col, comment := m[1], m[2], m[3], m[4]
		comment = strings.ReplaceAll(comment, "''", "'")
		var xerr error
		switch dialect {
		case "mysql":
			xerr = applyMySQLComment(ctx, kind, table, col, comment)
		case "dm", "kingbase":
			stmt := rebuildCommentOn(kind, table, col, comment, dialect)
			_, xerr = g.DB().Exec(ctx, stmt)
		default: // pgsql / opengauss
			_, xerr = g.DB().Exec(ctx, st)
		}
		if xerr != nil {
			failed++
			glog.Debugf(ctx, "[schema-comments] %s: %v", st, xerr) // DEBUG 级：单条失败不打扰启动日志
			continue
		}
		applied++
	}
	if applied > 0 || failed > 0 {
		glog.Debugf(ctx, "[schema-comments] applied=%d failed=%d", applied, failed)
	}
}

// rebuildCommentOn 给 dm/kingbase 重建 COMMENT ON 语句（保留字列名加双引号）。
func rebuildCommentOn(kind, table, col, comment, dialect string) string {
	esc := strings.ReplaceAll(comment, "'", "''")
	if kind == "TABLE" {
		return fmt.Sprintf("COMMENT ON TABLE %s IS '%s'", quoteCommentIdent(dialect, table), esc)
	}
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s'",
		quoteCommentIdent(dialect, table), quoteCommentIdent(dialect, col), esc)
}

// applyMySQLComment 把 COMMENT ON 转为 MySQL 系 DDL。
// 表注释：ALTER TABLE t COMMENT='...'（幂等轻量）。
// 字段注释：先读回现有 COMMENT 比对，一致则跳过；否则从 information_schema 拼完整列定义 MODIFY。
func applyMySQLComment(ctx context.Context, kind, table, col, comment string) error {
	if kind == "TABLE" {
		_, err := g.DB().Exec(ctx, fmt.Sprintf("ALTER TABLE `%s` COMMENT='%s'", table, escapeMySQLComment(comment)))
		return err
	}
	rows, err := g.DB().GetAll(ctx,
		"SELECT COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT FROM information_schema.COLUMNS "+
			"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", table, col)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil // 列不存在（表结构未含此列），跳过
	}
	r := rows[0]
	if strings.ReplaceAll(r["COLUMN_COMMENT"].String(), "''", "'") == comment {
		return nil // 注释已一致，零 DDL
	}
	colType := r["COLUMN_TYPE"].String()
	nullable := r["IS_NULLABLE"].String()
	def := r["COLUMN_DEFAULT"]
	extra := strings.ToUpper(r["EXTRA"].String())

	var b strings.Builder
	fmt.Fprintf(&b, "ALTER TABLE `%s` MODIFY COLUMN `%s` %s", table, col, colType)
	if strings.Contains(extra, "AUTO_INCREMENT") {
		b.WriteString(" AUTO_INCREMENT")
	}
	if nullable == "NO" {
		b.WriteString(" NOT NULL")
	}
	if !def.IsNil() {
		d := def.String()
		switch {
		case strings.HasPrefix(strings.ToLower(d), "current_timestamp"):
			b.WriteString(" DEFAULT CURRENT_TIMESTAMP")
		case isNumericDefault(d):
			fmt.Fprintf(&b, " DEFAULT %s", d)
		default:
			fmt.Fprintf(&b, " DEFAULT '%s'", escapeMySQLComment(d))
		}
	}
	if strings.Contains(extra, "ON UPDATE") {
		b.WriteString(" ON UPDATE CURRENT_TIMESTAMP")
	}
	fmt.Fprintf(&b, " COMMENT '%s'", escapeMySQLComment(comment))
	_, err = g.DB().Exec(ctx, b.String())
	return err
}

// escapeMySQLComment 转义 MySQL 字符串字面量里的单引号与反斜杠。
func escapeMySQLComment(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "'", `\'`)
}

// isNumericDefault 判断 DEFAULT 值是否为纯数字字面量（不加引号）。
func isNumericDefault(d string) bool {
	if d == "" {
		return false
	}
	for _, c := range d {
		if (c < '0' || c > '9') && c != '.' && c != '-' {
			return false
		}
	}
	return true
}
