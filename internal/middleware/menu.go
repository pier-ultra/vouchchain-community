package middleware

import "strings"

// ---------- 菜单与按钮权限的后端单一来源 ----------
// 前端只负责按 /api/me 下发的可见路径渲染分组，不再内置「哪个角色能看哪些菜单」。
// 内置四角色沿用既有可见性基线（写在服务端，升级不越权/失权）；
// 自定义角色可在 permissions 里显式写 "MENU /xxx" / "ACTION xxx" 精确授权，
// 未写菜单项时按每个菜单的探测端点自动推导（读端点默认可见、写端点需相应操作权限）。

type MenuDef struct {
	Path  string   `json:"path"`
	Title string   `json:"title"`
	Group string   `json:"group"`
	Probe string   `json:"probe"` // "METHOD /api/xxx"，供自定义角色自动推导
	Roles []string `json:"roles"` // 内置角色可见基线
}

var menuCatalog = []MenuDef{
	{"/contracts", "合同台账", "合同工作台", "GET /api/contracts", []string{"admin", "legal", "business", "viewer"}},
	{"/generate", "AI 生成", "合同工作台", "POST /api/contracts/generate", []string{"admin", "legal", "business"}},
	{"/extraction", "任务中心", "合同工作台", "POST /api/contracts/upload", []string{"admin", "legal", "business"}},
	{"/obligations", "义务看板", "合同工作台", "GET /api/obligations", []string{"admin", "legal", "business"}},
	{"/chat", "AI 助手", "审查与签署", "POST /api/chat", []string{"admin", "legal", "business", "viewer"}},
	{"/files", "文件中心", "合同工作台", "GET /api/files", []string{"admin", "legal", "business", "viewer"}},
	{"/invoice-class", "发票分类", "发票与财务", "GET /api/invoice-class/rules", []string{"admin", "legal", "business"}},
	{"/expense", "费控管理", "发票与财务", "GET /api/budgets/usage", []string{"admin", "legal", "business"}},
	{"/revrec", "收入确认", "发票与财务", "GET /api/revrec/ledger", []string{"admin", "legal", "business"}},
	{"/seals", "用印管理", "审查与签署", "GET /api/seals", []string{"admin", "legal", "business", "viewer"}},
	{"/skills", "AI 技能", "审查与签署", "GET /api/skills", []string{"admin", "legal", "business", "viewer"}},
	{"/query", "智能问答", "审查与签署", "POST /api/query", []string{"admin", "legal", "business", "viewer"}},
	{"/approvals", "审批中心", "审查与签署", "POST /api/approvals", []string{"admin", "legal", "business"}},
	{"/party-profiles", "相对方画像", "审查与签署", "GET /api/party-profiles", []string{"admin", "legal", "business"}},
	{"/sign", "签署管理", "审查与签署", "POST /api/contracts/files/sign", []string{"admin", "legal"}},
	{"/invoices", "发票中心", "发票与财务", "POST /api/invoices/import", []string{"admin", "legal", "business"}},
	{"/accounting", "应付与做账", "发票与财务", "POST /api/payables/generate", []string{"admin", "legal", "business"}},
	{"/archive-schedules", "归档调度", "发票与财务", "GET /api/archive-schedules", []string{"admin", "legal"}},
	{"/organization", "组织与主体", "组织与主数据", "POST /api/org/nodes", []string{"admin"}},
	{"/master-data", "主数据", "组织与主数据", "POST /api/master-data", []string{"admin", "legal"}},
	{"/mdm-dedup", "主数据去重", "组织与主数据", "POST /api/mdm/scan", []string{"admin", "legal"}},
	{"/compliance", "合规策略", "治理与配置", "GET /api/compliance/summary", []string{"admin", "legal"}},
	{"/roles", "角色权限", "治理与配置", "POST /api/rbac/roles", []string{"admin"}},
	{"/isolation", "数据隔离", "治理与配置", "GET /api/isolation/summary", []string{"admin"}},
	{"/identity-sync", "身份同步", "治理与配置", "GET /api/scim/status", []string{"admin", "legal"}},
	{"/dengbao", "等保证据", "治理与配置", "GET /api/dengbao/retention/status", []string{"admin", "legal", "viewer"}},
	{"/duban", "公文督办", "审查与签署", "GET /api/duban/tasks", []string{"admin", "legal", "business", "viewer"}},
	{"/model-configs", "模型配置", "治理与配置", "POST /api/model-config", []string{"admin"}},
	{"/mcp-servers", "MCP 客户端", "治理与配置", "POST /api/mcp-servers", []string{"admin"}},
	{"/about", "关于我们", "治理与配置", "GET /", []string{"admin", "legal", "business", "viewer"}},
}

