import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({plugins:[vue()],resolve:{alias:[{find:'@/features/admin-accounts/data/datasources/adminAccountQueries',replacement:'/work/frontend/ui-fixture/fixtureApi.ts'},{find:'@',replacement:'/work/frontend/src'}]},server:{host:'0.0.0.0',port:4173},css:{postcss:'/work/frontend/postcss.config.js'}})
