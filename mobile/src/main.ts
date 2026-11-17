import { createSSRApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'

// uni-app Vue3 要求导出 createApp 工厂函数（而非直接 mount）
export function createApp() {
  const app = createSSRApp(App)
  app.use(createPinia())
  return { app }
}
