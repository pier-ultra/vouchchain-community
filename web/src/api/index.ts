import axios from 'axios'
import type { SessionRow } from '../rbac'

// 统一走同源 /api：开发态由 Vite 代理转发，生产态由 Go 后端同源直接提供
// ElMessage 由 unplugin-auto-import 按需注入（含样式），不手动 import
const http = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('ch_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  // dev 态用请求头模拟角色/用户；SSO 态后端只认验签后的 JWT claims，这两个头一律忽略
  config.headers['X-User-Role'] = localStorage.getItem('ch_role') || 'admin'
  const user = localStorage.getItem('ch_user')
  if (user) config.headers['X-User-Name'] = user
  // K3 主体上下文：切换主体后所有业务请求按该主体过滤（服务端也存一份 user_context 兜底）
  const entity = localStorage.getItem('ch_entity')
  if (entity && entity !== '0') config.headers['X-Legal-Entity-Id'] = entity
  return config
})

http.interceptors.response.use(
  (resp) => {
    const body = resp.data
    // V3 业务码非 0（HTTP 200 但业务失败）：统一提示，暴露页面静默失败；
    // 不 reject，保持页面自行处理 code（如 /chat 的 code=2 未配置模型，页面会自渲染提示）
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      const msg = typeof body.message === 'string' ? body.message : ''
      if (msg) {
        console.warn('[api] 业务失败', body.code, msg)
        ElMessage.warning(msg)
      }
    }
    return body
  },
  (err) => {
    const status = err?.response?.status
    if (status === 401) {
      // token 失效：清掉本地会话并回登录页；登录页自身的公开接口不触发
      localStorage.removeItem('ch_token')
      localStorage.removeItem('ch_user')
      if (!location.hash.startsWith('#/login')) location.hash = '#/login'
    }
    const body = err?.response?.data
    const msg = typeof body === 'string' ? body : body?.message
    ElMessage.error(msg || err.message || '请求失败')
    return Promise.reject(err)
  },
)

export interface ApiResult<T = unknown> {
  code: number
  data?: T
  message?: string
}

// SSO 登录页配置公开端点；不返回证书内容或密钥
export interface AuthConfigRow {
  devMode: boolean
  ssoEnabled: boolean
  endpoint: string
  clientId: string
  orgName: string
  appName: string
  certConfigured: boolean
  redirectUri: string
  authorizeBase: string
}
export interface AuthCallbackRow {
  token: string
  username: string
  displayName: string
  roles: string[]
}

export interface Contract {
  id: number
  title: string
  counterParty: string
  counterPartyTaxId: string
  amount: number
  currency: string
  signDate: string
  status: string
  createdAt: string
  updatedAt: string
}

export interface ContractPage {
  list: Contract[]
  total: number
  page: number
  pageSize: number
}

export interface ModelConfig {
  id: number
  name: string
  category: string // llm / ocr / rerank / embedding / vector
  provider: string
  baseUrl: string
  modelName: string
  apiKey: string // 接口返回为脱敏值；录入走表单，服务端 AES-256-GCM 加密入库
  extraConfig?: Record<string, unknown> | null // 扩展参数：dim/protocol/path/timeoutSec/prompt 等活参数
  enabled: boolean
  keyUsable?: boolean // 当前进程能否解开密钥（APP_AES_KEY 缺失时为 false）
  createdAt: string
  updatedAt: string
}

// 外部 MCP server 连接（MCP Client）
export interface McpServer {
  id: number
  name: string
  endpoint: string
  token: string // 接口返回脱敏值；录入走表单，服务端 AES-256-GCM 加密入库
  extraHeaders: string // JSON 字符串，如 {"X-Org":"xxx"}
  description: string
  enabled: boolean
  toolCount: number
  lastStatus: string // idle / ok / error / pending
  lastError: string
  createdAt: string
  updatedAt: string
}

// 外部 MCP 工具预览
export interface McpToolInfo {
  name: string
  description: string
  required: string[]
}

export interface PipelineStage {
  key: string
  label: string
  status: 'planned' | 'running' | 'done' | 'failed'
  progress: number
  msg: string
}

export interface ContractFile {
  id: number
  filename: string
  fileHash: string
  status: 'pending' | 'extracting' | 'done' | 'failed'
  errorMsg: string
  stage: string
  progress: number
  pipeline: PipelineStage[]
  reviewState: 'in_progress' | 'complete'
  createdAt: string
  updatedAt: string
}

export interface ContractTemplateVar {
  key: string
  label: string
  required: boolean
  default: string
}
export interface ContractTemplateRow {
  id: number
  name: string
  category: string
  content: string
  variables: ContractTemplateVar[]
  varKeys: string[]
  remark: string
  createdAt: string
  updatedAt: string
}

export interface LongTermRow {
  id: number
  sourceType: string
  sourceId: number
  version: number
  digestAlg: string
  digest: string
  encryptAlg: string
  encPath: string
  tsToken: string
  tsTime: string
  remark: string
  createdAt: string
}

export interface ContractClause {
  id: number
  fileId: number
  type: string
  summary: string
  quote: string
  risk: string
  reviewStatus: 'unconfirmed' | 'confirmed' | 'doubt'
  reviewRemark: string
  createdAt: string
}

export interface Obligation {
  id: number
  fileId: number
  type: string
  description: string
  owner: string
  dueDate: string
  sourceQuote: string
  status: 'pending' | 'done'
  remark: string
  completedAt: string | null
  overdue: boolean
  dueSoon: boolean
  createdAt: string
  updatedAt: string
}

// AI 生成过程日志事件（SSE event: log）
export interface GenLogEvent {
  step: string
  level: string
  msg: string
  time: string
}
// AI 生成完成后的结果（SSE event: done → data）
export interface GenResultData {
  fileId: number
  docxFileId: number
  filename: string
  size: number
  chars: number
  lines: number
  clauses: number
  status: string
}

