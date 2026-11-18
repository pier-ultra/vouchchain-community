// 跨端请求封装：与 Web 管理台保持同一套约定——
//   1) 基地址可配置：原生 App 需指向真实后端（如 http://192.168.1.100:8000，存 ch_server）；
//      H5 同源走相对 /api（开发态由 vite proxy 转发，生产态由 Go 后端同源提供）
//   2) 请求头带 Bearer token + 角色/用户/主体上下文（SSO 态后端只认验签后的 JWT claims）
//   3) 响应体即业务体（code/data/message），是否成功由调用方判 code
//   4) 401 清会话并跳登录
import { getItem, removeItem } from './storage'

export interface ApiResult<T = unknown> {
  code: number
  data?: T
  message?: string
}

// 服务器地址存储键：客户端本地配置（首启在登录页填一次，Android SharedPreferences / iOS NSUserDefaults）
export const SERVER_ADDR_KEY = 'ch_server'

// 解析请求基地址：有配置用配置（+ /api），否则回退同源 /api（H5）
export function getBaseUrl(): string {
  const srv = (getItem(SERVER_ADDR_KEY) || '').trim().replace(/\/+$/, '')
  return srv ? `${srv}/api` : '/api'
}

// 归一化用户输入的服务器地址：去首尾空白、去尾部斜杠；非法（非 http/https）返回 ''
export function normalizeServerAddr(raw: string): string {
  const s = (raw || '').trim().replace(/\/+$/, '')
  if (!s) return ''
  return /^https?:\/\//i.test(s) ? s : ''
}

function buildHeader(): Record<string, string> {
  const header: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getItem('ch_token')
  if (token) header.Authorization = `Bearer ${token}`
  // dev 态用请求头模拟角色/用户；SSO 态后端忽略这两个头
  header['X-User-Role'] = getItem('ch_role') || 'admin'
  const user = getItem('ch_user')
  if (user) header['X-User-Name'] = user
  // K3 主体上下文：切换主体后所有业务请求按该主体过滤
  const entity = getItem('ch_entity')
  if (entity && entity !== '0') header['X-Legal-Entity-Id'] = entity
  return header
}

export interface RequestOptions {
  url: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  data?: unknown
  params?: Record<string, unknown>
  header?: Record<string, string>
  timeout?: number
}

function withQuery(url: string, params?: Record<string, unknown>): string {
  if (!params) return url
  const parts: string[] = []
  Object.keys(params).forEach((k) => {
    const v = params[k]
    if (v === undefined || v === null || v === '') return
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
  })
  if (!parts.length) return url
  return `${url}${url.includes('?') ? '&' : '?'}${parts.join('&')}`
}

export function request<T = unknown>(opt: RequestOptions): Promise<ApiResult<T>> {
  return new Promise((resolve, reject) => {
    uni.request({
      url: getBaseUrl() + withQuery(opt.url, opt.params),
      method: opt.method || 'GET',
      data: opt.data as never,
      header: { ...buildHeader(), ...(opt.header || {}) },
      timeout: opt.timeout || 30000,
      success: (res) => {
        if (res.statusCode === 401) {
          removeItem('ch_token')
          removeItem('ch_user')
          uni.reLaunch({ url: '/pages/login/index' })
          reject(new Error('登录已过期'))
          return
        }
        resolve(res.data as ApiResult<T>)
      },
      fail: (err) => {
        uni.showToast({ title: err?.errMsg || '网络异常', icon: 'none' })
        reject(err)
      },
    })
  })
}

export const http = {
  get: <T = unknown>(url: string, opt?: Omit<RequestOptions, 'url' | 'method'>) =>
    request<T>({ url, method: 'GET', ...opt }),
  post: <T = unknown>(url: string, data?: unknown, opt?: Omit<RequestOptions, 'url' | 'method' | 'data'>) =>
    request<T>({ url, method: 'POST', data, ...opt }),
  put: <T = unknown>(url: string, data?: unknown, opt?: Omit<RequestOptions, 'url' | 'method' | 'data'>) =>
    request<T>({ url, method: 'PUT', data, ...opt }),
  del: <T = unknown>(url: string, opt?: Omit<RequestOptions, 'url' | 'method'>) =>
    request<T>({ url, method: 'DELETE', ...opt }),
}
