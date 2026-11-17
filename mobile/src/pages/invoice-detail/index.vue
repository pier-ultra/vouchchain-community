<script setup lang="ts">
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { money, dateOnly } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const id = ref(0)
const inv = ref<Row | null>(null)
const loading = ref(true)
const checks = ref<Row[]>([])
const chain = ref<Row[]>([])
const checking = ref(false)

onLoad((opt) => {
  id.value = Number(opt?.id || 0)
  // 列表项通过 eventChannel 传入，避免再请求一次列表
  const pages = getCurrentPages() as any[]
  const cur: any = pages[pages.length - 1]
  if (cur?.getOpenerEventChannel) {
    cur.getOpenerEventChannel().on('invoiceData', (d: Row) => { inv.value = d })
  }
  load()
})

const VERIFY: Record<string, string> = { unverified: '未查验', verified: '已查验', failed: '查验失败', pending: '待查验' }
const ARCHIVE: Record<string, string> = { archived: '已归档', pending: '待归档', none: '未归档' }

async function load() {
  loading.value = true
  try {
    // 详情直接用列表数据；若没带过来则从列表接口按 id 找
    if (!inv.value) {
      const res = await api.invoices()
      if (res?.code === 0) inv.value = (res.data || []).find((v: Row) => Number(v.id) === id.value) || null
    }
  } catch {
    /* ignore */
  } finally {
    loading.value = false
  }
}

async function doIntegrity() {
  checking.value = true
  try {
    const res = await api.invoiceIntegrity(id.value)
    if (res?.code === 0) {
      checks.value = res.data || []
      if (!checks.value.length) uni.showToast({ title: '四性检测完成', icon: 'success' })
    } else {
      uni.showToast({ title: res?.message || '检测失败', icon: 'none' })
    }
  } catch {
    /* ignore */
  } finally {
    checking.value = false
  }
}

async function doChain() {
  try {
    const res = await api.invoiceChain(id.value)
    if (res?.code === 0) chain.value = res.data || []
    else uni.showToast({ title: res?.message || '暂无链路', icon: 'none' })
  } catch {
    /* ignore */
  }
}

function doVerify() {
  const statuses = [
    { label: '已查验', value: 'verified' },
    { label: '查验失败', value: 'failed' },
    { label: '未查验', value: 'unverified' },
  ]
  uni.showActionSheet({
    itemList: statuses.map((s) => s.label),
    success: async (r) => {
      const res = await api.invoiceVerify(id.value, statuses[r.tapIndex].value)
      if (res?.code === 0) {
        if (inv.value) inv.value.verify_status = statuses[r.tapIndex].value
        uni.showToast({ title: '已更新查验状态', icon: 'success' })
      } else {
        uni.showToast({ title: res?.message || '操作失败', icon: 'none' })
      }
    },
  })
}

