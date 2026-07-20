<template>
  <BaseDialog
    :show="show"
    :title="t('usage.detail.title')"
    width="wide"
    :close-on-click-outside="true"
    @close="emit('close')"
  >
    <div v-if="usage && calculation" data-testid="usage-detail" class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="getBillingModeBadgeClass(calculation.mode)">
              {{ getBillingModeLabel(calculation.mode, t) }}
            </span>
            <span v-if="usage.long_context_billing_applied" class="inline-flex items-center rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/20 dark:text-amber-300">
              {{ t('usage.detail.longContextPricing') }}
            </span>
          </div>
          <div class="mt-2 break-all text-base font-semibold text-gray-900 dark:text-white">{{ usage.model }}</div>
          <div class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(usage.created_at) }}</div>
        </div>
        <div class="shrink-0 sm:text-right">
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ t('usage.detail.actualCharge') }}</div>
          <div class="mt-1 text-xl font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatCost(usage.actual_cost) }}</div>
          <div class="mt-1 inline-flex items-center gap-1 text-xs font-medium" :class="calculation.reconciled ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">
            <Icon :name="calculation.reconciled ? 'checkCircle' : 'exclamationTriangle'" size="sm" />
            {{ calculation.reconciled ? t('usage.detail.reconciled') : t('usage.detail.needsReview') }}
          </div>
        </div>
      </div>

      <section>
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.detail.requestInformation') }}</h4>
        <dl class="mt-3 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
          <div v-for="item in requestDetails" :key="item.key" class="min-w-0 border-b border-gray-100 pb-2 dark:border-dark-700/70">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
            <dd class="mt-1 flex min-w-0 items-start gap-1.5 text-sm font-medium text-gray-900 dark:text-gray-100">
              <span class="min-w-0 break-all" :class="item.mono ? 'font-mono text-xs' : ''">{{ item.value }}</span>
              <button
                v-if="item.copyValue"
                type="button"
                class="shrink-0 rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('usage.detail.copyRequestId')"
                :aria-label="t('usage.detail.copyRequestId')"
                @click="copyRequestId(item.copyValue)"
              >
                <Icon name="copy" size="sm" />
              </button>
            </dd>
          </div>
        </dl>
      </section>

      <section>
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.detail.usageInformation') }}</h4>
        <dl class="mt-3 grid grid-cols-2 gap-x-8 gap-y-3 lg:grid-cols-4">
          <div v-for="item in usageDetails" :key="item.key" class="border-b border-gray-100 pb-2 dark:border-dark-700/70">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
            <dd class="mt-1 break-all text-sm font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ item.value }}</dd>
          </div>
        </dl>
      </section>

      <section>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h4 class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            <Icon name="calculator" size="sm" class="text-primary-500" />
            {{ t('usage.detail.billingCalculation') }}
          </h4>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.detail.currencyPrecision') }}</span>
        </div>

        <div class="mt-3 overflow-hidden border border-gray-200 dark:border-dark-700">
          <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500 dark:bg-dark-900 dark:text-gray-400 sm:grid-cols-[minmax(9rem,0.65fr)_minmax(12rem,1.35fr)_auto]">
            <span>{{ t('usage.detail.costItem') }}</span>
            <span class="hidden sm:block">{{ t('usage.detail.calculation') }}</span>
            <span class="text-right">{{ t('usage.detail.standardCost') }}</span>
          </div>
          <div
            v-for="line in calculation.lines"
            :key="line.key"
            class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 border-t border-gray-100 px-3 py-2.5 text-sm dark:border-dark-700/70 sm:grid-cols-[minmax(9rem,0.65fr)_minmax(12rem,1.35fr)_auto]"
          >
            <span class="font-medium text-gray-800 dark:text-gray-200">{{ costLineLabel(line.key) }}</span>
            <span class="col-span-2 break-words text-xs tabular-nums text-gray-500 dark:text-gray-400 sm:col-span-1 sm:text-sm">{{ costLineFormula(line) }}</span>
            <span class="text-right font-medium tabular-nums text-gray-900 dark:text-white">{{ formatCost(line.cost) }}</span>
          </div>
          <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 border-t border-gray-200 bg-gray-50 px-3 py-2.5 text-sm dark:border-dark-700 dark:bg-dark-900">
            <span class="font-semibold text-gray-800 dark:text-gray-200">{{ t('usage.detail.componentSubtotal') }}</span>
            <span class="text-right font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCost(calculation.componentSubtotal) }}</span>
            <template v-if="!sameAmount(calculation.componentSubtotal, calculation.recordedTotal)">
              <span class="text-xs text-amber-600 dark:text-amber-400">{{ t('usage.detail.recordedStandardCost') }}</span>
              <span class="text-right text-xs font-medium tabular-nums text-amber-600 dark:text-amber-400">{{ formatCost(calculation.recordedTotal) }}</span>
            </template>
          </div>
        </div>
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('usage.detail.effectiveUnitPriceNote') }}</p>
        <p v-if="hasCacheTierBreakdown" class="mt-1 text-xs leading-5 text-amber-600 dark:text-amber-400">{{ t('usage.detail.cacheTierAggregateNote') }}</p>

        <div class="mt-4 border-l-2 border-primary-400 pl-4">
          <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ t('usage.detail.finalChargeFormula') }}</div>
          <template v-if="calculation.formulaKind === 'split'">
            <div class="mt-2 grid gap-2 text-sm sm:grid-cols-[minmax(0,1fr)_auto]">
              <span class="text-gray-600 dark:text-gray-300">{{ t('usage.detail.textSubtotal') }}</span>
              <span class="whitespace-nowrap text-right font-mono text-[10px] tabular-nums text-gray-900 dark:text-white sm:text-sm">
                {{ formatCost(calculation.nonImageSubtotal) }} x {{ formatRate(calculation.textRateMultiplier) }} = {{ formatCost(calculation.textActualCost) }}
              </span>
              <span class="text-gray-600 dark:text-gray-300">{{ t('usage.detail.imageSubtotal') }}</span>
              <span class="whitespace-nowrap text-right font-mono text-[10px] tabular-nums text-gray-900 dark:text-white sm:text-sm">
                {{ formatCost(calculation.imageSubtotal) }} x {{ formatRate(calculation.imageRateMultiplier) }} = {{ formatCost(calculation.imageActualCost) }}
              </span>
            </div>
            <p class="mt-2 text-xs leading-5 text-amber-600 dark:text-amber-400">{{ t('usage.detail.splitRateHistoricalNote') }}</p>
          </template>
          <template v-else-if="calculation.formulaKind === 'effective'">
            <div class="mt-2 whitespace-nowrap font-mono text-[10px] tabular-nums text-gray-900 dark:text-white sm:text-sm">
              {{ formatCost(calculation.recordedTotal) }} x {{ formatRate(calculation.effectiveRateMultiplier) }} = {{ formatCost(calculation.recordedActual) }}
            </div>
            <p class="mt-2 text-xs leading-5 text-amber-600 dark:text-amber-400">{{ t('usage.detail.effectiveRateHistoricalNote') }}</p>
          </template>
          <template v-else-if="calculation.formulaKind === 'zero'">
            <div class="mt-2 font-mono text-sm tabular-nums text-gray-900 dark:text-white">{{ formatCost(0) }}</div>
          </template>
          <template v-else>
            <div class="mt-2 whitespace-nowrap font-mono text-[10px] tabular-nums text-gray-900 dark:text-white sm:text-sm">
              {{ formatCost(calculation.recordedTotal) }} x {{ formatRate(calculation.rateMultiplier) }} = {{ formatCost(calculation.calculatedActual) }}
            </div>
          </template>

          <div class="mt-3 flex items-center justify-between gap-4 border-t border-gray-200 pt-3 dark:border-dark-700">
            <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.detail.actualCharge') }}</span>
            <span class="text-lg font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ formatCost(calculation.recordedActual) }}</span>
          </div>
        </div>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime, formatReasoningEffort } from '@/utils/format'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import { calculateOutputTokensPerSecond } from '@/utils/usageMetrics'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  getBillingModeBadgeClass,
  getBillingModeLabel,
} from '@/utils/billingMode'
import {
  buildUsageBillingCalculation,
  type UsageBillingCostLine,
} from '@/utils/usageBillingCalculation'
import type { AdminUsageLog } from '@/types'

