<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { currentUserName, currentRole, roleLabels, getSession } from '../../core/roles'
import { statusTone, toneVar, shortTime, contractStatusLabel } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const unread = ref(0)
const contractTotal = ref(0)
const pendingCount = ref(0)
const sealCount = ref(0)
const recent = ref<Row[]>([])
const roleText = ref(roleLabels[currentRole()] || currentRole())

interface Entry {
  key: string
  label: string
  icon: string
  color: string
  url: string
  tab?: boolean
}

const entries: Entry[] = [
  { key: 'approvals', label: '审批待办', icon: '审', color: '#4f46e5', url: '/pages/approvals/index', tab: true },
  { key: 'contracts', label: '合同', icon: '合', color: '#0f9d6e', url: '/pages/contracts/index', tab: true },
  { key: 'contract-generate', label: 'AI 生成合同', icon: 'AI', color: '#7c3aed', url: '/pages/contract-generate/index' },
  { key: 'contract-template', label: '合同模板', icon: '模', color: '#06b6d4', url: '/pages/contract-template/index' },
  { key: 'invoices', label: '发票', icon: '票', color: '#c77c0a', url: '/pages/invoices/index' },
  { key: 'seals', label: '用印', icon: '印', color: '#d64545', url: '/pages/seals/index' },
  { key: 'dengbao', label: '等保证据', icon: '等', color: '#6b7280', url: '/pages/dengbao/index' },
  { key: 'revrec', label: '收入确认', icon: '收', color: '#7c3aed', url: '/pages/revrec/index' },
  { key: 'chat', label: 'AI 助手', icon: 'AI', color: '#4f46e5', url: '/pages/chat/index', tab: true },
]

// 首页统计卡：合同 / 待我审批 / 未读 / 用印待办
const stats = ref<{ key: string; label: string; num: string; tone: string; url: string; tab?: boolean }[]>([
  { key: 'contracts', label: '合同总数', num: '—', tone: 'var(--ch-accent)', url: '/pages/contracts/index', tab: true },
  { key: 'pending', label: '待我审批', num: '—', tone: 'var(--ch-warning)', url: '/pages/approvals/index', tab: true },
  { key: 'unread', label: '未读消息', num: '—', tone: 'var(--ch-success)', url: '/pages/mine/index', tab: true },
  { key: 'seals', label: '用印待办', num: '—', tone: 'var(--ch-danger)', url: '/pages/seals/index' },
])

onShow(async () => {
  const s = getSession()
  if (s) roleText.value = s.roleName || roleLabels[s.role] || s.role
  const tasks: Promise<void>[] = [
    api.notifications().then((r) => {
      if (r?.code === 0) {
        unread.value = r.data?.unread || 0
        recent.value = (r.data?.list || []).slice(0, 5)
      }
    }),
    api.approvalList({ status: 'running' }).then((r) => {
      if (r?.code === 0) pendingCount.value = (r.data || []).length
    }),
    api.contracts({ page: 1, pageSize: 1 }).then((r) => {
      if (r?.code === 0) contractTotal.value = r.data?.total || 0
    }),
    api.seals().then((r) => {
      if (r?.code === 0) {
        sealCount.value = (r.data || []).filter((x) => (x.status || x.status) === 'pending' || (x.status || x.status) === 'running').length
      }
    }),
  ]
  await Promise.allSettled(tasks)
  const map: Record<string, string> = {
    contracts: String(contractTotal.value || 0),
    pending: String(pendingCount.value || 0),
    unread: String(unread.value || 0),
    seals: String(sealCount.value || 0),
  }
  stats.value = stats.value.map((st) => ({ ...st, num: map[st.key] ?? '—' }))
})

function go(e: Entry) {
  if (e.tab) uni.switchTab({ url: e.url })
  else uni.navigateTo({ url: e.url })
}

function goStat(s: { url: string; tab?: boolean }) {
  if (s.tab) uni.switchTab({ url: s.url })
  else uni.navigateTo({ url: s.url })
}

function openNotice(n: Row) {
  const tid = n.targetId || n.target_id
  if (tid) uni.navigateTo({ url: `/pages/approval-detail/index?id=${tid}` })
}

