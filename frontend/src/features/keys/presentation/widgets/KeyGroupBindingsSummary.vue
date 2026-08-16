<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ApiKey, ApiKeyGroupBinding } from '@/types'
import type { GroupOption } from '../keysPageContext'

const props = defineProps<{
  apiKey: ApiKey
  groupOptions: GroupOption[]
}>()

const emit = defineEmits<{
  manage: []
}>()

const { t } = useI18n()

interface ResolvedBinding {
  binding: ApiKeyGroupBinding
  option: GroupOption | null
  label: string
}

const normalizedBindings = computed<ApiKeyGroupBinding[]>(() => {
  if (props.apiKey.group_bindings?.length) {
    return props.apiKey.group_bindings
  }
  return props.apiKey.group_id
    ? [{ group_id: props.apiKey.group_id, max_rate_multiplier: null }]
    : []
})

const optionByID = computed(() => new Map(props.groupOptions.map(option => [option.value, option])))

const resolvedBindings = computed<ResolvedBinding[]>(() => normalizedBindings.value.map((binding) => {
  let option = optionByID.value.get(binding.group_id) ?? null
  const legacyGroup = props.apiKey.group
  if (!option && legacyGroup?.id === binding.group_id) {
    option = {
      value: legacyGroup.id,
      label: legacyGroup.name,
      description: legacyGroup.description,
      rate: legacyGroup.rate_multiplier,
      userRate: null,
      peakRateEnabled: legacyGroup.peak_rate_enabled,
      peakStart: legacyGroup.peak_start,
      peakEnd: legacyGroup.peak_end,
      peakRateMultiplier: legacyGroup.peak_rate_multiplier,
      subscriptionType: legacyGroup.subscription_type,
      platform: legacyGroup.platform
    }
  }
  return {
    binding,
    option,
    label: option?.label ?? `#${binding.group_id}`
  }
}))

const primary = computed(() => resolvedBindings.value[0] ?? null)
const fallbacks = computed(() => resolvedBindings.value.slice(1))
const visibleFallbacks = computed(() => fallbacks.value.slice(0, 2))
const hiddenFallbackCount = computed(() => Math.max(0, fallbacks.value.length - visibleFallbacks.value.length))

const ceilingText = (binding: ApiKeyGroupBinding) => binding.max_rate_multiplier == null
  ? ''
  : t('keys.groupBindings.rateCeilingShort', { rate: binding.max_rate_multiplier })

const routeTitle = computed(() => {
  if (!resolvedBindings.value.length) {
    return t('keys.groupBindings.manageEmpty')
  }
  return resolvedBindings.value.map((item, index) => {
    const role = index === 0
      ? t('keys.groupBindings.primary')
      : t('keys.groupBindings.fallbackPosition', { position: index })
    const ceiling = ceilingText(item.binding)
    return `${role}: ${item.label}${ceiling ? ` (${ceiling})` : ''}`
  }).join(' → ')
})
</script>

<template>
  <button
    type="button"
    class="group/route -mx-2 -my-1 w-full min-w-0 max-w-[24rem] rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 md:min-w-[15rem] dark:hover:bg-dark-700/70"
    :title="routeTitle"
    :aria-label="t('keys.groupBindings.manageFor', { name: apiKey.name })"
    :data-test="`key-groups-summary-${apiKey.id}`"
    @click="emit('manage')"
  >
    <template v-if="primary">
      <div class="flex min-w-0 items-center gap-2">
        <span class="shrink-0 rounded-md bg-primary-50 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
          {{ t('keys.groupBindings.primary') }}
        </span>
        <div class="min-w-0 max-w-[13rem]">
          <GroupBadge
            v-if="primary.option"
            class="max-w-full"
            :name="primary.option.label"
            :platform="primary.option.platform"
            :subscription-type="primary.option.subscriptionType"
            :rate-multiplier="primary.option.rate"
            :user-rate-multiplier="primary.option.userRate"
            :peak-rate-enabled="primary.option.peakRateEnabled"
            :peak-start="primary.option.peakStart"
            :peak-end="primary.option.peakEnd"
            :peak-rate-multiplier="primary.option.peakRateMultiplier"
          />
          <span v-else class="text-sm font-medium text-gray-800 dark:text-gray-200">
            {{ primary.label }}
          </span>
        </div>
        <span
          v-if="primary.binding.max_rate_multiplier != null"
          class="shrink-0 rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/25 dark:text-amber-300"
        >
          {{ ceilingText(primary.binding) }}
        </span>
        <span class="ml-auto inline-flex shrink-0 items-center gap-1 text-[11px] font-medium text-gray-400 transition-colors group-hover/route:text-primary-600 dark:group-hover/route:text-primary-400">
          <Icon name="edit" size="xs" :stroke-width="2" />
          <span class="hidden sm:inline">{{ t('keys.groupBindings.manage') }}</span>
        </span>
      </div>

      <div v-if="fallbacks.length" class="mt-1.5 flex min-w-0 items-start gap-2">
        <span class="mt-1 shrink-0 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
          {{ t('keys.groupBindings.fallback') }}
        </span>
        <div class="flex min-w-0 flex-wrap items-center gap-1">
          <template v-for="(item, index) in visibleFallbacks" :key="item.binding.group_id">
            <Icon v-if="index > 0" name="chevronRight" size="xs" class="shrink-0 text-gray-300 dark:text-gray-600" />
            <span class="inline-flex min-w-0 items-center gap-1 rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              <span class="font-semibold text-gray-400 dark:text-gray-500">{{ index + 2 }}</span>
              <span class="max-w-24 truncate">{{ item.label }}</span>
              <span v-if="item.binding.max_rate_multiplier != null" class="font-semibold text-amber-600 dark:text-amber-300">
                {{ ceilingText(item.binding) }}
              </span>
            </span>
          </template>
          <span
            v-if="hiddenFallbackCount"
            class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] font-semibold text-gray-500 dark:bg-dark-700 dark:text-gray-400"
          >
            {{ t('keys.groupBindings.moreGroups', { count: hiddenFallbackCount }) }}
          </span>
        </div>
      </div>
      <p v-else class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
        {{ t('keys.groupBindings.singleGroup') }}
      </p>
    </template>

    <div v-else class="flex items-center gap-2">
      <span class="rounded-md border border-dashed border-gray-300 px-2 py-1 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
        {{ t('keys.groupBindings.notConfigured') }}
      </span>
      <span class="text-[11px] text-primary-600 dark:text-primary-400">
        {{ t('keys.groupBindings.configure') }}
      </span>
      <Icon name="chevronRight" size="xs" class="ml-auto text-gray-400" />
    </div>
  </button>
</template>
