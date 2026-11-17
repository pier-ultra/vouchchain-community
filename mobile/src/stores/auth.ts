import { defineStore } from 'pinia'
import { getItem, setItem, removeItem } from '../services/storage'
import { api } from '../services/api'
import { setSession, type SessionRow } from '../core/roles'

// 登录态：与 Web 端同构（dev 态任意用户名可进、token 占位；SSO 态为 Casdoor 签发的 JWT）。
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getItem('ch_token'),
    user: getItem('ch_user'),
    role: getItem('ch_role'),
    logging: false,
  }),
  getters: {
    logged: (s): boolean => !!s.token,
    displayName: (s): string => s.user || '',
  },
  actions: {
    restore(): void {
      this.token = getItem('ch_token')
      this.user = getItem('ch_user')
      this.role = getItem('ch_role')
    },
    // 登录成功后拉一次 /me 拿到角色/菜单/动作；失败不阻断进入（dev 态后端可能未起）
    async login(user: string, token = 'dev-token'): Promise<void> {
      this.logging = true
      this.token = token
      this.user = user
      setItem('ch_token', token)
      setItem('ch_user', user)
      try {
        const res = await api.me()
        if (res?.code === 0 && res.data) {
          const s = res.data
          this.role = s.role
          setItem('ch_role', s.role)
          setSession(s)
        }
      } catch {
        /* 忽略：允许离线/未配置后端时进入 */
      } finally {
        this.logging = false
      }
    },
    logout(): void {
      this.token = ''
      this.user = ''
      this.role = ''
      removeItem('ch_token')
      removeItem('ch_user')
      removeItem('ch_role')
      setSession(null)
      uni.reLaunch({ url: '/pages/login/index' })
    },
  },
})

export type { SessionRow }
