<template>
  <section class="mx-auto flex w-full max-w-7xl flex-col gap-5 py-2 sm:py-4">
    <header>
      <h4 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('admin.modelProviders.chooseProvider') }}</h4>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ hint }}</p>
    </header>

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="inline-flex w-fit rounded-full bg-gray-100 p-1 dark:bg-dark-900" :aria-label="t('admin.modelProviders.category')" role="group">
        <button
          v-for="filter in categories" :key="filter" type="button" :aria-pressed="category === filter"
          class="rounded-full px-3 py-1.5 text-sm font-medium focus-visible:outline-primary-500"
          :class="category === filter ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
          @click="category = filter"
        >{{ t(categoryKeys[filter]) }}</button>
      </div>
      <div class="relative sm:w-80">
        <Icon name="search" size="md" aria-hidden="true" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
        <input v-model="query" type="search" class="input pl-9" :placeholder="t('admin.modelProviders.search')" :aria-label="t('admin.modelProviders.search')" />
      </div>
    </div>

    <div v-if="matches.length" :data-tour="gridTour" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <button
        v-for="platform in matches" :key="platform" type="button" :data-testid="`account-platform-${platform}`"
        class="group flex min-w-0 flex-col rounded-xl border p-4 text-left transition-colors focus-visible:outline-primary-500 enabled:hover:border-primary-400 enabled:hover:bg-primary-50/50 disabled:cursor-default disabled:opacity-60 dark:enabled:hover:border-primary-500 dark:enabled:hover:bg-primary-900/15"
        :class="platform === current ? 'border-primary-400 bg-primary-50/50 dark:border-primary-500 dark:bg-primary-900/15' : 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800'"
        :disabled="configured.includes(platform) && platform !== current"
        @click="emit('select', platform)"
      >
        <span class="flex w-full items-center gap-3">
          <span :class="['grid h-9 w-9 shrink-0 place-items-center rounded-xl', platformBadgeLightClass(platform)]">
            <PlatformIcon :platform="platform" size="lg" aria-hidden="true" />
          </span>
          <span class="flex-1 font-semibold text-gray-900 dark:text-white">{{ modelProviderLabel(t, platform) }}</span>
          <Icon :name="configured.includes(platform) ? 'check' : 'chevronRight'" size="sm" class="text-gray-400 group-hover:text-primary-600" aria-hidden="true" />
        </span>
        <span class="mt-3 text-sm text-gray-600 dark:text-gray-300">{{ t(descriptionKeys[platform]) }}</span>
        <span class="mt-auto pt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ platform === current ? t('admin.modelProviders.current') : configured.includes(platform) ? t('admin.modelProviders.configured') : t(categoryKeys[providerCategories[platform]]) }}
        </span>
      </button>
    </div>
    <div v-else class="rounded-xl border border-dashed border-gray-200 py-12 text-center dark:border-dark-600" role="status">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.modelProviders.noMatches') }}</p>
      <button type="button" class="mt-2 text-sm font-medium text-primary-600 dark:text-primary-400" @click="clearFilters">{{ t('admin.modelProviders.clearFilters') }}</button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import { platformBadgeLightClass } from '@/core/utils/platformColors'
import type { AccountPlatform } from '@/types'
import { modelProviderLabel, modelProviderPlatforms, providerCategories, type ProviderCategory } from '@/core/constants/modelProviders'

withDefaults(defineProps<{
  hint: string
  gridTour?: string
  current?: AccountPlatform | null
  configured?: AccountPlatform[]
}>(), { current: null, configured: () => [] })
const emit = defineEmits<{ select: [platform: AccountPlatform] }>()
const { t } = useI18n()
const categoryKeys = {
  all: 'admin.modelProviders.all',
  vendors: 'admin.modelProviders.vendors',
  platforms: 'admin.modelProviders.platforms',
} as const
const descriptionKeys = {
  anthropic: 'admin.modelProviders.descriptions.anthropic',
  openai: 'admin.modelProviders.descriptions.openai',
  gemini: 'admin.modelProviders.descriptions.gemini',
  antigravity: 'admin.modelProviders.descriptions.antigravity',
  grok: 'admin.modelProviders.descriptions.grok',
  kimi: 'admin.modelProviders.descriptions.kimi',
  zhipu: 'admin.modelProviders.descriptions.zhipu',
  deepseek: 'admin.modelProviders.descriptions.deepseek',
  minimax: 'admin.modelProviders.descriptions.minimax',
  opencode_go: 'admin.modelProviders.descriptions.opencode_go',
  typesafe: 'admin.modelProviders.descriptions.typesafe',
  multimodal: 'admin.modelProviders.descriptions.multimodal',
} satisfies Record<AccountPlatform, string>
const categories = ['all', 'vendors', 'platforms'] as const
const category = ref<ProviderCategory | 'all'>('all')
const query = ref('')

const matches = computed(() => {
  const term = query.value.trim().toLowerCase()
  return modelProviderPlatforms.filter(platform => {
    if (category.value !== 'all' && providerCategories[platform] !== category.value) return false
    const keywords = [platform, modelProviderLabel(t, platform), t(descriptionKeys[platform])]
    return keywords.some(keyword => keyword.toLowerCase().includes(term))
  })
})

function clearFilters() {
  category.value = 'all'
  query.value = ''
}
</script>
