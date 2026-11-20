import { http, type ApiResult } from './http'
import { getItem } from './storage'
import type { SessionRow } from '../core/roles'

// 宽松行类型：移动端页面以展示/轻操作为主，字段按后端实际返回为准
export type Row = Record<string, any>

// ---------- 发票台账类型 ----------
export interface InvoiceLedgerRow {
  id: number
  invoice_no: string
  issue_date: string
  buyer_name: string
  seller_name: string
  amount: number
  tax: number
  total: number
  invoice_type: string
  verify_status: string
  archive_status: string
  match_status: string
  match_score: number
  contract_id: number
  payment_plan_id: number
  voucher_id: number
  category: string
  legal_entity_id: number
  // 新增税号字段（20261120 迁移补齐）
  seller_tax_id: string
  buyer_tax_id: string
}

export interface MatchLogRow {
  id: number
  invoice_id: number
  contract_id: number
  payment_plan_id: number
  score_tax_id: number
  score_name: number
  score_amount: number
  score_date: number
  score_item: number
  total_score: number
  match_status: string
  match_rule: string
  matched_by: string
  remark: string
  created_at: string
}

export interface AuditLogRow {
  id: number
  invoice_id: number
  audit_type: string
  field_name: string
  expected_hash: string
  actual_hash: string
  check_pass: boolean
  check_detail: Record<string, unknown>
  operator: string
  created_at: string
}

// 请求头构造（SSE/导出等非标准请求复用；与 http.ts buildHeader 同口径）
function authHeader(json = true): Record<string, string> {
  const h: Record<string, string> = {}
  if (json) h['Content-Type'] = 'application/json'
  const token = getItem('ch_token')
  if (token) h.Authorization = `Bearer ${token}`
  h['X-User-Role'] = getItem('ch_role') || 'admin'
  const user = getItem('ch_user')
  if (user) h['X-User-Name'] = user
  const entity = getItem('ch_entity')
  if (entity && entity !== '0') h['X-Legal-Entity-Id'] = entity
  return h
}

// ---------- AI 对话类型（与 Web 端 web/src/api.ts 同口径） ----------
export interface ChatSessionRow {
  id: number
  title: string
  created_at?: string
  updated_at?: string
}
export interface ChatStep {
  tool?: string
  action?: string
  [key: string]: any
}
export interface ChatMessageRow {
  id: number
  role: 'user' | 'assistant'
  content: string
  tool_trace?: string | ChatStep[]
  created_at?: string
}

