<template>
  <section class="mt-3 border-t border-gray-200 pt-3 dark:border-dark-600">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
      <div class="min-w-0 flex-1 sm:max-w-sm">
        <label class="block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.timePricing') }}</label>
        <label :for="`${idPrefix}-timezone`" class="mt-2 block text-xs text-gray-400">{{ t('admin.channels.form.timezone') }}</label>
        <input :id="`${idPrefix}-timezone`" :value="modelValue.timezone" list="channel-time-pricing-timezones" class="input mt-1 w-full text-sm" placeholder="Asia/Shanghai" @input="updateTimezone(($event.target as HTMLInputElement).value)" />
        <datalist id="channel-time-pricing-timezones">
          <option v-for="timezone in commonTimezones" :key="timezone" :value="timezone" />
        </datalist>
      </div>
      <button type="button" class="self-start text-xs text-primary-600 hover:text-primary-700 sm:self-end sm:pb-2" data-testid="add-time-period" @click="addPeriod">
        + {{ t('admin.channels.form.addTimePeriod') }}
      </button>
    </div>
    <div v-if="modelValue.periods.length > 0" class="mt-3 space-y-3">
      <div v-for="(period, index) in modelValue.periods" :key="index" class="grid grid-cols-1 gap-2 border-t border-gray-200 pt-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_2rem] sm:items-end dark:border-dark-600">
        <div>
          <label :for="`${idPrefix}-start-${index}`" class="block text-xs text-gray-400">{{ t('admin.channels.form.startTime') }}</label>
          <input :id="`${idPrefix}-start-${index}`" :value="period.start_time" type="time" step="1" class="input mt-1 w-full text-sm" @input="updatePeriod(index, 'start_time', ($event.target as HTMLInputElement).value)" />
        </div>
        <div>
          <label :for="`${idPrefix}-end-${index}`" class="block text-xs text-gray-400">{{ t('admin.channels.form.endTime') }}</label>
          <input :id="`${idPrefix}-end-${index}`" :value="period.end_time" type="time" step="1" class="input mt-1 w-full text-sm" @input="updatePeriod(index, 'end_time', ($event.target as HTMLInputElement).value)" />
        </div>
        <div>
          <label :for="`${idPrefix}-multiplier-${index}`" class="block text-xs text-gray-400">{{ t('admin.channels.form.multiplier') }}</label>
          <input :id="`${idPrefix}-multiplier-${index}`" :value="period.multiplier" type="number" min="0.01" step="0.01" class="input mt-1 w-full text-sm" @input="updatePeriod(index, 'multiplier', ($event.target as HTMLInputElement).value)" />
        </div>
        <button type="button" class="flex h-8 w-8 items-center justify-center rounded text-gray-400 hover:text-red-500" :title="t('admin.channels.form.removeTimePeriod')" :aria-label="t('admin.channels.form.removeTimePeriod')" :data-testid="`remove-time-period-${index}`" @click="removePeriod(index)">
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { getCurrentInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'

type TimePricingPeriod = { start_time: string; end_time: string; multiplier: number | string }
type TimePricingValue = { timezone: string; periods: TimePricingPeriod[] }

const props = defineProps<{ modelValue: TimePricingValue }>()
const emit = defineEmits<{ 'update:modelValue': [value: TimePricingValue] }>()
const { t } = useI18n()
const idPrefix = `channel-time-pricing-${getCurrentInstance()?.uid ?? 'entry'}`
const commonTimezones = ['UTC', 'Asia/Shanghai', 'Asia/Tokyo', 'Asia/Singapore', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'America/Los_Angeles']

function updateTimezone(timezone: string) { emit('update:modelValue', { ...props.modelValue, timezone }) }
function addPeriod() {
  emit('update:modelValue', { ...props.modelValue, periods: [...props.modelValue.periods, { start_time: '09:00', end_time: '18:00', multiplier: '1.00' }] })
}
function updatePeriod(index: number, field: keyof TimePricingPeriod, value: string) {
  const periods = props.modelValue.periods.map((period, current) => current === index ? { ...period, [field]: value } : period)
  emit('update:modelValue', { ...props.modelValue, periods })
}
function removePeriod(index: number) {
  emit('update:modelValue', { ...props.modelValue, periods: props.modelValue.periods.filter((_period, current) => current !== index) })
}
</script>
