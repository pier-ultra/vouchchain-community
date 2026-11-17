<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const loading = ref(false)
const stats = ref<Row>({ total: 0, todo: 0, overdue: 0, done: 0 })
const srcFilter = ref('')
const prioFilter = ref('')
const srcIdx = ref(0)
const prioIdx = ref(0)
const srcOptions = [{ label: '全部来源', value: '' }, { label: '会议决议', value: 'meeting' }, { label: '领导批示', value: 'directive' }, { label: '其他', value: 'other' }]
const prioOptions = [{ label: '全部优先级', value: '' }, { label: '高', value: 'high' }, { label: '中', value: 'mid' }, { label: '低', value: 'low' }]

function prioLabel(p: string) { return p === 'high' ? '高' : p === 'low' ? '低' : '中' }
function srcLabel(s: string) { return s === 'meeting' ? '会议决议' : s === 'directive' ? '领导批示' : '其他' }
function statusOf(t: Row): string {
  const st = t.derived_status || t.status || ''
  if (st === 'done') return 'done'
  if (st === 'overdue') return 'overdue'
  return 'todo'
}

async function load() {
  loading.value = true
  try {
    const res = await api.dubanTasks({ sourceType: srcFilter.value, priority: prioFilter.value })
    if (res?.code === 0) list.value = res.data || []
    else { list.value = []; if (res?.message) uni.showToast({ title: res.message, icon: 'none' }) }
  } catch { list.value = [] } finally { loading.value = false }
}

async function loadStats() {
  const res = await api.dubanStats()
  if (res?.code === 0) stats.value = res.data || { total: 0, todo: 0, overdue: 0, done: 0 }
}

function onSrc(e: any) { srcIdx.value = Number(e.detail.value); srcFilter.value = srcOptions[srcIdx.value].value; load() }
function onPrio(e: any) { prioIdx.value = Number(e.detail.value); prioFilter.value = prioOptions[prioIdx.value].value; load() }

function remind(t: Row) {
  uni.showModal({
    title: '催办', content: `确认催办「${t.title}」？`, success: async (r) => {
      if (!r.confirm) return
      await api.dubanRemind(t.id, '移动端催办')
      uni.showToast({ title: '已催办', icon: 'success' })
      load()
    },
  })
}

function done(t: Row) {
  uni.showModal({
    title: '办结', content: `确认完成「${t.title}」？`, success: async (r) => {
      if (!r.confirm) return
      await api.dubanDone(t.id, '移动端办结')
      uni.showToast({ title: '已完成', icon: 'success' })
      load(); loadStats()
    },
  })
}

function showLog(t: Row) {
  uni.showLoading({ title: '加载中…', mask: true })
  api.dubanLog(t.id).then((res) => {
    uni.hideLoading()
    const rows = res?.data || []
    const text = rows.length
      ? rows.map((l: Row) => `${(l.created_at || '').slice(5, 16)} ${l.actor || ''} ${l.action || ''}${l.note ? '：' + l.note : ''}`).join('\n')
      : '暂无留痕'
    uni.showModal({ title: '办理留痕', content: text, showCancel: false, confirmText: '关闭' })
  }).catch(() => uni.hideLoading())
}

onShow(() => { load(); loadStats() })
</script>