function doArchive() {
  uni.showModal({
    title: '单套制归档',
    content: '确认将该发票归档？',
    success: async (r) => {
      if (!r.confirm) return
      const res = await api.invoiceArchive(id.value)
      if (res?.code === 0) {
        if (inv.value) inv.value.archive_status = 'archived'
        uni.showToast({ title: '已归档', icon: 'success' })
      } else {
        uni.showToast({ title: res?.message || '归档失败', icon: 'none' })
      }
    },
  })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!inv" class="empty">发票不存在</view>
    <block v-else>
      <view class="card head">
        <view class="row">
          <view class="title">{{ inv.seller_name || inv.sellerName || '未知销方' }}</view>
          <view class="badge">{{ VERIFY[inv.verify_status] || inv.verify_status || '—' }}</view>
        </view>
        <view class="amount">{{ money(inv.total || inv.total_amount) }}</view>
        <view class="muted">发票号：{{ inv.invoice_no || '—' }} · 开票日期：{{ dateOnly(inv.issue_date) }}</view>
        <!-- 关键概览 -->
        <view class="ov-grid">
          <view class="ov">
            <view class="ov-num sm">{{ money(inv.total || inv.total_amount) }}</view>
            <view class="ov-cap">价税合计</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ VERIFY[inv.verify_status] || '—' }}</view>
            <view class="ov-cap">验真状态</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ dateOnly(inv.issue_date) || '—' }}</view>
            <view class="ov-cap">开票日期</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ inv.category || '—' }}</view>
            <view class="ov-cap">分类</view>
          </view>
        </view>
      </view>

      <view class="card">
        <view class="section-title sm">发票明细（八字段）</view>
        <view class="kv"><text class="k">发票号码</text><text class="v">{{ inv.invoice_no || '—' }}</text></view>
        <view class="kv"><text class="k">开票日期</text><text class="v">{{ dateOnly(inv.issue_date) || '—' }}</text></view>
        <view class="kv"><text class="k">购买方</text><text class="v">{{ inv.buyer_name || '—' }}</text></view>
        <view class="kv"><text class="k">销售方</text><text class="v">{{ inv.seller_name || '—' }}</text></view>
        <view class="kv"><text class="k">金额</text><text class="v">{{ money(inv.amount) }}</text></view>
        <view class="kv"><text class="k">税额</text><text class="v">{{ money(inv.tax) }}</text></view>
        <view class="kv"><text class="k">价税合计</text><text class="v">{{ money(inv.total || inv.total_amount) }}</text></view>
        <view class="kv" v-if="inv.category"><text class="k">分类</text><text class="v">{{ inv.category }}</text></view>
      </view>

      <view class="card">
        <view class="section-title sm">操作</view>
        <view class="btn-row">
          <view class="btn" @click="doVerify">查验状态</view>
          <view class="btn" @click="doIntegrity">四性检测</view>
          <view class="btn" @click="doArchive">单套制归档</view>
          <view class="btn" @click="doChain">查看链路</view>
        </view>
      </view>

      <view v-if="checks.length" class="card">
        <view class="section-title sm">四性检测结果</view>
        <view v-for="(c, idx) in checks" :key="idx" class="check-row">
          <text class="c-name">{{ c.name }}</text>
          <text class="c-res" :class="c.pass ? 'pass' : 'fail'">{{ c.pass ? '通过' : '不通过' }}</text>
        </view>
      </view>

      <view v-if="chain.length" class="card">
        <view class="section-title sm">凭证链路</view>
        <view v-for="(c, idx) in chain" :key="idx" class="muted chain-item">{{ c.type || c.kind || '节点' }} · {{ c.no || c.name || c.id || '' }}</view>
      </view>
    </block>
  </view>
</template>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: center; }
.head .title { flex: 1; font-size: 30rpx; font-weight: 500; margin-right: 16rpx; }
.badge { font-size: 24rpx; color: var(--ch-warning); }
.amount { margin: 16rpx 0 8rpx; font-size: 40rpx; font-weight: 500; color: var(--ch-danger); }
.ov-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16rpx; margin-top: 20rpx; }
.ov { background: var(--ch-surface-2); border: 1rpx solid var(--ch-line-soft); border-radius: var(--ch-radius-sm); padding: 20rpx; }
.ov-num { font-size: 34rpx; font-weight: 600; color: var(--ch-accent); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ov-num.sm { font-size: 26rpx; }
.ov-cap { margin-top: 6rpx; font-size: 22rpx; color: var(--ch-text-muted); }
.section-title { font-size: 28rpx; font-weight: 500; margin: 8rpx 0 16rpx; }
.section-title.sm { font-size: 28rpx; }
.kv { display: flex; justify-content: space-between; padding: 14rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.kv:last-child { border-bottom: none; }
.k { color: var(--ch-text-muted); font-size: 26rpx; }
.v { color: var(--ch-ink); font-size: 26rpx; text-align: right; flex: 1; margin-left: 20rpx; }
.btn-row { display: flex; flex-wrap: wrap; gap: 16rpx; }
.btn { background: var(--ch-accent); color: #fff; font-size: 26rpx; padding: 16rpx 24rpx; border-radius: 12rpx; }
.check-row { display: flex; justify-content: space-between; padding: 14rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.check-row:last-child { border-bottom: none; }
.c-name { font-size: 27rpx; }
.c-res { font-size: 26rpx; }
.c-res.pass { color: var(--ch-success); }
.c-res.fail { color: var(--ch-danger); }
.chain-item { padding: 10rpx 0; }
</style>
