<template>
  <AppLayout>
    <div ref="pageRef" class="media-studio-page bg-transparent">
      <MediaStudioCommandBar
        v-model="activeView"
        :gallery-count="galleryTotal"
        :sessions="sessionList"
        :active-session-id="conversation.id"
        @new-conversation="onNewConversation"
        @select-session="onSelectSession"
        @remove-session="removeConversation"
      />

      <MediaStudioCanvas
        v-if="activeView === 'studio'"
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
        :jobs="jobs"
        :can-submit="canSubmit"
        :submitting="submitting"
        :submit-error="submitError"
        :estimated-cost="estimatedCost"
        :estimated-currency="estimatedCurrency"
        :estimate-loading="estimateLoading"
        :estimate-error="estimateError"
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
        @submit="handleSubmit"
        @retry="retryMessage"
        @delete="deleteMessages"
        @edit-image="editGeneratedImage"
      />

      <MediaStudioGallery
        v-else
        :items="galleryItems"
        :sources="gallerySources"
        :loading="galleryLoading"
        :storage-available="galleryStorageAvailable"
        @favorite="toggleGalleryFavorite"
        @remove="removeGalleryItem"
        @edit="editGalleryItem"
        @clear="clearGallery"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import MediaStudioCanvas from '@/features/media-studio/presentation/widgets/MediaStudioCanvas.vue'
import MediaStudioCommandBar from '@/features/media-studio/presentation/widgets/MediaStudioCommandBar.vue'
import MediaStudioGallery from '@/features/media-studio/presentation/widgets/MediaStudioGallery.vue'
import type { MediaStudioGalleryItem } from '@/features/media-studio/data/datasources/mediaStudioGalleryStore'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import { useMediaStudioGallery } from '@/features/media-studio/presentation/composables/useMediaStudioGallery'

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
  estimatedCost,
  estimatedCurrency,
  estimateLoading,
  estimateError,
  conversation,
  hasMessages,
  sessionList,
  jobs,
  canSubmit,
  loadMediaGroups,
  loadModels,
  selectMode,
  submitPrompt,
  retryMessage,
  newConversation,
  switchConversation,
  removeConversation,
  deleteMessages,
  editGeneratedImage,
} = useMediaStudioController()

const {
  items: galleryItems,
  sources: gallerySources,
  loading: galleryLoading,
  storageAvailable: galleryStorageAvailable,
  totalCount: galleryTotal,
  refresh: refreshGallery,
  saveFromMessages: archiveConversation,
  toggleFavorite: toggleGalleryFavorite,
  remove: removeGalleryItem,
  clear: clearGallery,
} = useMediaStudioGallery()

const activeView = ref<'studio' | 'gallery'>('studio')
// 归档前必须先读取已有作品，否则会把收藏状态等元数据用默认值覆盖。
const galleryReady = ref(false)
const pageRef = ref<HTMLElement | null>(null)

// 页面滚动容器是 window，且主题在 html 上开了 scroll-smooth；
// 这里临时关掉平滑滚动再贴底，打开时才能直接停在最新消息，而不是慢慢滑下去。
let pinnedToBottom = true
let pageResizeObserver: ResizeObserver | null = null

function scrollToLatest() {
  if (!pinnedToBottom) return
  const root = document.documentElement
  const previousBehavior = root.style.scrollBehavior
  root.style.scrollBehavior = 'auto'
  window.scrollTo({ top: root.scrollHeight })
  root.style.scrollBehavior = previousBehavior
}

// 用户主动往上翻后不再强行贴底，回到底部附近时恢复跟随。
function syncPinnedState() {
  const root = document.documentElement
  pinnedToBottom = root.scrollHeight - window.scrollY - window.innerHeight <= 48
}

function handleSubmit() {
  pinnedToBottom = true
  submitPrompt()
}

function onNewConversation() {
  activeView.value = 'studio'
  pinnedToBottom = true
  newConversation()
  void nextTick(() => scrollToLatest())
}

async function onSelectSession(id: string) {
  activeView.value = 'studio'
  pinnedToBottom = true
  await switchConversation(id)
  await nextTick()
  scrollToLatest()
}

// 用完成状态的签名作为归档触发条件，避免深度侦听大体积的图片 data URL。
const completedSignature = computed(() => conversation.value.messages
  .filter(message => message.role === 'assistant' && message.status === 'completed')
  .map(message => [
    message.id,
    message.completedAt ?? 0,
    (message.images ?? []).length,
    message.video?.cacheKey ?? '',
  ].join(':'))
  .join('|'))

watch(completedSignature, async () => {
  // 结果渲染会撑高页面，贴底状态下静默跟到最后一条。
  await nextTick()
  scrollToLatest()
  if (!galleryReady.value) return
  void archiveConversation(conversation.value.messages)
})

watch(() => conversation.value.messages.length, async () => {
  await nextTick()
  scrollToLatest()
})

// 从作品库切回工坊时同样直接落在最新消息，不出现从顶部滑下来的过程。
watch(activeView, async (view) => {
  if (view !== 'studio') return
  pinnedToBottom = true
  await nextTick()
  scrollToLatest()
})

function editGalleryItem(item: MediaStudioGalleryItem) {
  const source = gallerySources[item.id]
  if (!source) return
  activeView.value = 'studio'
  void editGeneratedImage({ id: item.id, src: source })
}

onMounted(() => {
  // 首帧同步贴底：不能等作品库等异步数据回来，否则用户会先看到顶部再滑下去。
  scrollToLatest()
  // 图片和视频缓存是异步恢复的，高度变化时静默贴底，避免出现可见的平滑滚动。
  if (typeof ResizeObserver !== 'undefined' && pageRef.value) {
    pageResizeObserver = new ResizeObserver(() => scrollToLatest())
    pageResizeObserver.observe(pageRef.value)
  }
  window.addEventListener('scroll', syncPinnedState, { passive: true })
  void (async () => {
    void loadMediaGroups()
    await refreshGallery()
    galleryReady.value = true
    // 首次加载时把历史已完成结果补录进作品库，已存在条目会被去重跳过。
    void archiveConversation(conversation.value.messages)
    await nextTick()
    scrollToLatest()
  })()
})

onBeforeUnmount(() => {
  pageResizeObserver?.disconnect()
  pageResizeObserver = null
  window.removeEventListener('scroll', syncPinnedState)
})
</script>

<style scoped>
.media-studio-page {
  /* 预留表头 4rem 和左上角工具条 3.5rem，避免空会话时多出一条滚动条。 */
  min-height: calc(100vh - 10.5rem);
}
</style>
