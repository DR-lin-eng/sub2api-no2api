import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import Panel from '@/features/admin-accounts/presentation/widgets/GrokModelDiagnosticsPanel.vue'
import en from '@/core/i18n/locales/en/admin/accounts.grok-catalog'
import zh from '@/core/i18n/locales/zh/admin/accounts.grok-catalog'
import '@/core/themes/style.css'
const locale = new URLSearchParams(location.search).get('lang') || 'zh'
createApp(Panel, { accountId: 1 }).use(createI18n({ legacy: false, locale,
  messages: { en: { admin: { accounts: { grokCatalog: en } }, common: { status: 'Status' } }, zh: { admin: { accounts: { grokCatalog: zh } }, common: { status: '状态' } } },
})).mount('#app')
