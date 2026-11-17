import { ref, watch } from 'vue'

// 主题：亮 / 深（墨蓝夜色），与 Web 管理台同源（ch_theme）。
// uni-app 跨端：H5 用 localStorage + document.documentElement 属性；
// 小程序 / App 用 uni.getStorageSync + 动态 class 由各页根节点 data-theme 驱动。
// 统一约定：页面根 <view class="page" :data-theme="theme.theme">，样式用 CSS 变量取色。

const KEY = 'ch_theme'
export type ThemeMode = 'light' | 'dark'

function initial(): ThemeMode {
  try {
    // #ifdef H5
    const saved = localStorage.getItem(KEY)
    if (saved === 'light' || saved === 'dark') return saved
    const m = window.matchMedia?.('(prefers-color-scheme: dark)')
    return m?.matches ? 'dark' : 'light'
    // #endif
    // #ifndef H5
    const saved = uni.getStorageSync(KEY)
    if (saved === 'light' || saved === 'dark') return saved as ThemeMode
    return 'light'
    // #endif
  } catch {
    return 'light'
  }
}

const theme = ref<ThemeMode>(initial())

function applyToDoc() {
  // #ifdef H5
  try {
    document.documentElement.setAttribute('data-theme', theme.value)
  } catch { /* 忽略 */ }
  // #endif
}

function persist() {
  try {
    // #ifdef H5
    localStorage.setItem(KEY, theme.value)
    // #endif
    // #ifndef H5
    uni.setStorageSync(KEY, theme.value)
    // #endif
  } catch { /* 忽略 */ }
}

function toggle() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  persist()
  applyToDoc()
}

// 模块加载即对齐（防闪烁）
applyToDoc()
watch(theme, (v) => {
  persist()
  applyToDoc()
})

export function useTheme() {
  return { theme, isDark: theme, toggle }
}

export function isDarkNow(): boolean {
  return theme.value === 'dark'
}
