package middleware

import (
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
)

// 应用级 RBAC（AC6.7 的授权半边；认证半边见 auth.go）：
// - 角色：admin / legal / business / viewer（MVP 固定四角色；真实用户体系随 SSO 落地后由 Casdoor 角色喂入）
// - 规则按「方法 + 路径前缀」声明；未命中规则默认放行（读操作），敏感写操作显式收敛
// - devMode 下角色经登录页选择，前端以 X-User-Role 头携带；生产态从 JWT claims.roles 解析
// 按可抽取标准：规则表与引擎独立成段，未来抽成开源审批/权限包时直接复用。

type permRule struct {
	method  string
	prefix  string
	roles   []string
	comment string
}

var permRules = []permRule{
	// 模型配置（Core 内核管理面）：仅管理员
	{"POST", "/api/model-config", []string{"admin"}, "模型配置写入"},
	// 合同台账写操作
	{"POST", "/api/contracts/upload", []string{"admin", "legal", "business"}, "上传合同"},
	{"POST", "/api/contracts/generate", []string{"admin", "legal", "business"}, "AI 生成合同"},
	{"POST", "/api/contracts", []string{"admin", "legal", "business"}, "新增合同"},
	{"PUT", "/api/contracts/", []string{"admin", "legal", "business"}, "修改合同"},
	{"DELETE", "/api/contracts/", []string{"admin", "legal"}, "删除合同"},
	// 条款复核：法务域
	{"PUT", "/api/contracts/clauses/", []string{"admin", "legal"}, "条款复核修正"},
	{"POST", "/api/contracts/clauses/", []string{"admin", "legal"}, "批量复核"},
	{"POST", "/api/contracts/files/", []string{"admin", "legal", "business"}, "复核完成/义务生成/签署发起"},
	// 文件中心：全员可查/下载/预览；上传/删除不限业务角色（admin/legal/business），viewer 只读
	{"POST", "/api/files", []string{"admin", "legal", "business"}, "文件上传"},
	{"DELETE", "/api/files/", []string{"admin", "legal"}, "文件删除"},
	// 发票分类：规则配置归 admin/legal；执行分类 admin/legal/business；统计全员
	{"POST", "/api/invoice-class/rules", []string{"admin", "legal"}, "发票分类规则维护"},
	{"DELETE", "/api/invoice-class/rules/", []string{"admin", "legal"}, "发票分类规则删除"},
	{"POST", "/api/invoice-class/run", []string{"admin", "legal", "business"}, "发票执行分类"},
	// 收入确认：建议/确认归 admin/legal（会计动作），viewer 只读
	{"PUT", "/api/revrec/mode/", []string{"admin", "legal"}, "履约确认方式设置"},
	{"POST", "/api/revrec/suggest", []string{"admin", "legal"}, "收入确认建议"},
	{"POST", "/api/revrec/confirm/", []string{"admin", "legal"}, "收入确认"},
	{"POST", "/api/revrec/receipt", []string{"admin", "legal", "business"}, "回款登记"},
	// 用印：申请/登记归 admin/legal/business；审批归 admin/legal
	{"POST", "/api/seals", []string{"admin", "legal"}, "印章登记"},
	{"PUT", "/api/seals/", []string{"admin", "legal"}, "印章状态变更"},
	{"POST", "/api/seal-applies", []string{"admin", "legal", "business"}, "用印申请"},
	{"POST", "/api/seal-applies/", []string{"admin", "legal"}, "用印审批/登记"},
	{"POST", "/api/es/config", []string{"admin", "legal"}, "外部电子签平台配置"},
	{"POST", "/api/travel-check", []string{"admin", "legal", "business"}, "差旅超标检查"},
	// Agent Skill：上传/删除/启停归 admin/legal；查询全员
	{"POST", "/api/skills", []string{"admin", "legal"}, "技能包上传"},
	{"PUT", "/api/skills/", []string{"admin", "legal"}, "技能包启停"},
	{"DELETE", "/api/skills/", []string{"admin", "legal"}, "技能包删除"},
	// 费控：切片/预算/差标的写入归 admin/legal/business（财务日常），viewer 只读
	{"POST", "/api/expense-splits", []string{"admin", "legal", "business"}, "报销切片创建"},
	{"DELETE", "/api/expense-splits/", []string{"admin", "legal"}, "报销切片删除"},
	{"POST", "/api/budgets", []string{"admin", "legal"}, "费控预算设置"},
	{"DELETE", "/api/budgets/", []string{"admin", "legal"}, "费控预算删除"},
	{"POST", "/api/travel-standards", []string{"admin", "legal"}, "差旅标准设置"},
	{"DELETE", "/api/travel-standards/", []string{"admin", "legal"}, "差旅标准删除"},
	{"POST", "/api/travel-check", []string{"admin", "legal", "business"}, "差旅超标检查"},
	// 义务分派：业务可分派与反馈
	{"PUT", "/api/obligations/", []string{"admin", "legal", "business"}, "义务更新"},
	{"POST", "/api/obligations/", []string{"admin", "legal", "business"}, "证据上传"},
	// 审批流（AC6.3/6.4）：配置与发起归法务/管理员，审批动作业务也可（他是审批人时）
	{"POST", "/api/approval-templates", []string{"admin", "legal"}, "审批模板配置"},
	{"POST", "/api/approvals", []string{"admin", "legal", "business"}, "发起审批"},
	{"POST", "/api/approvals/", []string{"admin", "legal", "business"}, "审批动作"},
	// 财务域（P2P 闭环）：建议/计划可业务发起，改账期、确认入账、桥接推送只给管理员与法务/财务角色
	{"POST", "/api/payables/generate", []string{"admin", "legal", "business"}, "生成付款计划"},
	{"PUT", "/api/payables/", []string{"admin", "legal"}, "修改应付计划"},
	{"POST", "/api/payables/", []string{"admin", "legal"}, "登记打款"},
	{"POST", "/api/account-mappings", []string{"admin", "legal"}, "科目映射维护"},
	{"PUT", "/api/journal/drafts/", []string{"admin", "legal"}, "分录人工调整"},
	{"POST", "/api/journal/drafts/", []string{"admin", "legal"}, "分录确认入账"},
	{"POST", "/api/bridge/", []string{"admin", "legal"}, "总账桥接导出/推送"},
	// Core K3 法人主体：管理员专属
	{"POST", "/api/legal-entities", []string{"admin"}, "法人主体创建"},
	{"PUT", "/api/legal-entities/", []string{"admin"}, "法人主体修改"},
	{"DELETE", "/api/legal-entities/", []string{"admin"}, "法人主体删除"},
	// Core K2 组织树：管理员专属（读操作所有人可见）
	{"POST", "/api/org/", []string{"admin"}, "组织节点/人员写入"},
	{"PUT", "/api/org/", []string{"admin"}, "组织节点修改"},
	{"DELETE", "/api/org/", []string{"admin"}, "组织节点/人员删除"},
	// 主体切换：所有已登录角色可切（数据权限由 scopeFilter 决定可见范围）
	{"POST", "/api/context/switch", []string{"admin", "legal", "business", "viewer"}, "切换当前主体"},
	// Core K5 主数据
	{"POST", "/api/master-data", []string{"admin", "legal"}, "主数据写入与关联"},
	{"PUT", "/api/master-data/", []string{"admin", "legal"}, "主数据修改"},
	{"DELETE", "/api/master-data/", []string{"admin"}, "主数据删除"},
	// US-I9 归档调度
	{"POST", "/api/archive-schedules", []string{"admin", "legal"}, "归档调度配置"},
	{"PUT", "/api/archive-schedules/", []string{"admin", "legal"}, "归档调度修改"},
	{"DELETE", "/api/archive-schedules/", []string{"admin"}, "归档调度删除"},
	{"POST", "/api/archive-schedules/", []string{"admin", "legal"}, "归档调度启停/立即执行"},
	{"POST", "/api/archive-exceptions/", []string{"admin", "legal"}, "归档异常人工处理"},
	// 产品② 治理底座：合规规则 / MDM 合并 / 角色权限，写操作只给管理员与法务（角色配置仅管理员）
	{"POST", "/api/compliance/", []string{"admin", "legal"}, "合规校验/违规处理/规则写入"},
	{"PUT", "/api/compliance/", []string{"admin", "legal"}, "合规规则修改"},
	{"DELETE", "/api/compliance/", []string{"admin"}, "合规规则删除"},
	{"POST", "/api/mdm/", []string{"admin", "legal"}, "MDM 扫描/合并/规则配置"},
	{"PUT", "/api/mdm/", []string{"admin", "legal"}, "MDM 规则修改"},
	{"DELETE", "/api/mdm/", []string{"admin"}, "MDM 规则删除"},
	{"POST", "/api/rbac/", []string{"admin"}, "角色与授权配置"},
	{"PUT", "/api/rbac/", []string{"admin"}, "角色修改"},
	{"DELETE", "/api/rbac/", []string{"admin"}, "角色删除"},
	// 产品② SCIM 身份同步管理面：状态/日志仅管理员与法务可读，映射配置仅管理员
	{"GET", "/api/scim/", []string{"admin", "legal"}, "SCIM 同步状态与日志"},
	{"POST", "/api/scim/", []string{"admin"}, "SCIM 组角色映射配置"},
	{"PUT", "/api/scim/", []string{"admin"}, "SCIM 组角色映射修改"},
	{"DELETE", "/api/scim/", []string{"admin"}, "SCIM 组角色映射删除"},
	// 产品② AG5 强隔离策略：配置面只给管理员（读端点对所有已登录角色放行，便于自查生效范围）
	{"POST", "/api/isolation/", []string{"admin"}, "隔离策略配置/越权处理"},
	{"PUT", "/api/isolation/", []string{"admin"}, "隔离策略修改"},
	{"DELETE", "/api/isolation/", []string{"admin"}, "隔离策略删除"},
	// 审批增强（加签/转交/废止）沿用已有的 POST /api/approvals/ 规则；画像列表/详情为只读端点
	{"POST", "/api/party-profile/", []string{"admin", "legal", "business"}, "相对方关联维护"},
	// 产品② 等保证据链（企业侧）：留存策略/归档、资产台账、整改跟踪、备份登记归管理员与法务；查询导出全员（测评取证口径）
	{"PUT", "/api/dengbao/policy", []string{"admin", "legal"}, "审计留存策略配置"},
	{"POST", "/api/dengbao/retention/archive", []string{"admin", "legal"}, "审计日志到期归档"},
	{"POST", "/api/dengbao/assets", []string{"admin", "legal"}, "等保资产台账写入"},
	{"POST", "/api/dengbao/findings", []string{"admin", "legal"}, "整改任务登记"},
	{"POST", "/api/dengbao/findings/", []string{"admin", "legal"}, "整改复测/退回/关闭"},
	{"POST", "/api/dengbao/backups", []string{"admin", "legal", "business"}, "备份记录登记"},
	{"POST", "/api/dengbao/selfcheck", []string{"admin", "legal"}, "合规自查登记"},
	{"POST", "/api/dengbao/selfcheck/init", []string{"admin", "legal"}, "合规自查模板铺底"},
	{"DELETE", "/api/dengbao/selfcheck/", []string{"admin", "legal"}, "合规自查删除"},
}

