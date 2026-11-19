<script setup lang="ts">
import { ref, reactive, toRaw } from 'vue'
import { onLoad, onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const types = ['采购', '销售', '租赁', '服务', '其他'] as const
type ContractType = typeof types[number]

const form = reactive<{
  contractType: ContractType
  partyA: string
  partyB: string
  subject: string
  amount: string
  term: string
  notes: string
}>({
  contractType: '采购',
  partyA: '',
  partyB: '',
  subject: '',
  amount: '',
  term: '',
  notes: '',
})

const refFile = ref<{ path: string; name: string } | null>(null)
const generating = ref(false)
const generatedId = ref<number | null>(null)

// 支持从模板套打页跳转过来预填 notes
onLoad((opt) => {
  if (opt?.notes) {
    form.notes = decodeURIComponent(opt.notes)
  }
})

function pickRefFile() {
  // #ifdef MP-WEIXIN
  ;(uni as any).chooseMessageFile({
    count: 1,
    type: 'file',
    extension: ['pdf', 'docx', 'txt'],
    success: (r: any) => {
      const f = r.tempFiles && r.tempFiles[0]
      if (f) refFile.value = { path: f.path, name: f.name || 'ref.pdf' }
    },
    fail: () => {},
  })
  // #endif
  // #ifndef MP-WEIXIN
  ;(uni as any).chooseFile({
    count: 1,
    type: 'file',
    extension: ['pdf', 'docx', 'txt'],
    success: (r: any) => {
      const f = r.tempFiles && r.tempFiles[0]
      if (f) refFile.value = { path: f.path, name: f.name || 'ref.pdf' }
    },
    fail: () => {},
  })
  // #endif
}

function clearRefFile() {
  refFile.value = null
}

async function generate() {
  if (!form.partyA.trim() || !form.partyB.trim()) {
    uni.showToast({ title: '甲方与乙方必填', icon: 'none' })
    return
  }
  generating.value = true
  generatedId.value = null
  try {
    // 使用 toRaw 获取原始对象，避免响应式代理问题
    const payload = toRaw(form)
    const res = await api.generateContract(payload, undefined as any)
    if (res?.code === 0) {
      generatedId.value = res.data?.id || null
      uni.showToast({ title: '合同已生成并进入流水线', icon: 'success' })
    } else {
      uni.showToast({ title: res?.message || '生成失败', icon: 'none' })
    }
  } catch (e: any) {
    uni.showToast({ title: e?.message || '生成失败', icon: 'none' })
  } finally {
    generating.value = false
  }
}

function goTaskCenter() {
  uni.switchTab({ url: '/pages/home/index' })
  // TODO: 跳转到任务中心页（若有）
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <scroll-view class="container" scroll-y>
      <view class="card">
        <view class="card-title">AI 生成合同</view>
        <view class="card-sub">简略信息 / 一段话 / 参考文件 → AI 起草合同全文 → 自动进流水线（抽取→复核→审批→签署）</view>

        <view class="form">
          <view class="row">
            <view class="field">
              <text class="label">合同类型</text>
              <picker v-model="form.contractType" :range="types" @change="form.contractType = $event.detail.value">
                <view class="picker">{{ form.contractType }}</view>
              </picker>
            </view>
            <view class="field">
              <text class="label">履行期限</text>
              <input class="input" v-model="form.term" placeholder="如：2026-12-01 至 2027-05-31" />
            </view>
          </view>

          <view class="row">
            <view class="field">
              <text class="label">甲方（我方）</text>
              <input class="input" v-model="form.partyA" placeholder="如：北京华信科技有限公司" />
            </view>
            <view class="field">
              <text class="label">乙方（对方）</text>
              <input class="input" v-model="form.partyB" placeholder="如：上海蓝海机械制造有限公司" />
            </view>
          </view>

          <view class="field full">
            <text class="label">标的</text>
            <input class="input" v-model="form.subject" placeholder="如：工业自动化设备一批（含安装调试）" />
          </view>

          <view class="field full">
            <text class="label">金额</text>
            <input class="input" v-model="form.amount" placeholder="如：人民币 150 万元（含税）" />
          </view>

          <view class="field full">
            <text class="label">要点/草稿</text>
            <textarea class="textarea" v-model="form.notes" placeholder="一段话即可：比如『预付50%，验收后付尾款；逾期按千分之三付违约金；质保24个月；争议交北京仲裁』。留空则按要素生成通用条款。" />
          </view>

          <view class="field full">
            <text class="label">参考文件（可选）</text>
            <view class="file-row">
              <view class="file-btn" @click="refFile ? clearRefFile() : pickRefFile()">
                <text v-if="refFile" class="file-name">{{ refFile.name }}</text>
                <text v-else>选择参考文件（PDF/DOCX/TXT，仅参考风格与结构）</text>
              </view>
              <view v-if="refFile" class="file-tip">当前版本暂不支持移动端文件上传，建议在 Web 端使用</view>
            </view>
          </view>

          <view class="btn-area">
            <button class="btn-primary" :disabled="generating" @click="generate">
              <text v-if="generating" class="loading">生成中…</text>
              <text v-else>AI 生成合同</text>
            </button>
          </view>
        </view>

        <view v-if="generatedId" class="success-alert">
          <view class="alert-icon">✓</view>
          <view class="alert-text">
            <text class="alert-title">合同已生成（任务 #{{ generatedId }}），自动进入流水线</text>
            <text class="alert-desc">去「任务中心」查看抽取进度与条款复核；生成内容以草稿定位，正式使用前须经人工复核与审批。</text>
          </view>
        </view>
      </view>
    </scroll-view>
  </view>
</template>

<style scoped>
.page {
  background: var(--ch-canvas, #f5f6f8);
  min-height: 100vh;
  padding-bottom: 40rpx;
}
.container {
  padding: 20rpx;
}
.card {
  background: var(--ch-surface, #fff);
  border-radius: var(--ch-radius, 12rpx);
  padding: 24rpx;
  box-shadow: 0 2rpx 8rpx rgba(0,0,0,0.04);
}
.card-title {
  font-size: 32rpx;
  font-weight: 600;
  color: var(--ch-ink, #111);
  margin-bottom: 6rpx;
}
.card-sub {
  font-size: 24rpx;
  color: var(--ch-text-muted, #909399);
  margin-bottom: 24rpx;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
}
.row {
  display: flex;
  gap: 16rpx;
}
.field {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}
.field.full {
  width: 100%;
}
.label {
  font-size: 24rpx;
  color: var(--ch-text, #333);
  font-weight: 500;
}
.input {
  height: 80rpx;
  background: var(--ch-canvas, #f5f6f8);
  border-radius: 8rpx;
  padding: 0 20rpx;
  font-size: 26rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
}
.textarea {
  min-height: 160rpx;
  background: var(--ch-canvas, #f5f6f8);
  border-radius: 8rpx;
  padding: 16rpx 20rpx;
  font-size: 26rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
  line-height: 1.6;
}
.picker {
  height: 80rpx;
  background: var(--ch-canvas, #f5f6f8);
  border-radius: 8rpx;
  padding: 0 20rpx;
  font-size: 26rpx;
  color: var(--ch-ink, #111);
  border: 1rpx solid var(--ch-line, #e5e7eb);
  display: flex;
  align-items: center;
}
.file-row {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}
.file-btn {
  height: 80rpx;
  background: var(--ch-accent-soft, #eef2ff);
  border-radius: 8rpx;
  padding: 0 20rpx;
  font-size: 26rpx;
  color: var(--ch-accent, #4f46e5);
  border: 1rpx dashed var(--ch-accent, #4f46e5);
  display: flex;
  align-items: center;
}
.file-tip {
  font-size: 22rpx;
  color: var(--ch-warning, #e6a23c);
}
.btn-area {
  margin-top: 12rpx;
}
.btn-primary {
  width: 100%;
  height: 88rpx;
  background: var(--ch-accent, #4f46e5);
  color: #fff;
  font-size: 30rpx;
  font-weight: 500;
  border-radius: 12rpx;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-primary:disabled {
  opacity: 0.6;
}
.loading {
  color: #fff;
}
.success-alert {
  margin-top: 20rpx;
  padding: 20rpx;
  background: var(--ch-success-soft, #f0f9eb);
  border-radius: 12rpx;
  border: 1rpx solid var(--ch-success, #67c23a);
  display: flex;
  gap: 16rpx;
}
.alert-icon {
  width: 40rpx;
  height: 40rpx;
  background: var(--ch-success, #67c23a);
  color: #fff;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24rpx;
  flex-shrink: 0;
}
.alert-text {
  flex: 1;
}
.alert-title {
  font-size: 26rpx;
  color: var(--ch-success, #67c23a);
  font-weight: 500;
  display: block;
  margin-bottom: 6rpx;
}
.alert-desc {
  font-size: 22rpx;
  color: var(--ch-text-muted, #909399);
  line-height: 1.6;
}
</style>