export const api = {
  http, // 文件中心等复杂上传/下载场景直接用 axios 实例
  health: () => http.get<never, ApiResult>('/health'),
  // SSO 会话：菜单/角色/按钮权限的唯一来源（生产态需 Bearer token）
  me: () => http.get<never, ApiResult<SessionRow>>('/me'),
  authConfig: () => http.get<never, ApiResult<AuthConfigRow>>('/auth/config'),
  authCallback: (code: string, state: string) =>
    http.post<never, ApiResult<AuthCallbackRow>>('/auth/callback', { code, state }),
  contracts: (params?: { status?: string; keyword?: string; page?: number; pageSize?: number }) =>
    http.get<never, ApiResult<ContractPage>>('/contracts', { params }),
  createContract: (d: Partial<Contract>) => http.post<never, ApiResult>('/contracts', d),
  updateContract: (id: number, d: Partial<Contract>) => http.put<never, ApiResult>(`/contracts/${id}`, d),
  deleteContract: (id: number) => http.delete<never, ApiResult>(`/contracts/${id}`),
  modelConfigs: () => http.get<never, ApiResult<ModelConfig[]>>('/model-config'),
  createModelConfig: (d: Partial<ModelConfig>) => http.post<never, ApiResult>('/model-config', d),
  updateModelConfig: (id: number, d: Partial<ModelConfig>) => http.put<never, ApiResult>(`/model-config/${id}`, d),
  deleteModelConfig: (id: number) => http.delete<never, ApiResult>(`/model-config/${id}`),
  testModelConfig: (d: Partial<ModelConfig> & { id?: number }) => http.post<never, ApiResult<{ elapsedMs?: number; reply?: string; dim?: number; httpStatus?: number }>>('/model-config/test', d),
  // MCP Client：外部 MCP server 连接管理
  mcpServers: () => http.get<never, ApiResult<McpServer[]>>('/mcp-servers'),
  createMcpServer: (d: Partial<McpServer>) => http.post<never, ApiResult<{ id: number }>>('/mcp-servers', d),
  updateMcpServer: (id: number, d: Partial<McpServer>) => http.put<never, ApiResult>(`/mcp-servers/${id}`, d),
  deleteMcpServer: (id: number) => http.delete<never, ApiResult>(`/mcp-servers/${id}`),
  toggleMcpServer: (id: number, enabled: boolean) => http.put<never, ApiResult>(`/mcp-servers/${id}/toggle`, { enabled }),
  testMcpServer: (d: Partial<McpServer> & { id?: number }) => http.post<never, ApiResult<{ tools: number }>>('/mcp-servers/test', d),
  mcpServerTools: (id: number) => http.get<never, ApiResult<McpToolInfo[]>>(`/mcp-servers/${id}/tools`),
  // 合同上传 + AI 抽取状态机（AC1.1-1.6）；template 为 AC1.8 模板；versionOf 为 AC6.2 版本基线
  uploadContractFile: (file: File, template?: string, versionOf?: number) => {
    const fd = new FormData()
    fd.append('file', file)
    if (template) fd.append('template', template)
    if (versionOf) fd.append('versionOf', String(versionOf))
    return http.post<never, ApiResult<{ id: number; filename: string; status: string }>>(
      '/contracts/upload',
      fd,
      { timeout: 60000 },
    )
  },
  contractFiles: () => http.get<never, ApiResult<ContractFile[]>>('/contracts/files'),
  contractFileClauses: (id: number) => http.get<never, ApiResult<ContractClause[]>>(`/contracts/files/${id}/clauses`),
  retryContractFile: (id: number) => http.post<never, ApiResult>(`/contracts/files/${id}/retry`),
  // 条款复核修正（AC2.2/2.3）+ 复核完成门槛（AC2.6）
  updateClause: (
    id: number,
    d: { summary?: string; risk?: string; reviewStatus?: string; reviewRemark?: string },
  ) => http.put<never, ApiResult>(`/contracts/clauses/${id}`, d),
  // 批量复核（AC2.4）
  batchReview: (ids: number[], reviewStatus: string, reviewRemark?: string) =>
    http.post<never, ApiResult<{ count: number }>>('/contracts/clauses/batch-review', {
      ids,
      reviewStatus,
      reviewRemark: reviewRemark || '',
    }),
  completeReview: (id: number) => http.post<never, ApiResult>(`/contracts/files/${id}/complete-review`),
  // 抽取结果导出（AC1.7）——直接走下载链接
  exportClausesUrl: (id: number, format: 'json' | 'xlsx') => `/api/contracts/files/${id}/export?format=${format}`,
  // 义务清单（AC3.1-3.7）
  generateObligations: (fileId: number) =>
    http.post<never, ApiResult<{ count: number }>>(`/contracts/files/${fileId}/obligations/generate`, {}, { timeout: 120000 }),
  obligations: (params?: { owner?: string; status?: string; fileId?: number }) =>
    http.get<never, ApiResult<Obligation[]>>('/obligations', { params }),
  updateObligation: (id: number, d: { owner?: string; dueDate?: string; status?: string; remark?: string }) =>
    http.put<never, ApiResult>(`/obligations/${id}`, d),
  // 证据附件（AC3.6）
  uploadEvidence: (id: number, file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return http.post<never, ApiResult<{ evidenceName: string }>>(`/obligations/${id}/evidence`, fd)
  },
  evidenceUrl: (id: number) => `/api/obligations/${id}/evidence`,
  // AI 生成合同（产出自动进流水线）
  generateContract: (
    params: {
      contractType: string
      partyA: string
      partyB: string
      subject: string
      amount: string
      term: string
      notes: string
    },
    refFile?: File,
  ) => {
    const fd = new FormData()
    Object.entries(params).forEach(([k, v]) => fd.append(k, v))
    if (refFile) fd.append('file', refFile)
    return http.post<never, ApiResult<{ id: number; status: string }>>('/contracts/generate', fd, { timeout: 180000 })
  },
  // AI 生成合同（SSE 流式）：过程日志逐步回调，解决「点一下转圈 180 秒全黑盒」问题。
  // 不走 axios：需要读流；header 手动补齐（与 axios 拦截器一致）
  generateContractStream: (
    params: {
      contractType: string
      partyA: string
      partyB: string
      subject: string
      amount: string
      term: string
      notes: string
    },
    refFile: File | null,
    handlers: {
      onLog?: (e: GenLogEvent) => void
      onDone?: (d: { code: number; data?: GenResultData; message?: string }) => void
      onError?: (msg: string) => void
    },
  ): Promise<void> => {
    const fd = new FormData()
    Object.entries(params).forEach(([k, v]) => fd.append(k, v))
    if (refFile) fd.append('file', refFile)
    const headers: Record<string, string> = { Accept: 'text/event-stream' }
    const token = localStorage.getItem('ch_token')
    if (token) headers.Authorization = `Bearer ${token}`
    headers['X-User-Role'] = localStorage.getItem('ch_role') || 'admin'
    const user = localStorage.getItem('ch_user')
    if (user) headers['X-User-Name'] = user
    const entity = localStorage.getItem('ch_entity')
    if (entity && entity !== '0') headers['X-Legal-Entity-Id'] = entity

    return (async () => {
      let resp: Response
      try {
        resp = await fetch('/api/contracts/generate/stream', { method: 'POST', headers, body: fd })
      } catch (e: any) {
        handlers.onError?.('无法连接服务器：' + (e?.message || String(e)))
        return
      }
      if (!resp.ok || !resp.body) {
        handlers.onError?.('服务器返回异常（HTTP ' + resp.status + '）')
        return
      }
      const reader = resp.body.getReader()
      const dec = new TextDecoder('utf-8')
      let buf = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })
        // SSE 帧以空行分隔；未凑齐一帧的部分留在 buf 等下一块
        let idx = buf.indexOf('\n\n')
        while (idx >= 0) {
          const frame = buf.slice(0, idx)
          buf = buf.slice(idx + 2)
          let event = 'message'
          const dataLines: string[] = []
          frame.split('\n').forEach((line) => {
            if (line.startsWith('event:')) event = line.slice(6).trim()
            else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
          })
          if (!dataLines.length) continue
          let payload: any
          try {
            payload = JSON.parse(dataLines.join('\n'))
          } catch {
            continue
          }
          if (event === 'log') handlers.onLog?.(payload as GenLogEvent)
          else if (event === 'error') handlers.onError?.(payload?.message || '生成失败')
          else if (event === 'done') handlers.onDone?.(payload)
          idx = buf.indexOf('\n\n')
        }
      }
    })()
  },
  // 文件中心在线预览：docx/txt/pdf 正文 → 自包含 HTML（浏览器原生打不开 docx，故服务端转 HTML）
  filePreviewHtml: (id: number) =>
    http.get<never, ApiResult<{ fileId: number; filename: string; chars: number; html: string }>>(
      `/files/${id}/preview-html`,
    ),
  filePreviewUrl: (id: number) => `/api/files/${id}/preview`,
  fileDownloadUrl: (id: number) => `/api/files/${id}/download`,
  // 原文定位（AC1.5）
  fileText: (id: number) =>
    http.get<never, ApiResult<{ id: number; filename: string; text: string }>>(`/contracts/files/${id}/text`),
  // 版本历史（AC6.2）
  fileVersions: (id: number) => http.get<never, ApiResult<ContractFile[]>>(`/contracts/files/${id}/versions`),
  // A6 合同模板库 + 变量套打
  contractTemplates: (params?: { keyword?: string; category?: string }) =>
    http.get<never, ApiResult<ContractTemplateRow[]>>('/contract-templates', { params }),
  contractTemplateCreate: (d: { name: string; category: string; content: string; remark: string; variables: { key: string; label: string; required: boolean; default: string }[] }) =>
    http.post<never, ApiResult<{ id: number }>>('/contract-templates', d),
  contractTemplateUpdate: (id: number, d: { name: string; category: string; content: string; remark: string; variables: { key: string; label: string; required: boolean; default: string }[] }) =>
    http.put<never, ApiResult>(`/contract-templates/${id}`, d),
  contractTemplateDelete: (id: number) => http.delete<never, ApiResult>(`/contract-templates/${id}`),
  contractTemplateRender: (templateId: number, values: Record<string, string>) =>
    http.post<never, ApiResult<{ content: string; missing: string[] }>>('/contract-templates/render', { templateId, values }),
  // A5 长期保存/国密合规封装层
  longTermArchive: (d: { sourceType: string; sourceId: number; srcPath: string; remark: string }) =>
    http.post<never, ApiResult<{ id: number; version: number; digest: string; tsTime: string; encPath: string }>>('/longterm/archive', d),
  longTermList: (params?: { sourceType?: string }) =>
    http.get<never, ApiResult<LongTermRow[]>>('/longterm', { params }),
  longTermVerify: (id: number, srcPath?: string) =>
    http.post<never, ApiResult<{ id: number; digestOk: boolean | null; decryptOk: boolean | null; msg: string }>>(`/longterm/${id}/verify`, { srcPath }),
  // 自然语言查询（AC5）
  nlQuery: (question: string) => http.post<never, ApiResult<QueryAnswer>>('/query', { question }, { timeout: 120000 }),
  // 电子签适配（AC7）
  signCreate: (fileId: number, provider: string) =>
    http.post<never, ApiResult<{ taskId: number; status: string }>>(`/contracts/files/${fileId}/sign`, { provider }),
  signTasks: () => http.get<never, ApiResult<SignTask[]>>('/sign-tasks'),
  signPoll: (id: number) => http.post<never, ApiResult<{ id: number; status: string }>>(`/sign-tasks/${id}/poll`),
  // 审批流（AC6.3/6.4）
  templateCreate: (name: string, nodes: { name: string; approvers: string[]; mode: string; when?: { field: string; op: string; value: string } | null }[]) =>
    http.post<never, ApiResult<{ id: number }>>('/approval-templates', { name, nodes }),
  templateList: () => http.get<never, ApiResult<TemplateRow[]>>('/approval-templates'),
  approvalCreate: (targetId: number, templateId: number, context?: Record<string, unknown>) =>
    http.post<never, ApiResult<{ id: number; status: string; currentSeq: number; skipped: number }>>('/approvals', { targetType: 'contract_file', targetId, templateId, context }),
  approvalList: (params?: { targetId?: number; status?: string; approver?: string }) =>
    http.get<never, ApiResult<ApprovalRow[]>>('/approvals', { params }),
  approvalDetail: (id: number) =>
    http.get<never, ApiResult<{ instance: ApprovalRow; nodes: ApprovalNodeRow[] }>>(`/approvals/${id}`),
  approvalAct: (id: number, actor: string, action: string, comment: string) =>
    http.post<never, ApiResult<{ status: string; currentSeq: number }>>(`/approvals/${id}/act`, { actor, action, comment }),
  approvalAddSign: (id: number, actor: string, to: string, mode: string, comment: string) =>
    http.post<never, ApiResult<{ seq: number; mode: string }>>(`/approvals/${id}/addsign`, { actor, to, mode, comment }),
  approvalTransfer: (id: number, actor: string, to: string, comment: string) =>
    http.post<never, ApiResult<{ approver: string }>>(`/approvals/${id}/transfer`, { actor, to, comment }),
  approvalCancel: (id: number, actor: string, reason: string) =>
    http.post<never, ApiResult<{ status: string }>>(`/approvals/${id}/cancel`, { actor, reason }),
  approvalEvents: (id: number) =>
    http.get<never, ApiResult<ApprovalEventRow[]>>(`/approvals/${id}/events`),
  // 消息中心
  notifications: () => http.get<never, ApiResult<{ list: NotificationRow[]; unread: number }>>('/notifications'),
  notificationRead: (id: number) => http.post<never, ApiResult>(`/notifications/${id}/read`),
  notificationReadAll: () => http.post<never, ApiResult>('/notifications/read-all'),
  // 发票线（US-I1~I8）
  invoiceImport: (file: File, force?: boolean) => {
    const fd = new FormData()
    fd.append('file', file)
    if (force) fd.append('force', '1')
    return http.post<never, ApiResult<{ id: number; invoiceNo: string; total: string } | { duplicate: string; existId: number }>>(
      '/invoices/import', fd, { timeout: 60000 },
    )
  },
  invoices: (params?: { keyword?: string; verifyStatus?: string; archiveStatus?: string; category?: string }) =>
    http.get<never, ApiResult<InvoiceRow[]>>('/invoices', { params }),
  invoiceIntegrity: (id: number) =>
    http.post<never, ApiResult<{ name: string; pass: boolean; msg: string }[]>>(`/invoices/${id}/integrity`),
  invoiceVerify: (id: number, verifyStatus: string) =>
    http.post<never, ApiResult>(`/invoices/${id}/verify`, { verifyStatus }),
  invoiceArchive: (id: number) => http.post<never, ApiResult<{ archivePath: string }>>(`/invoices/${id}/archive`),
  invoiceLinkVoucher: (invoiceId: number, voucherId: number) =>
    http.post<never, ApiResult>(`/invoices/${invoiceId}/link-voucher`, { invoiceId, voucherId }),
  invoiceChain: (invoiceId: number) =>
    http.get<never, ApiResult<{ invoiceId: number; chain: { layer: string; id: number; label: string; broken: boolean }[] }>>(`/invoices/${invoiceId}/chain`),
  invoiceRedFlushScan: () =>
    http.post<never, ApiResult<{ red: number; reversed: number }>>('/invoices/red-flush-scan'),
  invoiceRedFlushAlerts: () =>
    http.get<never, ApiResult<InvoiceRow[]>>('/invoices/red-flush-alerts'),
  voucherCreate: (d: { voucherNo: string; summary: string; amount: number; voucherDate: string }) =>
    http.post<never, ApiResult<{ id: number }>>('/vouchers', d),
  vouchers: () => http.get<never, ApiResult<VoucherRow[]>>('/vouchers'),
  receiptCreate: (d: { payer: string; payee: string; amount: number; receiptDate: string; remark: string }) =>
    http.post<never, ApiResult<{ id: number }>>('/receipts', d),
  receipts: () => http.get<never, ApiResult<ReceiptRow[]>>('/receipts'),
  receiptAutoMatch: () => http.post<never, ApiResult<{ matched: number; unmatched: number }>>('/receipts/auto-match'),
  // P2P 闭环补格：应付台账 + AI 分录草稿（社区版免费）
  payableGenerate: (d: { dueDate?: string; days?: number; onlyVerified?: boolean }) =>
    http.post<never, ApiResult<{ created: number; skipped: number }>>('/payables/generate', d),
  payables: (params?: { status?: string }) =>
    http.get<never, ApiResult<{ list: PayableRow[]; summary: PayableSummary }>>('/payables', { params }),
  payableUpdate: (id: number, d: { dueDate?: string; amount?: number; status?: string; remark?: string; payeeTaxId?: string }) =>
    http.put<never, ApiResult>(`/payables/${id}`, d),
  payablePay: (id: number, d: { paidDate?: string; payer?: string; payeeTaxId?: string; remark?: string }) =>
    http.post<never, ApiResult<{ receiptId: number; status: string }>>(`/payables/${id}/pay`, d),
  accountMappings: () => http.get<never, ApiResult<AccountMappingRow[]>>('/account-mappings'),
  accountMappingSave: (d: { category: string; subjectCode: string; subjectName: string; direction: string }) =>
    http.post<never, ApiResult>('/account-mappings', d),
  journalDraftCreate: (d: { invoiceId: number; category: string; mode: string }) =>
    http.post<never, ApiResult<{ id: number; entries: JournalEntry[]; reason: string; source: string }>>(
      '/journal/drafts', d, { timeout: 120000 },
    ),
  journalDrafts: () => http.get<never, ApiResult<JournalDraftRow[]>>('/journal/drafts'),
  journalDraftUpdate: (id: number, entries: JournalEntry[]) =>
    http.put<never, ApiResult>(`/journal/drafts/${id}`, { entries }),
  journalDraftConfirm: (id: number) =>
    http.post<never, ApiResult<{ voucherId: number; voucherNo: string; amount: number }>>(`/journal/drafts/${id}/confirm`),
  // 原子需求补齐：AI8.4 证据链导出 / AI5.5 归档检索 / AK4.4-4.5 审计检索与导出
  invoiceChainExportUrl: (id: number, format: 'md' | 'xlsx' = 'md') =>
    `/api/invoices/${id}/chain/export?format=${format}`,
  archives: (params?: { keyword?: string; verifyStatus?: string; dateFrom?: string }) =>
    http.get<never, ApiResult<ArchiveRow[]>>('/archives', { params }),
  archiveDownloadUrl: (id: number) => `/api/archives/${id}/download`,
  auditLogs: (params?: { action?: string; targetType?: string; dateFrom?: string }) =>
    http.get<never, ApiResult<AuditLogRow[]>>('/audit-log', { params }),
  auditLogExportUrl: (params?: { action?: string; targetType?: string; dateFrom?: string }) => {
    const q = new URLSearchParams()
    Object.entries(params || {}).forEach(([k, v]) => { if (v) q.set(k, String(v)) })
    return `/api/audit-log/export${q.toString() ? '?' + q.toString() : ''}`
  },
  // ---------- 待办中心（聚合入口） ----------
  todoList: () =>
    http.get<never, ApiResult<TodoListResult>>('/todo'),
  // 一键全流程：从指定发票直接生成付款计划（发票→付款直通车）
  payableGenerateFromInvoices: (d: { invoiceIds: number[]; days?: number }) =>
    http.post<never, ApiResult<{ created: number; skipped: number }>>('/payables/from-invoices', d),

