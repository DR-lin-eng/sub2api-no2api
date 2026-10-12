import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  root: '/app/frontend/sync-harness', plugins: [vue()],
  resolve: { alias: [
    { find: /^@\/core\/i18n$/, replacement: '/app/frontend/sync-harness/i18nMock.ts' },
    { find: '@/core/stores/appStore', replacement: '/app/frontend/sync-harness/appStoreMock.ts' },
    { find: '@', replacement: '/app/frontend/src' }
  ] },
  server: { host: '0.0.0.0', port: 4173, strictPort: true }
})
