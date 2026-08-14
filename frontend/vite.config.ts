import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: { outDir: '../web-dist', emptyOutDir: true },
  server: { port: 4173, proxy: { '/api': 'http://127.0.0.1:12100', '/health': 'http://127.0.0.1:12100' } }
})
