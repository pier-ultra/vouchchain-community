<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, exportFile, type Row } from '../../services/api'
import { shortTime } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const tab = ref<'seals' | 'applies' | 'risks'>('seals')
const list = ref<Row[]>([])
const loading = ref(false)
const risks = ref<Row[]>([])

// 申请表单
const showApply = ref(false)
const sealOptions = ref<Row[]>([])
const applySealId = ref(0)
const applyReason = ref('')
const applyCopies = ref(1)
const applyBizType = ref('contract')

async function load() {
  loading.value = true
  try {
    if (tab.value === 'risks') {
      const res = await api.sealRisks()
      if (res?.code === 0) risks.value = res.data || []
    } else {
      const res = tab.value === 'seals' ? await api.seals() : await api.sealApplies()
      if (res?.code === 0) list.value = res.data || []
      else {
        list.value = []
        if (res?.message) uni.showToast({ title: res.message, icon: 'none' })
      }
    }
  } catch {
    list.value = []
  } finally {
    loading.value = false
  }
}

onShow(load)

function switchTab(t: 'seals' | 'applies' | 'risks') {
  if (tab.value === t) return
  tab.value = t
  load()
}

function label(r: Row): string {
  return r.name || r.title || r.sealName || r.applyNo || `#${r.id}`
}

async function openApply() {
  try {
    const res = await api.seals()
    if (res?.code === 0) sealOptions.value = res.data || []
  } catch {
    /* ignore */
  }
  applySealId.value = Number(sealOptions.value?.[0]?.id || 0)
  applyReason.value = ''
  applyCopies.value = 1
  showApply.value = true
}

function sealLabels(): string[] {
  return sealOptions.value.map((s: Row) => `${s.name || s.sealName || '印章'}（${s.type || '—'}）`)
}

function onPickSeal(e: any) {
  applySealId.value = Number(sealOptions.value[e.detail.value]?.id || 0)
}

function onPickBiz(e: any) {
  applyBizType.value = ['contract', 'invoice', 'other'][e.detail.value] || 'contract'
}

async function submitApply() {
  if (!applySealId.value) {
    uni.showToast({ title: '请选择印章', icon: 'none' })
    return
  }
  if (!applyReason.value.trim()) {
    uni.showToast({ title: '请填写用印事由', icon: 'none' })
    return
  }
  uni.showLoading({ title: '提交中…', mask: true })
  try {
    const res = await api.sealApplyCreate({
      sealId: applySealId.value,
      bizType: applyBizType.value,
      copies: applyCopies.value,
      reason: applyReason.value.trim(),
    })
    uni.hideLoading()
    if (res?.code === 0) {
      uni.showToast({ title: '申请已提交', icon: 'success' })
      showApply.value = false
      tab.value = 'applies'
      load()
    } else {
      uni.showToast({ title: res?.message || '提交失败', icon: 'none' })
    }
  } catch {
    uni.hideLoading()
  }
}

function doApprove(item: Row, approve: boolean) {
  uni.showModal({
    title: approve ? '同意用印' : '驳回用印',
    editable: true,
    placeholderText: approve ? '审批意见（可选）' : '驳回原因（必填）',
    success: async (r) => {
      if (!r.confirm) return
      const comment = (r.content || '').trim()
      if (!approve && !comment) {
        uni.showToast({ title: '驳回必须填写原因', icon: 'none' })
        return
      }
      const res = await api.sealApplyApprove(Number(item.id), approve, comment)
      if (res?.code === 0) {
        uni.showToast({ title: approve ? '已同意' : '已驳回', icon: 'success' })
        load()
      } else {
        uni.showToast({ title: res?.message || '操作失败', icon: 'none' })
      }
    },
  })
}

function doStamp(item: Row) {
  uni.showModal({
    title: '确认盖章',
    content: `确认对「${label(item)}」执行盖章？`,
    success: async (r) => {
      if (!r.confirm) return
      const res = await api.sealApplyStamp(Number(item.id))
      if (res?.code === 0) {
        uni.showToast({ title: '已盖章', icon: 'success' })
        load()
      } else {
        uni.showToast({ title: res?.message || '盖章失败', icon: 'none' })
      }
    },
  })
}

