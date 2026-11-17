<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { shortTime } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const loading = ref(false)
const filter = ref<'running' | 'approved' | 'all'>('running')

const FILTERS: { key: typeof filter.value; label: string }[] = [
  { key: 'running', label: '待我审批' },
  { key: 'approved', label: '已通过' },
  { key: 'all', label: '全部' },
]

const STATUS: Record<string, string> = {
  running: '审批中',
  pending: '待审批',
  approved: '已通过',
  rejected: '已驳回',
  revoked: '已废止',
}

async function load() {
  loading.value = true
  try {
    const res = await api.approvalList(filter.value === 'all' ? undefined : { status: filter.value })
    if (res?.code === 0) list.value = res.data || []
    else {
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

function switchFilter(t: typeof filter.value) {
  if (filter.value === t) return
  filter.value = t
  load()
}

function openDetail(item: Row) {
  uni.navigateTo({ url: `/pages/approval-detail/index?id=${item.id}` })
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

    <view class="count muted">{{ list.length }} 条</view>

    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无审批</view>

    <view v-for="item in list" :key="item.id" class="card" @click="openDetail(item)">
      <view class="row">
        <view class="title">{{ item.target_type === 'contract_file' ? '合同文件' : item.target_type }} #{{ item.target_id }}</view>
        <view class="status" :class="item.status">{{ STATUS[item.status] || item.status }}</view>
      </view>
      <view class="muted">发起人：{{ item.created_by || '—' }} · {{ shortTime(item.created_at) }}</view>
      <view class="muted">当前节点：第 {{ item.current_seq }} 级</view>
      <view class="more">查看详情 / 转交 / 加签 ›</view>
    </view>
  </view>
</template>

<style scoped>
.chips {
  display: flex;
  gap: 16rpx;
  margin-bottom: 16rpx;
}
.chip {
  padding: 14rpx 30rpx;
  border-radius: 999rpx;
  font-size: 26rpx;
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
.count {
  margin-bottom: 16rpx;
  padding-left: 4rpx;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.title {
  font-size: 30rpx;
  font-weight: 500;
}
.status {
  font-size: 24rpx;
  color: var(--ch-text-muted);
}
.status.approved {
  color: var(--ch-success);
}
.status.rejected {
  color: var(--ch-danger);
}
.status.pending {
  color: var(--ch-warning);
}
.muted {
  margin-top: 8rpx;
}
.acts {
  display: flex;
  gap: 20rpx;
  margin-top: 24rpx;
}
.act {
  flex: 1;
  text-align: center;
  padding: 20rpx 0;
  border-radius: 12rpx;
  font-size: 28rpx;
}
.act.ok {
  background: var(--ch-success);
  color: #fff;
}
.act.no {
  background: var(--ch-surface);
  color: var(--ch-danger);
  border: 1rpx solid var(--ch-danger);
}
.more {
  margin-top: 16rpx;
  text-align: right;
  font-size: 24rpx;
  color: var(--ch-accent);
}
</style>