// 移动端接口清单：与 Web 管理台共用同一个后端 /api。
// 只收录「移动端必做 6 模块 + 会话/通知」，治理配置类接口不在此列（留在 Web）。
export const api = {
  health: () => http.get<ApiResult>('/health'),
  me: () => http.get<ApiResult<SessionRow>>('/me'),

  // ---------- 合同线 ----------
  contracts: (params?: { status?: string; keyword?: string; page?: number; pageSize?: number }) =>
    http.get<ApiResult<{ list: Row[]; total: number; page: number; pageSize: number }>>('/contracts', { params }),
  contractDetail: (id: number) => http.get<ApiResult<Row>>(`/contracts/${id}`),
  // 合同原文 / 版本历史（附件预览，docs §6.1）
  contractFileText: (fileId: number) => http.get<ApiResult<{ text: string }>>(`/contracts/files/${fileId}/text`),
  contractFileVersions: (fileId: number) => http.get<ApiResult<Row[]>>(`/contracts/files/${fileId}/versions`),
  contractFiles: () => http.get<ApiResult<Row[]>>('/contracts/files'),
  obligations: (params?: { fileId?: number }) => http.get<ApiResult<Row[]>>('/obligations', { params }),

  // ---------- 审批流（移动端最刚需） ----------
  approvalList: (params?: { status?: string; approver?: string; targetId?: number }) =>
    http.get<ApiResult<Row[]>>('/approvals', { params }),
  approvalDetail: (id: number) => http.get<ApiResult<{ instance: Row; nodes: Row[] }>>(`/approvals/${id}`),
  approvalAct: (id: number, actor: string, action: string, comment: string) =>
    http.post<ApiResult<{ status: string; currentSeq: number }>>(`/approvals/${id}/act`, { actor, action, comment }),
  approvalEvents: (id: number) => http.get<ApiResult<Row[]>>(`/approvals/${id}/events`),
  // AG3.4：转交 / 加签（docs 治理底座原子需求）
  approvalTransfer: (id: number, actor: string, to: string, comment: string) =>
    http.post<ApiResult>(`/approvals/${id}/transfer`, { actor, to, comment }),
  approvalAddSign: (id: number, actor: string, to: string, mode: string, comment: string) =>
    http.post<ApiResult>(`/approvals/${id}/addsign`, { actor, to, mode, comment }),

  // ---------- 发票线 ----------
  invoices: (params?: { keyword?: string; verifyStatus?: string; archiveStatus?: string; category?: string }) =>
    http.get<ApiResult<Row[]>>('/invoices', { params }),
  invoiceIntegrity: (id: number) => http.post<ApiResult<Row[]>>(`/invoices/${id}/integrity`),
  invoiceVerify: (id: number, verifyStatus: string) =>
    http.post<ApiResult>(`/invoices/${id}/verify`, { verifyStatus }),
  invoiceArchive: (id: number) => http.post<ApiResult<{ archivePath: string }>>(`/invoices/${id}/archive`),
  invoiceChain: (id: number) => http.get<ApiResult<Row[]>>(`/invoices/${id}/chain`),

  // ---------- 发票台账：三方匹配 + 四性归档（P0 免费版核心） ----------
  invoiceLedger: (params?: { keyword?: string; matchStatus?: string }) =>
    http.get<ApiResult<InvoiceLedgerRow[]>>('/invoices/ledger', { params }),
  invoiceMatchRun: (ids: number[]) =>
    http.post<ApiResult<MatchLogRow[]>>('/invoices/match', { ids }),
  invoiceMatchLogs: (id: number) =>
    http.get<ApiResult<MatchLogRow[]>>(`/invoices/${id}/match-logs`),
  invoiceAuditRun: (id: number) =>
    http.post<ApiResult<AuditLogRow[]>>(`/invoices/${id}/audit`),
  invoiceAuditLogs: (id: number) =>
    http.get<ApiResult<AuditLogRow[]>>(`/invoices/${id}/audit-logs`),
  invoiceLedgerExportUrl: (ids: number[], format: 'xlsx' | 'md' = 'xlsx') =>
    `/api/invoices/ledger/export?ids=${ids.join(',')}&format=${format}`,

  // ---------- 用印线 ----------
  seals: () => http.get<ApiResult<Row[]>>('/seals'),
  sealApplies: (params?: { status?: string; sealId?: number }) => http.get<ApiResult<Row[]>>('/seal-applies', { params }),
  // R-B2/B3：用印申请提交（sealId/reason 必填，copies 默认 1）
  sealApplyCreate: (data: { sealId: number; bizType?: string; bizId?: number; fileId?: number; copies?: number; reason: string }) =>
    http.post<ApiResult<{ id: number; warnings: string[]; approvalInstanceId: number }>>('/seal-applies', data),
  sealApplyApprove: (id: number, approve: boolean, comment: string) =>
    http.post<ApiResult>(`/seal-applies/${id}/approve`, { approve, comment }),
  sealApplyStamp: (id: number) => http.post<ApiResult>(`/seal-applies/${id}/stamp`),
  sealRisks: () => http.get<ApiResult<Row[]>>('/seal-risks'),

  // ---------- 等保证据线 ----------
  dengbaoRetentionStatus: () => http.get<ApiResult<Row>>('/dengbao/retention/status'),
  dengbaoPermChain: (params?: { limit?: number }) =>
    http.get<ApiResult<Row[]>>('/dengbao/perm-chain', { params }),
  dengbaoEvidenceBlocks: () => http.get<ApiResult<Row[]>>('/dengbao/evidence/blocks'),

  // ---------- 公文督办线（v23 线D，移动端补入口） ----------
  dubanTasks: (params?: { sourceType?: string; priority?: string }) =>
    http.get<ApiResult<Row[]>>('/duban/tasks', { params }),
  dubanBoard: (params?: { sourceType?: string; priority?: string }) =>
    http.get<ApiResult<Record<string, Row[]>>>('/duban/board', { params }),
  dubanRemind: (id: number, note: string) =>
    http.post<ApiResult>(`/duban/tasks/${id}/remind`, { note }),
  dubanDone: (id: number, note: string) =>
    http.post<ApiResult>(`/duban/tasks/${id}/done`, { note }),
  dubanLog: (id: number) => http.get<ApiResult<Row[]>>(`/duban/tasks/${id}/log`),
  dubanStats: () => http.get<ApiResult<Row>>('/duban/stats'),

  // ---------- 收入确认线 ----------
  revrecList: () => http.get<ApiResult<Row[]>>('/revrec/list'),
  // R-A8：AI/规则只出建议，会计确认才落账
  revrecConfirm: (id: number) => http.post<ApiResult<{ draftId: number }>>(`/revrec/confirm/${id}`),
  revrecLedger: () => http.get<ApiResult<Row[]>>('/revrec/ledger'),

  // ---------- AI 合同生成 ----------
  // POST /api/contracts/generate {contractType, partyA, partyB, subject, amount, term, notes} + 可选文件
  // 返回 {id: number} 合同文件任务ID
  generateContract: (data: {
    contractType: string
    partyA: string
    partyB: string
    subject: string
    amount: string
    term: string
    notes: string
  }, file?: File) => {
    const form = new FormData()
    Object.entries(data).forEach(([k, v]) => form.append(k, v))
    if (file) form.append('file', file)
    return http.post<ApiResult<{ id: number }>>('/contracts/generate', form, {
      timeout: 180000,
      // multipart 不设 Content-Type，由框架自动带 boundary
      headers: { 'Content-Type': undefined as any },
    })
  },

  // ---------- 合同模板库（移动端只读 + 套打） ----------
  contractTemplates: (params?: { keyword?: string }) =>
    http.get<ApiResult<{ id: number; name: string; category: string; content: string; remark: string; variables: Array<{ key: string; label: string; required: boolean; default: string }>; varKeys: string[] }[]>>('/contract-templates', { params }),
  contractTemplateRender: (id: number, values: Record<string, string>) =>
    http.post<ApiResult<{ content: string; missing: string[] }>>(`/contract-templates/render`, { templateId: id, values }),

  // ---------- AI 对话（非流式 /chat，三端通吃；SSE 流式仅 Web 用） ----------
  // 后端无 sessionId 时自动新建会话；code=2 表示未配置 AI 模型
  chatSend: (sessionId: number | undefined, message: string) =>
    http.post<ApiResult<{ sessionId: number; reply: string; steps: ChatStep[] }>>(
      '/chat', { sessionId: sessionId || 0, message }, { timeout: 120000 },
    ),
  chatSessions: () => http.get<ApiResult<ChatSessionRow[]>>('/chat/sessions'),
  chatMessages: (id: number) => http.get<ApiResult<ChatMessageRow[]>>(`/chat/sessions/${id}/messages`),
  chatSessionRename: (id: number, title: string) =>
    http.put<ApiResult>(`/chat/sessions/${id}`, { title }),
  chatSessionDelete: (id: number) => http.del<ApiResult>(`/chat/sessions/${id}`),

  // ---------- 通知中心 ----------
  notifications: () => http.get<ApiResult<{ list: Row[]; unread: number }>>('/notifications'),
  notificationRead: (id: number) => http.post<ApiResult>(`/notifications/${id}/read`),
  notificationReadAll: () => http.post<ApiResult>('/notifications/read-all'),
}

