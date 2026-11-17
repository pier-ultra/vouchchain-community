<script setup lang="ts">
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { money, dateOnly, contractStatusLabel, statusTone, toneColor } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const id = ref(0)
const detail = ref<Row | null>(null)
const loading = ref(true)
const tab = ref<'info' | 'text' | 'versions'>('info')
const fileText = ref('')
const versions = ref<Row[]>([])
const subLoading = ref(false)

onLoad((opt) => {
  id.value = Number(opt?.id || 0)
  loadDetail()
})

async function loadDetail() {
  loading.value = true
  try {
    const res = await api.contractDetail(id.value)
    if (res?.code === 0) detail.value = res.data || null
    else uni.showToast({ title: res?.message || '加载失败', icon: 'none' })
  } catch {
    /* ignore */
  } finally {
    loading.value = false
  }
}

// 合同原文 / 版本：后端按 contract_file.id 归档，详情里可能带 fileId；没有则用合同 id 尝试
function fileId(): number {
  const d: any = detail.value || {}
  return Number(d.fileId ?? d.file_id ?? d.contractFileId ?? d.id ?? id.value)
}

async function loadText() {
  if (fileText.value) return
  subLoading.value = true
  try {
    const res = await api.contractFileText(fileId())
    if (res?.code === 0) fileText.value = res.data?.text || ''
    else uni.showToast({ title: res?.message || '暂无原文', icon: 'none' })
  } catch {
    /* ignore */
  } finally {
    subLoading.value = false
  }
}

async function loadVersions() {
  if (versions.value.length) return
  subLoading.value = true
  try {
    const res = await api.contractFileVersions(fileId())
    if (res?.code === 0) versions.value = res.data || []
    else uni.showToast({ title: res?.message || '暂无版本', icon: 'none' })
  } catch {
    /* ignore */
  } finally {
    subLoading.value = false
  }
}

function switchTab(t: 'info' | 'text' | 'versions') {
  tab.value = t
  if (t === 'text') loadText()
  else if (t === 'versions') loadVersions()
}

function showText(text: string) {
  // 原文可能很长，用弹窗展示（跨端一致）
  uni.showModal({ title: '合同原文', content: text.slice(0, 2000) || '（空）', showCancel: false })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view v-if="loading" class="empty">加载中…</view>
    <block v-else-if="detail">
      <view class="card head">
        <view class="row">
          <view class="title">{{ detail.title || '未命名合同' }}</view>
          <view class="tag" :style="{ color: toneColor(statusTone(detail.status)), borderColor: toneColor(statusTone(detail.status)) }">
            {{ contractStatusLabel(detail.status) }}
          </view>
        </view>
        <view class="muted">相对方：{{ detail.counterParty || detail.counterparty || '—' }}</view>
        <!-- 关键概览：金额 / 状态 / 签署日 / 编号 四格 -->
        <view class="ov-grid">
          <view class="ov">
            <view class="ov-num">{{ money(detail.amount, detail.currency || '¥') }}</view>
            <view class="ov-cap">合同金额</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ contractStatusLabel(detail.status) }}</view>
            <view class="ov-cap">当前状态</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ dateOnly(detail.signDate || detail.createdAt) || '—' }}</view>
            <view class="ov-cap">签署日期</view>
          </view>
          <view class="ov">
            <view class="ov-num sm">{{ detail.contractNo || detail.no || '—' }}</view>
            <view class="ov-cap">合同编号</view>
          </view>
        </view>
      </view>

      <view class="tabs">
        <view class="tab" :class="{ on: tab === 'info' }" @click="switchTab('info')">基本信息</view>
        <view class="tab" :class="{ on: tab === 'text' }" @click="switchTab('text')">原文预览</view>
        <view class="tab" :class="{ on: tab === 'versions' }" @click="switchTab('versions')">版本历史</view>
      </view>

      <view v-if="tab === 'info'" class="card">
        <view class="kv"><text class="k">合同编号</text><text class="v">{{ detail.contractNo || detail.no || '—' }}</text></view>
        <view class="kv"><text class="k">签署日期</text><text class="v">{{ dateOnly(detail.signDate || detail.createdAt) || '—' }}</text></view>
        <view class="kv"><text class="k">币种</text><text class="v">{{ detail.currency || '—' }}</text></view>
        <view class="kv"><text class="k">创建时间</text><text class="v">{{ dateOnly(detail.createdAt) || '—' }}</text></view>
        <view class="kv" v-if="detail.remark"><text class="k">备注</text><text class="v">{{ detail.remark }}</text></view>
      </view>

      <view v-else-if="tab === 'text'" class="card">
        <view v-if="subLoading" class="empty">加载中…</view>
        <block v-else>
          <view v-if="fileText" class="text-preview" @click="showText(fileText)">{{ fileText.slice(0, 120) }}…（点击查看全文）</view>
          <view v-else class="empty">该合同暂无抽取原文</view>
        </block>
      </view>

      <view v-else class="card">
        <view v-if="subLoading" class="empty">加载中…</view>
        <view v-else-if="!versions.length" class="empty">暂无版本记录</view>
        <view v-for="v in versions" :key="v.id || v.version" class="ver-item">
          <view class="row">
            <text class="v-name">{{ v.version || v.label || ('v' + (v.id || '')) }}</text>
            <text class="muted">{{ dateOnly(v.createdAt || v.created_at) }}</text>
          </view>
        </view>
      </view>
    </block>
    <view v-else class="empty">合同不存在或已删除</view>
  </view>
</template>

<style scoped>
.head .title { flex: 1; font-size: 32rpx; font-weight: 500; margin-right: 16rpx; }
.row { display: flex; justify-content: space-between; align-items: center; }
.tag { font-size: 22rpx; border: 1rpx solid; border-radius: 8rpx; padding: 2rpx 12rpx; white-space: nowrap; }
.amount { margin-top: 18rpx; font-size: 40rpx; font-weight: 500; color: var(--ch-danger); }
.ov-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16rpx; margin-top: 24rpx; }
.ov { background: var(--ch-surface-2); border: 1rpx solid var(--ch-line-soft); border-radius: var(--ch-radius-sm); padding: 20rpx; }
.ov-num { font-size: 34rpx; font-weight: 600; color: var(--ch-accent); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ov-num.sm { font-size: 26rpx; }
.ov-cap { margin-top: 6rpx; font-size: 22rpx; color: var(--ch-text-muted); }
.tabs { display: flex; background: var(--ch-surface); border-radius: 16rpx; overflow: hidden; margin-bottom: 20rpx; }
.tab { flex: 1; text-align: center; padding: 24rpx 0; font-size: 28rpx; color: var(--ch-text); }
.tab.on { color: var(--ch-ink); font-weight: 500; border-bottom: 4rpx solid var(--ch-accent); }
.kv { display: flex; justify-content: space-between; padding: 18rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.kv:last-child { border-bottom: none; }
.k { color: var(--ch-text-muted); font-size: 28rpx; }
.v { color: var(--ch-ink); font-size: 28rpx; text-align: right; flex: 1; margin-left: 24rpx; }
.text-preview { font-size: 26rpx; line-height: 1.7; color: var(--ch-text); background: var(--ch-surface-2); padding: 24rpx; border-radius: 12rpx; }
.ver-item { padding: 20rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.ver-item:last-child { border-bottom: none; }
.v-name { font-size: 28rpx; font-weight: 500; }
</style>
