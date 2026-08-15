<template>
  <AppLayout>
    <div class="media-studio-page bg-transparent">
      <MediaStudioCanvas
        v-model:prompt="prompt"
        v-model:selected-api-key-id="selectedApiKeyId"
        v-model:model="model"
        v-model:size="size"
        v-model:quality="quality"
        v-model:count="count"
        v-model:resolution="resolution"
        v-model:duration="duration"
        :modes="modes"
        :selected-mode="selectedMode"
        :selected-mode-id="selectedModeId"
        :api-keys="apiKeys"
        :loading-keys="loadingKeys"
        :api-key-load-error="apiKeyLoadError"
        :model-options="modelOptions"
        :loading-models="loadingModels"
        :model-load-error="modelLoadError"
        :conversation="conversation"
        :has-messages="hasMessages"
        :can-submit="canSubmit"
        :submitting="submitting"
        :submit-error="submitError"
        @select-mode="selectMode"
        @reload-keys="loadApiKeys"
        @reload-models="loadModels"
        @submit="submitPrompt"
        @retry="retryMessage"
        @clear="clearConversation"
        @open-batch="openBatchWorkspace"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import MediaStudioCanvas from '@/features/media-studio/presentation/widgets/MediaStudioCanvas.vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'

const {
  modes,
  selectedMode,
  selectedModeId,
  prompt,
  selectedApiKeyId,
  model,
  size,
  quality,
  count,
  resolution,
  duration,
  apiKeys,
  loadingKeys,
  apiKeyLoadError,
  modelOptions,
  loadingModels,
  modelLoadError,
  submitting,
  submitError,
  conversation,
  hasMessages,
  canSubmit,
  loadApiKeys,
  loadModels,
  selectMode,
  submitPrompt,
  retryMessage,
  clearConversation,
} = useMediaStudioController()

const router = useRouter()

function openBatchWorkspace() {
  void router.push({ name: 'BatchImageGuide' })
}

onMounted(() => {
  void loadApiKeys()
})
</script>

<style scoped>
.media-studio-page {
  min-height: calc(100vh - 7rem);
}
</style>