// ---------- 导出 / 文件下载（跨端）----------
// 后端导出接口（等保证据包、台账、审计日志等）返回二进制流。
// H5：带鉴权头 fetch → blob → a 标签下载；小程序/App：uni.downloadFile → saveFile + openDocument。
export interface ExportResult {
  ok: boolean
  message?: string
}

export function exportFile(url: string, fileName: string, params?: Record<string, unknown>): Promise<ExportResult> {
  const full = '/api' + withQuery(url, params)
  // #ifdef H5
  return (async () => {
    try {
      const header: Record<string, string> = {}
      const token = getItem('ch_token')
      if (token) header.Authorization = `Bearer ${token}`
      header['X-User-Role'] = getItem('ch_role') || 'admin'
      const user = getItem('ch_user')
      if (user) header['X-User-Name'] = user
      const resp = await fetch(full, { headers: header })
      if (!resp.ok) return { ok: false, message: `导出失败（HTTP ${resp.status}）` }
      const blob = await resp.blob()
      const href = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = href
      a.download = fileName
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(href)
      return { ok: true }
    } catch {
      return { ok: false, message: '导出失败：网络异常' }
    }
  })()
  // #endif
  // #ifndef H5
  return new Promise((resolve) => {
    const header: Record<string, string> = {}
    const token = getItem('ch_token')
    if (token) header.Authorization = `Bearer ${token}`
    header['X-User-Role'] = getItem('ch_role') || 'admin'
    uni.downloadFile({
      url: full,
      header,
      success: (res) => {
        if (res.statusCode !== 200) {
          resolve({ ok: false, message: `导出失败（HTTP ${res.statusCode}）` })
          return
        }
        // #ifdef APP-PLUS
        uni.saveFile({
          tempFilePath: res.tempFilePath,
          success: (s) => resolve({ ok: true, message: `已保存到 ${s.savedFilePath}` }),
          fail: () => resolve({ ok: false, message: '保存失败' }),
        })
        // #endif
        // #ifdef MP-WEIXIN
        uni.openDocument({
          filePath: res.tempFilePath,
          fileType: (fileName.split('.').pop() || '').toLowerCase() as never,
          showMenu: true,
          success: () => resolve({ ok: true }),
          fail: () => resolve({ ok: true, message: '已下载，请在聊天文件中查看' }),
        })
        // #endif
      },
      fail: () => resolve({ ok: false, message: '导出失败：网络异常' }),
    })
  })
  // #endif
}