function noticeIcon(n: Row): { ch: string; bg: string } {
  const kind = String(n.type || n.kind || '')
  if (kind.includes('approv')) return { ch: '审', bg: 'var(--ch-accent)' }
  if (kind.includes('invoice')) return { ch: '票', bg: 'var(--ch-warning)' }
  if (kind.includes('seal') || kind.includes('stamp')) return { ch: '印', bg: 'var(--ch-danger)' }
  return { ch: '通', bg: 'var(--ch-success)' }
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="hero">
      <view class="hello">你好，{{ currentUserName() || '同事' }}</view>
      <view class="role">{{ roleText }}</view>
    </view>

    <!-- 统计卡：一屏看到四个关键数 -->
    <view class="stats">
      <view v-for="st in stats" :key="st.key" class="stat" @click="goStat(st)">
        <view class="num" :style="{ color: st.tone }">{{ st.num }}</view>
        <view class="cap">{{ st.label }}</view>
      </view>
    </view>

    <view class="grid">
      <view v-for="e in entries" :key="e.key" class="cell" @click="go(e)">
        <view class="ico" :style="{ background: e.color }">{{ e.icon }}</view>
        <view class="cell-label">{{ e.label }}</view>
      </view>
    </view>

    <!-- 最近动态：通知流 -->
    <view class="sec-head">
      <text class="sec-title">最近动态</text>
      <text v-if="unread > 0" class="sec-badge">{{ unread }} 未读</text>
    </view>
    <view class="card feed">
      <view v-if="!recent.length" class="feed-empty">暂无动态</view>
      <view v-for="n in recent" :key="n.id" class="feed-item" @click="openNotice(n)">
        <view class="feed-ico" :style="{ background: noticeIcon(n).bg }">{{ noticeIcon(n).ch }}</view>
        <view class="feed-body">
          <view class="feed-title">{{ n.title || n.message || n.content || '通知' }}</view>
          <view class="feed-meta muted">{{ shortTime(n.created_at || n.createdAt) }}</view>
        </view>
        <view class="arrow">›</view>
      </view>
    </view>

    <view class="hint muted">移动端覆盖执行 / 审批 / 查看；角色权限、组织、主数据等治理配置请在 Web 管理台操作。</view>
  </view>
</template>

<style scoped>
.hero {
  padding: 40rpx 32rpx 36rpx;
  background: linear-gradient(135deg, var(--ch-accent) 0%, var(--ch-accent-strong) 100%);
  color: #fff;
  border-radius: 24rpx;
  margin-bottom: 24rpx;
}
.hello {
  font-size: 42rpx;
  font-weight: 600;
}
.role {
  margin-top: 10rpx;
  font-size: 26rpx;
  opacity: 0.9;
}

.stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20rpx;
  margin-bottom: 24rpx;
}
.stat {
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-line-soft);
  border-radius: var(--ch-radius-card);
  padding: 30rpx 24rpx;
}
.num {
  font-size: 56rpx;
  font-weight: 700;
  line-height: 1.1;
}
.cap {
  margin-top: 10rpx;
  font-size: 24rpx;
  color: var(--ch-text-muted);
}

.grid {
  display: flex;
  flex-wrap: wrap;
  margin-bottom: 28rpx;
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-line-soft);
  border-radius: var(--ch-radius-card);
  padding: 8rpx 0;
}
.cell {
  width: 25%;
  padding: 30rpx 0;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.ico {
  width: 84rpx;
  height: 84rpx;
  line-height: 84rpx;
  text-align: center;
  border-radius: 24rpx;
  color: #fff;
  font-size: 38rpx;
}
.cell-label {
  margin-top: 14rpx;
  font-size: 24rpx;
  color: var(--ch-text);
}

.sec-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4rpx;
  margin-bottom: 16rpx;
}
.sec-title {
  font-size: 30rpx;
  font-weight: 600;
  color: var(--ch-ink);
}
.sec-badge {
  font-size: 22rpx;
  color: var(--ch-accent);
  background: var(--ch-accent-soft);
  border-radius: 999rpx;
  padding: 4rpx 16rpx;
}

.feed {
  padding: 8rpx 28rpx;
}
.feed-empty {
  text-align: center;
  color: var(--ch-text-placeholder);
  padding: 48rpx 0;
  font-size: 26rpx;
}
.feed-item {
  display: flex;
  align-items: center;
  padding: 22rpx 0;
  border-bottom: 1rpx solid var(--ch-line-soft);
}
.feed-item:last-child {
  border-bottom: none;
}
.feed-ico {
  width: 64rpx;
  height: 64rpx;
  line-height: 64rpx;
  text-align: center;
  border-radius: 18rpx;
  color: #fff;
  font-size: 28rpx;
  margin-right: 20rpx;
  flex-shrink: 0;
}
.feed-body {
  flex: 1;
  min-width: 0;
}
.feed-title {
  font-size: 27rpx;
  color: var(--ch-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feed-meta {
  margin-top: 6rpx;
}
.arrow {
  color: var(--ch-text-placeholder);
  font-size: 34rpx;
  margin-left: 12rpx;
}

.hint {
  margin: 8rpx 32rpx 0;
  line-height: 1.7;
}
</style>
