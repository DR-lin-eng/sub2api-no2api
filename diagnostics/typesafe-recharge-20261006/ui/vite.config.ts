import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  root: '/workspace/frontend/sync-harness',
  plugins: [vue()],
  cacheDir: '/workspace/frontend/node_modules/.vite-adapt-20261006',
  resolve: { alias: [
    { find: '@/core/stores/appStore', replacement: '/workspace/frontend/sync-harness/mockAppStore.ts' },
    { find: '@', replacement: '/workspace/frontend/src' }
  ] },
  server: { host: '0.0.0.0', port: 5173, strictPort: true, fs: { allow: ['/workspace'] } }
})
