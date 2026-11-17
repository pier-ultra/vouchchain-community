import { defineConfig } from 'vite'
import uni from '@dcloudio/vite-plugin-uni'

// uni-app 统一由该插件驱动多端编译（H5 / 微信小程序 / App）
export default defineConfig({
  plugins: [uni()],
  server: {
    port: 5273,
    proxy: {
      // 开发态把 /api 转发到本地 Go 后端，与 web/ 的约定一致
      '/api': {
        target: 'http://127.0.0.1:8000',
        changeOrigin: true,
      },
    },
  },
})
