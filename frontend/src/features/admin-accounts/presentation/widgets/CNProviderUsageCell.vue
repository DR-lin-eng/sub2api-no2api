<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import { platformTextClass } from '@/core/utils/platformColors'
import { defaultCNBaseURL } from '@/core/constants/account'
import type { Account } from '@/types'
import {
  queryCNProviderBalance,
  queryCNProviderQuota,
  type CNProviderBalanceEntry,
  type CNProviderBalanceResult,
  type CNProviderQuotaProbeResult,
} from '../../data/datasources/cnProviderDatasource'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()

const loading = ref(false)
const error = ref('')
const quotaResult = ref<CNProviderQuotaProbeResult | null>(null)
const balanceResult = ref<CNProviderBalanceResult | null>(null)

const credentials = computed(() => props.account.credentials as Record<string, unknown> | undefined)
const mode = computed(() => typeof credentials.value?.account_mode === 'string' ? credentials.value.account_mode : '')
const kimiOfficialBalanceEndpoint = computed(() => {
  if (props.account.platform !== 'kimi') return false
  const apiBaseURLs = credentials.value?.api_base_urls && typeof credentials.value.api_base_urls === 'object'
    ? credentials.value.api_base_urls as Record<string, unknown>
    : {}
  const rawBaseURL = credentials.value?.api_protocol === 'adaptive' && typeof apiBaseURLs.chat_completions === 'string'
    ? apiBaseURLs.chat_completions
    : typeof credentials.value?.base_url === 'string'
      ? credentials.value.base_url
      : defaultCNBaseURL('kimi', mode.value === 'coding' ? 'coding' : 'payg')
  try {
    return new URL(rawBaseURL).hostname.toLowerCase() === 'api.moonshot.cn'
  } catch {
    return false
  }
})
const quotaVisible = computed(() => props.account.platform === 'opencode_go'
  ? mode.value !== 'zen'
  : ['kimi', 'zhipu', 'minimax'].includes(props.account.platform) && mode.value === 'coding')
const balanceVisible = computed(() => mode.value !== 'coding' && (
  props.account.platform === 'deepseek' || kimiOfficialBalanceEndpoint.value
))
const probeAvailable = computed(() => quotaVisible.value || balanceVisible.value)

const readExtraNumber = (suffix: string): number | null => {
  const value = props.account.extra?.[`${props.account.platform}_${suffix}`]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

const readExtraString = (suffix: string): string => {
  const value = props.account.extra?.[`${props.account.platform}_${suffix}`]
  return typeof value === 'string' ? value : ''
}

const snapshotQuota = computed<CNProviderQuotaProbeResult | null>(() => {
  const tiers: NonNullable<CNProviderQuotaProbeResult['tiers']> = []
  for (const window of ['5h', 'weekly', 'monthly'] as const) {
    const used = readExtraNumber(`${window}_used_percent`)
    if (used == null) continue
    tiers.push({
      window,
      used_percent: used,
      reset_at: readExtraString(`${window}_reset_at`) || undefined,
    })
  }
  return tiers.length > 0
    ? { provider: props.account.platform, success: true, credential_valid: true, tiers, fetched_at: 0, persisted: true }
    : null
})

const displayedQuota = computed(() => quotaResult.value?.success ? quotaResult.value : snapshotQuota.value)

const snapshotBalances = computed<CNProviderBalanceEntry[]>(() => {
  const raw = props.account.extra?.[`${props.account.platform}_balances`]
  if (Array.isArray(raw)) {
    return raw.flatMap((entry): CNProviderBalanceEntry[] => {
      if (!entry || typeof entry !== 'object') return []
      const value = entry as Record<string, unknown>
      return typeof value.balance === 'number' && typeof value.currency === 'string'
        ? [{ balance: value.balance, currency: value.currency }]
        : []
    })
  }
  const balance = readExtraNumber('balance')
  return balance == null ? [] : [{ balance, currency: readExtraString('balance_currency') }]
})

const displayedBalances = computed(() => {
  if (balanceResult.value?.success) {
    return balanceResult.value.balances?.length
      ? balanceResult.value.balances
      : [{ balance: balanceResult.value.balance, currency: balanceResult.value.currency || '' }]
  }
  return snapshotBalances.value
})

const balanceLabel = computed(() => displayedBalances.value.map((entry) => {
  const value = entry.balance >= 100 ? entry.balance.toFixed(0) : entry.balance.toFixed(2)
  return `${entry.currency || 'CNY'} ${value}`
}).join(' / '))

const windowLabel = (window: string) => t(`admin.accounts.cnProvider.window.${window}`)

const refresh = async () => {
  if (loading.value || !probeAvailable.value) return
  loading.value = true
  error.value = ''
  try {
    if (quotaVisible.value) {
      const result = await queryCNProviderQuota(props.account.id)
      if (result.success) quotaResult.value = result
      else error.value = result.error || t('common.error')
    } else {
      const result = await queryCNProviderBalance(props.account.id)
      if (result.success) balanceResult.value = result
      else error.value = result.error || t('common.error')
    }
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : t('common.error')
  } finally {
    loading.value = false
  }
}

watch(() => props.account.id, () => {
  quotaResult.value = null
  balanceResult.value = null
  error.value = ''
})
</script>

<template>
  <div class="min-w-[180px] space-y-1" data-test="cn-provider-usage">
    <template v-if="quotaVisible">
      <UsageProgressBar
        v-for="tier in displayedQuota?.tiers || []"
        :key="tier.window"
        :label="windowLabel(tier.window)"
        :utilization="tier.used_percent"
        :resets-at="tier.reset_at"
        :color="tier.window === 'weekly' || tier.window === 'monthly' ? 'emerald' : 'indigo'"
      />
    </template>
    <p v-else-if="balanceVisible && balanceLabel" :class="['text-[10px] font-medium', platformTextClass(account.platform)]">
      {{ balanceLabel }}
    </p>
    <p v-else-if="!probeAvailable" class="text-[10px] text-gray-400" :title="t('admin.accounts.cnProvider.noPublicEndpoint')">
      {{ t('admin.accounts.cnProvider.noPublicEndpoint') }}
    </p>

    <button
      v-if="probeAvailable"
      type="button"
      class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 hover:bg-blue-50 disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
      :disabled="loading"
      :title="t('admin.accounts.cnProvider.refresh')"
      @click="refresh"
    >
      <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
      {{ t('admin.accounts.cnProvider.refresh') }}
    </button>
    <p v-if="error" class="max-w-[220px] truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ error }}
    </p>
  </div>
</template>