const props = defineProps<{
  show: boolean
  usage: AdminUsageLog | null
}>()

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const calculation = computed(() => props.usage ? buildUsageBillingCalculation(props.usage) : null)

const displayValue = (value: unknown): string => {
  if (value == null || value === '') return '-'
  return String(value)
}

const requestTypeLabel = computed(() => {
  if (!props.usage) return '-'
  const type = resolveUsageRequestType(props.usage)
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
})

const requestDetails = computed(() => {
  const usage = props.usage
  if (!usage) return []
  const adminDetails = [
    usage.upstream_model ? { key: 'upstream_model', label: t('usage.upstreamModel'), value: usage.upstream_model } : null,
    usage.model_mapping_chain ? { key: 'model_mapping_chain', label: t('usage.detail.modelMappingChain'), value: usage.model_mapping_chain } : null,
    usage.account ? { key: 'account', label: t('admin.usage.account'), value: `${usage.account.name} (#${usage.account.id})` } : null,
    usage.channel_id != null ? { key: 'channel_id', label: t('usage.detail.channelId'), value: `#${usage.channel_id}` } : null,
    usage.billing_tier ? { key: 'billing_tier', label: t('usage.detail.billingTier'), value: usage.billing_tier } : null,
  ].filter((item): item is NonNullable<typeof item> => item != null)

  return [
    { key: 'request_id', label: t('usage.detail.requestId'), value: usage.request_id, copyValue: usage.request_id, mono: true },
    { key: 'api_key', label: t('usage.detail.apiKey'), value: usage.api_key?.name || `#${usage.api_key_id}` },
    { key: 'model', label: t('usage.model'), value: usage.model },
    { key: 'request_type', label: t('usage.type'), value: requestTypeLabel.value },
    { key: 'endpoint', label: t('usage.inboundEndpoint'), value: displayValue(usage.inbound_endpoint), mono: true },
    ...(usage.upstream_endpoint ? [{ key: 'upstream_endpoint', label: t('usage.upstreamEndpoint'), value: usage.upstream_endpoint, mono: true }] : []),
    { key: 'group', label: t('admin.usage.group'), value: usage.group?.name || (usage.group_id ? `#${usage.group_id}` : '-') },
    { key: 'billing_type', label: t('usage.detail.billingType'), value: usage.billing_type === 1 ? t('usage.detail.subscriptionBilling') : t('usage.detail.balanceBilling') },
    { key: 'service_tier', label: t('usage.serviceTier'), value: displayValue(usage.service_tier) },
    { key: 'reasoning_effort', label: t('usage.reasoningEffort'), value: formatReasoningEffort(usage.reasoning_effort) },
    { key: 'ip_address', label: 'IP', value: displayValue(usage.ip_address), mono: true },
    { key: 'user_agent', label: t('usage.userAgent'), value: displayValue(usage.user_agent) },
    ...adminDetails,
  ]
})

