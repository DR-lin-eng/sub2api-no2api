import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import Harness from './Harness.vue'
import accounts from '@/core/i18n/locales/zh/admin/accounts'
import '@/core/themes/style.css'
createApp(Harness).use(createPinia()).use(createI18n({ legacy: false, locale: 'zh', messages: { zh: { admin: accounts } } })).mount('#app')
