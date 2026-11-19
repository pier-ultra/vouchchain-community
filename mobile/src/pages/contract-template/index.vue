<script setup lang="ts">
import { ref, computed } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const loading = ref(false)
const keyword = ref('')

const renderVisible = ref(false)
const renderLoading = ref(false)
const renderTemplate = ref<Row | null>(null)
const renderValues = ref<Record<string, string>>({})
const renderPreview = ref('')
const renderMissing = ref<string[]>([])

async function load() {
  loading.value = true
  try {
    const res = await api.contractTemplates({ keyword: keyword.value.trim() || undefined })
    list.value = res.data || []
  } catch (e: any) {
    uni.showToast({ title: e?.message || '加载失败', icon: 'none' })
    list.value = []
  } finally {
    loading.value = false
  }
}

onShow(load)

function openRender(tpl: Row) {
  renderTemplate.value = tpl
  renderValues.value = {}
  renderPreview.value = tpl.content || ''
  renderMissing.value = []
  const vars = tpl.variables?.length ? tpl.variables : (tpl.varKeys || []).map((k: string) => ({ key: k, label: k, required: true, default: '' }))
  vars.forEach((v: any) => { renderValues.value[v.key] = v.default || '' })
  renderVisible.value = true
}

async function doRender() {
  if (!renderTemplate.value) return
  renderLoading.value = true
  try {
    const res = await api.contractTemplateRender(renderTemplate.value.id, renderValues.value)
    renderPreview.value = res.data?.content || ''
    renderMissing.value = res.data?.missing || []
    if (renderMissing.value.length) {
      uni.showToast({ title: '缺失必填变量：' + renderMissing.value.join('、'), icon: 'none' })
    } else {
      uni.showToast({ title: '套打完成，可复制/生成合同', icon: 'success' })
    }
  } catch (e: any) {
    uni.showToast({ title: e?.message || '套打失败', icon: 'none' })
  } finally {
    renderLoading.value = false
  }
}

function copyPreview() {
  uni.setClipboardData({ data: renderPreview.value })
  uni.showToast({ title: '已复制到剪贴板', icon: 'success' })
}

function createContractFromPreview() {
  if (!renderPreview.value.trim()) {
    uni.showToast({ title: '内容为空', icon: 'none' })
    return
  }
  // 跳转到 AI 生成页，把套打结果作为"要点/草稿"预填
  uni.navigateTo({
    url: `/pages/contract-generate/index?notes=${encodeURIComponent(renderPreview.value)}`,
  })
  renderVisible.value = false
}

