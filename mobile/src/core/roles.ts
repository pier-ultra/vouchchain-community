// 应用级 RBAC（前端半边）：只消费后端 /api/me 下发的会话权限。
// 逻辑与 Web 管理台 web/src/rbac.ts 同源，仅把 localStorage 换成跨端 storage。
import { ref } from 'vue'
import { getItem, setItem } from '../services/storage'

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
// 响应式版本号：/me 返回后触发页面重算
export const sessionVersion = ref(0)

export function setSession(s: SessionRow | null): void {
  session = s
  sessionVersion.value++
  if (s) setItem('ch_role', s.role)
}

export function getSession(): SessionRow | null {
  return session
}

export function currentRole(): Role {
  return session?.role || getItem('ch_role') || 'admin'
}

export function currentUserName(): string {
  return session?.displayName || session?.username || getItem('ch_user') || ''
}

export function can(action: string): boolean {
  void sessionVersion.value
  if (session && Array.isArray(session.actions)) return session.actions.includes(action)
  return false
}
