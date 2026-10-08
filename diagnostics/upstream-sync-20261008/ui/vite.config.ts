import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  root: '/workspace/frontend/sync-harness',
  plugins: [vue()],
  cacheDir: '/workspace/frontend/node_modules/.vite-sync-20261008',
  resolve: { alias: { '@': '/workspace/frontend/src' } },
  server: { host: '0.0.0.0', port: 5173, strictPort: true, fs: { allow: ['/workspace'] } }
})
