<template>
  <section class="relative overflow-hidden rounded-3xl border border-white/60 bg-gradient-to-br from-slate-950 via-slate-900 to-slate-800 p-8 text-white shadow-2xl shadow-slate-900/20 dark:border-white/10">
    <div class="absolute -right-16 -top-16 h-48 w-48 rounded-full bg-fuchsia-500/30 blur-3xl"></div>
    <div class="absolute bottom-0 left-1/3 h-40 w-40 rounded-full bg-sky-500/20 blur-3xl"></div>

    <div class="relative grid gap-8 lg:grid-cols-[1.2fr_0.8fr] lg:items-end">
      <div class="space-y-6">
        <div class="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/10 px-3 py-1 text-sm font-medium text-slate-100 backdrop-blur">
          <span class="h-2 w-2 rounded-full bg-emerald-400"></span>
          {{ t('mediaStudio.hero.eyebrow') }}
        </div>
        <div class="space-y-4">
          <h1 class="max-w-3xl text-4xl font-bold tracking-tight sm:text-5xl">
            {{ t('mediaStudio.title') }}
          </h1>
          <p class="max-w-2xl text-base leading-7 text-slate-300 sm:text-lg">
            {{ t('mediaStudio.description') }}
          </p>
        </div>
        <div class="flex flex-wrap gap-3">
          <span
            v-for="item in heroTags"
            :key="item"
            class="rounded-full bg-white/10 px-3 py-1 text-sm text-slate-100 ring-1 ring-white/15"
          >
            {{ t(`mediaStudio.hero.tags.${item}`) }}
          </span>
        </div>
      </div>

      <div class="rounded-2xl border border-white/10 bg-white/10 p-5 backdrop-blur">
        <div class="text-sm text-slate-300">{{ t('mediaStudio.hero.previewLabel') }}</div>
        <div class="mt-3 text-2xl font-semibold">{{ primaryModeLabel }}</div>
        <div class="mt-5 grid grid-cols-3 gap-3 text-center">
          <div
            v-for="stat in stats"
            :key="stat"
            class="rounded-2xl bg-slate-950/40 p-3 ring-1 ring-white/10"
          >
            <div class="text-xl font-bold">{{ t(`mediaStudio.hero.stats.${stat}.value`) }}</div>
            <div class="mt-1 text-xs text-slate-300">{{ t(`mediaStudio.hero.stats.${stat}.label`) }}</div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  primaryModeLabel: string
}>()

const { t } = useI18n()
const heroTags = ['images', 'videos', 'preview'] as const
const stats = ['modes', 'flow', 'api'] as const
</script>