<template>
  <view class="page" :data-theme="theme">
    <!-- 统计条 -->
    <view class="stats">
      <view class="stat"><text class="num" :style="{ color: 'var(--ch-accent)' }">{{ stats.total || 0 }}</text><text class="cap">总数</text></view>
      <view class="stat"><text class="num" :style="{ color: 'var(--ch-warning)' }">{{ stats.todo || 0 }}</text><text class="cap">待办</text></view>
      <view class="stat"><text class="num" :style="{ color: 'var(--ch-danger)' }">{{ stats.overdue || 0 }}</text><text class="cap">逾期</text></view>
      <view class="stat"><text class="num" :style="{ color: 'var(--ch-success)' }">{{ stats.done || 0 }}</text><text class="cap">完成</text></view>
    </view>

    <!-- 筛选 -->
    <view class="filters">
      <picker :range="srcOptions" range-key="label" :value="srcIdx" @change="onSrc">
        <view class="pick">{{ srcOptions[srcIdx].label }} ▾</view>
      </picker>
      <picker :range="prioOptions" range-key="label" :value="prioIdx" @change="onPrio">
        <view class="pick">{{ prioOptions[prioIdx].label }} ▾</view>
      </picker>
      <view class="refresh" @click="load">刷新</view>
    </view>

    <!-- 任务列表 -->
    <view class="list">
      <view v-if="!loading && !list.length" class="empty">暂无督办任务</view>
      <view v-for="t in list" :key="t.id" class="card">
        <view class="row">
          <text class="title">{{ t.title }}</text>
          <text class="tag" :class="statusOf(t)">{{ statusOf(t) === 'done' ? '已完成' : statusOf(t) === 'overdue' ? '逾期' : '待办' }}</text>
        </view>
        <view class="meta">
          <text class="chip" :class="t.priority === 'high' ? 'ph' : t.priority === 'low' ? 'pl' : 'pm'">{{ prioLabel(t.priority) }}</text>
          <text class="chip">{{ srcLabel(t.source_type) }}</text>
          <text v-if="t.owner" class="chip">责任:{{ t.owner }}</text>
        </view>
        <view class="meta muted">
          <text v-if="t.deadline">期限 {{ (t.deadline || '').slice(0, 10) }}</text>
          <text v-if="t.remind_cnt > 0">催办×{{ t.remind_cnt }}</text>
        </view>
        <view v-if="statusOf(t) !== 'done'" class="acts">
          <text class="act" @click="remind(t)">催办</text>
          <text class="act primary" @click="done(t)">办结</text>
        </view>
        <text class="act link" @click="showLog(t)">查看留痕</text>
      </view>
    </view>
  </view>
</template>

<style scoped>
.stats { display: flex; gap: 16rpx; margin: 24rpx 24rpx 20rpx; }
.stat { flex: 1; background: var(--ch-surface); border: 1rpx solid var(--ch-line-soft); border-radius: var(--ch-radius-card); padding: 24rpx 0; text-align: center; }
.num { font-size: 44rpx; font-weight: 700; display: block; }
.cap { font-size: 22rpx; color: var(--ch-text-muted); margin-top: 6rpx; display: block; }

.filters { display: flex; gap: 16rpx; padding: 0 24rpx; margin-bottom: 20rpx; align-items: center; }
.pick { background: var(--ch-surface); border: 1rpx solid var(--ch-line-soft); border-radius: 12rpx; padding: 12rpx 22rpx; font-size: 24rpx; color: var(--ch-text); }
.refresh { margin-left: auto; color: var(--ch-accent); font-size: 26rpx; padding: 12rpx; }

.list { padding: 0 24rpx 24rpx; }
.card { background: var(--ch-surface); border: 1rpx solid var(--ch-line-soft); border-radius: var(--ch-radius-card); padding: 26rpx; margin-bottom: 20rpx; }
.row { display: flex; align-items: center; justify-content: space-between; }
.title { font-size: 30rpx; font-weight: 600; color: var(--ch-ink); flex: 1; margin-right: 16rpx; }
.tag { font-size: 22rpx; border-radius: 999rpx; padding: 4rpx 16rpx; }
.tag.todo { color: var(--ch-warning); background: var(--ch-warning-soft); }
.tag.overdue { color: var(--ch-danger); background: var(--ch-danger-soft); }
.tag.done { color: var(--ch-success); background: var(--ch-success-soft); }
.meta { display: flex; gap: 12rpx; margin-top: 14rpx; flex-wrap: wrap; }
.chip { font-size: 22rpx; color: var(--ch-text-muted); background: var(--ch-bg); border-radius: 8rpx; padding: 4rpx 14rpx; }
.chip.ph { color: var(--ch-danger); }
.chip.pm { color: var(--ch-warning); }
.chip.pl { color: var(--ch-text-muted); }
.muted { color: var(--ch-text-placeholder); font-size: 22rpx; }
.acts { display: flex; gap: 20rpx; margin-top: 18rpx; }
.act { font-size: 26rpx; color: var(--ch-accent); padding: 8rpx 0; }
.act.primary { color: var(--ch-success); }
.act.link { font-size: 24rpx; color: var(--ch-text-muted); }
.empty { text-align: center; color: var(--ch-text-placeholder); padding: 80rpx 0; font-size: 26rpx; }
</style>