// ---------- 每日工作台 ----------
  // 聚合概览（本周关注 + 进行中追踪 + 下一步建议 + 最近晨/晚总结）
  dailyOverview: () => http.get<never, ApiResult<{ overview: any; lastBrief: { kind: string; text: string; at: string } | null }>>('/daily'),
  // 生成一次晨报 / 晚间总结（主动触发，后端会 WS 广播给在线页面）
  dailyBrief: (kind: 'morning' | 'evening') =>
    http.post<never, ApiResult<{ kind: string; text: string }>>('/daily/brief', { kind }),
  // ---------- Core K3 多组织法人主体 ----------
  legalEntities: () =>
    http.get<never, ApiResult<{ list: LegalEntityRow[]; tree: LegalEntityRow[] }>>('/legal-entities'),
  legalEntityCreate: (d: { code: string; name: string; entityType: string; parentId: number }) =>
    http.post<never, ApiResult<{ id: number }>>('/legal-entities', d),
  legalEntityUpdate: (id: number, d: Partial<LegalEntityRow>) =>
    http.put<never, ApiResult>(`/legal-entities/${id}`, d),
  legalEntityDelete: (id: number) => http.delete<never, ApiResult>(`/legal-entities/${id}`),
  context: () => http.get<never, ApiResult<ContextRow>>('/context'),
  contextSwitch: (legalEntityId: number, includeSubtree?: boolean) =>
    http.post<never, ApiResult>('/context/switch', {
      legalEntityId,
      ...(includeSubtree === undefined ? {} : { includeSubtree }),
    }),
  // ---------- Core K2 组织树（含父子树递归） ----------
  orgTree: (legalEntityId?: number) =>
    http.get<never, ApiResult<{ legalEntityId: number; tree: OrgNodeRow[]; flat: OrgNodeRow[] }>>('/org/tree',
      { params: { legalEntityId } }),
  orgSubtree: (nodeId: number) =>
    http.get<never, ApiResult<OrgSubtreeRow>>(`/org/nodes/${nodeId}/subtree`),
  orgNodeCreate: (d: Partial<OrgNodeRow>) => http.post<never, ApiResult<{ id: number }>>('/org/nodes', d),
  orgNodeUpdate: (id: number, d: Partial<OrgNodeRow>) => http.put<never, ApiResult>(`/org/nodes/${id}`, d),
  orgNodeDelete: (id: number) => http.delete<never, ApiResult>(`/org/nodes/${id}`),
  orgNodeImport: (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return http.post<never, ApiResult<{ created: number; errors: string[] }>>('/org/nodes/import', fd)
  },
  orgMembers: (nodeId?: number) =>
    http.get<never, ApiResult<OrgMemberRow[]>>('/org/members', { params: { nodeId } }),
  orgMemberAttach: (nodeId: number, username: string) =>
    http.post<never, ApiResult>('/org/members', { nodeId, username }),
  orgMemberRemove: (id: number) => http.delete<never, ApiResult>(`/org/members/${id}`),
  // ---------- Core K5 主数据 ----------
  masterDataTypes: () => http.get<never, ApiResult<MasterDataTypeRow[]>>('/master-data/types'),
  masterData: (params?: { mdType?: string; keyword?: string; status?: string }) =>
    http.get<never, ApiResult<MasterDataRow[]>>('/master-data', { params }),
  masterDataCreate: (d: Partial<MasterDataRow>) =>
    http.post<never, ApiResult<{ id: number; mdUid: string }>>('/master-data', d),
  masterDataUpdate: (id: number, d: Partial<MasterDataRow>) =>
    http.put<never, ApiResult>(`/master-data/${id}`, d),
  masterDataDelete: (id: number) => http.delete<never, ApiResult>(`/master-data/${id}`),
  masterDataLinks: (fromId?: number) =>
    http.get<never, ApiResult<MasterDataLinkRow[]>>('/master-data/links', { params: { fromId } }),
  masterDataLinkCreate: (d: { fromId: number; toId: number; relation: string }) =>
    http.post<never, ApiResult>('/master-data/links', d),
  masterDataLinkDelete: (id: number) => http.delete<never, ApiResult>(`/master-data/links/${id}`),
  masterDataExportUrl: (mdType?: string) => `/api/master-data/export${mdType ? '?mdType=' + encodeURIComponent(mdType) : ''}`,
  // ---------- US-I9 归档调度 ----------
  archiveSchedules: () => http.get<never, ApiResult<ArchiveScheduleRow[]>>('/archive-schedules'),
  archiveScheduleCreate: (d: Partial<ArchiveScheduleRow>) =>
    http.post<never, ApiResult<{ id: number; nextRunAt: string }>>('/archive-schedules', d),
  archiveScheduleUpdate: (id: number, d: Partial<ArchiveScheduleRow>) =>
    http.put<never, ApiResult>(`/archive-schedules/${id}`, d),
  archiveScheduleDelete: (id: number) => http.delete<never, ApiResult>(`/archive-schedules/${id}`),
  archiveScheduleToggle: (id: number) => http.post<never, ApiResult>(`/archive-schedules/${id}/toggle`, {}),
  archiveScheduleRun: (id: number) =>
    http.post<never, ApiResult<{ runId: number }>>(`/archive-schedules/${id}/run`, {}),
  archiveScheduleRuns: (scheduleId?: number) =>
    http.get<never, ApiResult<ArchiveScheduleRunRow[]>>('/archive-schedule-runs', { params: { scheduleId } }),
  archiveScheduleRunItems: (id: number) =>
    http.get<never, ApiResult<ArchiveScheduleItemRow[]>>(`/archive-schedule-runs/${id}/items`),
  archiveExceptions: (status?: string) =>
    http.get<never, ApiResult<ArchiveExceptionRow[]>>('/archive-exceptions', { params: { status } }),
  archiveExceptionHandle: (id: number, action: 'retry' | 'ignore', note: string) =>
    http.post<never, ApiResult>(`/archive-exceptions/${id}/handle`, { action, note }),
  // AI 对话（LLM 多轮工具调用可能耗时较长，单独放宽超时；code=2 表示未配置模型）
  chatSend: (d: { sessionId?: number; message: string }) =>
    http.post<never, ApiResult<ChatSendResult> & { code?: number; message?: string }>('/chat', d, { timeout: 180000 }),
  chatSessions: () => http.get<never, ApiResult<ChatSessionRow[]>>('/chat/sessions'),
  chatMessages: (id: number) =>
    http.get<never, ApiResult<ChatMessageRow[]>>(`/chat/sessions/${id}/messages`),
  chatSessionDelete: (id: number) => http.delete<never, ApiResult>(`/chat/sessions/${id}`),
  chatSessionRename: (id: number, title: string) =>
    http.put<never, ApiResult>(`/chat/sessions/${id}`, { title }),
  mcpStatus: () => http.get<never, ApiResult<McpStatus>>('/mcp/status'),

  // ---------- 发票台账：三方匹配 + 四性归档（P0 免费版核心） ----------
  invoiceLedger: (params?: { keyword?: string; matchStatus?: string }) =>
    http.get<never, ApiResult<InvoiceLedgerRow[]>>('/invoices/ledger', { params }),
  invoiceMatchRun: (ids: number[]) =>
    http.post<never, ApiResult<MatchLogRow[]>>('/invoices/match', { ids }),
  invoiceMatchLogs: (id: number) =>
    http.get<never, ApiResult<MatchLogRow[]>>(`/invoices/${id}/match-logs`),
  invoiceAuditRun: (id: number) =>
    http.post<never, ApiResult<AuditLogRow[]>>(`/invoices/${id}/audit`),
  invoiceAuditLogs: (id: number) =>
    http.get<never, ApiResult<AuditLogRow[]>>(`/invoices/${id}/audit-logs`),
  invoiceLedgerExportUrl: (ids: number[], format: 'xlsx' | 'md' = 'xlsx') =>
    `/api/invoices/ledger/export?ids=${ids.join(',')}&format=${format}`,
}

