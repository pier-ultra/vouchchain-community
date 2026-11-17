package middleware

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// ---------- G1 RBAC 配置化（AG1.1–AG1.4） ----------
// 角色定义、用户授权、数据范围、字段级权限从库里读取，带短 TTL 缓存；
// 读取失败时回落静态四角色，保证数据库异常或迁移未跑完时不会把系统锁死。

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

var roleCache = struct {
	sync.RWMutex
	at    time.Time
	roles map[string]RoleDef
	binds map[string]string // username → 主角色
}{roles: map[string]RoleDef{}, binds: map[string]string{}}

var staticRoles = map[string]bool{"admin": true, "legal": true, "business": true, "viewer": true}

// InvalidateRoleCache 角色/授权变更后由控制器主动调用，保证实时生效（AG1.4）
func InvalidateRoleCache() {
	roleCache.Lock()
	roleCache.at = time.Time{}
	roleCache.Unlock()
}

func refreshIfStale() {
	roleCache.RLock()
	fresh := time.Since(roleCache.at) < 20*time.Second
	roleCache.RUnlock()
	if fresh {
		return
	}
	roleCache.Lock()
	defer roleCache.Unlock()
	if time.Since(roleCache.at) < 20*time.Second {
		return
	}
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
	// 库里没有角色数据时保留静态角色可用
	if len(roles) == 0 {
		for code := range staticRoles {
			roles[code] = RoleDef{Code: code, Builtin: true, DataScope: "all", Enabled: true}
		}
	}
	roleCache.roles, roleCache.binds, roleCache.at = roles, binds, time.Now()
}

func RoleDefFor(code string) (RoleDef, bool) {
	refreshIfStale()
	roleCache.RLock()
	defer roleCache.RUnlock()
	d, ok := roleCache.roles[code]
	return d, ok
}

func IsKnownRole(code string) bool {
	refreshIfStale()
	roleCache.RLock()
	_, ok := roleCache.roles[code]
	roleCache.RUnlock()
	if ok {
		return true
	}
	return staticRoles[code]
}

// BoundRole 用户显式授权的角色优先于请求头角色（AG1.3）
func BoundRole(username string) string {
	if username == "" {
		return ""
	}
	refreshIfStale()
	roleCache.RLock()
	defer roleCache.RUnlock()
	return roleCache.binds[username]
}

func AllRoleDefs() []RoleDef {
	refreshIfStale()
	roleCache.RLock()
	defer roleCache.RUnlock()
	out := make([]RoleDef, 0, len(roleCache.roles))
	for _, d := range roleCache.roles {
		out = append(out, d)
	}
	return out
}

func AllBindings() map[string]string {
	refreshIfStale()
	roleCache.RLock()
	defer roleCache.RUnlock()
	out := map[string]string{}
	for k, v := range roleCache.binds {
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
