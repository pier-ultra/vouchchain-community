// 跨端本地存储适配：统一走 uni storage，屏蔽 H5 / 小程序 / App 的差异。
// core / stores 只依赖本模块，不直接碰 uni.*，便于未来把 core 抽成共享包。
export function getItem(key: string): string {
  try {
    const v = uni.getStorageSync(key)
    if (v === '' || v === null || v === undefined) return ''
    return typeof v === 'string' ? v : String(v)
  } catch {
    return ''
  }
}

export function setItem(key: string, val: string): void {
  try {
    uni.setStorageSync(key, val)
  } catch {
    /* 静默失败：存储不可用不应阻断主流程 */
  }
}

export function removeItem(key: string): void {
  try {
    uni.removeStorageSync(key)
  } catch {
    /* ignore */
  }
}
