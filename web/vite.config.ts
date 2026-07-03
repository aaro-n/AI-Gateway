import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:18080',
        changeOrigin: true
      },
      '/openai': {
        target: 'http://localhost:18080',
        changeOrigin: true
      }
    }
  },
  build: {
    // 生成 manifest.json：映射原始路径 → 带 hash 的文件名
    manifest: true,
    rollupOptions: {
      output: {
        // JS: assets/login-[hash].js
        entryFileNames: 'assets/[name]-[hash].js',
        // 代码分割 chunk: assets/chunk-[hash].js
        chunkFileNames: 'assets/[name]-[hash].js',
        // CSS / 图片 / 字体等: assets/[name]-[hash].[ext]
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
})
