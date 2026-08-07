<template>
  <AppLayout>
    <div class="space-y-6">
      <MediaStudioHero :primary-mode-label="t(`mediaStudio.modeItems.${selectedMode.id}.title`)" />
      <MediaStudioModeGrid
        :modes="modes"
        :selected-mode-id="selectedModeId"
        @select="selectedModeId = $event"
      />
      <MediaStudioWorkspace :selected-mode="selectedMode" :preview-stages="previewStages" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import MediaStudioHero from '@/features/media-studio/presentation/widgets/MediaStudioHero.vue'
import MediaStudioModeGrid from '@/features/media-studio/presentation/widgets/MediaStudioModeGrid.vue'
import MediaStudioWorkspace from '@/features/media-studio/presentation/widgets/MediaStudioWorkspace.vue'
import { useMediaStudioPreview, type MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const { t } = useI18n()
const { modes, previewStages, getModeById } = useMediaStudioPreview()
const selectedModeId = ref<MediaStudioModeId>('image')
const selectedMode = computed(() => getModeById(selectedModeId.value))
</script>