var actionCatalog = []string{
	"contract:delete", "review:complete", "obligation:edit",
	"model:write", "sign:create", "contract:edit", "isolation:write", "party:link",
}

var staticActions = map[string][]string{
	"admin":    actionCatalog,
	"legal":    {"review:complete", "obligation:edit", "sign:create", "contract:edit", "party:link"},
	"business": {"obligation:edit", "contract:edit", "party:link"},
	"viewer":   {},
}

// MenuCatalog 供角色配置页列出可分配的菜单项
func MenuCatalog() []MenuDef {
	out := make([]MenuDef, len(menuCatalog))
	copy(out, menuCatalog)
	return out
}

func ActionCatalog() []string {
	out := make([]string, len(actionCatalog))
	copy(out, actionCatalog)
	return out
}

func ValidMenuPath(path string) bool {
	for _, m := range menuCatalog {
		if m.Path == path {
			return true
		}
	}
	return false
}

func ValidAction(action string) bool {
	return containsString(actionCatalog, action)
}

func isBuiltinRole(role string) bool {
	if staticRoles[role] {
		return true
	}
	def, ok := RoleDefFor(role)
	return ok && def.Builtin
}

// VisibleMenus 当前角色可见的菜单路径（/api/me 下发，前端按此过滤分组）
func VisibleMenus(role string) []string {
	out := make([]string, 0, len(menuCatalog))
	if isBuiltinRole(role) {
		for _, m := range menuCatalog {
			if containsString(m.Roles, role) {
				out = append(out, m.Path)
			}
		}
		return out
	}
	def, ok := RoleDefFor(role)
	if !ok {
		return []string{}
	}
	allow, deny := map[string]bool{}, map[string]bool{}
	for _, p := range def.Permissions {
		neg := strings.HasPrefix(p, "-")
		q := strings.TrimPrefix(p, "-")
		if !strings.HasPrefix(q, "MENU ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(q, "MENU "))
		if neg {
			deny[path] = true
		} else {
			allow[path] = true
		}
	}
	if len(allow) > 0 || len(deny) > 0 {
		// 显式模式：只放行明确授权的菜单，显式拒绝优先
		for _, m := range menuCatalog {
			if allow[m.Path] && !deny[m.Path] {
				out = append(out, m.Path)
			}
		}
		return out
	}
	// 推导模式：按菜单探测端点走与 HTTP 请求一致的授权判定
	for _, m := range menuCatalog {
		parts := strings.SplitN(m.Probe, " ", 2)
		if len(parts) == 2 && customAllowed(def, parts[0], parts[1]) {
			out = append(out, m.Path)
		}
	}
	return out
}

// VisibleActions 当前角色的按钮级动作权限
func VisibleActions(role string) []string {
	if isBuiltinRole(role) {
		return append([]string{}, staticActions[role]...)
	}
	def, ok := RoleDefFor(role)
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, p := range def.Permissions {
		if !strings.HasPrefix(p, "ACTION ") {
			continue
		}
		a := strings.TrimSpace(strings.TrimPrefix(p, "ACTION "))
		if !containsString(out, a) {
			out = append(out, a)
		}
	}
	return out
}
