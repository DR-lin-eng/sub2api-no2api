<template>
  <section class="grid gap-6 lg:grid-cols-[0.95fr_1.05fr]">
    <div class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <p class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ t('mediaStudio.workspace.eyebrow') }}</p>
      <h2 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('mediaStudio.workspace.title') }}</h2>
      <p class="mt-3 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('mediaStudio.workspace.description') }}</p>

      <div class="mt-6 space-y-3">
        <div
          v-for="stage in previewStages"
          :key="stage.id"
          class="flex gap-3 rounded-2xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70"
        >
          <span class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-600 text-sm font-bold text-white">{{ stage.step }}</span>
          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">{{ t(`mediaStudio.stages.${stage.id}.title`) }}</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t(`mediaStudio.stages.${stage.id}.description`) }}</p>
          </div>
        </div>
      </div>
    </div>

    <div class="overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <div class="border-b border-gray-100 p-5 dark:border-dark-800">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.2em] text-gray-400">{{ t('mediaStudio.workspace.canvasLabel') }}</p>
            <h3 class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ t(`mediaStudio.modeItems.${selectedMode.id}.title`) }}</h3>
          </div>
          <span class="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-gray-300">{{ t('mediaStudio.status.comingSoon') }}</span>
        </div>
      </div>

      <div class="bg-[radial-gradient(circle_at_top_left,_rgba(99,102,241,0.22),_transparent_32%),linear-gradient(135deg,_rgba(15,23,42,0.96),_rgba(30,41,59,0.92))] p-6">
        <div class="grid min-h-[360px] gap-4 md:grid-cols-[0.82fr_1.18fr]">
          <div class="rounded-2xl border border-white/10 bg-white/10 p-4 backdrop-blur">
            <div class="h-3 w-24 rounded-full bg-white/25"></div>
            <div class="mt-5 space-y-3">
              <div class="h-11 rounded-xl bg-white/15"></div>
              <div class="h-24 rounded-xl bg-white/10"></div>
              <div class="grid grid-cols-2 gap-3">
                <div class="h-10 rounded-xl bg-white/10"></div>
                <div class="h-10 rounded-xl bg-white/10"></div>
              </div>
            </div>
            <button class="mt-5 w-full rounded-xl bg-white/90 px-4 py-2 text-sm font-semibold text-slate-900 opacity-70" disabled>
              {{ t('mediaStudio.workspace.generatePlaceholder') }}
            </button>
          </div>

          <div class="relative overflow-hidden rounded-2xl border border-white/10 bg-slate-950/50 p-4">
            <div class="absolute inset-x-8 top-8 h-24 rounded-full bg-gradient-to-r blur-3xl" :class="selectedMode.accentClass"></div>
            <div class="relative flex h-full flex-col justify-between rounded-xl border border-white/10 bg-white/[0.06] p-5">
              <div>
                <div class="flex items-center gap-2 text-sm text-slate-300">
                  <span class="h-2 w-2 rounded-full bg-emerald-400"></span>
                  {{ t('mediaStudio.workspace.previewState') }}
                </div>
                <div class="mt-8 aspect-video rounded-2xl border border-white/10 bg-gradient-to-br from-white/20 to-white/5 p-4 shadow-inner">
                  <div class="h-full rounded-xl border border-dashed border-white/25 bg-black/10"></div>
                </div>
              </div>
              <p class="mt-6 text-sm leading-6 text-slate-300">{{ t(`mediaStudio.modeItems.${selectedMode.id}.hint`) }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MediaStudioMode, MediaStudioPreviewStage } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

defineProps<{
  selectedMode: MediaStudioMode
  previewStages: MediaStudioPreviewStage[]
}>()

const { t } = useI18n()
</script>
