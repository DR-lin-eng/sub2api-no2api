<template>
  <AppLayout>
    <div class="relative min-h-[calc(100vh-7rem)] overflow-hidden rounded-3xl bg-transparent">
      <div class="absolute right-5 top-5 z-10">
        <button type="button" class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-800 shadow-sm transition hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800">
          <Icon name="inbox" size="sm" />
          {{ t('mediaStudio.composer.assets') }}
        </button>
      </div>

      <MediaStudioCanvas
        :modes="modes"
        :selected-mode="selectedMode"
        :selected-mode-id="selectedModeId"
        @select="selectedModeId = $event"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import MediaStudioCanvas from '@/features/media-studio/presentation/widgets/MediaStudioCanvas.vue'
import { useMediaStudioPreview, type MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const { t } = useI18n()
const { modes, getModeById } = useMediaStudioPreview()
const selectedModeId = ref<MediaStudioModeId>('image')
const selectedMode = computed(() => getModeById(selectedModeId.value))
</script>

