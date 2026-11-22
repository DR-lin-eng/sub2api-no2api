<template>
  <div class="space-y-5">
    <fieldset class="rounded-xl border border-gray-200 p-4 dark:border-dark-600 sm:p-5">
      <legend class="sr-only">{{ t('admin.channels.form.groups') }}</legend>
      <div class="mb-3 flex items-center justify-between gap-2">
        <h4 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.form.groups') }} <span class="text-red-500">*</span></h4>
        <span v-if="section.group_ids.length" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.selectedCount', { count: section.group_ids.length }) }}</span>
      </div>
      <p v-if="loading" class="text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</p>
      <p v-else-if="!groups.length" class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.channels.wizard.noGroups') }}</p>
      <input v-if="!loading && groups.length" v-model="groupSearch" type="search" class="input mb-2 text-sm"
        :aria-label="t('admin.channels.wizard.groupSearch')" :placeholder="t('admin.channels.wizard.groupSearch')" />
      <p v-if="groups.length && !loading && !matchingGroups.length" class="text-sm text-gray-500">{{ t('admin.channels.form.noGroupsMatch') }}</p>
      <div v-if="!loading && matchingGroups.length" class="max-h-64 space-y-1 overflow-y-auto">
        <label
          v-for="group in matchingGroups" :key="group.id"
          class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm"
          :class="[section.group_ids.includes(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-600 dark:bg-primary-900/20' : 'border-transparent hover:bg-gray-50 dark:hover:bg-dark-700', occupied.has(group.id) ? 'opacity-60' : 'cursor-pointer']"
        >
          <input v-model="section.group_ids" :value="group.id" type="checkbox" :disabled="occupied.has(group.id)" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
          <span class="min-w-0 flex-1 break-words text-gray-800 dark:text-gray-200">{{ group.name }}</span>
          <span v-if="occupied.has(group.id)" class="text-xs text-gray-500">{{ t('admin.channels.form.inOtherChannel', { name: occupied.get(group.id) }) }}</span>
          <span :class="['rounded-md px-1.5 py-0.5 text-xs', platformBadgeLightClass(group.platform)]">{{ group.rate_multiplier }}×</span>
          <span class="text-xs text-gray-500" :title="t('admin.channels.form.ruleAccounts')">{{ group.account_count || 0 }}</span>
        </label>
      </div>
    </fieldset>

    <section v-if="options.length" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600 sm:p-5">
      <h4 class="mb-2 font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.wizard.providerOptions', { name: groupPlatformLabel(t, section.platform) }) }}</h4>
      <div v-for="option in options" :key="option" class="flex items-start justify-between gap-4 border-t border-gray-100 py-3 first-of-type:border-0 last:pb-0 dark:border-dark-700">
        <div>
          <p class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t(optionText[option].label) }}</p>
          <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t(optionText[option].hint) }}</p>
        </div>
        <Toggle v-model="section[option]" :aria-label="t(optionText[option].label)" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { platformBadgeLightClass } from '@/core/utils/platformColors'
import { groupPlatformLabel } from '@/features/admin-groups/groupsLocale'
import type { AdminGroup } from '@/types'
import type { PlatformSection } from '../channelFormCodec'
import { channelProviders } from '../channelProviders'

const props = defineProps<{
  groups: AdminGroup[]
  occupied: ReadonlyMap<number, string>
  loading: boolean
  webSearchEnabled: boolean
}>()
const section = defineModel<PlatformSection>({ required: true })
const { t } = useI18n()
const groupSearch = ref('')
const matchingGroups = computed(() => {
  const term = groupSearch.value.trim().toLowerCase()
  return props.groups.filter(group => group.name.toLowerCase().includes(term) || String(group.id) === term.replace(/^#/, ''))
})
const optionText = {
  web_search_emulation: { label: 'admin.channels.form.webSearchEmulation', hint: 'admin.channels.form.webSearchEmulationHint' },
  codex_image_generation_bridge: { label: 'admin.channels.form.codexImageGenerationBridge', hint: 'admin.channels.form.codexImageGenerationBridgeHint' },
  bedrock_cc_compat: { label: 'admin.channels.form.bedrockCCCompat', hint: 'admin.channels.form.bedrockCCCompatHint' },
}
const options = computed(() => (channelProviders[section.value.platform].options ?? [])
  .filter(option => option !== 'web_search_emulation' || props.webSearchEnabled))
</script>
