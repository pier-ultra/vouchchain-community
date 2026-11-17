<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, type Row } from '../../services/api'
import { money, dateOnly } from '../../core/format'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const list = ref<Row[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await api.revrecList()
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

function title(r: Row): string {
  return r.contractTitle || r.title || r.contractNo || r.contract_title || `确认单 #${r.id}`
}
function amount(r: Row): number {
  return Number(r.amount ?? r.confirmedAmount ?? r.total ?? 0)
}

// R-A8：AI/规则只出建议（draft），会计确认（confirm）才落账
function doConfirm(r: Row) {
  uni.showModal({
    title: '确认收入',
    content: `确认将「${title(r)}」落账？${money(amount(r))}\n（确认后生成凭证草稿，不可撤销）`,
    success: async (res) => {
      if (!res.confirm) return
      uni.showLoading({ title: '确认中…', mask: true })
      try {
        const out = await api.revrecConfirm(Number(r.id))
        uni.hideLoading()
        if (out?.code === 0) {
          uni.showToast({ title: '已确认落账', icon: 'success' })
          load()
        } else {
          uni.showToast({ title: out?.message || '确认失败', icon: 'none' })
        }
      } catch {
        uni.hideLoading()
      }
    },
  })
}

function isDraft(r: Row): boolean {
  return (r.status || '').toLowerCase() === 'draft'
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view v-if="loading" class="empty">加载中…</view>
    <view v-else-if="!list.length" class="empty">暂无收入确认单</view>

    <view v-for="r in list" :key="r.id" class="card">
      <view class="row">
        <view class="title">{{ title(r) }}</view>
        <view class="badge" v-if="r.status">{{ r.status }}</view>
      </view>
      <view class="row2">
        <view class="amount">{{ money(amount(r)) }}</view>
        <view class="muted">{{ dateOnly(r.confirmDate || r.confirm_date || r.createdAt) }}</view>
      </view>
      <view v-if="r.period || r.periodMonth" class="muted">期间：{{ r.period || r.periodMonth }}</view>
      <view v-if="isDraft(r)" class="acts">
        <view class="act confirm" @click="doConfirm(r)">确认落账</view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.title {
  flex: 1;
  font-size: 30rpx;
  font-weight: 500;
  margin-right: 16rpx;
}
.badge {
  font-size: 24rpx;
  color: var(--ch-text-muted);
}
.row2 {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16rpx;
}
.amount {
  font-size: 32rpx;
  font-weight: 500;
  color: var(--ch-success);
}
.muted {
  margin-top: 10rpx;
}
.acts {
  margin-top: 20rpx;
}
.act {
  text-align: center;
  padding: 18rpx 0;
  border-radius: 12rpx;
  font-size: 26rpx;
}
.act.confirm {
  background: var(--ch-success);
  color: #fff;
}
</style>
