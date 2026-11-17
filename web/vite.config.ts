import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// 本地开发：npm run dev
// 后端代理目标可用环境变量覆盖：API_PROXY_TARGET=http://127.0.0.1:8000 npm run dev
export default defineConfig({
  plugins: [
    vue(),
    // Element Plus 按需引入：API（ElMessage 等）自动导入 + 组件/样式按需注册
    AutoImport({ resolvers: [ElementPlusResolver()] }),
    Components({ resolvers: [ElementPlusResolver()] }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.API_PROXY_TARGET || 'http://127.0.0.1:8000',
        changeOrigin: true,
        ws: true, // 任务中心 WebSocket 进度推送
      },
      '/openapi.json': {
        target: process.env.API_PROXY_TARGET || 'http://127.0.0.1:8000',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // 业务代码与第三方库分离，浏览器可独立缓存 vendor，改善首屏加载
        manualChunks(id) {
          if (id.includes('node_modules/element-plus') || id.includes('node_modules/@element-plus')) {
            return 'vendor-element'
          }
          if (
            id.includes('node_modules/vue') ||
            id.includes('node_modules/pinia') ||
            id.includes('node_modules/axios') ||
            id.includes('node_modules/vue-router')
          ) {
            return 'vendor-vue'
          }
          return undefined
        },
      },
    },
  },
})
