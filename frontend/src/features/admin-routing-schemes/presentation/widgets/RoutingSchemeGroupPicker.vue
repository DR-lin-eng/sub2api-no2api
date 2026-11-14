<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type {
  RoutingSchemeGroupOption,
  RoutingSchemeItem,
  RoutingSchemeRule,
} from '../../data/dtos/routingSchemeDtos'
import {
  scoreBarClass,
  scorePercents,
  scoreToneClass,
} from '../../data/routingScorePreview'
import RoutingSchemeRulesDialog from './RoutingSchemeRulesDialog.vue'

const props = defineProps<{
  groups: RoutingSchemeGroupOption[]
  modelValue: RoutingSchemeItem[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: RoutingSchemeItem[]]
}>()

const { t } = useI18n()
const search = ref('')
const rulesDialogGroupID = ref<number | null>(null)

const selectedIDs = computed(() => new Set(props.modelValue.map(item => item.group_id)))
const groupMap = computed(() => new Map(props.groups.map(group => [group.id, group])))

const isSelectable = (group: RoutingSchemeGroupOption): boolean => {
  if ((group.status ?? 'active') !== 'active') return false
  if ((group.subscription_type ?? 'standard') !== 'standard') return false
  return true
}

const filteredGroups = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return props.groups
  return props.groups.filter(group =>
    group.name.toLowerCase().includes(query)
    || (group.description ?? '').toLowerCase().includes(query),
  )
})

const selectedItems = computed(() => props.modelValue
  .map((item) => {
    const group = groupMap.value.get(item.group_id)
    return group ? { ...item, group } : null
  })
  .filter((item): item is RoutingSchemeItem & { group: RoutingSchemeGroupOption } => item !== null))

const scoreOf = (groupID: number) => {
  const item = props.modelValue.find(entry => entry.group_id === groupID)
  const rawScore = item?.score
  const known = rawScore != null
  const sampleCount = item?.sample_count ?? 0
  // Without samples the backend has no success/failure signal; never render a
  // synthetic 100% / 0% / 0s.
  const metricsKnown = known && sampleCount > 0
  const p95 = item?.p95_ttft_ms ?? 0
  const failureRate = metricsKnown ? (item?.failure_rate ?? 0) : 0
  return {
    overall: rawScore == null ? 0 : Math.round(rawScore),
    known,
    ttft_seconds: metricsKnown && p95 > 0 ? Math.round(p95 / 100) / 10 : null,
    success_rate: metricsKnown ? Math.round((1 - failureRate) * 1000) / 10 : null,
    error_rate: metricsKnown ? Math.round(failureRate * 1000) / 10 : null,
    error_count: item?.failure_count ?? 0,
    sample_count: sampleCount,
    window_seconds: 900,
    cold_start: item?.cold_start ?? false,
  }
}

const scoredItems = computed(() => {
  // Only known scores participate in the normalized share; largest-remainder
  // rounding keeps the displayed percentages at 100% in total.
  const known = selectedItems.value.filter(item => scoreOf(item.group_id).known)
  const percents = scorePercents(known.map(item => scoreOf(item.group_id).overall))
  const percentByID = new Map<number, number>()
  known.forEach((item, index) => percentByID.set(item.group_id, percents[index] ?? 0))
  return selectedItems.value.map((item) => {
    const score = scoreOf(item.group_id)
    return {
      ...item,
      score,
      percent: percentByID.get(item.group_id) ?? 0,
    }
  })
})

// Display order only; the backend distributes traffic by the composite score
// through session-stable weighted rendezvous.
const sortedItems = computed(() => {
  const list = [...scoredItems.value]
  list.sort((a, b) => b.score.overall - a.score.overall)
  return list
})

const toggleGroup = (group: RoutingSchemeGroupOption) => {
  if (!isSelectable(group)) return
  if (selectedIDs.value.has(group.id)) {
    emit('update:modelValue', props.modelValue.filter(item => item.group_id !== group.id))
    return
  }
  emit('update:modelValue', [...props.modelValue, { group_id: group.id, rules: [] }])
}

const removeGroup = (groupID: number) => {
  if (rulesDialogGroupID.value === groupID) rulesDialogGroupID.value = null
  emit('update:modelValue', props.modelValue.filter(item => item.group_id !== groupID))
}