// withQuery 的本地引用（避免循环依赖 http.ts，逻辑与其一致）
function withQuery(url: string, params?: Record<string, unknown>): string {
  if (!params) return url
  const parts: string[] = []
  Object.keys(params).forEach((k) => {
    const v = params[k]
    if (v === undefined || v === null || v === '') return
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
  })
  return parts.length ? `${url}${url.includes('?') ? '&' : '?'}${parts.join('&')}` : url
}

// ---------- 文件上传（uni.uploadFile 不走 http.request，独立封装） ----------
// 数电票为 OFD/XML 电子文件：POST /api/invoices/import，multipart 字段名 file，重复可带 force=1。
// 注意：不能带 Content-Type（multipart boundary 由框架生成）；header 只保留鉴权/上下文。
export function uploadInvoice(
  filePath: string,
  fileName: string,
  force: boolean,
): Promise<ApiResult<{ id: number; invoiceNo: string; total: string }>> {
  return new Promise((resolve, reject) => {
    const header: Record<string, string> = {}
    const token = getItem('ch_token')
    if (token) header.Authorization = `Bearer ${token}`
    header['X-User-Role'] = getItem('ch_role') || 'admin'
    const user = getItem('ch_user')
    if (user) header['X-User-Name'] = user
    const entity = getItem('ch_entity')
    if (entity && entity !== '0') header['X-Legal-Entity-Id'] = entity

    uni.uploadFile({
      url: '/api/invoices/import',
      filePath,
      name: 'file',
      formData: force ? { force: '1' } : {},
      header,
      timeout: 60000,
      success: (res) => {
        try {
          resolve(JSON.parse(res.data) as ApiResult<{ id: number; invoiceNo: string; total: string }>)
        } catch {
          reject(new Error('上传响应解析失败'))
        }
      },
      fail: (err) => {
        uni.showToast({ title: err?.errMsg || '上传失败', icon: 'none' })
        reject(err)
      },
    })
  })
}

