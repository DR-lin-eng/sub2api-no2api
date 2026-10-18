<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import { getOAuthPlanExpiryDistribution } from '@/features/admin-ops/data/datasources/opsMetricsQueries'
import type { OpsOAuthPlanExpiryDistributionResponse } from '@/features/admin-ops/data/dtos/opsMetricsDtos'
import { formatDateTime } from '@/core/utils/format'
import { formatCompactNumber } from '../opsFormatter'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOAuthPlanExpiryDistributionResponse | null>(null)

const buckets = computed(() => response.value?.buckets ?? [])
const plans = computed(() => response.value?.plans ?? [])
const maxBucketCount = computed(() => Math.max(1, ...buckets.value.map((bucket) => bucket.count)))

function bucketLabel(key: string): string {
  const translated = t(`admin.ops.oauthPlanExpiry.buckets.${key}`)
  return translated === `admin.ops.oauthPlanExpiry.buckets.${key}` ? key : translated
}

function bucketWidth(count: number): string {
  return `${Math.max(0, Math.min(100, (count / maxBucketCount.value) * 100))}%`
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await getOAuthPlanExpiryDistribution(
      props.platformFilter || undefined,
      typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined
    )
  } catch (err: any) {
    console.error('[OpsOAuthPlanExpiryDistributionCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.oauthPlanExpiry.failedToLoad')
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.platformFilter, props.groupIdFilter, props.refreshToken],
  () => {
    void loadData()
  },
  { immediate: true }
)
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.oauthPlanExpiry.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.oauthPlanExpiry.description') }}
        </p>
      </div>
      <span v-if="response?.generated_at" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.oauthPlanExpiry.updatedAt', { value: formatDateTime(response.generated_at) }) }}
      </span>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <EmptyState
      v-else-if="response && response.total_oauth_accounts === 0"
      :title="t('common.noData')"
      :description="t('admin.ops.oauthPlanExpiry.empty')"
    />

    <div v-else-if="response" class="space-y-4">
      <div class="grid grid-cols-2 gap-2 md:grid-cols-4">
        <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.oauthPlanExpiry.totalAccounts') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCompactNumber(response.total_oauth_accounts) }}</div>
        </div>
        <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.oauthPlanExpiry.withExpiry') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCompactNumber(response.accounts_with_expiry) }}</div>
        </div>
        <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.oauthPlanExpiry.withoutExpiry') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-amber-600 dark:text-amber-400">{{ formatCompactNumber(response.accounts_without_expiry) }}</div>
        </div>
        <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.oauthPlanExpiry.planCount') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCompactNumber(plans.length) }}</div>
        </div>
      </div>

      <div class="space-y-2" role="list" :aria-label="t('admin.ops.oauthPlanExpiry.title')">
        <div v-for="bucket in buckets" :key="bucket.key" class="flex items-center gap-3 text-xs">
          <span class="w-16 shrink-0 text-right tabular-nums text-gray-500 dark:text-gray-400">{{ bucketLabel(bucket.key) }}</span>
          <div class="h-5 flex-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-800">
            <div
              class="h-full rounded bg-primary-500 transition-all dark:bg-primary-400"
              :style="{ width: bucketWidth(bucket.count) }"
              role="presentation"
            />
          </div>
          <span class="w-12 shrink-0 text-right font-semibold tabular-nums text-gray-700 dark:text-gray-200">{{ formatCompactNumber(bucket.count) }}</span>
        </div>
      </div>

      <div v-if="plans.length > 0" class="flex flex-wrap items-center gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.oauthPlanExpiry.planBreakdown') }}</span>
        <span
          v-for="plan in plans"
          :key="plan.plan_type"
          class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
        >
          {{ plan.plan_type }} · {{ formatCompactNumber(plan.count) }}
        </span>
      </div>
    </div>
  </section>
</template>
