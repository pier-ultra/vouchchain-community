<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, exportFile, type Row } from '../../services/api'
import { shortTime } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const retentionDays = ref<number | null>(null)
const compliant = ref<boolean | null>(null)
const floor = ref(180)
const chain = ref<Row[]>([])
const blocks = ref<Row[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const st = await api.dengbaoRetentionStatus()
    if (st?.code === 0 && st.data) {
      const d = st.data
      retentionDays.value = d.retentionDays ?? d.retention_days ?? null
      compliant.value = d.compliant ?? d.ok ?? null
      floor.value = d.floorDays ?? d.floor ?? 180
    }
  } catch {
    /* ignore */
  }
  try {
    const pc = await api.dengbaoPermChain({ limit: 50 })
    if (pc?.code === 0) chain.value = pc.data || []
  } catch {
    /* ignore */
  }
  try {
    const bk = await api.dengbaoEvidenceBlocks()
    if (bk?.code === 0) blocks.value = bk.data || []
  } catch {
    /* ignore */
  }
  loading.value = false
}

onShow(load)

async function doExportPack() {
  uni.showLoading({ title: '打包中…', mask: true })
  const r = await exportFile('/dengbao/evidence/pack', `等保证据包_${Date.now()}.zip`, {})
  uni.hideLoading()
  uni.showToast({ title: r.ok ? '证据包已导出' : (r.message || '导出失败'), icon: r.ok ? 'success' : 'none' })
}

async function doExportBlock(block: string) {
  const nameMap: Record<string, string> = { 身份鉴别: 'auth', 访问控制: 'access', 安全审计: 'audit' }
  const slug = nameMap[block] || 'audit'
  const r = await exportFile('/dengbao/evidence/export', `evidence-${slug}.xlsx`, { block })
  uni.showToast({ title: r.ok ? '已导出' : (r.message || '导出失败'), icon: r.ok ? 'success' : 'none' })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="card">
      <view class="row">
        <view class="h">审计日志留存</view>
        <view
          class="pill"
          :class="compliant === false ? 'bad' : compliant === true ? 'ok' : ''"
        >
          {{ compliant === false ? '不达标' : compliant === true ? '达标' : '未知' }}
        </view>
      </view>
      <view class="big">{{ retentionDays ?? '—' }} <text class="unit">天</text></view>
      <view class="muted">等保 2.0（GB/T 22239-2019）安全审计留存下限 {{ floor }} 天</view>
      <view v-if="compliant === false" class="warn">当前留存天数低于合规下限，请在 Web 管理台调整留存策略。</view>
    </view>

    <view class="card">
      <view class="section-title sm">证据导出</view>
      <view class="btn-row">
        <view class="btn primary" @click="doExportPack">导出完整证据包（zip）</view>
      </view>
      <view v-if="blocks.length" class="blocks">
        <view v-for="b in blocks" :key="b.block || b.name" class="block-item">
          <view class="b-info">
            <view class="b-name">{{ b.name || b.block || '证据块' }}</view>
            <view class="muted">{{ b.count ?? b.rows ?? 0 }} 条记录</view>
          </view>
          <view class="btn mini" @click="doExportBlock(b.name || b.block)">导出</view>
        </view>
      </view>
    </view>

    <view class="section-title">权限变更证据链（近 50 条）</view>
    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!chain.length" class="empty">暂无权限变更记录</view>

    <view v-for="r in chain" :key="r.id" class="card slim">
      <view class="row">
        <view class="act">{{ r.action || r.op || '—' }}</view>
        <view class="muted">{{ shortTime(r.createdAt || r.created_at) }}</view>
      </view>
      <view class="muted">操作人：{{ r.username || r.user || '—' }} · 对象：{{ r.targetType || r.target_type || '—' }} {{ r.targetId || r.target_id || '' }}</view>
    </view>
  </view>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.h {
  font-size: 30rpx;
  font-weight: 500;
}
.pill {
  font-size: 24rpx;
  color: var(--ch-text-muted);
  border: 1rpx solid var(--ch-line);
  border-radius: 24rpx;
  padding: 4rpx 20rpx;
}
.pill.ok {
  color: var(--ch-success);
  border-color: var(--ch-success);
}
.pill.bad {
  color: var(--ch-danger);
  border-color: var(--ch-danger);
}
.big {
  font-size: 64rpx;
  font-weight: 500;
  margin: 20rpx 0 8rpx;
  color: var(--ch-ink);
}
.unit {
  font-size: 28rpx;
  color: var(--ch-text-muted);
}
.warn {
  margin-top: 16rpx;
  color: var(--ch-danger);
  font-size: 26rpx;
  line-height: 1.6;
}
.section-title {
  font-size: 28rpx;
  font-weight: 500;
  margin: 28rpx 8rpx 16rpx;
}
.section-title.sm {
  margin: 0 0 20rpx;
}
.btn-row {
  display: flex;
  gap: 16rpx;
}
.btn {
  padding: 16rpx 24rpx;
  border-radius: 12rpx;
  font-size: 26rpx;
  text-align: center;
}
.btn.primary {
  background: var(--ch-accent);
  color: #fff;
  flex: 1;
}
.btn.mini {
  background: var(--ch-accent);
  color: #fff;
  padding: 12rpx 28rpx;
}
.blocks {
  margin-top: 16rpx;
}
.block-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 18rpx 0;
  border-bottom: 1rpx solid var(--ch-line-soft);
}
.block-item:last-child {
  border-bottom: none;
}
.b-name {
  font-size: 28rpx;
  font-weight: 500;
}
.card.slim {
  padding: 22rpx 28rpx;
}
.act {
  font-size: 28rpx;
  font-weight: 500;
}
</style>