const renderVars = computed(() => {
  const tpl = renderTemplate.value
  if (!tpl) return []
  return tpl.variables?.length ? tpl.variables : (tpl.varKeys || []).map((k: string) => ({ key: k, label: k, required: true, default: '' }))
})
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="header">
      <view class="title">合同模板库</view>
      <view class="sub">私有模板：{{变量}} 占位符 + 变量套打，与 AI 生成互补</view>
    </view>

    <view class="search-bar">
      <input class="search-input" v-model="keyword" placeholder="搜索模板名/正文" @confirm="load" />
      <button class="search-btn" @click="load">搜索</button>
    </view>

    <view v-if="loading" class="loading">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无模板，请在 Web 端新建</view>

    <view v-else class="list">
      <view v-for="tpl in list" :key="tpl.id" class="item" @click="openRender(tpl)">
        <view class="item-main">
          <view class="item-name">{{ tpl.name }}</view>
          <view class="item-meta">
            <text class="tag">{{ tpl.category || '通用' }}</text>
            <text class="var-count">{{ (tpl.varKeys || []).length }} 个变量</text>
          </view>
          <view class="item-remark" v-if="tpl.remark">{{ tpl.remark }}</view>
        </view>
        <view class="item-action">套打 ›</view>
      </view>
    </view>

    <!-- 套打弹窗 -->
    <view v-if="renderVisible" class="modal-overlay" @click="renderVisible = false">
      <view class="modal" @click.stop>
        <view class="modal-header">
          <view class="modal-title">套打：{{ renderTemplate?.name }}</view>
          <view class="modal-close" @click="renderVisible = false">×</view>
        </view>

        <scroll-view class="modal-body" scroll-y>
          <view class="vars-section" v-if="renderVars.length">
            <view class="section-title">填写变量</view>
            <view v-for="v in renderVars" :key="v.key" class="var-field">
              <text class="var-label">{{ v.label || v.key }} <text v-if="v.required" class="required">*</text></text>
              <input class="var-input" :placeholder="v.default ? '默认：' + v.default : (v.required ? '必填' : '选填')" v-model="renderValues[v.key]" />
            </view>
            <button class="btn-render" :disabled="renderLoading" @click="doRender">
              <text v-if="renderLoading">套打中…</text>
              <text v-else>套打生成</text>
            </button>
          </view>

          <view class="preview-section">
            <view class="section-title">预览 <text v-if="renderMissing.length" class="missing-badge">缺失 {{ renderMissing.length }} 项</text></view>
            <textarea class="preview-text" :value="renderPreview" readonly />
            <view class="preview-actions">
              <button class="btn-copy" @click="copyPreview">复制全文</button>
              <button class="btn-create" @click="createContractFromPreview">生成合同</button>
            </view>
          </view>
        </scroll-view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.page {
  background: var(--ch-canvas, #f5f6f8);
  min-height: 100vh;
}
.header {
  padding: 24rpx 20rpx 12rpx;
}
.title {
  font-size: 34rpx;
  font-weight: 600;
  color: var(--ch-ink, #111);
}
.sub {
  margin-top: 4rpx;
  font-size: 24rpx;
  color: var(--ch-text-muted, #909399);
}
.search-bar {
  display: flex;
  gap: 12rpx;
  padding: 0 20rpx 20rpx;
}
.search-input {
  flex: 1;
  height: 72rpx;
  background: var(--ch-surface, #fff);
  border-radius: 10rpx;
  padding: 0 20rpx;
  font-size: 26rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
}
.search-btn {
  width: 120rpx;
  height: 72rpx;
  background: var(--ch-accent, #4f46e5);
  color: #fff;
  font-size: 26rpx;
  border-radius: 10rpx;
  border: none;
}
.loading, .empty {
  padding: 60rpx 20rpx;
  text-align: center;
  color: var(--ch-text-muted, #909399);
  font-size: 26rpx;
}
.list {
  background: var(--ch-surface, #fff);
  margin: 0 20rpx 20rpx;
  border-radius: 12rpx;
  overflow: hidden;
}
.item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24rpx 20rpx;
  border-bottom: 1rpx solid var(--ch-line-soft, #f3f4f6);
}
.item:last-child {
  border-bottom: none;
}
.item-main {
  flex: 1;
  min-width: 0;
}
.item-name {
  font-size: 28rpx;
  font-weight: 500;
  color: var(--ch-ink, #111);
  margin-bottom: 8rpx;
}
.item-meta {
  display: flex;
  gap: 16rpx;
  font-size: 22rpx;
}
.tag {
  color: var(--ch-accent, #4f46e5);
  background: var(--ch-accent-soft, #eef2ff);
  padding: 2rpx 10rpx;
  border-radius: 999rpx;
}
.var-count {
  color: var(--ch-text-muted, #909399);
}
.item-remark {
  margin-top: 8rpx;
  font-size: 22rpx;
  color: var(--ch-text-muted, #909399);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.item-action {
  font-size: 24rpx;
  color: var(--ch-text-placeholder, #c0c4cc);
  margin-left: 16rpx;
}

/* Modal */
.modal-overlay {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0,0,0,0.4);
  display: flex;
  align-items: flex-end;
  z-index: 1000;
  animation: fadeIn 0.2s ease;
}
@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
.modal {
  width: 100%;
  max-height: 85vh;
  background: var(--ch-surface, #fff);
  border-radius: 20rpx 20rpx 0 0;
  display: flex;
  flex-direction: column;
  animation: slideUp 0.25s ease;
}
@keyframes slideUp { from { transform: translateY(100%); } to { transform: translateY(0); } }
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20rpx 24rpx;
  border-bottom: 1rpx solid var(--ch-line, #e5e7eb);
}
.modal-title {
  font-size: 30rpx;
  font-weight: 600;
  color: var(--ch-ink, #111);
}
.modal-close {
  width: 48rpx; height: 48rpx;
  font-size: 30rpx;
  color: var(--ch-text-muted, #909399);
  display: flex; align-items: center; justify-content: center;
}
.modal-body {
  flex: 1;
  overflow: hidden;
}
.section-title {
  padding: 20rpx 24rpx 12rpx;
  font-size: 26rpx;
  font-weight: 500;
  color: var(--ch-ink, #111);
}
.vars-section {
  padding: 0 24rpx 20rpx;
}
.var-field {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  margin-bottom: 20rpx;
}
.var-label {
  font-size: 24rpx;
  color: var(--ch-text, #333);
  display: flex; align-items: center; gap: 6rpx;
}
.required { color: var(--ch-danger, #f56c6c); }
.var-input {
  height: 72rpx;
  background: var(--ch-canvas, #f5f6f8);
  border-radius: 8rpx;
  padding: 0 16rpx;
  font-size: 26rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
}
.btn-render {
  width: 100%;
  height: 88rpx;
  background: var(--ch-accent, #4f46e5);
  color: #fff;
  font-size: 28rpx;
  font-weight: 500;
  border-radius: 12rpx;
  border: none;
  margin-top: 8rpx;
}
.btn-render:disabled { opacity: 0.6; }

.preview-section {
  padding: 0 24rpx 30rpx;
}
.preview-text {
  min-height: 300rpx;
  background: var(--ch-canvas, #f5f6f8);
  border-radius: 8rpx;
  padding: 16rpx;
  font-size: 24rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
  line-height: 1.7;
  margin-bottom: 16rpx;
}
.missing-badge {
  margin-left: 8rpx;
  font-size: 20rpx;
  color: var(--ch-danger, #f56c6c);
  background: var(--ch-danger-soft, #fef0f0);
  padding: 2rpx 8rpx;
  border-radius: 999rpx;
}
.preview-actions {
  display: flex;
  gap: 16rpx;
}
.preview-actions button {
  flex: 1;
  height: 80rpx;
  font-size: 26rpx;
  font-weight: 500;
  border-radius: 10rpx;
  border: none;
}
.btn-copy {
  background: var(--ch-surface, #fff);
  color: var(--ch-accent, #4f46e5);
  border: 1rpx solid var(--ch-accent, #4f46e5);
}
.btn-create {
  background: var(--ch-success, #67c23a);
  color: #fff;
}
</style>