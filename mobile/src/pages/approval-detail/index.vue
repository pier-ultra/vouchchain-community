<script setup lang="ts">
import { ref } from 'vue'
import { onLoad, onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { currentUserName } from '../../core/roles'
import { shortTime } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const id = ref(0)
const instance = ref<Row | null>(null)
const nodes = ref<Row[]>([])
const events = ref<Row[]>([])
const loading = ref(true)

const NODE_STATUS: Record<string, string> = {
  pending: '待审批', approved: '已通过', rejected: '已驳回', skipped: '已跳过',
}
const STATUS: Record<string, string> = {
  pending: '待审批', running: '审批中', approved: '已通过', rejected: '已驳回', revoked: '已废止',
}

onLoad((opt) => {
  id.value = Number(opt?.id || 0)
})
onShow(load)

async function load() {
  loading.value = true
  try {
    const [d, ev] = await Promise.all([
      api.approvalDetail(id.value),
      api.approvalEvents(id.value),
    ])
    if (d?.code === 0) {
      instance.value = d.data?.instance || null
      nodes.value = d.data?.nodes || []
    }
    if (ev?.code === 0) events.value = ev.data || []
  } catch {
    /* ignore */
  } finally {
    loading.value = false
  }
}

function actor(): string {
  return currentUserName() || 'admin'
}

function doAct(action: 'approve' | 'reject') {
  const reject = action === 'reject'
  uni.showModal({
    title: reject ? '驳回审批' : '同意审批',
    editable: true,
    placeholderText: reject ? '请填写驳回意见（必填）' : '审批意见（可选）',
    success: async (r) => {
      if (!r.confirm) return
      const comment = (r.content || '').trim()
      if (reject && !comment) {
        uni.showToast({ title: '驳回必须填写意见', icon: 'none' })
        return
      }
      const res = await api.approvalAct(id.value, actor(), action, comment)
      if (res?.code === 0) {
        uni.showToast({ title: reject ? '已驳回' : '已同意', icon: 'success' })
        load()
      } else {
        uni.showToast({ title: res?.message || '操作失败', icon: 'none' })
      }
    },
  })
}

function doTransfer() {
  uni.showModal({
    title: '转交审批',
    editable: true,
    placeholderText: '输入接收人用户名',
    success: (r) => {
      if (!r.confirm || !(r.content || '').trim()) return
      const to = r.content!.trim()
      uni.showModal({
        title: '转交说明',
        editable: true,
        placeholderText: '转交原因（可选）',
        success: async (r2) => {
          const res = await api.approvalTransfer(id.value, actor(), to, (r2.content || '').trim())
          if (res?.code === 0) {
            uni.showToast({ title: '已转交', icon: 'success' })
            load()
          } else {
            uni.showToast({ title: res?.message || '转交失败', icon: 'none' })
          }
        },
      })
    },
  })
}

function doAddSign() {
  uni.showModal({
    title: '加签',
    editable: true,
    placeholderText: '输入加签人用户名',
    success: (r) => {
      if (!r.confirm || !(r.content || '').trim()) return
      const to = r.content!.trim()
      const modes = ['and', 'or']
      uni.showActionSheet({
        itemList: ['会签（and）', '或签（or）'],
        success: async (s) => {
          const res = await api.approvalAddSign(id.value, actor(), to, modes[s.tapIndex] || 'and', '移动端加签')
          if (res?.code === 0) {
            uni.showToast({ title: '已加签', icon: 'success' })
            load()
          } else {
            uni.showToast({ title: res?.message || '加签失败', icon: 'none' })
          }
        },
      })
    },
  })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view v-if="loading" class="empty">加载中…</view>
    <block v-else-if="instance">
      <view class="card head">
        <view class="row">
          <view class="title">{{ instance.targetType === 'contract_file' ? '合同文件' : (instance.targetType || '审批') }} #{{ instance.targetId }}</view>
          <view class="badge">{{ STATUS[instance.status] || instance.status }}</view>
        </view>
        <view class="muted">发起人：{{ instance.createdBy || '—' }} · 第 {{ instance.currentSeq }} 级</view>
        <view class="acts" v-if="instance.status === 'running' || instance.status === 'pending'">
          <view class="act ok" @click="doAct('approve')">同意</view>
          <view class="act no" @click="doAct('reject')">驳回</view>
          <view class="act mut" @click="doTransfer">转交</view>
          <view class="act mut" @click="doAddSign">加签</view>
        </view>
      </view>

      <view class="section-title">审批节点</view>
      <view class="card">
        <view v-for="(n, idx) in nodes" :key="n.id || idx" class="node">
          <view class="dot" :class="n.status"></view>
          <view class="node-body">
            <view class="row">
              <text class="node-name">第{{ n.seq }}级 · {{ n.name || n.approver || '节点' }}</text>
              <text class="node-status" :class="n.status">{{ NODE_STATUS[n.status] || n.status }}</text>
            </view>
            <view class="muted">审批人：{{ n.approver || '—' }}<text v-if="n.mode"> · {{ n.mode === 'and' ? '会签' : '或签' }}</text></view>
            <view v-if="n.comment" class="comment">意见：{{ n.comment }}</view>
          </view>
        </view>
      </view>

      <view class="section-title">流转记录</view>
      <view class="card">
        <view v-if="!events.length" class="empty sm">暂无流转记录</view>
        <view v-for="(e, idx) in events" :key="idx" class="event">
          <view class="row">
            <text class="e-act">{{ e.action || e.event || '操作' }}</text>
            <text class="muted">{{ shortTime(e.createdAt || e.created_at) }}</text>
          </view>
          <view class="muted" v-if="e.actor || e.username">{{ e.actor || e.username }}<text v-if="e.to"> → {{ e.to }}</text></view>
        </view>
      </view>
    </block>
    <view v-else class="empty">审批不存在</view>
  </view>
</template>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: center; }
.head .title { flex: 1; font-size: 30rpx; font-weight: 500; margin-right: 16rpx; }
.badge { font-size: 24rpx; color: var(--ch-warning); }
.acts { display: flex; gap: 14rpx; margin-top: 26rpx; }
.act { flex: 1; text-align: center; padding: 18rpx 0; border-radius: 12rpx; font-size: 26rpx; }
.act.ok { background: var(--ch-success); color: #fff; }
.act.no { background: var(--ch-surface); color: var(--ch-danger); border: 1rpx solid var(--ch-danger); }
.act.mut { background: var(--ch-surface); color: var(--ch-accent); border: 1rpx solid var(--ch-accent); }
.section-title { font-size: 28rpx; font-weight: 500; margin: 28rpx 8rpx 16rpx; }
.node { display: flex; padding: 16rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.node:last-child { border-bottom: none; }
.dot { width: 18rpx; height: 18rpx; border-radius: 50%; background: var(--ch-line); margin: 10rpx 20rpx 0 0; flex-shrink: 0; }
.dot.approved { background: var(--ch-success); }
.dot.rejected { background: var(--ch-danger); }
.dot.pending { background: var(--ch-warning); }
.node-body { flex: 1; }
.node-name { font-size: 28rpx; font-weight: 500; }
.node-status { font-size: 24rpx; color: var(--ch-text-muted); }
.node-status.approved { color: var(--ch-success); }
.node-status.rejected { color: var(--ch-danger); }
.node-status.pending { color: var(--ch-warning); }
.comment { margin-top: 10rpx; font-size: 26rpx; color: var(--ch-text); background: var(--ch-surface-2); padding: 12rpx 16rpx; border-radius: 8rpx; }
.event { padding: 16rpx 0; border-bottom: 1rpx solid var(--ch-line-soft); }
.event:last-child { border-bottom: none; }
.e-act { font-size: 28rpx; font-weight: 500; }
.empty.sm { padding: 40rpx 0; }
</style>
