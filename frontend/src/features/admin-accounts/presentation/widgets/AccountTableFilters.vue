<template>
  <div class="min-w-0 w-full space-y-2">
    <div class="grid grid-cols-2 items-center gap-2 sm:flex sm:flex-wrap">
      <SearchInput :model-value="searchQuery" :placeholder="t('admin.accounts.searchAccounts')" class="col-span-2 min-w-0 w-full sm:w-56" @update:model-value="$emit('update:searchQuery', $event)" @search="$emit('change')" />
      <Select :model-value="filters.platform" :aria-label="t('admin.accounts.allPlatforms')" class="min-w-0 w-full sm:w-36" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
      <Select :model-value="filters.status" :aria-label="t('admin.accounts.allStatus')" class="min-w-0 w-full sm:w-36" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
      <button type="button" class="btn btn-secondary col-span-2 min-w-0 w-full sm:w-auto" :aria-expanded="moreFiltersOpen" :aria-controls="moreFiltersId" :aria-label="activeSecondaryCount ? t('admin.accounts.moreFiltersActive', { count: activeSecondaryCount }) : t('admin.accounts.moreFilters')" @click="moreFiltersOpen = !moreFiltersOpen">
        {{ t('admin.accounts.moreFilters') }}
        <span v-if="activeSecondaryCount" aria-hidden="true" class="rounded-full bg-primary-100 px-2 text-xs font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">{{ activeSecondaryCount }}</span>
      </button>
    </div>
    <div v-show="moreFiltersOpen" :id="moreFiltersId" class="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3">
      <Select :model-value="filters.type" :aria-label="t('admin.accounts.allTypes')" class="min-w-0 w-full" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
      <Select data-test="oauth-quota-filter" :model-value="filters.oauth_quota" :aria-label="t('admin.accounts.allOAuthQuota')" class="min-w-0 w-full" :options="qOpts" @update:model-value="updateOAuthQuota" @change="$emit('change')" />
      <Select data-test="excel-bps-filter" :model-value="filters.excel_bps" :aria-label="t('admin.accounts.allExcelBPS')" class="min-w-0 w-full" :options="excelBpsOpts" @update:model-value="updateExcelBps" @change="$emit('change')" />
      <Select :model-value="filters.privacy_mode" :aria-label="t('admin.accounts.allPrivacyModes')" class="min-w-0 w-full" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
      <Select :model-value="filters.group" :aria-label="t('admin.accounts.allGroups')" class="min-w-0 w-full" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue'; import { useI18n } from 'vue-i18n'; import Select from '@/common/widgets/forms/Select.vue'; import SearchInput from '@/common/widgets/forms/SearchInput.vue'
import type { AdminGroup } from '@/types'
import { ACCOUNT_OAUTH_QUOTA_FILTER } from '@/features/admin-accounts/data/dtos/accountQuotaFilters'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t } = useI18n()
const moreFiltersOpen = ref(false)
const moreFiltersId = useId()
const activeSecondaryCount = computed(() => ['type', 'oauth_quota', 'excel_bps', 'privacy_mode', 'group'].filter(key => {
  const value = props.filters[key]
  return value !== '' && value !== null && value !== undefined
}).length)
const openAIQuotaFilters = new Set<string>([
  ACCOUNT_OAUTH_QUOTA_FILTER.withReset,
  ACCOUNT_OAUTH_QUOTA_FILTER.fiveHourExhausted,
  ACCOUNT_OAUTH_QUOTA_FILTER.sevenDayExhausted
])
const updatePlatform = (value: string | number | boolean | null) => {
  const clearQuota = value !== 'openai' && openAIQuotaFilters.has(props.filters.oauth_quota)
  emit('update:filters', {
    ...props.filters,
    platform: value,
    excel_bps: value !== 'openai' ? '' : props.filters.excel_bps,
    oauth_quota: clearQuota ? '' : props.filters.oauth_quota
  })
}
const updateType = (value: string | number | boolean | null) => {
  const clearQuota = value !== 'oauth' && Boolean(props.filters.oauth_quota)
  emit('update:filters', {
    ...props.filters,
    type: value,
    excel_bps: value !== 'oauth' ? '' : props.filters.excel_bps,
    oauth_quota: clearQuota ? '' : props.filters.oauth_quota
  })
}
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updateExcelBps = (value: string | number | boolean | null) => {
  const nextValue = value === 'enabled' || value === 'disabled' ? value : ''
  emit('update:filters', {
    ...props.filters,
    excel_bps: nextValue,
    platform: nextValue ? 'openai' : props.filters.platform,
    type: nextValue ? 'oauth' : props.filters.type
  })
}
const updateOAuthQuota = (value: string | number | boolean | null) => {
  const hasQuotaFilter = typeof value === 'string' && value !== ''
  const openAIFilter = typeof value === 'string' && openAIQuotaFilters.has(value)
  emit('update:filters', {
    ...props.filters,
    platform: openAIFilter ? 'openai' : props.filters.platform,
    type: hasQuotaFilter ? 'oauth' : props.filters.type,
    oauth_quota: value
  })
}
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, { value: 'anthropic', label: 'Anthropic' }, { value: 'openai', label: 'OpenAI' }, { value: 'gemini', label: 'Gemini' }, { value: 'antigravity', label: 'Antigravity' }, { value: 'grok', label: 'Grok' }, { value: 'kimi', label: 'Kimi' }, { value: 'zhipu', label: 'Zhipu GLM' }, { value: 'deepseek', label: 'DeepSeek' }, { value: 'minimax', label: 'MiniMax' }, { value: 'opencode_go', label: 'OpenCode' }])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') }, { value: 'oauth', label: t('admin.accounts.oauthType') }, { value: 'setup-token', label: t('admin.accounts.setupToken') }, { value: 'apikey', label: t('admin.accounts.apiKey') }, { value: 'bedrock', label: 'AWS Bedrock' }])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') }, { value: 'active', label: t('admin.accounts.status.active') }, { value: 'inactive', label: t('admin.accounts.status.inactive') }, { value: 'error', label: t('admin.accounts.status.error') }, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') }, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') }, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }])
const excelBpsOpts = computed(() => [
  { value: '', label: t('admin.accounts.allExcelBPS') },
  { value: 'enabled', label: t('admin.accounts.excelBPSEnabled') },
  { value: 'disabled', label: t('admin.accounts.excelBPSDisabled') }
])
const qOpts = computed(() => [
  { value: '', label: t('admin.accounts.allOAuthQuota') },
  { value: ACCOUNT_OAUTH_QUOTA_FILTER.hasQuota, label: t('admin.accounts.oauthQuotaHasQuota') },
  { value: ACCOUNT_OAUTH_QUOTA_FILTER.exhausted, label: t('admin.accounts.oauthQuotaExhausted') },
  { value: ACCOUNT_OAUTH_QUOTA_FILTER.withReset, label: t('admin.accounts.openAIQuotaWithReset') },
  { value: ACCOUNT_OAUTH_QUOTA_FILTER.fiveHourExhausted, label: t('admin.accounts.openAIQuota5hExhausted') },
  { value: ACCOUNT_OAUTH_QUOTA_FILTER.sevenDayExhausted, label: t('admin.accounts.openAIQuota7dExhausted') }
])
const privacyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name }))
])
</script>
