package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
)

// RunMigrations 启动期自动迁移：按 migrations/*.sql 文件名顺序执行，schema_migrations 表记录已执行文件。
// 所有迁移必须写成幂等（CREATE ... IF NOT EXISTS / ON CONFLICT DO NOTHING），单二进制自托管分发时
// 用户不需要手工 docker cp 执行 SQL——这是「当收费版做好」的基本盘。
func RunMigrations() error {
	// 总超时：SSH 隧道半死时 DB 请求会挂住而不是报错，启动时不能被迁移拖死。
	// 120s：经 SSH 隧道跑 27+ 个迁移 + 建新表，20s 不够（隧道单请求延迟大），放宽到 120s。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if _, err := g.DB().Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		file VARCHAR(200) PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT NOW()
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
		if _, xerr := g.DB().Exec(ctx, string(body)); xerr != nil {
			return fmt.Errorf("apply migration %s: %w", name, xerr)
		}
		if _, ierr := g.DB().Model("schema_migrations").Data(g.Map{"file": name}).Insert(); ierr != nil {
			return fmt.Errorf("record migration %s: %w", name, ierr)
		}
		glog.Infof(ctx, "[migrate] applied %s", name)
	}
	return nil
}
