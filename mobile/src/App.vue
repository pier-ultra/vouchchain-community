<script setup lang="ts">
import { onLaunch } from '@dcloudio/uni-app'
import { useAuthStore } from './stores/auth'
import { useTheme } from './core/theme'

const { theme } = useTheme()

onLaunch(() => {
  // 恢复本地登录态（token/user 存在 uni storage，跨端一致）
  useAuthStore().restore()
})
</script>

<style>
/* ================= VouchChain 移动端设计系统（与 Web 同源） =================
 * 靛蓝主色 #4f46e5 / 深色墨蓝夜色。双主题用 page[data-theme] 属性驱动 CSS 变量，
 * 页面内一律 var(--ch-*) 取色，禁止硬编码颜色，保证亮/深切换无露白。
 * 统一尺度：圆角 8/12/16，字阶 24/26/28/30/34/40rpx，细边框、大留白、克制阴影。
 */
page[data-theme='light'],
page {
  --ch-accent: #4f46e5;
  --ch-accent-strong: #4338ca;
  --ch-accent-soft: #eef2ff;
  --ch-canvas: #f6f7fb;
  --ch-surface: #ffffff;
  --ch-surface-2: #fafbfd;
  --ch-surface-3: #f0f2f8;
  --ch-line: #d6dbe6;
  --ch-line-soft: #d9dde7;
  --ch-ink: #111827;
  --ch-text: #374151;
  --ch-text-muted: #566070;
  --ch-text-placeholder: #7a8598;
  --ch-user-bubble: #1e293b;
  --ch-user-bubble-text: #f8fafc;
  --ch-primary: #4f46e5;
  --ch-success: #0f9d6e;
  --ch-warning: #c77c0a;
  --ch-danger: #d64545;
  --ch-radius-card: 16rpx;
  --ch-radius-sm: 8rpx;
  --ch-shadow: 0 1rpx 2rpx rgba(16, 24, 40, 0.03), 0 8rpx 24rpx rgba(16, 24, 40, 0.04);
}

page[data-theme='dark'] {
  --ch-accent: #6d6ff3;
  --ch-accent-strong: #8b8df6;
  --ch-accent-soft: #23254e;
  --ch-canvas: #0b0f1a;
  --ch-surface: #131a2b;
  --ch-surface-2: #101626;
  --ch-surface-3: #1a2338;
  --ch-line: rgba(148, 163, 199, 0.16);
  --ch-line-soft: rgba(148, 163, 199, 0.10);
  --ch-ink: #e6eaf3;
  --ch-text: #c2cad9;
  --ch-text-muted: #8f9ab3;
  --ch-text-placeholder: #5f6b84;
  --ch-user-bubble: #253052;
  --ch-user-bubble-text: #eef1f8;
  --ch-primary: #6d6ff3;
  --ch-success: #34d399;
  --ch-warning: #fbbf24;
  --ch-danger: #f87171;
  --ch-radius-card: 16rpx;
  --ch-radius-sm: 8rpx;
  --ch-shadow: 0 1rpx 2rpx rgba(0, 0, 0, 0.2), 0 8rpx 24rpx rgba(0, 0, 0, 0.28);
}

page {
  background-color: var(--ch-canvas);
  color: var(--ch-text);
  font-size: 28rpx;
  line-height: 1.6;
  transition: background-color 0.25s ease, color 0.25s ease;
}

.page {
  min-height: 100vh;
  padding: 24rpx;
  box-sizing: border-box;
  background-color: var(--ch-canvas);
}

.card {
  background: var(--ch-surface);
  border: 1rpx solid var(--ch-line-soft);
  border-radius: var(--ch-radius-card);
  padding: 28rpx;
  margin-bottom: 20rpx;
  box-shadow: var(--ch-shadow);
}

.muted {
  color: var(--ch-text-muted);
  font-size: 24rpx;
}

.empty {
  text-align: center;
  color: var(--ch-text-placeholder);
  padding: 120rpx 0;
  font-size: 26rpx;
}

/* 主题切换过渡：切主题瞬间柔和渐变，避免生硬跳变 */
page.theme-switching,
page.theme-switching *,
page.theme-switching *::before,
page.theme-switching *::after {
  transition: background-color 0.25s ease, border-color 0.25s ease, color 0.25s ease !important;
}
</style>
