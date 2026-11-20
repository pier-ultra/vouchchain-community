<script setup lang="ts">
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { api, type InvoiceLedgerRow, type MatchLogRow, type AuditLogRow } from '../../services/api'
import { money, dateOnly } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const id = ref(0)
const inv = ref<InvoiceLedgerRow | null>(null)
const loading = ref(true)
const matchLogs = ref<MatchLogRow[]>([])
const auditLogs = ref<AuditLogRow[]>([])

const MATCH_LABEL: Record<string, string> = {
  matched: '已匹配',
  mismatch: '不匹配',
  pending: '待确认',
  unverified: '未匹配',
}
const VERIFY_LABEL: Record<string, string> = {
  unverified: '未查验',
  verified: '已查验',
  failed: '查验失败',
}
const ARCHIVE_LABEL: Record<string, string> = {
  archived: '已归档',
  pending: '待归档',
  none: '未归档',
}
const AUDIT_TYPE_LABEL: Record<string, string> = {
  authenticity: '真实性',
  integrity: '完整性',
  compliance: '合规性',
  association: '关联性',
}
const AUDIT_TYPE_COLOR: Record<string, string> = {
  authenticity: '#67C23A',
  integrity: '#409EFF',
  compliance: '#E6A23C',
  association: '#F56C6C',
}

onLoad((opt) => {
  id.value = Number(opt?.id || 0)
  const pages = getCurrentPages() as any[]
  const cur: any = pages[pages.length - 1]
  if (cur?.getOpenerEventChannel) {
    cur.getOpenerEventChannel().on('ledgerData', (d: InvoiceLedgerRow) => { inv.value = d })
  }
  load()
})

