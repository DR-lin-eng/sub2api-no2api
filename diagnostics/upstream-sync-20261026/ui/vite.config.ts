import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  root: '/app/frontend/node_modules/.sync-ui-20261026',
  plugins: [vue()],
  cacheDir: '/app/frontend/node_modules/.vite-sync-20261026',
  resolve: { alias: { '@': '/app/frontend/src' } },
  css: { postcss: '/app/frontend' },
  server: { host: '0.0.0.0', port: 5178, strictPort: true, fs: { allow: ['/app'] } },
})
