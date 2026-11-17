// Package dbutil 提供数据库方言工具（当前：区分 PostgreSQL 与 SQLite，用于 SQL 分支）。
package dbutil

import (
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

// IsSQLite 判断当前数据库是否为 SQLite。
// 与 bootstrap.dbType 逻辑一致：优先配置 Type，Type 为空时从 link 前缀推断。
func IsSQLite() bool {
	cfg := g.DB().GetConfig()
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t != "" {
		return t == "sqlite"
	}
	link := strings.ToLower(strings.TrimSpace(cfg.Link))
	return strings.HasPrefix(link, "sqlite")
}
