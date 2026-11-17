import { createApp } from 'vue'
import { createPinia } from 'pinia'
import * as Icons from '@element-plus/icons-vue'
import { createHead } from '@unhead/vue/client'
import App from './App.vue'
import router from './router'
import './styles/theme.css'
// Element Plus 官方暗色变量（html.dark 生效），具体色值在 theme.css 里按墨蓝风覆盖
import 'element-plus/theme-chalk/dark/css-vars.css'

const app = createApp(App)

// @scalar/api-reference 内部用 useHead()（unhead），需在此提供 head 上下文
const head = createHead()
app.use(head)

app.use(createPinia())
app.use(router)
// Element Plus 组件改为 unplugin 按需引入（见 vite.config.ts），不再全量 use

// 全量注册 Element Plus 图标（体积小；后续可换 unplugin-icons 进一步精简）
for (const [name, comp] of Object.entries(Icons)) {
  app.component(name, comp as never)
}

app.mount('#app')
