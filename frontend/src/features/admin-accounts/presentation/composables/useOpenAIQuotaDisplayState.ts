import { computed, ref, type Ref } from 'vue'
import type { OpenAIQuotaUsage } from '@/features/admin-accounts/data/dtos/openAIQuotaDtos'
import type { AccountUsageInfo } from '@/types'

/**
 * Coordinates the explicit upstream quota snapshot with the local account
 * usage windows. The two sources intentionally remain separate: ChatGPT owns
 * bucket percentages, while Sub2API owns request/token/cost aggregates.
 */
export function useOpenAIQuotaDisplayState(usageInfo: Ref<AccountUsageInfo | null>) {
  const quotaUsage = ref<OpenAIQuotaUsage | null>(null)

  const localWindowStats = computed(() => ({
    five_hour: usageInfo.value?.five_hour?.window_stats ?? null,
    seven_day: usageInfo.value?.seven_day?.window_stats ?? null
  }))

  const hasUpstreamWindowData = computed(() => {
    const quota = quotaUsage.value
    if (!quota) return false
    const buckets = quota.rate_limits_by_limit_id ?? quota.rateLimitsByLimitId
    return Boolean(
      (buckets && Object.keys(buckets).length > 0) ||
      quota.rate_limit ||
      quota.rateLimit
    )
  })

  const updateQuotaUsage = (usage: OpenAIQuotaUsage) => {
    quotaUsage.value = usage
  }

  const clearQuotaUsage = () => {
    quotaUsage.value = null
  }

  return {
    quotaUsage,
    localWindowStats,
    hasUpstreamWindowData,
    updateQuotaUsage,
    clearQuotaUsage
  }
}
