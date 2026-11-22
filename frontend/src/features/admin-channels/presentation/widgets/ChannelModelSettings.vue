<template>
  <div class="space-y-4">
    <section class="model-panel">
      <div class="mb-1 flex items-center justify-between">
        <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.form.modelMapping', 'Model Mapping') }}</label>
        <button type="button" @click="addMappingEntry()" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('common.add', 'Add') }}
        </button>
      </div>
      <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.modelMappingHint') }}</p>
      <div
        v-if="Object.keys(section.model_mapping).length === 0"
        class="rounded-lg border border-dashed border-gray-300 px-3 py-5 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400"
      >
        {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="(_, srcModel) in section.model_mapping"
          :key="srcModel"
          class="flex items-center gap-2"
        >
          <input
            :value="srcModel"
            type="text"
            class="input min-w-0 flex-1 text-sm"
            :class="platformTextClass(section.platform)"
            :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
            @change="renameMappingKey(srcModel, $event)"
          />
          <span class="text-gray-400 text-xs">→</span>
          <input
            :value="section.model_mapping[srcModel]"
            type="text"
            class="input min-w-0 flex-1 text-sm"
            :class="platformTextClass(section.platform)"
            :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
            @input="section.model_mapping[srcModel] = ($event.target as HTMLInputElement).value"
          />
          <button
            type="button"
            :aria-label="t('common.delete')"
            @click="delete section.model_mapping[srcModel]"
            class="rounded p-0.5 text-gray-400 hover:text-red-500"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </section>

    <section class="model-panel">
      <div class="mb-1 flex items-center justify-between">
        <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }}</label>
        <div class="flex items-center gap-2">
          <button
            type="button"
            v-if="channelProviders[section.platform].catalogSync"
            @click="emit('sync')"
            :disabled="syncing"
            class="text-xs text-gray-500 hover:text-primary-600 disabled:opacity-50"
          >
            {{ syncing ? t('admin.channels.form.syncingModels') : t('admin.channels.form.syncLatestModels') }}
          </button>
          <button type="button" @click="section.model_pricing.push(newPricingEntry())" class="text-xs text-primary-600 hover:text-primary-700">
            + {{ t('common.add', 'Add') }}
          </button>
        </div>
      </div>
      <div
        v-if="section.model_pricing.length === 0"
        class="rounded-lg border border-dashed border-gray-300 px-3 py-5 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400"
      >
        {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-2">
        <PricingEntryCard
          v-for="(entry, idx) in section.model_pricing"
          :key="idx"
          :entry="entry"
          :platform="section.platform"
          :enable-time-pricing="true"
          @update="section.model_pricing.splice(idx, 1, $event)"
          @remove="section.model_pricing.splice(idx, 1)"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import { platformTextClass } from '@/core/utils/platformColors'
import { channelProviders } from '../channelProviders'
import { newPricingEntry } from '../adminChannelSignals'
import type { PlatformSection } from '../channelFormCodec'
import PricingEntryCard from './PricingEntryCard.vue'

defineProps<{ syncing: boolean }>()
const emit = defineEmits<{ sync: [] }>()
const section = defineModel<PlatformSection>({ required: true })
const { t } = useI18n()

function addMappingEntry() {
  const mapping = section.value.model_mapping
  let key = ''
  let i = 1
  while (key === '' || key in mapping) {
    key = `model-${i}`
    i++
  }
  mapping[key] = ''
}

function renameMappingKey(oldKey: string, event: Event) {
  const input = event.target as HTMLInputElement
  const newKey = input.value.trim()
  const mapping = section.value.model_mapping
  if (!newKey || newKey in mapping) {
    input.value = oldKey
    return
  }
  const value = mapping[oldKey]
  delete mapping[oldKey]
  mapping[newKey] = value
}
</script>

<style scoped>
.model-panel {
  @apply min-w-0 rounded-xl border border-gray-200 p-4 dark:border-dark-600 sm:p-5;
}
</style>
