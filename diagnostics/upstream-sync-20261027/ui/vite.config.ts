import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'
const source = '/src/frontend'
const fixture = '/src/diagnostics/upstream-sync-20261027/ui'
export default defineConfig({
  root: fixture, plugins: [vue()], css: { postcss: source },
  resolve: { alias: [
    { find: '@/features/admin-accounts/data/datasources/adminAccountQueries', replacement: resolve(fixture, 'api.ts') },
    { find: '@/features/admin-accounts/data/datasources/adminAccountActions', replacement: resolve(fixture, 'api.ts') },
    { find: '@', replacement: resolve(source, 'src') },
  ] },
  server: { host: '0.0.0.0', port: 18370, fs: { allow: ['/src'] } },
})
