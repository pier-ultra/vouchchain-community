// 跨端请求封装：与 Web 管理台保持同一套约定——
//   1) 统一走同源 /api（开发态由 vite proxy 转发，生产态由 Go 后端同源提供）
//   2) 请求头带 Bearer token + 角色/用户/主体上下文（SSO 态后端只认验签后的 JWT claims）
//   3) 响应体即业务体（code/data/message），是否成功由调用方判 code
//   4) 401 清会话并跳登录
import { getItem, removeItem } from './storage'

export interface ApiResult<T = unknown> {
  code: number
  data?: T
  message?: string
}

const BASE_URL = '/api'

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
      url: BASE_URL + withQuery(opt.url, opt.params),
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
