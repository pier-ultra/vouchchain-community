// Package dbutil 提供数据库方言工具（区分 PostgreSQL / openGauss / KingBase / DM / MySQL / SQLite，用于 SQL 分支）。
package dbutil

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

// PG 内核家族（openGauss / KingBase）走 pgsql 驱动，但方言各有裁剪：
//   - openGauss：无 ON CONFLICT / ADD COLUMN IF NOT EXISTS / pgvector
//   - KingBase：语法最接近 PG，但无 pgvector
//
// 达梦走独立 dm 驱动（Oracle 系方言）。以下探测结果进程内缓存一次。
var (
	pgVariantOnce sync.Once
	pgVariant     string // "opengauss" / "kingbase" / ""
	typeOnce      sync.Once
	typeName      string
)

func currentType() string {
	typeOnce.Do(func() {
		cfg := g.DB().GetConfig()
		typeName = strings.ToLower(strings.TrimSpace(cfg.Type))
		if typeName == "" {
			link := strings.ToLower(strings.TrimSpace(cfg.Link))
			for _, p := range []string{"sqlite", "pgsql", "postgres", "mysql", "dm", "opengauss", "kingbase"} {
				if strings.HasPrefix(link, p) {
					typeName = p
					break
				}
			}
		}
	})
	return typeName
}

// detectPGVariant 在 pgsql/postgres 驱动下读 version() 判断具体是哪种 PG 内核发行版。
func detectPGVariant() string {
	pgVariantOnce.Do(func() {
		t := currentType()
		if t == "opengauss" || t == "kingbase" {
			pgVariant = t
			return
		}
		if t != "pgsql" && t != "postgres" {
			return
		}
		v, err := g.DB().GetValue(context.Background(), "SELECT lower(version())")
		if err != nil {
			return
		}
		s := v.String()
		switch {
		case strings.Contains(s, "opengauss"):
			pgVariant = "opengauss"
		case strings.Contains(s, "kingbase"):
			pgVariant = "kingbase"
		}
	})
	return pgVariant
}

// IsOpenGauss 判断当前库是否为 openGauss（显式 type 或 version() 探测）。
func IsOpenGauss() bool {
	return detectPGVariant() == "opengauss"
}

// IsKingBase 判断当前库是否为人大金仓（显式 type 或 version() 探测）。
func IsKingBase() bool {
	return detectPGVariant() == "kingbase"
}

// IsDM 判断当前库是否为达梦（Oracle 系，GoFrame dm 驱动）。
func IsDM() bool {
	return currentType() == "dm"
}

// IsSQLite 判断当前数据库是否为 SQLite。
func IsSQLite() bool {
	return currentType() == "sqlite"
}

// IsMySQL 判断当前数据库是否为 MySQL（含 MariaDB / TiDB / OceanBase 等 MySQL 协议兼容库）。
func IsMySQL() bool {
	return currentType() == "mysql"
}

// IsPG 判断当前数据库是否为原生 PostgreSQL（不含 openGauss / KingBase 等 PG 内核裁剪版）。
// 知识库 pgvector 向量检索只用 true；裁剪版无 pgvector 需降级。
func IsPG() bool {
	t := currentType()
	if t != "pgsql" && t != "postgres" {
		return false
	}
	return detectPGVariant() == ""
}
