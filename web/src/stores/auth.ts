import { defineStore } from 'pinia'

// dev 态：登录页任意用户名可进，token 为占位；
// SSO 态：token 是 Casdoor 签发的 JWT（OAuth 回调换取，或直接粘贴），后端用应用证书本地验签。
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('ch_token') || '',
    user: localStorage.getItem('ch_user') || '',
  }),
  actions: {
    login(user: string, token = 'dev-token') {
      this.token = token
      this.user = user
      localStorage.setItem('ch_token', this.token)
      localStorage.setItem('ch_user', this.user)
    },
    logout() {
      this.token = ''
      this.user = ''
      localStorage.removeItem('ch_token')
      localStorage.removeItem('ch_user')
    },
  },
})