async function doExport() {
  const r = await exportFile('/seal-applies/export', `用印台账_${Date.now()}.xlsx`, {})
  uni.showToast({ title: r.ok ? '已导出' : (r.message || '导出失败'), icon: r.ok ? 'success' : 'none' })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="tabs">
      <view class="tab" :class="{ on: tab === 'seals' }" @click="switchTab('seals')">印章台账</view>
      <view class="tab" :class="{ on: tab === 'applies' }" @click="switchTab('applies')">用印申请</view>
      <view class="tab" :class="{ on: tab === 'risks' }" @click="switchTab('risks')">风险提示</view>
    </view>

    <view v-if="tab === 'applies'" class="toolbar">
      <view class="caption muted">共 {{ list.length }} 条申请</view>
      <view class="ops">
        <view class="op ghost" @click="doExport">导出</view>
        <view class="op" @click="openApply">＋ 申请</view>
      </view>
    </view>

    <view v-if="loading" class="empty">加载中…</view>
    <block v-else-if="tab === 'risks'">
      <view v-if="!risks.length" class="empty">暂无风险提示</view>
      <view v-for="(r, idx) in risks" :key="idx" class="card risk">
        <view class="row">
          <view class="title">{{ r.title || r.name || r.type || '风险项' }}</view>
          <view class="risk-level" :class="r.level || r.severity">{{ r.level || r.severity || '提示' }}</view>
        </view>
        <view v-if="r.detail || r.message || r.desc" class="muted">{{ r.detail || r.message || r.desc }}</view>
      </view>
    </block>
    <block v-else>
      <view v-if="!list.length" class="empty">暂无数据</view>
      <view v-for="s in list" :key="s.id" class="card">
        <view class="row">
          <view class="title">{{ label(s) }}</view>
          <view class="badge" v-if="s.status">{{ s.status }}</view>
        </view>
        <view v-if="s.applicant || s.owner" class="muted">申请人：{{ s.applicant || s.owner }}</view>
        <view v-if="s.created_at || s.createdAt" class="muted">{{ shortTime(s.created_at || s.createdAt) }}</view>
        <view v-if="s.reason || s.remark" class="muted">事由：{{ s.reason || s.remark }}</view>
        <view v-if="s.copies" class="muted">份数：{{ s.copies }}</view>
        <view v-if="tab === 'applies' && (s.status === 'pending' || s.status === 'approving')" class="acts">
          <view class="act ok" @click="doApprove(s, true)">同意</view>
          <view class="act no" @click="doApprove(s, false)">驳回</view>
          <view class="act stamp" @click="doStamp(s)">盖章</view>
        </view>
      </view>
    </block>

    <!-- 申请弹窗 -->
    <view v-if="showApply" class="mask" @click="showApply = false">
      <view class="sheet" @click.stop>
        <view class="sheet-title">新建用印申请</view>
        <view class="field">
          <text class="flabel">印章</text>
          <picker :range="sealLabels()" @change="onPickSeal">
            <view class="fpick">{{ sealOptions.find((s: Row) => Number(s.id) === applySealId)?.name || '请选择印章' }} ▾</view>
          </picker>
        </view>
        <view class="field">
          <text class="flabel">业务类型</text>
          <picker :range="['contract', 'invoice', 'other']" @change="onPickBiz">
            <view class="fpick">{{ applyBizType }} ▾</view>
          </picker>
        </view>
        <view class="field">
          <text class="flabel">份数</text>
          <input class="finput" type="number" v-model.number="applyCopies" min="1" />
        </view>
        <view class="field col">
          <text class="flabel">用印事由（必填）</text>
          <textarea class="ftextarea" v-model="applyReason" placeholder="请输入用印事由" />
        </view>
        <view class="sheet-acts">
          <view class="sbtn cancel" @click="showApply = false">取消</view>
          <view class="sbtn confirm" @click="submitApply">提交申请</view>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.tabs { display: flex; background: var(--ch-surface); border-radius: 16rpx; overflow: hidden; margin-bottom: 20rpx; }
.tab { flex: 1; text-align: center; padding: 24rpx 0; font-size: 28rpx; color: var(--ch-text); }
.tab.on { color: var(--ch-ink); font-weight: 500; border-bottom: 4rpx solid var(--ch-accent); }
.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16rpx; }
.caption { font-size: 26rpx; }
.ops { display: flex; gap: 14rpx; }
.op { background: var(--ch-accent); color: #fff; font-size: 26rpx; padding: 12rpx 22rpx; border-radius: 12rpx; }
.op.ghost { background: var(--ch-surface); color: var(--ch-ink); border: 1rpx solid var(--ch-line); }
.row { display: flex; justify-content: space-between; align-items: center; }
.title { flex: 1; font-size: 30rpx; font-weight: 500; margin-right: 16rpx; }
.badge { font-size: 24rpx; color: var(--ch-text-muted); }
.muted { margin-top: 10rpx; }
.acts { display: flex; gap: 14rpx; margin-top: 24rpx; }
.act { flex: 1; text-align: center; padding: 18rpx 0; border-radius: 12rpx; font-size: 26rpx; }
.act.ok { background: var(--ch-success); color: #fff; }
.act.no { background: var(--ch-surface); color: var(--ch-danger); border: 1rpx solid var(--ch-danger); }
.act.stamp { background: var(--ch-surface); color: var(--ch-accent); border: 1rpx solid var(--ch-accent); }
.risk-level { font-size: 24rpx; color: var(--ch-warning); }
.risk-level.high { color: var(--ch-danger); }
.mask { position: fixed; left: 0; right: 0; top: 0; bottom: 0; background: rgba(0,0,0,.45); z-index: 100; display: flex; align-items: flex-end; }
.sheet { width: 100%; background: var(--ch-surface); border-radius: 28rpx 28rpx 0 0; padding: 32rpx; box-sizing: border-box; }
.sheet-title { font-size: 32rpx; font-weight: 500; text-align: center; margin-bottom: 24rpx; }
.field { display: flex; align-items: center; margin-bottom: 24rpx; }
.field.col { flex-direction: column; align-items: stretch; }
.flabel { width: 180rpx; font-size: 28rpx; color: var(--ch-text); flex-shrink: 0; }
.fpick, .finput { flex: 1; background: var(--ch-canvas); border-radius: 10rpx; padding: 18rpx 20rpx; font-size: 28rpx; }
.ftextarea { width: 100%; height: 160rpx; background: var(--ch-canvas); border-radius: 10rpx; padding: 18rpx 20rpx; font-size: 28rpx; margin-top: 12rpx; box-sizing: border-box; }
.sheet-acts { display: flex; gap: 20rpx; margin-top: 12rpx; }
.sbtn { flex: 1; text-align: center; padding: 24rpx 0; border-radius: 14rpx; font-size: 30rpx; }
.sbtn.cancel { background: var(--ch-canvas); color: var(--ch-text); }
.sbtn.confirm { background: var(--ch-accent); color: #fff; }
</style>
