<template>
  <div class="flex aspect-[4/3] w-full items-center justify-center overflow-hidden rounded-lg bg-slate-50 dark:bg-dark-900">
    <iframe v-if="previewHTML && !failed" :srcdoc="previewHTML" sandbox="allow-scripts" referrerpolicy="no-referrer" loading="lazy" :title="t('common.accountQuality.preview')" class="h-full w-full border-0" @error="failed = true" />
    <img v-else-if="hasPreview(point) && !failed" :src="previewURL(point)" :alt="t('common.accountQuality.preview')" class="h-full w-full object-contain" loading="lazy" @error="failed = true" />
    <p v-else class="px-4 text-center text-sm text-slate-400">{{ t('common.accountQuality.noPreview') }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountQualityPublicPoint } from '../../data/datasources/accountQualityPublicDatasource'
import { hasPreview, previewURL } from '../qualityDisplay'
const props = defineProps<{ point: AccountQualityPublicPoint }>()
const { t } = useI18n()
const failed = ref(false)
const previewHTML = computed(() => props.point.details?.stage2?.preview_html || '')
watch(() => props.point.id, () => { failed.value = false })
</script>