export interface McpStatus {
  enabled: boolean
  endpoint: string
  tools: number
  headers: Record<string, string>
}

// ---------- Core / 归档调度 类型 ----------
export interface LegalEntityRow {
  id: number
  code: string
  name: string
  entityType: string
  parentId: number
  status: string
  invoiceCount: number
  createdAt: string
  // 父子树递归：层级 / 路径 / 直接子节点数 / 含下级主体累计数
  depth?: number
  path?: string
  childCount?: number
  descendantCount?: number
  totalInvoiceCount?: number
  children?: LegalEntityRow[]
}
export interface ContextRow {
  legalEntityId: number
  legalEntityName: string
  entities: LegalEntityRow[]
  includeSubtree?: boolean
  username: string
}
export interface OrgNodeRow {
  id: number
  legalEntityId: number
  parentId: number
  code: string
  name: string
  nodeType: string
  sortOrder: number
  memberCount: number
  totalMemberCount: number
  depth?: number
  path?: string
  subtreeNodes?: number
  children: OrgNodeRow[]
}
export interface OrgSubtreeRow {
  nodeId: number
  nodeName: string
  path: string
  nodeCount: number
  nodes: { id: number; parentId: number; name: string; nodeType: string; depth: number; path: string }[]
  members: { id: number; username: string; nodeId: number; nodeName: string; isDirect: boolean }[]
}
export interface OrgMemberRow {
  id: number
  node_id: number
  legal_entity_id: number
  username: string
  created_at: string
}
export interface MasterDataTypeRow { code: string; name: string; description: string; sort_order: number }
export interface MasterDataRow {
  id: number
  md_uid: string
  md_type: string
  code: string
  name: string
  legal_entity_id: number
  attributes: Record<string, unknown> | string
  status: string
  created_at: string
}
export interface MasterDataLinkRow {
  id: number
  from_id: number
  to_id: number
  relation: string
  from_name: string
  to_name: string
  from_uid: string
  to_uid: string
}
export interface ArchiveScheduleRow {
  id: number
  name: string
  period_type: string
  due_days: number
  scope_type: string
  scope_id: number
  require_verified: boolean
  require_voucher: boolean
  require_paid: boolean
  enabled: boolean
  next_run_at: string
  last_run_at: string
}
export interface ArchiveScheduleRunRow {
  id: number
  schedule_id: number
  trigger: string
  status: string
  total: number
  success: number
  failed: number
  skipped: number
  cutoff_date: string
  message: string
  started_at: string
  finished_at: string
}
export interface ArchiveScheduleItemRow {
  id: number
  run_id: number
  invoice_id: number
  invoice_no: string
  result: string
  message: string
  archive_path: string
}
export interface ArchiveExceptionRow {
  id: number
  run_id: number
  invoice_id: number
  invoice_no: string
  reason: string
  status: string
  handled_by: string
  handled_note: string
  created_at: string
}

