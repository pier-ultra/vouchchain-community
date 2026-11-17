// 主题切换：亮色 / 深色（墨蓝夜色）
// html.dark class 驱动，localStorage 持久化，index.html 里有防闪白预读脚本
import { ref } from 'vue'

const THEME_KEY = 'ch_theme'

function initialTheme(): 'light' | 'dark' {
  try {
    const saved = localStorage.getItem(THEME_KEY)
    if (saved === 'dark' || saved === 'light') return saved
    // 未设置过：跟随系统
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

const isDark = ref(initialTheme())

function apply() {
  document.documentElement.classList.toggle('dark', isDark.value === 'dark')
}

// 模块加载即对齐当前 class（配合 index.html 预读，双保险）
apply()

export function useTheme() {
  function toggle() {
    isDark.value = isDark.value === 'dark' ? 'light' : 'dark'
    try { localStorage.setItem(THEME_KEY, isDark.value) } catch { /* 忽略 */ }
    // 加过渡类，切完移除，避免常驻 transition 影响交互反馈
    const el = document.documentElement
    el.classList.add('theme-switching')
    apply()
    window.setTimeout(() => el.classList.remove('theme-switching'), 300)
  }
  return { isDark, toggle }
}
