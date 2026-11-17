// 平台无关的展示工具（金额 / 日期 / 状态文案与色调）。

export function money(n?: number | string | null, currency = '¥'): string {
  const v = Number(n ?? 0)
  if (Number.isNaN(v)) return `${currency}0.00`
  return `${currency}${v.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function dateOnly(s?: string | null): string {
  return (s || '').slice(0, 10)
}

export function shortTime(s?: string | null): string {
  const t = (s || '').replace('T', ' ')
  return t.slice(0, 16)
}

// 合同状态：与后端 status 取值对齐（保持宽松，未知值原样回显）
const CONTRACT_STATUS: Record<string, string> = {
  draft: '草稿',
  pending: '待审',
  reviewing: '审查中',
  approved: '已通过',
  rejected: '已驳回',
  signed: '已签署',
  archived: '已归档',
  active: '生效中',
}

export function contractStatusLabel(s?: string): string {
  if (!s) return '未知'
  return CONTRACT_STATUS[s] || s
}

export type Tone = 'default' | 'success' | 'warning' | 'danger' | 'primary'

export function statusTone(s?: string): Tone {
  switch (s) {
    case 'approved':
    case 'signed':
    case 'active':
    case 'done':
    case 'verified':
    case 'passed':
      return 'success'
    case 'pending':
    case 'reviewing':
    case 'draft':
    case 'open':
      return 'warning'
    case 'rejected':
    case 'failed':
    case 'overdue':
    case 'blocked':
      return 'danger'
    default:
      return 'default'
  }
}

export function toneColor(t: Tone): string {
  switch (t) {
    case 'success':
      return '#67c23a'
    case 'warning':
      return '#e6a23c'
    case 'danger':
      return '#f56c6c'
    case 'primary':
      return '#409eff'
    default:
      return '#909399'
  }
}

// 主题化 tone：返回 CSS 变量引用，亮/深主题自动适配（与 web 同源）
export function toneVar(t: Tone): string {
  switch (t) {
    case 'success':
      return 'var(--ch-success)'
    case 'warning':
      return 'var(--ch-warning)'
    case 'danger':
      return 'var(--ch-danger)'
    case 'primary':
      return 'var(--ch-accent)'
    default:
      return 'var(--ch-text-muted)'
  }
}