const usageDetails = computed(() => {
  const usage = props.usage
  const calc = calculation.value
  if (!usage || !calc) return []

  if (calc.mode === BILLING_MODE_IMAGE) {
    return [
      { key: 'image_count', label: t('usage.imageCount'), value: usage.image_count.toLocaleString() },
      { key: 'image_size', label: t('usage.imageBillingSize'), value: displayValue(usage.image_size) },
      { key: 'duration', label: t('usage.duration'), value: formatDuration(usage.duration_ms) },
      { key: 'rate', label: t('usage.detail.rateSnapshot'), value: formatRate(usage.rate_multiplier) },
    ]
  }
  if (calc.mode === BILLING_MODE_VIDEO) {
    return [
      { key: 'video_count', label: t('usage.detail.videoCount'), value: (usage.video_count ?? 0).toLocaleString() },
      { key: 'video_resolution', label: t('usage.detail.videoResolution'), value: displayValue(usage.video_resolution) },
      { key: 'video_duration', label: t('usage.detail.videoDuration'), value: `${usage.video_duration_seconds ?? 0}s` },
      { key: 'duration', label: t('usage.duration'), value: formatDuration(usage.duration_ms) },
    ]
  }

  const speed = calculateOutputTokensPerSecond(usage)
  return [
    { key: 'input', label: t('admin.usage.inputTokens'), value: usage.input_tokens.toLocaleString() },
    { key: 'output', label: t('admin.usage.outputTokens'), value: usage.output_tokens.toLocaleString() },
    { key: 'cache_creation', label: t('admin.usage.cacheCreationTokens'), value: usage.cache_creation_tokens.toLocaleString() },
    ...(usage.cache_creation_5m_tokens > 0 ? [{ key: 'cache_creation_5m', label: t('admin.usage.cacheCreation5mTokens'), value: usage.cache_creation_5m_tokens.toLocaleString() }] : []),
    ...(usage.cache_creation_1h_tokens > 0 ? [{ key: 'cache_creation_1h', label: t('admin.usage.cacheCreation1hTokens'), value: usage.cache_creation_1h_tokens.toLocaleString() }] : []),
    { key: 'cache_read', label: t('admin.usage.cacheReadTokens'), value: usage.cache_read_tokens.toLocaleString() },
    { key: 'first_token', label: t('usage.firstToken'), value: formatDuration(usage.first_token_ms) },
    { key: 'duration', label: t('usage.duration'), value: formatDuration(usage.duration_ms) },
    { key: 'speed', label: t('usage.outputSpeed'), value: speed == null ? '-' : `${speed.toFixed(2)} ${t('usage.tokensPerSecondUnit')}` },
    { key: 'rate', label: t('usage.detail.rateSnapshot'), value: formatRate(usage.rate_multiplier) },
  ]
})

