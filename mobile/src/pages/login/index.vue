<script setup lang="ts">
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { useAuthStore } from '../../stores/auth'
import { getItem } from '../../services/storage'
import { useTheme } from '../../core/theme'

const { theme } = useTheme()

const auth = useAuthStore()
const username = ref('admin')
const loading = ref(false)

onLoad(() => {
  // 已有登录态直接进首页
  if (getItem('ch_token')) uni.reLaunch({ url: '/pages/home/index' })
})

async function onSubmit() {
  const u = username.value.trim()
  if (!u) {
    uni.showToast({ title: '请输入用户名', icon: 'none' })
    return
  }
  if (loading.value) return
  loading.value = true
  try {
    await auth.login(u)
    uni.reLaunch({ url: '/pages/home/index' })
  } catch {
    uni.showToast({ title: '登录失败', icon: 'none' })
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <view class="login" :data-theme="theme">
    <view class="brand">
      <view class="logo">合</view>
      <view class="name">VouchChain</view>
      <view class="slogan">法务 · 财务一体化的随身端</view>
    </view>
    <view class="card">
      <view class="label">用户名</view>
      <input class="ipt" v-model="username" placeholder="请输入用户名" placeholder-class="ph" />
      <view class="btn" :class="{ disabled: loading }" @click="onSubmit">
        {{ loading ? '登录中…' : '进入' }}
      </view>
      <view class="tip">开发态任意用户名可进入；生产态走 SSO 单点登录（Casdoor）。</view>
    </view>
  </view>
</template>

<style scoped>
.login {
  min-height: 100vh;
  padding: 160rpx 48rpx 0;
  box-sizing: border-box;
}
.brand {
  text-align: center;
  margin-bottom: 64rpx;
}
.logo {
  width: 128rpx;
  height: 128rpx;
  line-height: 128rpx;
  margin: 0 auto 24rpx;
  border-radius: 32rpx;
  background: var(--ch-accent);
  color: #fff;
  font-size: 60rpx;
  text-align: center;
}
.name {
  font-size: 40rpx;
  font-weight: 500;
  color: var(--ch-ink);
}
.slogan {
  margin-top: 12rpx;
  font-size: 26rpx;
  color: var(--ch-text-muted);
}
.card {
  background: var(--ch-surface);
  border-radius: 20rpx;
  padding: 40rpx 36rpx;
}
.label {
  font-size: 26rpx;
  color: var(--ch-text);
  margin-bottom: 16rpx;
}
.ipt {
  height: 88rpx;
  border: 1rpx solid var(--ch-line);
  border-radius: 12rpx;
  padding: 0 24rpx;
  font-size: 30rpx;
}
.ph {
  color: var(--ch-text-placeholder);
}
.btn {
  margin-top: 40rpx;
  height: 88rpx;
  line-height: 88rpx;
  text-align: center;
  border-radius: 12rpx;
  background: var(--ch-accent);
  color: #fff;
  font-size: 32rpx;
}
.btn.disabled {
  opacity: 0.6;
}
.tip {
  margin-top: 24rpx;
  font-size: 24rpx;
  color: var(--ch-text-muted);
  line-height: 1.6;
}
</style>
