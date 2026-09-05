<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
    <Select data-test="oauth-quota-filter" :model-value="filters.oauth_quota" class="w-56" :options="qOpts" @update:model-value="updateOAuthQuota" @change="$emit('change')" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'; import { useI18n } from 'vue-i18n'; import Select from '@/common/widgets/forms/Select.vue'; import SearchInput from '@/common/widgets/forms/SearchInput.vue'
import type { AdminGroup } from '@/types'
import { ACCOUNT_OAUTH_QUOTA_FILTER } from '@/features/admin-accounts/data/dtos/accountQuotaFilters'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change']); const { t } = useI18n()
const openAIQuotaFilters = new Set<string>([
  ACCOUNT_OAUTH_QUOTA_FILTER.withReset,
  ACCOUNT_OAUTH_QUOTA_FILTER.fiveHourExhausted,
  ACCOUNT_OAUTH_QUOTA_FILTER.sevenDayExhausted
])
const updatePlatform = (value: string | number | boolean | null) => {
  const clearQuota = value !== 'openai' && openAIQuotaFilters.has(props.filters.oauth_quota)
  emit('update:filters', { ...props.filters, platform: value, oauth_quota: clearQuota ? '' : props.filters.oauth_quota })
}
const updateType = (value: string | number | boolean | null) => {
  const clearQuota = value !== 'oauth' && Boolean(props.filters.oauth_quota)
  emit('update:filters', { ...props.filters, type: value, oauth_quota: clearQuota ? '' : props.filters.oauth_quota })
}
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
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
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, { value: 'anthropic', label: 'Anthropic' }, { value: 'openai', label: 'OpenAI' }, { value: 'gemini', label: 'Gemini' }, { value: 'antigravity', label: 'Antigravity' }, { value: 'grok', label: 'Grok' }])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') }, { value: 'oauth', label: t('admin.accounts.oauthType') }, { value: 'setup-token', label: t('admin.accounts.setupToken') }, { value: 'apikey', label: t('admin.accounts.apiKey') }, { value: 'bedrock', label: 'AWS Bedrock' }])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') }, { value: 'active', label: t('admin.accounts.status.active') }, { value: 'inactive', label: t('admin.accounts.status.inactive') }, { value: 'error', label: t('admin.accounts.status.error') }, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') }, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') }, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }])
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
