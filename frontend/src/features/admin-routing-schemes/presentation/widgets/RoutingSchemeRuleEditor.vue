<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import Select from '@/common/widgets/forms/Select.vue'
import type {
  RoutingRuleActionKind,
  RoutingRuleErrorClass,
  RoutingRuleTrigger,
  RoutingRuleTriggerKind,
  RoutingSchemeRule,
} from '../../data/dtos/routingSchemeDtos'
import {
  ROUTING_RULE_ACTION_KINDS,
  ROUTING_RULE_ERROR_CLASSES,
  ROUTING_RULE_TRIGGER_KINDS,
  createRouteRule,
  defaultAction,
  defaultTrigger,
} from '../../data/routingRuleCatalog'

const props = withDefaults(defineProps<{
  rules: RoutingSchemeRule[]
}>(), {
  rules: () => [],
})

const emit = defineEmits<{
  'update:rules': [value: RoutingSchemeRule[]]
}>()

const { t } = useI18n()

const triggerOptions = computed(() =>
  ROUTING_RULE_TRIGGER_KINDS.map(kind => ({
    value: kind,
    label: t(`admin.routingSchemes.policy.triggers.${kind}`),
  })),
)

const actionOptions = computed(() =>
  ROUTING_RULE_ACTION_KINDS.map(kind => ({
    value: kind,
    label: t(`admin.routingSchemes.policy.actions.${kind}`),
  })),
)

const errorClassOptions = computed(() =>
  ROUTING_RULE_ERROR_CLASSES.map(kind => ({
    value: kind,
    label: t(`admin.routingSchemes.policy.errorClasses.${kind}`),
  })),
)

const emitRules = (rules: RoutingSchemeRule[]) => {
  emit('update:rules', rules.map(rule => ({
    ...rule,
    trigger: { ...rule.trigger } as RoutingRuleTrigger,
    action: { ...rule.action },
  })))
}

const patchRule = (id: string, updater: (rule: RoutingSchemeRule) => RoutingSchemeRule) => {
  emitRules(props.rules.map(rule => (rule.id === id ? updater(rule) : rule)))
}

const addRule = () => {
  emitRules([...props.rules, createRouteRule()])
}

const removeRule = (id: string) => {
  emitRules(props.rules.filter(rule => rule.id !== id))
}

const toggleRule = (id: string, event: Event) => {
  const enabled = (event.target as HTMLInputElement).checked
  patchRule(id, rule => ({ ...rule, enabled }))
}

const changeTriggerKind = (rule: RoutingSchemeRule, value: string | number | boolean | null) => {
  const kind = value as RoutingRuleTriggerKind
  if (rule.trigger.kind === kind) return
  patchRule(rule.id, current => ({ ...current, trigger: defaultTrigger(kind) }))
}

const changeActionKind = (rule: RoutingSchemeRule, value: string | number | boolean | null) => {
  const kind = value as RoutingRuleActionKind
  if (rule.action.kind === kind) return
  patchRule(rule.id, current => ({ ...current, action: defaultAction(kind) }))
}

const numberFromEvent = (
  event: Event,
  min: number,
  max: number,
  fallback: number,
): number => {
  const raw = (event.target as HTMLInputElement).value.trim()
  const parsed = Number(raw)
  if (!Number.isFinite(parsed)) return fallback
  return Math.min(max, Math.max(min, Math.round(parsed)))
}

const updateTriggerNumber = (
  rule: RoutingSchemeRule,
  key: string,
  event: Event,
  min: number,
  max: number,
  fallback: number,
) => {
  const value = numberFromEvent(event, min, max, fallback)
  patchRule(rule.id, current => ({
    ...current,
    trigger: { ...current.trigger, [key]: value } as RoutingRuleTrigger,
  }))
}

const updateCooldown = (rule: RoutingSchemeRule, event: Event) => {
  const value = numberFromEvent(event, 5, 86400, 60)
  patchRule(rule.id, current => ({
    ...current,
    action: { ...current.action, cooldown_seconds: value },
  }))
}

const toggleErrorClass = (rule: RoutingSchemeRule, errorClass: RoutingRuleErrorClass) => {
  if (rule.trigger.kind !== 'error_class') return
  const selected = rule.trigger.classes.includes(errorClass)
    ? rule.trigger.classes.filter(item => item !== errorClass)
    : [...rule.trigger.classes, errorClass]
  patchRule(rule.id, current => ({
    ...current,
    trigger: { kind: 'error_class', classes: selected.length > 0 ? selected : [errorClass] },
  }))
}
</script>

