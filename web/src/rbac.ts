// 应用级 RBAC（前端半边）：只消费后端 /api/me 下发的会话权限。
// 菜单可见性、按钮动作、生效角色全部由服务端按 RBAC 配置算好；
// 这里保留内置四角色的静态表，仅作为「服务不可达 / 会话还没加载」时的兜底。
import { ref } from 'vue'

export type Role = string

export const roleLabels: Record<string, string> = {
  admin: '管理员',
  legal: '法务',
  business: '业务',
  viewer: '只读',
}

export interface SessionRow {
  username: string
  displayName: string
  email?: string
  avatar?: string
  owner?: string
  authMode: string
  devMode: boolean
  claimRoles: string[]
  role: string
  roleName: string
  dataScope: string
  fieldMask: string[]
  menus: string[]
  actions: string[]
}

let session: SessionRow | null = null
// 响应式版本号：/api/me 返回后触发侧栏 computed 重算
export const sessionVersion = ref(0)

export function setSession(s: SessionRow | null) {
  session = s
  sessionVersion.value++
  if (s) localStorage.setItem('ch_role', s.role)
}

export function getSession(): SessionRow | null {
  return session
}

export function currentRole(): Role {
  return session?.role || localStorage.getItem('ch_role') || 'admin'
}

// 兜底基线（与服务端 middleware/menu.go 的内置角色表一致）
export const roleMenus: Record<string, string[]> = {
  admin: ['/contracts', '/generate', '/extraction', '/obligations', '/query', '/sign', '/approvals', '/party-profiles', '/invoices', '/accounting', '/model-configs', '/organization', '/master-data', '/archive-schedules', '/compliance', '/mdm-dedup', '/roles', '/isolation', '/identity-sync'],
  legal: ['/contracts', '/generate', '/extraction', '/obligations', '/query', '/sign', '/approvals', '/party-profiles', '/invoices', '/accounting', '/master-data', '/archive-schedules', '/compliance', '/mdm-dedup', '/identity-sync'],
  business: ['/contracts', '/generate', '/extraction', '/obligations', '/query', '/approvals', '/party-profiles', '/invoices', '/accounting'],
  viewer: ['/contracts', '/query'],
}

export const roleActions: Record<string, string[]> = {
  admin: ['contract:delete', 'clause:review', 'review:complete', 'obligation:edit', 'model:write', 'sign:create', 'contract:edit', 'isolation:write', 'party:link'],
  legal: ['clause:review', 'review:complete', 'obligation:edit', 'sign:create', 'contract:edit', 'party:link'],
  business: ['obligation:edit', 'contract:edit', 'party:link'],
  viewer: [],
}

export function visibleMenus(): string[] {
  void sessionVersion.value // /api/me 返回后触发模板重算
  if (session && Array.isArray(session.menus)) return session.menus
  return roleMenus[currentRole()] || []
}

export function can(action: string): boolean {
  void sessionVersion.value
  if (session && Array.isArray(session.actions)) return session.actions.includes(action)
  return (roleActions[currentRole()] || []).includes(action)
}