export interface ArchiveRow {
  id: number
  invoiceNo: string
  issueDate: string
  sellerName: string
  total: number
  verifyStatus: string
  archivePath: string
  fileHash: string
}
export interface AuditLogRow {
  id: number
  username: string
  action: string
  targetType: string
  targetId: number
  detail: Record<string, unknown>
  ip: string
  createdAt: string
}

export interface PayableSummary {
  plannedAmount: number
  paidAmount: number
  overdueCount: number
}
export interface PayableRow {
  id: number
  invoiceId: number
  invoiceNo: string
  sellerName: string
  verifyStatus: string
  payee: string
  payeeTaxId: string
  amount: number
  dueDate: string
  status: string
  paidDate: string
  receiptId: number
  remark: string
  overdue: boolean
}
export interface AccountMappingRow {
  id: number
  category: string
  subject_code: string
  subject_name: string
  direction: string
}
export interface JournalEntry {
  summary: string
  subjectCode: string
  subjectName: string
  debit: number
  credit: number
}
export interface JournalDraftRow {
  id: number
  invoiceId: number
  invoiceNo: string
  sellerName: string
  total: number
  category: string
  entries: string
  source: string
  reason: string
  status: string
  voucherId: number
  createdAt: string
}
export interface InvoiceRow {
  id: number
  invoice_no: string
  issue_date: string
  buyer_name: string
  seller_name: string
  amount: number
  tax: number
  total: number
  invoice_type: string
  items: string
  file_name: string
  file_hash: string
  verify_status: string
  integrity_report: string
  archive_status: string
  archive_path: string
  voucher_id: number
}
export interface VoucherRow {
  id: number
  voucher_no: string
  summary: string
  amount: number
  voucher_date: string
}
export interface ReceiptRow {
  id: number
  payer: string
  payee: string
  amount: number
  receipt_date: string
  matched_invoice_id: number
  match_rule: string
}

