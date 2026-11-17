<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { money, dateOnly, contractStatusLabel, statusTone, toneColor } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const keyword = ref('')
const loading = ref(false)
const total = ref(0)
const filter = ref<'all' | 'active' | 'approved' | 'draft' | 'pending'>('all')

const FILTERS: { key: typeof filter.value; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'active', label: '生效中' },
  { key: 'approved', label: '已通过' },
  { key: 'draft', label: '草稿' },
  { key: 'pending', label: '待审' },
]

async function load() {
  loading.value = true
  try {
    const res = await api.contracts({
      keyword: keyword.value.trim() || undefined,
      status: filter.value === 'all' ? undefined : filter.value,
      page: 1,
      pageSize: 50,
    })
    if (res?.code === 0) {
      list.value = res.data?.list || []
      total.value = res.data?.total || 0
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

function onSearch() {
  load()
}

function switchFilter(f: typeof filter.value) {
  if (filter.value === f) return
  filter.value = f
  load()
}

function openDetail(c: Row) {
  uni.navigateTo({ url: `/pages/contract-detail/index?id=${c.id}` })
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

    <view class="search">
      <input
        class="ipt"
        v-model="keyword"
        placeholder="搜索合同标题 / 相对方"
        placeholder-class="ph"
        confirm-type="search"
        @confirm="onSearch"
      />
      <view class="btn" @click="onSearch">搜索</view>
    </view>

    <view v-if="!loading" class="muted total">共 {{ total }} 份合同</view>
    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无合同</view>

    <view v-for="c in list" :key="c.id" class="card" @click="openDetail(c)">
      <view class="row">
        <view class="title">{{ c.title || '未命名合同' }}</view>
        <view class="tag" :style="{ color: toneColor(statusTone(c.status)), borderColor: toneColor(statusTone(c.status)) }">
          {{ contractStatusLabel(c.status) }}
        </view>
      </view>
      <view class="muted">相对方：{{ c.counterParty || '—' }}</view>
      <view class="row2">
        <view class="amount">{{ money(c.amount, c.currency || '¥') }}</view>
        <view class="muted">{{ dateOnly(c.signDate || c.createdAt) }}</view>
      </view>
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
.search {
  display: flex;
  gap: 16rpx;
  margin-bottom: 20rpx;
}
.ipt {
  flex: 1;
  height: 80rpx;
  background: var(--ch-surface);
  border-radius: 12rpx;
  padding: 0 24rpx;
  font-size: 28rpx;
}
.ph {
  color: var(--ch-text-placeholder);
}
.btn {
  width: 140rpx;
  height: 80rpx;
  line-height: 80rpx;
  text-align: center;
  background: var(--ch-accent);
  color: #fff;
  border-radius: 12rpx;
  font-size: 28rpx;
}
.total {
  margin-bottom: 16rpx;
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
