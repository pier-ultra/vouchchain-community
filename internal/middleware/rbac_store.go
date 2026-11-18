package middleware

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"vouchchain/pkg/cache"

	"github.com/gogf/gf/v2/frame/g"
)

// ---------- G1 RBAC 配置化（AG1.1–AG1.4） ----------
// 角色定义、用户授权、数据范围、字段级权限从库里读取，带短 TTL 缓存；
// 读取失败时回落静态四角色，保证数据库异常或迁移未跑完时不会把系统锁死。
//
// 缓存经 pkg/cache 可插拔层：默认 memory（进程内，行为与早期内联实现等价）；
// 配置 cache.driver=kvrocks 时切到外部 Kvrocks，供多实例共享。未配置一律 memory。

type RoleDef struct {
	Code        string
	Name        string
	Description string
	Builtin     bool
	Permissions []string // "POST /api/xxx" 允许；"-PUT /api/xxx" 显式拒绝
	DataScope   string   // all / entity / dept / self
	FieldMask   []string // 不可见字段
	Enabled     bool
}

// roleSnapshotData roles+binds 合成一份 JSON 快照存缓存，保证两者一起刷新。
type roleSnapshotData struct {
	Roles map[string]RoleDef `json:"roles"`
	Binds map[string]string  `json:"binds"` // username → 主角色
}

var (
	roleCacheKey = "rbac:roles"
	roleStore    = cache.New(context.Background())
	roleLoadMu   sync.Mutex // 防击穿：缓存 miss 时并发只查一次库（memory/kvrocks 通用）
)

var staticRoles = map[string]bool{"admin": true, "legal": true, "business": true, "viewer": true}

// InvalidateRoleCache 角色/授权变更后由控制器主动调用，保证实时生效（AG1.4）
func InvalidateRoleCache() {
	_ = roleStore.Del(context.Background(), roleCacheKey)
}

// roleSnapshot 读角色快照（20s TTL 缓存），miss 时查库回填；返回 (roles, binds)。
func roleSnapshot() (map[string]RoleDef, map[string]string) {
	if roles, binds, ok := roleCached(); ok {
		return roles, binds
	}
	roleLoadMu.Lock()
	defer roleLoadMu.Unlock()
	if roles, binds, ok := roleCached(); ok { // 双检：等锁期间已被别人回填
		return roles, binds
	}
	roles, binds := loadRolesFromDB()
	if b, err := json.Marshal(roleSnapshotData{Roles: roles, Binds: binds}); err == nil {
		_ = roleStore.Set(context.Background(), roleCacheKey, string(b), 20*time.Second)
	}
	return roles, binds
}

func roleCached() (map[string]RoleDef, map[string]string, bool) {
	v, ok, err := roleStore.Get(context.Background(), roleCacheKey)
	if err != nil || !ok {
		return nil, nil, false
	}
	var snap roleSnapshotData
	if json.Unmarshal([]byte(v), &snap) != nil {
		return nil, nil, false
	}
	return snap.Roles, snap.Binds, true
}

func loadRolesFromDB() (map[string]RoleDef, map[string]string) {
	roles, binds := map[string]RoleDef{}, map[string]string{}
	rows, err := g.DB().Model("rbac_role").Where("enabled", true).Order("id asc").All()
	if err == nil {
		for _, row := range rows {
			perms, masks := []string{}, []string{}
			_ = json.Unmarshal([]byte(row["permissions"].String()), &perms)
			_ = json.Unmarshal([]byte(row["field_mask"].String()), &masks)
			roles[row["code"].String()] = RoleDef{
				Code: row["code"].String(), Name: row["name"].String(),
				Description: row["description"].String(), Builtin: row["is_builtin"].Bool(),
				Permissions: perms, DataScope: row["data_scope"].String(),
				FieldMask: masks, Enabled: true,
			}
		}
	}
	brs, err := g.DB().Model("rbac_user_role").Order("id asc").All()
	if err == nil {
		for _, row := range brs {
			if _, exists := binds[row["username"].String()]; !exists {
				binds[row["username"].String()] = row["role_code"].String()
			}
		}
	}
	// 库里没有角色数据时保留静态角色可用（数据库异常或迁移未跑完不锁死系统）
	if len(roles) == 0 {
		for code := range staticRoles {
			roles[code] = RoleDef{Code: code, Builtin: true, DataScope: "all", Enabled: true}
		}
	}
	return roles, binds
}

func RoleDefFor(code string) (RoleDef, bool) {
	roles, _ := roleSnapshot()
	d, ok := roles[code]
	return d, ok
}

func IsKnownRole(code string) bool {
	roles, _ := roleSnapshot()
	if _, ok := roles[code]; ok {
		return true
	}
	return staticRoles[code]
}

// BoundRole 用户显式授权的角色优先于请求头角色（AG1.3）
func BoundRole(username string) string {
	if username == "" {
		return ""
	}
	_, binds := roleSnapshot()
	return binds[username]
}

func AllRoleDefs() []RoleDef {
	roles, _ := roleSnapshot()
	out := make([]RoleDef, 0, len(roles))
	for _, d := range roles {
		out = append(out, d)
	}
	return out
}

func AllBindings() map[string]string {
	_, binds := roleSnapshot()
	out := map[string]string{}
	for k, v := range binds {
		out[k] = v
	}
	return out
}

// StaticRules 返回内置的方法+路径规则，供角色配置页展示可分配权限项
func StaticRules() [][3]string {
	out := make([][3]string, 0, len(permRules))
	for _, p := range permRules {
		out = append(out, [3]string{p.method, p.prefix, p.comment})
	}
	return out
}
