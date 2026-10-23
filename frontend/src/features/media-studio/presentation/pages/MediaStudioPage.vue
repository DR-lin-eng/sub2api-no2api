<template>
  <AppLayout>
    <div class="media-studio-page bg-transparent">
      <MediaStudioCanvas
        v-model:prompt="prompt"
        v-model:selected-group-id="selectedGroupId"
        v-model:model="model"
        :model-selection-locked="modelSelectionLocked"
        v-model:image-resolution="imageResolution"
        v-model:image-aspect-ratio="imageAspectRatio"
        :custom-image-aspect-ratios="customImageAspectRatios"
        v-model:quality="quality"
        v-model:count="count"
        v-model:resolution="resolution"
        v-model:video-aspect-ratio="videoAspectRatio"
        v-model:duration="duration"
        :duration-options="durationOptions"
        :image-quality-options="imageQualityOptions"
        :modes="modes"
        :selected-mode="selectedMode"
        :selected-mode-id="selectedModeId"
        :group-options="groupOptions"
        :loading-groups="loadingGroups"
        :group-load-error="groupLoadError"
        :image-attachments="imageAttachments"
        :image-attachment-limits="imageAttachmentLimits"
        :reference-attachments="referenceAttachments"
        :reference-limits="referenceLimits"
        :model-limits="activeModelLimits"
        :storage-available="storageAvailable"
        :attachment-rejection="attachmentRejection"
        :model-options="modelOptions"
        :attribute-fields="visibleAttributeFields"
        :attribute-values="attributeValues"
        :hidden-parameter-controls="hiddenParameterControls"
        :loading-models="loadingModels"
        :model-load-error="modelLoadError"
        :conversation="conversation"
        :has-messages="hasMessages"
        :can-submit="canSubmit"
        :submitting="submitting"
        :submit-error="submitError"
        @select-mode="selectMode"
        @reload-groups="loadMediaGroups"
        @add-image-attachments="addImageAttachments"
        @remove-image-attachment="removeImageAttachment"
        @add-reference-attachments="addReferenceAttachments"
        @remove-reference-attachment="removeReferenceAttachment"
        @retry-reference-upload="retryReferenceUpload"
        @add-custom-image-aspect-ratio="addCustomImageAspectRatio"
        @reload-models="loadModels"
        @update:attribute="({ key, value }) => setAttributeValue(key, value)"
        @submit="submitPrompt"
        @retry="retryMessage"
        @clear="clearConversation"
        @delete="deleteMessages"
        @edit-image="editGeneratedImage"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, onMounted } from 'vue'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import MediaStudioCanvas from '@/features/media-studio/presentation/widgets/MediaStudioCanvas.vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'

const {
  modes,
  selectedMode,
  selectedModeId,
  prompt,
  selectedGroupId,
  model,
  modelSelectionLocked,
  imageResolution,
  imageAspectRatio,
  customImageAspectRatios,
  addCustomImageAspectRatio,
  quality,
  count,
  resolution,
  videoAspectRatio,
  duration,
  durationOptions,
  imageQualityOptions,
  groupOptions,
  loadingGroups,
  groupLoadError,
  imageAttachments,
  imageAttachmentLimits,
  addImageAttachments,
  removeImageAttachment,
  attachmentRejection,
  referenceAttachments,
  referenceLimits,
  addReferenceAttachments,
  removeReferenceAttachment,
  retryReferenceUpload,
  activeModelLimits,
  storageAvailable,
  modelOptions,
  visibleAttributeFields,
  attributeValues,
  hiddenParameterControls,
  setAttributeValue,
  loadingModels,
  modelLoadError,
  submitting,
  submitError,
  conversation,
  hasMessages,
  canSubmit,
  loadMediaGroups,
  loadModels,
  selectMode,
  submitPrompt,
  retryMessage,
  clearConversation,
  deleteMessages,
  editGeneratedImage,
} = useMediaStudioController()

function scrollToLatest() {
  window.scrollTo({ top: document.documentElement.scrollHeight })
}

onMounted(async () => {
  void loadMediaGroups()
  await nextTick()
  scrollToLatest()
  // 图片和视频缓存是异步恢复的，等布局稳定后再对齐一次底部。
  window.setTimeout(scrollToLatest, 300)
})
</script>

<style scoped>
.media-studio-page {
  min-height: calc(100vh - 7rem);
}
</style>