export interface TemplateRow {
  id: number
  name: string
  nodes: string
  createdAt: string
}
export interface ApprovalRow {
  id: number
  target_type: string
  target_id: number
  status: string
  current_seq: number
  created_by: string
  created_at: string
}
export interface ApprovalNodeRow {
  seq: number
  name: string
  approver: string
  mode: string
  status: string
  comment: string
  acted_at: string | null
  when_expr?: string
  origin?: string
}
export interface ApprovalEventRow {
  id: number
  instance_id: number
  seq: number
  actor: string
  action: string
  from_user: string
  to_user: string
  comment: string
  created_at: string
}
export interface NotificationRow {
  id: number
  username: string
  type: string
  title: string
  content: string
  is_read: boolean
  created_at: string
}
export interface DiffSegment {
  type: 'same' | 'add' | 'del'
  text: string
}
export interface RedlineChange {
  del: string
  add: string
}
export interface RedlineSuggestion {
  index: number
  class: 'substantive' | 'wording' | 'none'
  explanation: string
  conservative: string
  neutral: string
  aggressive: string
}
export interface QueryRef {
  fileId: number
  filename: string
  quote: string
}
export interface QueryAnswer {
  answer: string
  refs: QueryRef[]
}
export interface SignTask {
  id: number
  fileId: number
  provider: string
  status: 'created' | 'sent' | 'signed' | 'failed'
  externalId: string
  failMsg: string
  createdAt: string
  updatedAt: string
}