<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">
          {{ t('admin.routingSchemes.policy.ruleList') }}
        </span>
        <span class="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-300">
          {{ t('admin.routingSchemes.policy.ruleCount', { count: rules.length }) }}
        </span>
      </div>
      <span class="text-[11px] text-gray-400 dark:text-gray-500">
        {{ t('admin.routingSchemes.policy.hint') }}
      </span>
    </div>

    <div
      v-for="(rule, index) in rules"
      :key="rule.id"
      class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900"
      :class="!rule.enabled && 'opacity-60'"
    >
      <div class="mb-2 flex items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <span class="text-[11px] font-semibold text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.policy.ruleIndex', { index: index + 1 }) }}
          </span>
          <label class="inline-flex items-center gap-1 text-[11px] text-gray-500 dark:text-gray-400">
            <input
              type="checkbox"
              class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="rule.enabled"
              @change="toggleRule(rule.id, $event)"
            />
            {{ t('admin.routingSchemes.policy.enabled') }}
          </label>
        </div>
        <button
          type="button"
          class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950/30"
          :title="t('admin.routingSchemes.policy.remove')"
          @click="removeRule(rule.id)"
        >
          <Icon name="trash" size="xs" />
        </button>
      </div>

      <div class="grid gap-2 sm:grid-cols-2">
        <div class="min-w-0">
          <label class="mb-1 block text-[11px] font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.policy.trigger') }}
          </label>
          <Select
            :model-value="rule.trigger.kind"
            :options="triggerOptions"
            class="w-full"
            @update:model-value="changeTriggerKind(rule, $event)"
          />
        </div>
        <div class="min-w-0">
          <label class="mb-1 block text-[11px] font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.policy.action') }}
          </label>
          <Select
            :model-value="rule.action.kind"
            :options="actionOptions"
            class="w-full"
            @update:model-value="changeActionKind(rule, $event)"
          />
        </div>
      </div>

      <div v-if="rule.trigger.kind === 'ttft'" class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[11px] text-gray-600 dark:text-gray-300">
        <span>{{ t('admin.routingSchemes.policy.ttft.threshold') }}</span>
        <input
          :value="rule.trigger.threshold_seconds"
          type="number"
          min="1"
          max="600"
          step="1"
          class="input h-7 w-16 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'threshold_seconds', $event, 1, 600, 5)"
        />
        <span>{{ t('admin.routingSchemes.policy.seconds') }}</span>
        <span class="text-gray-300 dark:text-dark-500">·</span>
        <span>{{ t('admin.routingSchemes.policy.window') }}</span>
        <input
          :value="rule.trigger.window_seconds"
          type="number"
          min="10"
          max="3600"
          step="10"
          class="input h-7 w-20 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'window_seconds', $event, 10, 3600, 120)"
        />
        <span>{{ t('admin.routingSchemes.policy.seconds') }}</span>
        <span class="text-gray-300 dark:text-dark-500">·</span>
        <span>{{ t('admin.routingSchemes.policy.ttft.consecutive') }}</span>
        <input
          :value="rule.trigger.consecutive"
          type="number"
          min="1"
          max="100"
          step="1"
          class="input h-7 w-16 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'consecutive', $event, 1, 100, 3)"
        />
        <span>{{ t('admin.routingSchemes.policy.times') }}</span>
      </div>

      <div v-else-if="rule.trigger.kind === 'error_class'" class="mt-2 flex flex-wrap items-center gap-1.5">
        <span class="text-[11px] text-gray-600 dark:text-gray-300">
          {{ t('admin.routingSchemes.policy.errorClasses.label') }}
        </span>
        <button
          v-for="option in errorClassOptions"
          :key="option.value"
          type="button"
          class="rounded-full border px-2 py-0.5 text-[11px] font-medium transition-colors"
          :class="rule.trigger.classes.includes(option.value)
            ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-500/15 dark:text-primary-300'
            : 'border-gray-200 bg-white text-gray-500 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400'"
          @click="toggleErrorClass(rule, option.value)"
        >
          {{ option.label }}
        </button>
      </div>

      <div v-else class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[11px] text-gray-600 dark:text-gray-300">
        <span>{{ t('admin.routingSchemes.policy.errorRate.threshold') }}</span>
        <input
          :value="rule.trigger.failure_percent"
          type="number"
          min="1"
          max="100"
          step="1"
          class="input h-7 w-16 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'failure_percent', $event, 1, 100, 30)"
        />
        <span>%</span>
        <span class="text-gray-300 dark:text-dark-500">·</span>
        <span>{{ t('admin.routingSchemes.policy.window') }}</span>
        <input
          :value="rule.trigger.window_seconds"
          type="number"
          min="10"
          max="3600"
          step="10"
          class="input h-7 w-20 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'window_seconds', $event, 10, 3600, 300)"
        />
        <span>{{ t('admin.routingSchemes.policy.seconds') }}</span>
        <span class="text-gray-300 dark:text-dark-500">·</span>
        <span>{{ t('admin.routingSchemes.policy.errorRate.minRequests') }}</span>
        <input
          :value="rule.trigger.min_requests"
          type="number"
          min="1"
          max="1000"
          step="1"
          class="input h-7 w-16 px-2 py-0 text-center text-xs"
          @input="updateTriggerNumber(rule, 'min_requests', $event, 1, 1000, 10)"
        />
        <span>{{ t('admin.routingSchemes.policy.times') }}</span>
      </div>

      <div class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[11px] text-gray-600 dark:text-gray-300">
        <span>{{ t('admin.routingSchemes.policy.cooldown') }}</span>
        <input
          :value="rule.action.cooldown_seconds"
          type="number"
          min="5"
          max="86400"
          step="5"
          class="input h-7 w-20 px-2 py-0 text-center text-xs"
          @input="updateCooldown(rule, $event)"
        />
        <span>{{ t('admin.routingSchemes.policy.secondsThenRecover') }}</span>
      </div>
    </div>

    <p
      v-if="rules.length === 0"
      class="rounded-lg border border-dashed border-gray-200 px-3 py-3 text-center text-[11px] text-gray-400 dark:border-dark-600 dark:text-gray-500"
    >
      {{ t('admin.routingSchemes.policy.empty') }}
    </p>

    <button
      type="button"
      class="flex w-full items-center justify-center gap-1 rounded-lg border border-dashed border-gray-300 px-3 py-1.5 text-[11px] font-medium text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-600 dark:hover:text-primary-400"
      @click="addRule"
    >
      <Icon name="plus" size="xs" />
      {{ t('admin.routingSchemes.policy.addRule') }}
    </button>
  </div>
</template>