async function load() {
  loading.value = true
  try {
    if (!inv.value) {
      const res = await api.invoiceLedger()
      if (res?.code === 0) inv.value = (res.data || []).find((v: InvoiceLedgerRow) => Number(v.id) === id.value) || null
    }
    if (inv.value) {
      const [mRes, aRes] = await Promise.all([
        api.invoiceMatchLogs(id.value),
        api.invoiceAuditLogs(id.value),
      ])
      if (mRes?.code === 0) matchLogs.value = mRes.data || []
      if (aRes?.code === 0) auditLogs.value = aRes.data || []
    }
  } catch {
    /* ignore */
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!inv" class="empty">发票不存在</view>
    <block v-else>
      <view class="card head">
        <view class="row">
          <view class="title">{{ inv.seller_name || '未知销方' }}</view>
          <view class="tag" :style="{ background: AUDIT_TYPE_COLOR.association }">
            {{ MATCH_LABEL[inv.match_status || 'unverified'] }}
          </view>
        </view>
        <view class="amount">{{ money(inv.total) }}</view>
        <view class="muted">发票号：{{ inv.invoice_no || '—' }} · 开票日期：{{ dateOnly(inv.issue_date) }}</view>
        <view class="muted">验真：{{ VERIFY_LABEL[inv.verify_status] || inv.verify_status }} · 归档：{{ ARCHIVE_LABEL[inv.archive_status] || inv.archive_status }}</view>
        <view class="muted">匹配分：{{ inv.match_score || 0 }} · 合同#{{ inv.contract_id || '—' }} · 付款#{{ inv.payment_plan_id || '—' }}</view>
        <view class="muted">销方税号：{{ inv.seller_tax_id || '—' }} · 购方税号：{{ inv.buyer_tax_id || '—' }}</view>
      </view>

      <view class="card">
        <view class="section-title sm">三方匹配日志</view>
        <view v-if="matchLogs.length">
          <view v-for="(m, idx) in matchLogs" :key="idx" class="match-item">
            <view class="match-header">
              <text class="match-rule">{{ m.match_rule || 'default' }}</text>
              <text class="match-status" :style="{ color: statusColor(m.match_status) }">{{ MATCH_LABEL[m.match_status] }}</text>
            </view>
            <view class="match-scores">
              <text>税号：{{ m.score_tax_id }}</text>
              <text>名称：{{ m.score_name }}</text>
              <text>金额：{{ m.score_amount }}</text>
              <text>日期：{{ m.score_date }}</text>
              <text>品目：{{ m.score_item }}</text>
              <text class="total-score">总分：{{ m.total_score }}</text>
            </view>
            <view class="match-meta">
              <text>合同#{{ m.contract_id || '—' }}</text>
              <text>付款#{{ m.payment_plan_id || '—' }}</text>
              <text>{{ m.created_at }}</text>
            </view>
          </view>
        </view>
        <view v-else class="muted">暂无匹配记录</view>
      </view>

      <view class="card">
        <view class="section-title sm">四性归档审计</view>
        <view v-if="auditLogs.length">
          <view v-for="(a, idx) in auditLogs" :key="idx" class="audit-item">
            <view class="audit-header">
              <text class="audit-type" :style="{ color: AUDIT_TYPE_COLOR[a.audit_type] }">{{ AUDIT_TYPE_LABEL[a.audit_type] }}</text>
              <text class="audit-result" :class="a.check_pass ? 'pass' : 'fail'">{{ a.check_pass ? '通过' : '不通过' }}</text>
            </view>
            <view class="audit-fields">{{ a.field_name }}</view>
            <view class="audit-detail">{{ formatDetail(a.check_detail) }}</view>
            <view class="audit-meta">{{ a.operator }} · {{ a.created_at }}</view>
          </view>
        </view>
        <view v-else class="muted">暂无审计记录</view>
      </view>
    </block>
  </view>
</template>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: center; }
.head .title { flex: 1; font-size: 30rpx; font-weight: 500; margin-right: 16rpx; }
.tag { font-size: 24rpx; color: #fff; border-radius: 8rpx; padding: 2rpx 12rpx; }
.amount { margin: 16rpx 0 8rpx; font-size: 40rpx; font-weight: 500; color: var(--ch-danger); }
.muted { font-size: 24rpx; color: var(--ch-text-muted); }
.section-title { font-size: 28rpx; font-weight: 500; margin: 8rpx 0 16rpx; }
.section-title.sm { font-size: 28rpx; }
.card { background: var(--ch-surface); border: 1rpx solid var(--ch-line); border-radius: var(--ch-radius); padding: 24rpx; margin-bottom: 20rpx; }
.match-item, .audit-item { padding: 20rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.match-item:last-child, .audit-item:last-child { border-bottom: none; }
.match-header, .audit-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8rpx; }
.match-rule { font-size: 24rpx; color: var(--ch-accent); }
.match-status { font-size: 24rpx; font-weight: 500; }
.match-scores { display: flex; flex-wrap: wrap; gap: 20rpx; font-size: 22rpx; color: var(--ch-text-muted); margin-bottom: 8rpx; }
.total-score { font-weight: 600; color: var(--ch-ink); }
.match-meta { display: flex; flex-wrap: wrap; gap: 20rpx; font-size: 22rpx; color: var(--ch-text-muted); }
.audit-type { font-size: 26rpx; font-weight: 500; }
.audit-result { font-size: 24rpx; font-weight: 500; }
.audit-result.pass { color: var(--ch-success); }
.audit-result.fail { color: var(--ch-danger); }
.audit-fields { font-size: 22rpx; color: var(--ch-text-muted); margin-bottom: 8rpx; }
.audit-detail { font-size: 20rpx; color: var(--ch-text-muted); background: var(--ch-surface-2); padding: 12rpx; border-radius: var(--ch-radius-sm); margin-bottom: 8rpx; max-height: 200rpx; overflow: auto; }
.audit-meta { font-size: 20rpx; color: var(--ch-text-muted); }
.empty { text-align: center; padding: 80rpx 0; font-size: 28rpx; color: var(--ch-text-muted); }
</style>