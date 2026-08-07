<template>
  <section class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="mb-5 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ t('mediaStudio.modes.eyebrow') }}</p>
        <h2 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('mediaStudio.modes.title') }}</h2>
      </div>
      <p class="max-w-xl text-sm text-gray-500 dark:text-gray-400">{{ t('mediaStudio.modes.description') }}</p>
    </div>

    <div class="grid gap-4 md:grid-cols-3">
      <button
        v-for="mode in modes"
        :key="mode.id"
        type="button"
        class="group rounded-2xl border p-5 text-left transition hover:-translate-y-0.5 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-primary-500/70"
        :class="mode.id === selectedModeId ? 'border-primary-500 bg-primary-50/70 shadow-md dark:border-primary-400 dark:bg-primary-950/20' : 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/70'"
        :aria-pressed="mode.id === selectedModeId"
        @click="$emit('select', mode.id)"
      >
        <div class="flex items-start justify-between gap-4">
          <span class="flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br text-xl font-bold text-white shadow-lg" :class="mode.accentClass">
            {{ mode.icon }}
          </span>
          <span class="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-200">
            {{ t('mediaStudio.status.preview') }}
          </span>
        </div>
        <h3 class="mt-5 text-lg font-semibold text-gray-900 dark:text-white">{{ t(`mediaStudio.modeItems.${mode.id}.title`) }}</h3>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(`mediaStudio.modeItems.${mode.id}.description`) }}</p>
        <div class="mt-4 flex items-center text-sm font-medium text-primary-600 dark:text-primary-400">
          {{ t('mediaStudio.modes.selectPreview') }}
          <span class="ml-1 transition group-hover:translate-x-1">→</span>
        </div>
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MediaStudioMode, MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

defineProps<{
  modes: MediaStudioMode[]
  selectedModeId: MediaStudioModeId
}>()

defineEmits<{
  select: [id: MediaStudioModeId]
}>()

const { t } = useI18n()
</script>
