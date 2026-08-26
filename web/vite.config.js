import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:18080',
      '/sub': 'http://127.0.0.1:18080',
    },
  },
  build: {
    outDir: resolve(__dirname, '../internal/web/dist'),
    emptyOutDir: true,
  },
})