// ---------- AI 对话 SSE 流式（event: start/tool/token/done/error）----------
export interface ChatStreamHandlers {
  onStart?: (sessionId: number) => void
  onToken?: (text: string) => void
  onTool?: (payload: Row) => void
  onDone?: (reply: string, steps: ChatStep[], sessionId: number) => void
  onError?: (message: string) => void
  onClose?: () => void
}

// 通用 SSE 帧解析器：跨 chunk 缓冲，按空行切帧，逐帧回调
function makeSSEParser(handlers: ChatStreamHandlers) {
  let buf = ''
  return (chunk: string) => {
    buf += chunk
    let pos: number
    while ((pos = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, pos)
      buf = buf.slice(pos + 2)
      let event = 'message'
      let data = ''
      for (const line of frame.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('data:')) data += line.slice(5).trimStart()
      }
      if (!data) continue
      let payload: Row
      try { payload = JSON.parse(data) } catch { continue }
      switch (event) {
        case 'start': handlers.onStart?.(Number(payload.sessionId || 0)); break
        case 'token': handlers.onToken?.(String(payload.t || '')); break
        case 'tool': handlers.onTool?.(payload); break
        case 'done':
          handlers.onDone?.(String(payload.reply || ''), (payload.steps || []) as ChatStep[], Number(payload.sessionId || 0))
          break
        case 'error': handlers.onError?.(String(payload.message || '生成失败')); break
      }
    }
  }
}

// 返回取消函数；调一次即中断流
export function chatStream(
  sessionId: number | undefined,
  message: string,
  handlers: ChatStreamHandlers,
): () => void {
  const body = JSON.stringify({ sessionId: sessionId || 0, message })
  const feed = makeSSEParser(handlers)
  let closed = false
  const close = () => {
    if (closed) return
    closed = true
    handlers.onClose?.()
  }

  // #ifdef H5
  const ac = new AbortController()
  ;(async () => {
    try {
      const resp = await fetch('/api/chat/stream', {
        method: 'POST', headers: authHeader(), body, signal: ac.signal,
      })
      if (!resp.ok || !resp.body) {
        handlers.onError?.(`HTTP ${resp.status}`); close(); return
      }
      const reader = resp.body.getReader()
      const decoder = new TextDecoder('utf-8')
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        feed(decoder.decode(value, { stream: true }))
      }
    } catch (e: any) {
      if (e?.name !== 'AbortError') handlers.onError?.(e?.message || '网络异常')
    } finally {
      close()
    }
  })()
  return () => ac.abort()
  // #endif

  // #ifndef H5
  const task = uni.request({
    url: '/api/chat/stream',
    method: 'POST',
    header: authHeader(),
    data: { sessionId: sessionId || 0, message } as never,
    enableChunked: true,
    responseType: 'arraybuffer',
    success: () => { /* 流结束 */ },
    fail: (err) => { if (!closed) { handlers.onError?.(err?.errMsg || '网络异常'); close() } },
    complete: () => close(),
  })
  task.onChunkReceived((res) => {
    if (closed) return
    try {
      const ab = res.data as ArrayBuffer
      const decoder = new TextDecoder('utf-8')
      feed(decoder.decode(ab, { stream: true }))
    } catch (e: any) {
      handlers.onError?.(e?.message || '解码失败')
    }
  })
  return () => { try { task.abort() } catch { /* ignore */ } close() }
  // #endif
}
