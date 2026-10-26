import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import zh from '@/core/i18n/locales/zh/admin/overview'
import en from '@/core/i18n/locales/en/admin/overview'
import Harness from './Harness.vue'
import '@/core/themes/style.css'
createApp(Harness).use(createI18n({legacy:false,locale:'zh',messages:{zh:{admin:zh},en:{admin:en}}})).mount('#app')
