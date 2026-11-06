import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
const fixture = '/app/frontend/ui-fixture/fixtureApi.ts'
export default defineConfig({
 plugins:[vue()],
 resolve:{alias:[
 {find:'@/features/admin-settings/data/datasources/registrationEmailBlacklistDatasource', replacement:fixture},
 {find:'@/features/admin-users/data/datasources/userBlacklistActions', replacement:fixture},
 {find:'@/features/auth', replacement:fixture},
 {find:'@/core/stores/appStore', replacement:fixture},
 {find:'@',replacement:'/app/frontend/src'}
 ]},
 server:{host:'0.0.0.0',port:4173}, css:{postcss:'/app/frontend/postcss.config.js'}
})