// ---------- 产品② 治理底座：G4 合规 / G6 MDM 去重 / G1 RBAC ----------
export const govApi = {
  // G4 合规策略引擎
  complianceSummary: () => http.get<never, ApiResult<{ total: number; enabled: number; open: number; blocked: number }>>('/compliance/summary'),
  complianceRules: () => http.get<never, ApiResult<ComplianceRuleRow[]>>('/compliance/rules'),
  complianceRuleSave: (d: Partial<ComplianceRuleRow>) => http.post<never, ApiResult<{ id: number; version: number }>>('/compliance/rules', d),
  complianceRuleUpdate: (id: number, d: Partial<ComplianceRuleRow>) => http.put<never, ApiResult<{ version: number }>>(`/compliance/rules/${id}`, d),
  complianceRuleToggle: (id: number) => http.post<never, ApiResult<{ enabled: boolean }>>(`/compliance/rules/${id}/toggle`, {}),
  complianceRuleHistory: (id: number) => http.get<never, ApiResult<{ version: number; snapshot: string; changed_by: string; changed_at: string }[]>>(`/compliance/rules/${id}/history`),
  complianceViolations: (params?: { status?: string; severity?: string; objectType?: string }) =>
    http.get<never, ApiResult<ComplianceViolationRow[]>>('/compliance/violations', { params }),
  complianceViolationHandle: (id: number, action: 'resolved' | 'waived', note: string) =>
    http.post<never, ApiResult>(`/compliance/violations/${id}/handle`, { action, note }),
  complianceCheck: (objectType: string, record: Record<string, unknown>) =>
    http.post<never, ApiResult<{ evaluated: number; failed: number; blocked: boolean; results: ComplianceCheckResult[] }>>('/compliance/check', { objectType, record }),
  // G6 MDM 去重
  mdmSummary: () => http.get<never, ApiResult<{ records: number; pending: number; merged: number; mappings: number }>>('/mdm/summary'),
  mdmRules: () => http.get<never, ApiResult<MdmRuleRow[]>>('/mdm/rules'),
  mdmRuleSave: (d: Partial<MdmRuleRow>) => http.post<never, ApiResult<{ id: number }>>('/mdm/rules', d),
  mdmScan: (ruleId?: number) => http.post<never, ApiResult<{ mdType: string; records: number; compared: number; pairs: number; created: number; threshold: number }>>('/mdm/scan', { ruleId: ruleId || 0 }),
  mdmDuplicates: (status?: string) => http.get<never, ApiResult<MdmPairRow[]>>('/mdm/duplicates', { params: { status } }),
  mdmMerge: (id: number, survivorId: number, note: string) => http.post<never, ApiResult<{ survivorUid: string; linksRedirected: number }>>(`/mdm/duplicates/${id}/merge`, { survivorId, note }),
  mdmIgnore: (id: number, note: string) => http.post<never, ApiResult>(`/mdm/duplicates/${id}/ignore`, { note }),
  mdmMappings: () => http.get<never, ApiResult<MdmMappingRow[]>>('/mdm/mappings'),
  // G1 RBAC 配置化
  roles: () => http.get<never, ApiResult<RoleRow[]>>('/rbac/roles'),
  roleSave: (d: Partial<RoleRow>) => http.post<never, ApiResult<{ id: number }>>('/rbac/roles', d),
  roleDelete: (id: number) => http.delete<never, ApiResult>(`/rbac/roles/${id}`),
  roleToggle: (id: number) => http.post<never, ApiResult>(`/rbac/roles/${id}/toggle`, {}),
  roleBindings: () => http.get<never, ApiResult<RoleBindingRow[]>>('/rbac/bindings'),
  roleBindingSave: (username: string, roleCode: string, action: 'bind' | 'unbind') =>
    http.post<never, ApiResult>('/rbac/bindings', { username, roleCode, action }),
  roleResources: () => http.get<never, ApiResult<{ value: string; method: string; path: string; label: string; kind?: string }[]>>('/rbac/resources'),
  roleEffective: () => http.get<never, ApiResult<{ username: string; headerRole: string; boundRole: string; effectiveRole: string; definition: unknown }>>('/rbac/effective'),
  // SCIM 2.0 身份同步（管理面）
  scimStatus: () => http.get<never, ApiResult<ScimStatusRow>>('/scim/status'),
  scimUsers: () => http.get<never, ApiResult<ScimUserRow[]>>('/scim/users'),
  scimUserPurge: (id: number) => http.delete<never, ApiResult>(`/scim/users/${id}`),
  scimGroups: () => http.get<never, ApiResult<ScimGroupRow[]>>('/scim/groups'),
  scimLogs: (limit = 50) => http.get<never, ApiResult<ScimLogRow[]>>('/scim/logs', { params: { limit } }),
  scimRoleMaps: () => http.get<never, ApiResult<ScimRoleMapRow[]>>('/scim/role-mappings'),
  scimRoleMapSave: (d: Partial<ScimRoleMapRow>) => http.post<never, ApiResult>('/scim/role-mappings', d),
  scimRoleMapDelete: (id: number) => http.delete<never, ApiResult>(`/scim/role-mappings/${id}`),
  // AG5 强隔离
  isoDimensions: () => http.get<never, ApiResult<IsoDimensionRow[]>>('/isolation/dimensions'),
  isoSummary: () => http.get<never, ApiResult<{ total: number; enabled: number; violations: number; open: number }>>('/isolation/summary'),
  isoPolicies: () => http.get<never, ApiResult<IsoPolicyRow[]>>('/isolation/policies'),
  isoPolicySave: (d: Partial<IsoPolicyRow>, id?: number) =>
    id ? http.put<never, ApiResult<{ id: number }>>(`/isolation/policies/${id}`, d)
       : http.post<never, ApiResult<{ id: number }>>('/isolation/policies', d),
  isoPolicyDelete: (id: number) => http.delete<never, ApiResult>(`/isolation/policies/${id}`),
  isoPolicyToggle: (id: number) => http.post<never, ApiResult<{ enabled: boolean }>>(`/isolation/policies/${id}/toggle`, {}),
  isoEffective: () => http.get<never, ApiResult<IsoEffectiveRow>>('/isolation/effective'),
  isoViolations: (status?: string) => http.get<never, ApiResult<IsoViolationRow[]>>('/isolation/violations', { params: { status } }),
  isoViolationHandle: (id: number, status: 'resolved' | 'waived', note: string) =>
    http.post<never, ApiResult>(`/isolation/violations/${id}/handle`, { status, note }),
  // US-C8 相对方画像
  partySummary: () => http.get<never, ApiResult<{ parties: number; attention: number; contracts: number; withObligation: number }>>('/party-profile/summary'),
  partyList: (keyword?: string) => http.get<never, ApiResult<PartyProfileRow[]>>('/party-profiles', { params: { keyword } }),
  partyDetail: (name: string) => http.get<never, ApiResult<{ profile: PartyProfileDetailRow; riskLabel: string; scope: string }>>('/party-profile', { params: { name } }),
  partyFiles: (name: string) => http.get<never, ApiResult<{ id: number; filename: string; counter_party: string; status: string; created_at: string }[]>>('/party-profile/files', { params: { name } }),
  partyLink: (fileId: number, counterParty: string) =>
    http.post<never, ApiResult>('/party-profile/link', { fileId, counterParty }),
}