const hasCacheTierBreakdown = computed(() =>
  (props.usage?.cache_creation_5m_tokens ?? 0) > 0 || (props.usage?.cache_creation_1h_tokens ?? 0) > 0
)

const formatCost = (value: number | null | undefined): string => `$${(value ?? 0).toFixed(10)}`
const formatRate = (value: number | null | undefined): string => value == null || !Number.isFinite(value) ? '-' : `${value.toFixed(4)}x`
const sameAmount = (left: number, right: number): boolean => Math.abs(left - right) <= Math.max(1e-10, Math.max(Math.abs(left), Math.abs(right), 1) * 1e-8)

const formatDuration = (milliseconds: number | null | undefined): string => {
  if (milliseconds == null) return '-'
  if (milliseconds < 1000) return `${milliseconds}ms`
  return `${(milliseconds / 1000).toFixed(2)}s`
}

const costLineLabel = (key: UsageBillingCostLine['key']): string => t(`usage.detail.costLines.${key}`)

const quantityUnitLabel = (line: UsageBillingCostLine): string => {
  if (line.quantityUnit === 'tokens') return t('usage.detail.tokensUnit')
  if (line.quantityUnit === 'images') return t('usage.detail.imagesUnit')
  if (line.quantityUnit === 'video_seconds') return t('usage.detail.videoSecondsUnit')
  return t('usage.detail.requestsUnit')
}

const costLineFormula = (line: UsageBillingCostLine): string => {
  if (line.unitPrice == null) return t('usage.detail.noUnitPrice')
  const quantity = line.quantity.toLocaleString()
  if (line.quantityUnit === 'tokens') {
    return `${quantity} ${quantityUnitLabel(line)} x ${formatCost(line.unitPrice * 1_000_000)} / 1M`
  }
  return `${quantity} ${quantityUnitLabel(line)} x ${formatCost(line.unitPrice)}`
}

const copyRequestId = (requestId: string) => {
  void copyToClipboard(requestId, t('usage.detail.requestIdCopied'))
}
</script>
