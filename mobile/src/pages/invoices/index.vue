<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, uploadInvoice, type Row } from '../../services/api'
import { money, dateOnly, statusTone, toneColor } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const loading = ref(false)
const filter = ref<'all' | 'verified' | 'unverified' | 'failed'>('all')

const FILTERS: { key: typeof filter.value; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'verified', label: '已查验' },
  { key: 'unverified', label: '未查验' },
  { key: 'failed', label: '查验失败' },
]

const VERIFY: Record<string, string> = {
  unverified: '未查验',
  verified: '已查验',
  failed: '查验失败',
  pending: '待查验',
}

async function load() {
  loading.value = true
  try {
    const res = await api.invoices(filter.value === 'all' ? undefined : { verifyStatus: filter.value })
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

function pickInvoice() {
  // 数电票是 OFD/XML 电子文件（后端只收文件，不收照片）：
  // 微信端从「聊天/文件」选，其余端走文件选择器，再 multipart 上传。
  pickInvoiceFile().then((f) => {
    if (f) uploadWithForce(f, false)
  })
}

function pickInvoiceFile(): Promise<{ path: string; name: string } | null> {
  return new Promise((resolve) => {
    // #ifdef MP-WEIXIN
    ;(uni as any).chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['xml', 'ofd'],
      success: (r: any) => {
        const f = r.tempFiles && r.tempFiles[0]
        resolve(f ? { path: f.path, name: f.name || 'invoice.xml' } : null)
      },
      fail: () => resolve(null),
    })
    // #endif
    // #ifndef MP-WEIXIN
    ;(uni as any).chooseFile({
      count: 1,
      type: 'file',
      extension: ['xml', 'ofd'],
      success: (r: any) => {
        const f = r.tempFiles && r.tempFiles[0]
        resolve(f ? { path: f.path, name: f.name || 'invoice.xml' } : null)
      },
      fail: () => resolve(null),
    })
    // #endif
  })
}

async function uploadWithForce(file: { path: string; name: string }, force: boolean) {
  uni.showLoading({ title: '上传中…', mask: true })
  let res: Awaited<ReturnType<typeof uploadInvoice>>
  try {
    res = await uploadInvoice(file.path, file.name, force)
  } catch {
    uni.hideLoading()
    return
  }
  uni.hideLoading()
  if (res.code === 0) {
    uni.showToast({ title: '导入成功', icon: 'success' })
    load()
    return
  }
  if (res.code === 2) {
    const dup = res.data?.duplicate === 'invoiceNo'
    uni.showModal({
      title: '重复发票',
      content: dup ? (res.message || '该发票号已存在') + '\n请确认是否强制放行？' : res.message || '该文件已导入过',
      showCancel: dup,
      confirmText: dup ? '强制放行' : '知道了',
      success: (m) => {
        if (m.confirm && dup) uploadWithForce(file, true)
      },
    })
    return
  }
  uni.showToast({ title: res.message || '导入失败', icon: 'none' })
}

function openDetail(v: Row) {
  uni.navigateTo({
    url: `/pages/invoice-detail/index?id=${v.id}`,
    success: (res) => {
      res.eventChannel?.emit('invoiceData', v)
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
      <view class="upload" @click="pickInvoice">＋ 导入发票</view>
    </view>

    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无发票</view>

    <view v-for="v in list" :key="v.id" class="card" @click="openDetail(v)">
      <view class="row">
        <view class="title">{{ v.seller_name || v.sellerName || '未知销方' }}</view>
        <view class="tag" :style="{ color: toneColor(statusTone(v.verify_status)), borderColor: toneColor(statusTone(v.verify_status)) }">
          {{ VERIFY[v.verify_status] || v.verify_status || '—' }}
        </view>
      </view>
      <view class="muted">发票号：{{ v.invoice_no || '—' }}</view>
      <view class="row2">
        <view class="amount">{{ money(v.total) }}</view>
        <view class="muted">{{ dateOnly(v.issue_date) }}</view>
      </view>
      <view class="more">查看详情 / 查验 / 归档 ›</view>
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
.upload {
  background: var(--ch-accent);
  color: #fff;
  font-size: 26rpx;
  padding: 14rpx 28rpx;
  border-radius: 12rpx;
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
  border: 1rpx solid;
  border-radius: 8rpx;
  padding: 2rpx 12rpx;
  white-space: nowrap;
}
.muted {
  margin-top: 10rpx;
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
</style>