export interface IsoDimensionRow {
  value: string
  label: string
  valueLabel: string
  numeric: boolean
  children: boolean
}
export interface IsoPolicyRow {
  id: number
  name: string
  dimension: string
  subjectType: string
  subject: string
  allowedValues: string[]
  includeChildren: boolean
  enabled: boolean
  priority: number
  createdBy: string
  createdAt: string
  updatedAt: string
}
export interface IsoEffectiveRow {
  username: string
  role: string
  enforced: boolean
  allowed: Record<string, string[]>
  entityIds: number[]
  deptIds: number[]
  projects: string[]
  matched: number[]
  overrides: string[]
  sourceLabels: Record<string, string>
}
export interface IsoViolationRow {
  id: number
  username: string
  role: string
  method: string
  path: string
  resource: string
  object_id: number
  dimension: string
  attempted_value: string
  policy_ids: string
  status: string
  handled_by: string
  handled_note: string
  created_at: string
}
export interface PartyProfileRow {
  name: string
  contractCount: number
  totalAmount: number
  lastSignDate: string
  riskScore: number
  riskLevel: string
  riskLabel: string
  tags: string[]
  fulfillRate: number
  disputeClauseCount: number
  sampleWarning: string
}
export interface PartyProfileDetailRow {
  counterParty: string
  contractCount: number
  activeCount: number
  rejectedCount: number
  totalAmount: number
  firstSignDate: string
  lastSignDate: string
  obligationTotal: number
  obligationDone: number
  onTimeCount: number
  lateDoneCount: number
  overduePendingCount: number
  fulfillRate: number
  disputeClauseCount: number
  highRiskClauseCount: number
  clauseCategories: { type: string; count: number }[]
  riskScore: number
  riskLevel: string
  tags: string[]
  reasons: string[]
  sampleWarning: string
  contracts: { id: number; title: string; amount: number; status: string; signDate: string }[]
}

export interface ScimStatusRow {
  configured: boolean
  tokenLength: number
  baseUrl: string
  authMode: string
  users: number
  activeUsers: number
  groups: number
  logs: number
  roleMappings: number
  lastSync: { operation: string; resourceType: string; username: string; message: string; httpStatus: number; createdAt: string }
  endpoints: string[]
}
export interface ScimUserRow {
  id: number
  externalId: string
  username: string
  displayName: string
  email: string
  title: string
  department: string
  active: boolean
  groups: string[]
  roles: string[]
  createdAt: string
  lastSyncedAt: string
}
export interface ScimGroupRow {
  id: number
  externalId: string
  displayName: string
  members: string[]
  createdAt: string
}
export interface ScimLogRow {
  id: number
  operation: string
  resourceType: string
  resourceId: string
  username: string
  externalId: string
  httpStatus: number
  message: string
  actor: string
  payload: string
  createdAt: string
}
export interface ScimRoleMapRow {
  id: number
  groupName: string
  roleCode: string
  note: string
}

export interface ComplianceRuleRow {
  id: number
  code: string
  name: string
  description: string
  rule_type: string
  object_type: string
  field: string
  op: string
  rule_value: string
  condition_expr: string
  severity: string
  enabled: boolean
  version: number
  doc: string
}
export interface ComplianceViolationRow {
  id: number
  rule_code: string
  rule_name: string
  object_type: string
  object_id: number
  object_label: string
  field: string
  message: string
  severity: string
  status: string
  owner: string
  handled_by: string
  handled_note: string
  created_at: string
}
export interface ComplianceCheckResult {
  ruleCode: string
  ruleName: string
  severity: string
  passed: boolean
  message: string
  field: string
}
export interface MdmRuleRow {
  id: number
  name: string
  md_type: string
  fields: string
  threshold: number
  enabled: boolean
}
export interface MdmPairRow {
  id: number
  left_id: number
  right_id: number
  left_uid: string
  right_uid: string
  left_name: string
  right_name: string
  score: number
  reasons: string[]
  suggestion: string
  status: string
  handled_note: string
  created_at: string
}
export interface MdmMappingRow {
  id: number
  old_id: number
  old_uid: string
  old_name: string
  survivor_id: number
  survivor_uid: string
  merged_at: string
}
export interface RoleRow {
  id: number
  code: string
  name: string
  description: string
  isBuiltin: boolean
  permissions: string[]
  dataScope: string
  fieldMask: string[]
  enabled: boolean
  userCount: number
}
export interface RoleBindingRow {
  id: number
  username: string
  role_code: string
  created_at: string
}

// ---------- AI 对话助手（对话即操作） ----------
export interface ChatStep {
  tool: string
  args: string
  summary: string
  ok: boolean
}
export interface ChatMessageRow {
  id: number
  role: 'user' | 'assistant'
  content: string
  tool_trace: string // JSON 序列化的 ChatStep[]
  created_at: string
}
export interface ChatSessionRow {
  id: number
  title: string
  created_at: string
  updated_at: string
}
export interface ChatSendResult {
  sessionId: number
  reply: string
  steps: ChatStep[]
}

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

// 待办中心接口类型（定义在 api 对象外部，避免破坏对象字面量结构）
export interface TodoItem {
  id: string
  title: string
  desc: string
  type: 'invoice' | 'contract' | 'approval' | 'journal' | 'payable'
  status: 'pending' | 'urgent' | 'overdue'
  count: number
  url: string
  icon: string
}
export interface TodoListResult {
  list: TodoItem[]
  total: number
}