// CurrentRole 当前有效角色（AG1.3/AG1.4）：用户显式授权 > 身份来源 > viewer 兜底。
// 身份来源随鉴权模式切换：dev 态读 X-User-Role（本地演示可模拟）；
// 生产态只读验签后的 Casdoor JWT claims.roles，请求头伪造的角色一律无效。
func CurrentRole(r *ghttp.Request) string {
	role := ""
	if IsDevMode(r) {
		role = r.Header.Get("X-User-Role")
	} else {
		for _, c := range ClaimRoles(r) {
			if IsKnownRole(c) {
				role = c
				break
			}
		}
	}
	if bound := BoundRole(r.GetCtxVar("user").String()); bound != "" {
		role = bound
	}
	if !IsKnownRole(role) {
		role = "viewer"
	}
	return role
}

func staticAllowed(role, method, path string) bool {
	for _, rule := range permRules {
		if rule.method != method || !strings.HasPrefix(path, rule.prefix) {
			continue
		}
		for _, allowedRole := range rule.roles {
			if allowedRole == role {
				return true
			}
		}
		return false // 命中规则但角色不符 → 显式拒绝
	}
	return true // 未命中规则（读操作等）默认放行
}

// uiPermission 菜单/按钮级授权项（"MENU /contracts"、"ACTION contract:delete"）
// 只由菜单计算逻辑消费，不参与 HTTP 端点匹配，避免把 "MENU" 当成请求方法误判。
func uiPermission(pattern string) bool {
	return strings.HasPrefix(pattern, "MENU ") || strings.HasPrefix(pattern, "ACTION ")
}

