// Package dbutil 提供数据库方言工具（区分 PostgreSQL / openGauss / MySQL / SQLite，用于 SQL 分支）。
package dbutil

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

// openGauss 走 pgsql 驱动（PG 内核、libpq 协议兼容），但方言有关键差异：
// 无 ON CONFLICT、无 ADD COLUMN IF NOT EXISTS、无 pgvector。故需单独识别。
var (
	ogOnce sync.Once
	ogYes  bool
)

// IsOpenGauss 判断当前库是否为 openGauss。
// 识别两条路：① 配置显式声明 type: opengauss；② 已连库时 SELECT version() 探测（含 "opengauss"）。
// 探测结果进程内缓存一次；探测失败（如 DB 未连）按 false 处理，不影响 SQLite/MySQL/PG。
func IsOpenGauss() bool {
	ogOnce.Do(func() {
		cfg := g.DB().GetConfig()
		t := strings.ToLower(strings.TrimSpace(cfg.Type))
		if t == "opengauss" {
			ogYes = true
			return
		}
		if t != "pgsql" && t != "postgres" {
			return
		}
		v, err := g.DB().GetValue(context.Background(), "SELECT lower(version())")
		if err == nil && strings.Contains(v.String(), "opengauss") {
			ogYes = true
		}
	})
	return ogYes
}

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

// IsMySQL 判断当前数据库是否为 MySQL（含 MariaDB / TiDB / OceanBase 等 MySQL 协议兼容库）。
func IsMySQL() bool {
	cfg := g.DB().GetConfig()
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t != "" {
		return t == "mysql"
	}
	link := strings.ToLower(strings.TrimSpace(cfg.Link))
	return strings.HasPrefix(link, "mysql")
}

// IsPG 判断当前数据库是否为 PostgreSQL（pg 内核，含 openGauss）。
func IsPG() bool {
	cfg := g.DB().GetConfig()
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t != "" {
		return t == "pgsql" || t == "postgres"
	}
	link := strings.ToLower(strings.TrimSpace(cfg.Link))
	return strings.HasPrefix(link, "pgsql") || strings.HasPrefix(link, "postgres")
}
