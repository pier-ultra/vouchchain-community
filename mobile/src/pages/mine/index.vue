<script setup lang="ts">
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { useAuthStore } from '../../stores/auth'
import { getSession, roleLabels, currentRole } from '../../core/roles'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const auth = useAuthStore()
const name = ref(auth.user)
const roleText = ref('')
const authMode = ref('dev')

onShow(() => {
  const s = getSession()
  if (s) {
    name.value = s.displayName || s.username
    roleText.value = s.roleName || roleLabels[s.role] || s.role
    authMode.value = s.authMode || 'dev'
  } else {
    roleText.value = roleLabels[currentRole()] || currentRole()
  }
})

function onLogout() {
  uni.showModal({
    title: '退出登录',
    content: '确认退出当前账号？',
    success: (r) => {
      if (r.confirm) auth.logout()
    },
  })
}
</script>

<template>
  <view class="page" :data-theme="theme">
    <view class="card head">
      <view class="avatar">{{ (name || 'U').slice(0, 1).toUpperCase() }}</view>
      <view class="info">
        <view class="name">{{ name || '未登录' }}</view>
        <view class="muted">{{ roleText }} · {{ authMode === 'dev' ? '开发态' : 'SSO 单点登录' }}</view>
      </view>
    </view>

    <view class="card list">
      <view class="item" @click="uni.switchTab({ url: '/pages/approvals/index' })">
        <view class="k">审批待办</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.switchTab({ url: '/pages/contracts/index' })">
        <view class="k">合同台账</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.navigateTo({ url: '/pages/invoices/index' })">
        <view class="k">发票中心</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.navigateTo({ url: '/pages/seals/index' })">
        <view class="k">用印管理</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.navigateTo({ url: '/pages/dengbao/index' })">
        <view class="k">等保证据</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.navigateTo({ url: '/pages/revrec/index' })">
        <view class="k">收入确认</view>
        <view class="arrow">›</view>
      </view>
      <view class="item" @click="uni.switchTab({ url: '/pages/chat/index' })">
        <view class="k">AI 助手</view>
        <view class="arrow">›</view>
      </view>
    </view>

    <view class="card community">
      <view class="title">加入社区交流群</view>
      <view class="qrs">
        <view class="qr">
          <image class="qr-img" src="/static/community/qrcode-qqgroup.jpg" mode="aspectFit" />
          <view class="qr-label">QQ 群 · 开源社区</view>
        </view>
        <view class="qr">
          <image class="qr-img" src="/static/community/qrcode-wechat-personal.jpg" mode="aspectFit" />
          <view class="qr-label">微信 · 拉你进群</view>
        </view>
      </view>
      <view class="muted hint">扫码加 QQ 群，或加作者微信（备注 VouchChain）拉你进微信群</view>
    </view>

    <view class="card logout" @click="onLogout">退出登录</view>
    <view class="muted ver">VouchChain · 移动端 v0.3.0</view>
  </view>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
}
.avatar {
  width: 104rpx;
  height: 104rpx;
  line-height: 104rpx;
  text-align: center;
  border-radius: 50%;
  background: var(--ch-accent);
  color: #fff;
  font-size: 44rpx;
  margin-right: 24rpx;
}
.name {
  font-size: 34rpx;
  font-weight: 500;
}
.list {
  padding: 0 28rpx;
}
.item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 30rpx 0;
  border-bottom: 1rpx solid var(--ch-line-soft);
}
.item:last-child {
  border-bottom: none;
}
.k {
  font-size: 30rpx;
}
.arrow {
  color: var(--ch-text-placeholder);
  font-size: 36rpx;
}
.community {
  padding: 28rpx;
}
.community .title {
  font-size: 30rpx;
  font-weight: 500;
  margin-bottom: 20rpx;
}
.qrs {
  display: flex;
  gap: 24rpx;
  margin-bottom: 16rpx;
}
.qr {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8rpx;
}
.qr-img {
  width: 220rpx;
  height: 220rpx;
  border-radius: 16rpx;
  border: 1rpx solid var(--ch-line-soft);
  background: #fff;
}
.qr-label {
  font-size: 24rpx;
  color: var(--ch-text-secondary, #666);
}
.hint {
  font-size: 22rpx;
}
.logout {
  text-align: center;
  color: var(--ch-danger);
  font-size: 30rpx;
}
.ver {
  text-align: center;
  margin-top: 24rpx;
}
</style>