// customAllowed 自定义角色：显式拒绝优先 → 显式允许 → 读放行 / 写拒绝（AG1.2/AG1.4）
func customAllowed(def RoleDef, method, path string) bool {
	for _, p := range def.Permissions {
		if !strings.HasPrefix(p, "-") || uiPermission(p[1:]) {
			continue
		}
		if matchPerm(p[1:], method, path) {
			return false
		}
	}
	for _, p := range def.Permissions {
		if strings.HasPrefix(p, "-") || uiPermission(p) {
			continue
		}
		if matchPerm(p, method, path) {
			return true
		}
	}
	return method == "GET" || method == "HEAD" || method == "OPTIONS"
}

// matchPerm 自定义角色权限项：
// - "POST /api/contracts"          精确匹配（只授权这一个端点）
// - "PUT /api/contracts/*"        显式通配（授权该资源树下全部子路径）
// 不用隐式前缀，避免授权「新增合同」连带放行「AI 生成合同」这类越权。
func matchPerm(pattern, method, path string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return false
	}
	if parts[0] != "*" && !strings.EqualFold(parts[0], method) {
		return false
	}
	target := parts[1]
	if strings.HasSuffix(target, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(target, "*"))
	}
	return path == target
}

func allowed(role, method, path string) bool {
	if def, ok := RoleDefFor(role); ok && !def.Builtin {
		return customAllowed(def, method, path)
	}
	return staticAllowed(role, method, path) // 内置角色沿用静态规则，保证升级不越权/失权
}

// Rbac RBAC 校验中间件（须在 Auth 之后挂载）
func Rbac() ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		role := CurrentRole(r)
		if !allowed(role, r.Method, r.URL.Path) {
			r.Response.WriteStatusExit(403, "无权限（角色 "+role+"）")
			return
		}
		r.SetCtxVar("role", role)
		r.Middleware.Next()
	}
}
