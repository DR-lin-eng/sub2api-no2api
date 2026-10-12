import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import App from './Harness.vue'
import en from '@/core/i18n/locales/en/admin/overview'
import zh from '@/core/i18n/locales/zh/admin/overview'
import '@/core/themes/style.css'
const locale = new URLSearchParams(location.search).get('locale') || 'en'
createApp(App).use(createI18n({ legacy: false, locale, messages: { en: { admin: en }, zh: { admin: zh } } })).mount('#app')