const openRulesDialog = (groupID: number) => {
  rulesDialogGroupID.value = groupID
}

const closeRulesDialog = () => {
  rulesDialogGroupID.value = null
}

const rulesDialogItem = computed(() =>
  selectedItems.value.find(item => item.group_id === rulesDialogGroupID.value) ?? null,
)

const updateRules = (groupID: number, rules: RoutingSchemeRule[]) => {
  emit('update:modelValue', props.modelValue.map(item =>
    item.group_id === groupID ? { ...item, rules } : item,
  ))
}
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)]">
    <section class="min-w-0 rounded-xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-800/60">
      <div class="mb-3 flex items-center justify-between gap-3">
        <div>
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.routingSchemes.picker.availableTitle') }}
          </h4>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.picker.availableHint') }}
          </p>
        </div>
        <span class="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-300">
          {{ groups.length }}
        </span>
      </div>

      <div class="relative mb-3">
        <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
        <input
          v-model="search"
          type="search"
          class="input h-9 pl-9"
          :placeholder="t('admin.routingSchemes.picker.searchPlaceholder')"
        />
      </div>

      <div class="max-h-80 space-y-1.5 overflow-y-auto pr-1">
        <button
          v-for="group in filteredGroups"
          :key="group.id"
          type="button"
          class="flex w-full items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors"
          :class="[
            selectedIDs.has(group.id)
              ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700 dark:bg-primary-500/10'
              : 'border-transparent bg-white hover:border-gray-200 dark:bg-dark-900 dark:hover:border-dark-600',
            !isSelectable(group) && 'cursor-not-allowed opacity-55',
          ]"
          :disabled="!isSelectable(group)"
          @click="toggleGroup(group)"
        >
          <span
            class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded border"
            :class="selectedIDs.has(group.id)
              ? 'border-primary-500 bg-primary-500 text-white'
              : 'border-gray-300 bg-white dark:border-dark-600 dark:bg-dark-800'"
          >
            <Icon v-if="selectedIDs.has(group.id)" name="check" size="xs" :stroke-width="3" />
          </span>
          <span class="min-w-0 flex-1">
            <GroupBadge
              :name="group.name"
              :platform="group.platform"
              :subscription-type="group.subscription_type || 'standard'"
              :rate-multiplier="group.rate_multiplier"
              truncate
            />
            <span
              v-if="group.description"
              class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400"
            >
              {{ group.description }}
            </span>
          </span>
          <span
            v-if="!isSelectable(group)"
            class="flex-shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400"
          >
            {{ (group.status ?? 'active') !== 'active'
              ? t('admin.routingSchemes.picker.inactiveGroup')
              : t('admin.routingSchemes.picker.subscriptionGroup') }}
          </span>
        </button>

        <p
          v-if="filteredGroups.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-3 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('admin.routingSchemes.picker.noGroups') }}
        </p>
      </div>
    </section>

    <section class="min-w-0 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
      <div class="mb-2 flex items-center justify-between gap-3">
        <div>
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.routingSchemes.picker.selectedTitle') }}
          </h4>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.picker.selectedHint') }}
          </p>
        </div>
        <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          {{ selectedItems.length }}
        </span>
      </div>

      <div
        v-if="selectedItems.length > 0"
        class="mb-3 flex h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"
      >
        <span
          v-for="(item, index) in sortedItems"
          :key="item.group_id"
          class="h-full transition-all"
          :class="index === 0
            ? 'bg-primary-500'
            : index === 1
              ? 'bg-sky-400'
              : index === 2
                ? 'bg-emerald-400'
                : 'bg-amber-400'"
          :style="{ width: `${item.percent}%` }"
        />
      </div>

      <div class="max-h-[28rem] space-y-2 overflow-y-auto pr-1">
        <div
          v-for="(item, index) in sortedItems"
          :key="item.group_id"
          class="rounded-lg border border-gray-200 bg-gray-50/70 p-3 transition-colors dark:border-dark-700 dark:bg-dark-800/60"
        >
          <div class="flex items-start gap-2">
            <span
              class="mt-1 shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold"
              :class="index === 0
                ? 'bg-primary-50 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300'
                : 'bg-white text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300'"
            >
              {{ index + 1 }}
            </span>
            <div class="flex min-w-0 flex-1 items-center gap-2">
              <span class="min-w-0 flex-1">
                <GroupBadge
                  :name="item.group.name"
                  :platform="item.group.platform"
                  :subscription-type="item.group.subscription_type || 'standard'"
                  :rate-multiplier="item.group.rate_multiplier"
                  truncate
                />
              </span>
              <span
                v-if="(item.rules?.length ?? 0) > 0"
                class="shrink-0 rounded-full bg-primary-50 px-1.5 py-0.5 text-[10px] font-medium text-primary-700 dark:bg-primary-500/15 dark:text-primary-300"
              >
                {{ t('admin.routingSchemes.policy.ruleCount', { count: item.rules?.length ?? 0 }) }}
              </span>
            </div>
            <button
              type="button"
              class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950/30"
              :title="t('admin.routingSchemes.picker.remove')"
              @click="removeGroup(item.group_id)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <div class="mt-2.5">
            <div class="flex items-center gap-2">
              <span class="whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                {{ t('admin.routingSchemes.picker.overallScore') }}
              </span>
              <span class="w-8 text-base font-semibold tabular-nums" :class="item.score.known ? scoreToneClass(item.score.overall) : 'text-gray-400 dark:text-gray-500'">
                {{ item.score.known ? item.score.overall : '—' }}
              </span>
              <div class="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                <div
                  class="h-full rounded-full transition-all"
                  :class="item.score.known ? scoreBarClass(item.score.overall) : 'bg-gray-300 dark:bg-dark-600'"
                  :style="{ width: `${item.score.known ? item.score.overall : 0}%` }"
                />
              </div>
              <span class="rounded-full bg-primary-50 px-2 py-0.5 text-xs font-semibold text-primary-700 dark:bg-primary-500/15 dark:text-primary-300">
                {{ item.score.known ? `${item.percent}%` : '—' }}
              </span>
            </div>

            <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
              <span class="inline-flex items-center rounded-md bg-white px-1.5 py-0.5 text-[10px] text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300">
                {{ t('admin.routingSchemes.picker.ttft') }}
                <span class="ml-1 font-semibold text-gray-700 dark:text-gray-200">{{ item.score.ttft_seconds == null ? '—' : `${item.score.ttft_seconds}s` }}</span>
              </span>
              <span class="inline-flex items-center rounded-md bg-white px-1.5 py-0.5 text-[10px] text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300">
                {{ t('admin.routingSchemes.picker.successRate') }}
                <span class="ml-1 font-semibold text-gray-700 dark:text-gray-200">{{ item.score.success_rate == null ? '—' : `${item.score.success_rate}%` }}</span>
              </span>
              <span class="inline-flex items-center rounded-md bg-white px-1.5 py-0.5 text-[10px] text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300">
                {{ t('admin.routingSchemes.picker.failureRate') }}
                <span class="ml-1 font-semibold text-gray-700 dark:text-gray-200">{{ item.score.error_rate == null ? '—' : `${item.score.error_rate}%` }}</span>
              </span>
              <span class="inline-flex items-center rounded-md bg-white px-1.5 py-0.5 text-[10px] text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-300">
                {{ t('admin.routingSchemes.picker.sampleCount', { count: item.score.sample_count }) }}
              </span>
              <button
                type="button"
                class="ml-auto inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium text-gray-500 transition-colors hover:bg-white hover:text-primary-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                @click="openRulesDialog(item.group_id)"
              >
                <Icon name="filter" size="xs" />
                {{ t('admin.routingSchemes.policy.configure') }}
              </button>
            </div>
          </div>
        </div>

        <p
          v-if="sortedItems.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-3 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('admin.routingSchemes.picker.emptySelected') }}
        </p>
      </div>

      <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">
        {{ t('admin.routingSchemes.picker.scoreHint') }}
      </p>
    </section>

    <RoutingSchemeRulesDialog
      :show="!!rulesDialogItem"
      :group-name="rulesDialogItem?.group.name ?? ''"
      :rules="rulesDialogItem?.rules ?? []"
      @close="closeRulesDialog"
      @save="rulesDialogItem && updateRules(rulesDialogItem.group_id, $event)"
    />
  </div>
</template>
