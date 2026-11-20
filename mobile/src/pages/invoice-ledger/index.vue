<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type InvoiceLedgerRow } from '../../services/api'
import { money, dateOnly } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<InvoiceLedgerRow[]>([])
const loading = ref(false)
const filter = ref<'all' | 'matched' | 'mismatch' | 'pending' | 'unverified'>('all')

const FILTERS: { key: typeof filter.value; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'matched', label: '已匹配' },
  { key: 'mismatch', label: '不匹配' },
  { key: 'pending', label: '待确认' },
  { key: 'unverified', label: '未匹配' },
]

const MATCH_LABEL: Record<string, string> = {
  matched: '已匹配',
  mismatch: '不匹配',
  pending: '待确认',
  unverified: '未匹配',
}

const MATCH_TONE: Record<string, 'success' | 'danger' | 'warning' | 'info'> = {
  matched: 'success',
  mismatch: 'danger',
  pending: 'warning',
  unverified: 'info',
}

async function load() {
  loading.value = true
  try {
    const params = filter.value === 'all' ? undefined : { matchStatus: filter.value }
    const res = await api.invoiceLedger(params)
    if (res?.code === 0) {
      list.value = res.data || []
    } else {
      list.value = []
      if (res?.message) uni.showToast({ title: res.message, icon: 'none' })
    }
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

onShow(load)

function switchFilter(f: typeof filter.value) {
  if (filter.value === f) return
  filter.value = f
  load()
}

function openDetail(v: InvoiceLedgerRow) {
  uni.navigateTo({
    url: `/pages/invoice-ledger/detail?id=${v.id}`,
    success: (res) => {
      res.eventChannel?.emit('ledgerData', v)
    },
  })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="chips">
      <view
        v-for="f in FILTERS"
        :key="f.key"
        class="chip"
        :class="{ on: filter === f.key }"
        @click="switchFilter(f.key)"
      >{{ f.label }}</view>
    </view>

    <view class="toolbar">
      <view class="caption muted">共 {{ list.length }} 张发票</view>
    </view>

    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无发票</view>

    <view v-for="v in list" :key="v.id" class="card" @click="openDetail(v)">
      <view class="row">
        <view class="title">{{ v.seller_name || '未知销方' }}</view>
        <view class="tag" :class="MATCH_TONE[v.match_status || 'unverified']">
          {{ MATCH_LABEL[v.match_status || 'unverified'] }}
        </view>
      </view>
      <view class="muted">发票号：{{ v.invoice_no || '—' }}</view>
      <view class="row2">
        <view class="amount">{{ money(v.total) }}</view>
        <view class="muted">{{ dateOnly(v.issue_date) }}</view>
      </view>
      <view class="muted">分数：{{ v.match_score || 0 }} · 合同：{{ v.contract_id || '—' }} · 付款：{{ v.payment_plan_id || '—' }}</view>
      <view class="more">查看详情 ›</view>
    </view>
  </view>
</template>

<style scoped>
.chips {
  display: flex;
  gap: 14rpx;
  margin-bottom: 16rpx;
  flex-wrap: wrap;
}
.chip {
  padding: 12rpx 26rpx;
  border-radius: 999rpx;
  font-size: 24rpx;
  color: var(--ch-text);
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-line);
}
.chip.on {
  color: #fff;
  background: var(--ch-accent);
  border-color: var(--ch-accent);
  font-weight: 500;
}
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20rpx;
}
.caption {
  font-size: 26rpx;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.title {
  flex: 1;
  font-size: 30rpx;
  font-weight: 500;
  margin-right: 16rpx;
}
.tag {
  font-size: 22rpx;
  border-radius: 8rpx;
  padding: 2rpx 12rpx;
  white-space: nowrap;
}
.tag.success { background: var(--ch-success); color: #fff; }
.tag.danger { background: var(--ch-danger); color: #fff; }
.tag.warning { background: var(--ch-warning); color: #fff; }
.tag.info { background: var(--ch-info); color: #fff; }
.muted {
  margin-top: 10rpx;
  font-size: 24rpx;
  color: var(--ch-text-muted);
}
.row2 {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 14rpx;
}
.amount {
  font-size: 32rpx;
  font-weight: 500;
  color: var(--ch-danger);
}
.more {
  margin-top: 16rpx;
  text-align: right;
  font-size: 24rpx;
  color: var(--ch-accent);
}
.empty {
  text-align: center;
  padding: 80rpx 0;
  font-size: 28rpx;
  color: var(--ch-text-muted);
}
.page {
  padding: 24rpx;
}
.card {
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-line);
  border-radius: var(--ch-radius);
  padding: 24rpx;
  margin-bottom: 20rpx;
}
</style>