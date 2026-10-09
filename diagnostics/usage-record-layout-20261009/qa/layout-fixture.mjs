import { createApp, h, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import UsageTable from './src/features/admin-usage/presentation/widgets/UsageTable.vue'
import zh from './src/core/i18n/locales/zh/dashboard.ts'
import en from './src/core/i18n/locales/en/dashboard.ts'
import './src/core/themes/style.css'

const params = new URLSearchParams(location.search)
const locale = params.get('locale') || 'zh'
const audience = params.get('audience') || 'admin'
document.documentElement.classList.toggle('dark', params.get('theme') !== 'light')
const i18n = createI18n({ legacy: false, locale, fallbackLocale: 'en', messages: { zh, en }, missingWarn: false, fallbackWarn: false })
const input = await fetch('/qa-input.json').then((response) => response.json())
const rows = input.rows.map((row) => ({
  ...row,
  request_id: `req-layout-${row.id}`,
  model: 'gpt-6-sol',
  billing_mode: 'token',
  stream: true,
  actual_cost: 0.01,
  total_cost: 0.01,
  rate_multiplier: 1,
  account_rate_multiplier: 1,
  input_tokens: 100,
  cache_read_tokens: 0,
  cache_creation_tokens: 0,
  image_count: 0,
}))
if (params.get('missing') === 'first') rows[0].first_token_ms = null
if (params.get('missing') === 'timing') delete rows[0].openai_timing
if (params.get('missing') === 'zero') {
  rows[0].first_token_ms = 0
  rows[0].duration_ms = 0
  rows[0].local_first_token_ms = 0
  rows[0].openai_timing = { engine_service_ttft_total_ms: 0 }
}
const columns = [
  { key: 'cost', label: i18n.global.t('usage.cost') },
  { key: 'latency', label: i18n.global.t('usage.latency') },
]
const app = createApp({ render: () => h('main', { class: 'p-4', style: 'max-width: 900px; margin: auto' }, [h(UsageTable, { data: rows, columns, audience, flat: true })]) })
app.use(createPinia()).use(i18n).mount('#app')
await nextTick()
if (params.get('extra') === '1') {
  // Model only the additional label/value placement visible in the attachment.
  // This is a fixture, not a claim about the source of the extra production nodes.
  for (const [index, speed] of [...document.querySelectorAll('[data-testid="output-token-speed"]')].entries()) {
    const root = speed.closest('.grid-cols-\\[max-content_max-content\\]')
    const totalLabel = [...root.querySelectorAll('span,dt')].find((element) => element.textContent === i18n.global.t('usage.latencyDuration'))
    const totalValue = totalLabel.nextElementSibling
    let anchor = totalValue
    while (anchor.parentElement !== root) anchor = anchor.parentElement
    const label = document.createElement('span')
    label.className = 'text-gray-400 dark:text-gray-500'
    label.textContent = '输出 TPS'
    const value = document.createElement('span')
    value.className = 'tabular-nums text-gray-600 dark:text-gray-300'
    value.textContent = index === 0 ? '36.5 tok/s' : '9.4 tok/s'
    root.insertBefore(label, anchor)
    root.insertBefore(value, anchor)
  }
}
document.body.dataset.ready = 'true'
