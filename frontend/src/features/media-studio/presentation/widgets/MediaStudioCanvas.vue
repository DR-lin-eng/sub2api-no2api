<template>
  <section class="flex min-h-[calc(100vh-7rem)] flex-col bg-transparent">
    <div v-if="hasMessages" class="mx-auto flex w-full max-w-5xl items-center justify-between gap-3 px-1 pb-4">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('mediaStudio.title') }}</h1>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('mediaStudio.session.localHint') }}</p>
      </div>
      <button
        type="button"
        class="inline-flex h-9 items-center gap-2 rounded-xl border border-gray-200 bg-white/85 px-3 text-sm font-medium text-gray-700 shadow-sm transition hover:bg-white dark:border-dark-700 dark:bg-dark-900/80 dark:text-gray-200 dark:hover:bg-dark-800"
        @click="emit('clear')"
      >
        <Icon name="trash" size="sm" />
        {{ t('mediaStudio.session.clear') }}
      </button>
    </div>

    <div
      v-if="hasMessages"
      class="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-5 overflow-hidden px-1 pb-5"
    >
      <article
        v-for="message in conversation.messages"
        :key="message.id"
        class="flex"
        :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
      >
        <div
          class="max-w-[min(760px,92%)] rounded-[1.35rem] border px-4 py-3 shadow-sm"
          :class="message.role === 'user'
            ? 'border-gray-900 bg-gray-950 text-white dark:border-gray-700 dark:bg-white dark:text-gray-950'
            : 'border-gray-200 bg-white/88 text-gray-900 dark:border-dark-700 dark:bg-dark-900/86 dark:text-gray-100'"
        >
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium opacity-70">
              {{ message.role === 'user' ? t('mediaStudio.session.you') : t('mediaStudio.session.studio') }}
            </span>
            <span class="text-[11px] opacity-50">{{ formatTime(message.createdAt) }}</span>
          </div>
          <p class="mt-2 whitespace-pre-wrap text-sm leading-6">{{ message.prompt }}</p>

          <div v-if="message.role === 'assistant'" class="mt-3">
            <div class="mb-3 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="meta-pill">{{ message.model || model }}</span>
              <template v-if="message.mode === 'image'">
                <span class="meta-pill">{{ message.imageResolution || imageResolution }}</span>
                <span class="meta-pill">{{ message.imageAspectRatio || imageAspectRatio }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.countValue', { count: message.count || 1 }) }}</span>
              </template>
              <template v-else>
                <span class="meta-pill">{{ message.resolution || resolution }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.durationValue', { count: message.duration || duration }) }}</span>
              </template>
              <span v-if="message.taskId" class="meta-pill font-mono">{{ message.taskId }}</span>
            </div>

            <div
              v-if="message.mode === 'image' && (message.status === 'processing' || message.status === 'queued')"
              class="grid gap-3 sm:grid-cols-2"
            >
              <button
                v-for="image in message.images || []"
                :key="image.id"
                type="button"
                class="group cursor-zoom-in overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 text-left dark:border-dark-700 dark:bg-dark-800"
                :aria-label="t('mediaStudio.session.enlargeImage')"
                @click="openImagePreview(image)"
              >
                <img
                  :src="image.src"
                  :alt="image.revisedPrompt || message.prompt"
                  referrerpolicy="no-referrer"
                  class="aspect-square w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                />
              </button>
              <div
                v-for="index in Math.max((message.count || 1) - (message.images?.length || 0), 0)"
                :key="`pending-${index}`"
                class="animate-pulse rounded-2xl border border-gray-200 bg-gray-100 aspect-square dark:border-dark-700 dark:bg-dark-800"
              ></div>
            </div>

            <div
              v-else-if="message.status === 'processing' || message.status === 'queued'"
              class="grid gap-3"
              :class="message.mode === 'image' ? 'sm:grid-cols-2' : ''"
            >
              <div
                v-for="index in (message.mode === 'image' ? (message.count || 1) : 1)"
                :key="index"
                class="animate-pulse rounded-2xl border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800"
                :class="message.mode === 'image' ? 'aspect-square' : 'aspect-video'"
              ></div>
            </div>

            <div v-else-if="message.status === 'completed' && message.mode === 'image' && message.images?.length" class="grid gap-3 sm:grid-cols-2">
              <button
                v-for="image in message.images"
                :key="image.id"
                type="button"
                class="group cursor-zoom-in overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 text-left dark:border-dark-700 dark:bg-dark-800"
                :aria-label="t('mediaStudio.session.enlargeImage')"
                @click="openImagePreview(image)"
              >
                <img
                  :src="image.src"
                  :alt="image.revisedPrompt || message.prompt"
                  referrerpolicy="no-referrer"
                  class="aspect-square w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                />
              </button>
            </div>

            <div v-else-if="message.status === 'completed' && message.mode === 'video' && message.video" class="overflow-hidden rounded-2xl border border-gray-200 bg-black dark:border-dark-700">
              <video
                :src="message.video.src"
                :type="message.video.mimeType"
                class="aspect-video w-full"
                controls
                playsinline
                preload="metadata"
              ></video>
            </div>

            <div
              v-else-if="message.status === 'failed'"
              class="rounded-2xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
            >
              <div class="flex items-start gap-2">
                <Icon name="exclamationCircle" size="sm" class="mt-0.5 flex-shrink-0" />
                <p class="min-w-0 flex-1">{{ message.error || t('mediaStudio.session.failed') }}</p>
                <button type="button" class="text-xs font-semibold underline underline-offset-4" @click="emit('retry', message)">
                  {{ t('mediaStudio.session.retry') }}
                </button>
              </div>
            </div>

            <div
              v-else-if="message.status === 'completed'"
              class="rounded-2xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300"
            >
              {{ message.mode === 'video' ? t('mediaStudio.session.noVideoResult') : t('mediaStudio.session.noImageResult') }}
            </div>
          </div>
        </div>
      </article>
    </div>

    <div
      class="mx-auto w-full max-w-5xl px-1"
      :class="hasMessages ? 'sticky bottom-0 mt-auto bg-gradient-to-t from-gray-50 via-gray-50/95 to-transparent pb-1 pt-5 dark:from-dark-950 dark:via-dark-950/95' : 'flex flex-1 items-center'"
    >
      <div class="w-full">
        <h2 v-if="!hasMessages" class="mb-8 text-center text-3xl font-semibold tracking-tight text-gray-950 dark:text-white">
          {{ t('mediaStudio.composer.greeting') }}
        </h2>

        <div class="relative mx-auto max-w-4xl">
          <div
            v-if="typeMenuOpen"
            ref="typeMenuRef"
            class="absolute bottom-16 left-3 z-20 w-56 rounded-2xl border border-gray-200 bg-white p-1.5 shadow-xl shadow-gray-900/10 dark:border-dark-700 dark:bg-dark-900"
          >
            <button
              v-for="mode in modes"
              :key="mode.id"
              type="button"
              class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-sm transition-colors"
              :class="modeButtonClass(mode)"
              @click="selectMode(mode.id)"
            >
              <span class="flex items-center gap-2.5">
                <Icon :name="mode.iconName" size="sm" />
                <span>{{ t(`mediaStudio.modeItems.${mode.id}.title`) }}</span>
              </span>
              <span v-if="!mode.available" class="text-xs text-gray-400">{{ t('mediaStudio.modeItems.disabled') }}</span>
              <Icon v-else-if="mode.id === selectedModeId" name="check" size="sm" />
            </button>
          </div>

          <div class="rounded-[1.65rem] border border-gray-200 bg-white/92 p-3 shadow-[0_16px_48px_rgba(15,23,42,0.07)] backdrop-blur dark:border-dark-700 dark:bg-dark-900/90 dark:shadow-black/20">
            <textarea
              :value="prompt"
              rows="3"
              class="min-h-24 w-full resize-none rounded-[1.1rem] border-0 bg-transparent px-2 py-2 text-base leading-7 text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
              :placeholder="selectedModeId === 'video' ? t('mediaStudio.composer.videoPlaceholder') : t('mediaStudio.composer.placeholder')"
              @input="emit('update:prompt', ($event.target as HTMLTextAreaElement).value)"
              @keydown.meta.enter.prevent="emit('submit')"
              @keydown.ctrl.enter.prevent="emit('submit')"
            />

            <MediaStudioImageAttachments
              v-if="selectedModeId === 'image'"
              :attachments="imageAttachments"
              @update="emit('update:image-attachments', $event)"
            />

            <div v-if="groupLoadError || modelLoadError || submitError" class="mb-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
              {{ submitError || groupLoadError || modelLoadError }}
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                ref="typeMenuTriggerRef"
                class="composer-chip bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white"
                :aria-expanded="typeMenuOpen"
                @click="typeMenuOpen = !typeMenuOpen"
              >
                <Icon :name="selectedMode.iconName" size="sm" />
                <span>{{ t(`mediaStudio.modeItems.${selectedMode.id}.title`) }}</span>
                <Icon :name="typeMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="text-gray-400" />
              </button>

              <label class="composer-select">
                <Icon name="grid" size="sm" />
                <select
                  :value="selectedGroupId"
                  class="select-inner min-w-[170px]"
                  :disabled="loadingGroups"
                  @change="emit('update:selectedGroupId', Number(($event.target as HTMLSelectElement).value))"
                >
                  <option :value="0">{{ loadingGroups ? t('mediaStudio.composer.loadingGroups') : t('mediaStudio.composer.selectGroup') }}</option>
                  <option v-for="group in groupOptions" :key="group.group_id" :value="group.group_id">
                    {{ group.group_name }} · {{ group.platform }}
                  </option>
                </select>
              </label>

              <label class="composer-select">
                <Icon name="cube" size="sm" />
                <select
                  v-if="modelSelectionLocked"
                  :value="model"
                  class="select-inner w-44"
                  :disabled="loadingModels || modelOptions.length === 0"
                  @change="emit('update:model', ($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ loadingModels ? t('mediaStudio.composer.loadingModels') : t('mediaStudio.composer.model') }}</option>
                  <option v-for="option in modelOptions" :key="option" :value="option">{{ option }}</option>
                </select>
                <template v-else>
                  <input
                    :value="model"
                    list="media-studio-models"
                    class="select-inner w-36"
                    :placeholder="loadingModels ? t('mediaStudio.composer.loadingModels') : t('mediaStudio.composer.model')"
                    @input="emit('update:model', ($event.target as HTMLInputElement).value)"
                  />
                  <datalist id="media-studio-models">
                    <option v-for="option in modelOptions" :key="option" :value="option" />
                  </datalist>
                </template>
              </label>

              <label v-if="selectedModeId === 'image'" class="composer-select">
                <Icon name="grid" size="sm" />
                <select
                  :value="imageResolution"
                  class="select-inner w-20"
                  @change="handleImageResolutionSelect"
                >
                  <option v-for="option in imageResolutionOptions" :key="option" :value="option">{{ option }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'image'" class="composer-select">
                <Icon name="arrowsUpDown" size="sm" />
                <select
                  :value="imageAspectRatio"
                  class="select-inner w-20"
                  @change="handleAspectRatioSelect"
                >
                  <option v-for="option in imageAspectRatioOptions" :key="option" :value="option">{{ option }}</option>
                  <option v-for="option in customImageAspectRatios" :key="`custom:${option}`" :value="`custom:${option}`">
                    {{ option }}
                  </option>
                  <option value="__custom__">{{ t('mediaStudio.composer.customAspectRatio.option') }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'image' && imageQualityOptions.length > 0" class="composer-select">
                <Icon name="sparkles" size="sm" />
                <select :value="quality" class="select-inner w-24" @change="emit('update:quality', ($event.target as HTMLSelectElement).value)">
                  <option v-for="option in imageQualityOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'image'" class="composer-select">
                <Icon name="copy" size="sm" />
                <select :value="count" class="select-inner w-20" @change="emit('update:count', Number(($event.target as HTMLSelectElement).value))">
                  <option v-for="option in countOptions" :key="option" :value="option">{{ option }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'video'" class="composer-select">
                <Icon name="play" size="sm" />
                <select :value="resolution" class="select-inner w-24" @change="emit('update:resolution', ($event.target as HTMLSelectElement).value as MediaStudioVideoResolution)">
                  <option v-for="option in resolutionOptions" :key="option" :value="option">{{ option }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'video'" class="composer-select">
                <Icon name="clock" size="sm" />
                <select :value="duration" class="select-inner w-20" @change="emit('update:duration', Number(($event.target as HTMLSelectElement).value))">
                  <option v-for="option in durationOptions" :key="option" :value="option">{{ option }}s</option>
                </select>
              </label>

              <button
                type="button"
                class="ml-auto flex h-10 w-10 items-center justify-center rounded-full transition"
                :class="canSubmit ? 'bg-gray-950 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-950 dark:hover:bg-gray-200' : 'bg-gray-200 text-gray-400 dark:bg-dark-700 dark:text-gray-500'"
                :aria-label="t('mediaStudio.composer.send')"
                :disabled="!canSubmit"
                @click="emit('submit')"
              >
                <Icon :name="submitting ? 'refresh' : 'arrowUp'" size="sm" :class="submitting ? 'animate-spin' : ''" />
              </button>
            </div>

            <div v-if="!loadingGroups && groupOptions.length === 0" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ groupLoadError || t('mediaStudio.composer.noGroups') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadGroups')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
            <div v-else-if="modelOptions.length === 0 && !loadingModels" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ t('mediaStudio.composer.manualModelHint') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadModels')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
          </div>

          <p v-if="!hasMessages" class="mx-auto mt-4 max-w-2xl text-center text-xs leading-5 text-gray-400 dark:text-gray-500">
            {{ t('mediaStudio.composer.shortHint') }}
          </p>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <MediaStudioCustomResolutionDialog
        :visible="customAspectRatioDialogOpen"
        @close="customAspectRatioDialogOpen = false"
        @save="handleCustomResolutionSave"
      />

      <div
        v-if="selectedImage"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm sm:p-8"
        role="dialog"
        aria-modal="true"
        :aria-label="t('mediaStudio.session.enlargeImage')"
        @click.self="closeImagePreview"
      >
        <button
          type="button"
          class="absolute right-4 top-4 z-10 inline-flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20 focus:outline-none focus:ring-2 focus:ring-white/70"
          :aria-label="t('common.close')"
          @click="closeImagePreview"
        >
          <Icon name="x" size="md" />
        </button>
        <figure class="flex max-h-full max-w-full flex-col items-center gap-3" @click.stop>
          <img
            :src="selectedImage.url || selectedImage.src"
            :alt="selectedImage.revisedPrompt || t('mediaStudio.session.enlargeImage')"
            referrerpolicy="no-referrer"
            class="max-h-[calc(100vh-5rem)] max-w-[calc(100vw-2rem)] rounded-xl object-contain shadow-2xl sm:max-h-[calc(100vh-7rem)] sm:max-w-[calc(100vw-4rem)]"
          />
          <figcaption
            v-if="selectedImage.revisedPrompt"
            class="max-w-3xl text-center text-xs leading-5 text-white/80"
          >
            {{ selectedImage.revisedPrompt }}
          </figcaption>
        </figure>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import MediaStudioImageAttachments from '@/features/media-studio/presentation/widgets/MediaStudioImageAttachments.vue'
import MediaStudioCustomResolutionDialog from '@/features/media-studio/presentation/widgets/MediaStudioCustomResolutionDialog.vue'
import type { MediaStudioVideoResolution } from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type { MediaStudioGroupOption } from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type { MediaStudioParameterOption } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import type { MediaStudioImageAttachment } from '@/features/media-studio/presentation/composables/useMediaStudioAttachments'
import type { MediaStudioMode, MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'
import type {
  MediaStudioConversation,
  MediaStudioGeneratedImagePreview,
  MediaStudioImageAspectRatio,
  MediaStudioImageResolution,
  MediaStudioMessage,
} from '@/features/media-studio/presentation/composables/useMediaStudioController'

const props = defineProps<{
  modes: MediaStudioMode[]
  selectedMode: MediaStudioMode
  selectedModeId: MediaStudioModeId
  prompt: string
  selectedGroupId: number
  model: string
  modelSelectionLocked: boolean
  imageResolution: MediaStudioImageResolution
  imageAspectRatio: MediaStudioImageAspectRatio
  customImageAspectRatios: string[]
  quality: string
  count: number
  resolution: MediaStudioVideoResolution
  duration: number
  imageQualityOptions: MediaStudioParameterOption[]
  groupOptions: MediaStudioGroupOption[]
  loadingGroups: boolean
  groupLoadError: string
  imageAttachments: MediaStudioImageAttachment[]
  modelOptions: string[]
  loadingModels: boolean
  modelLoadError: string
  conversation: MediaStudioConversation
  hasMessages: boolean
  canSubmit: boolean
  submitting: boolean
  submitError: string
}>()

const emit = defineEmits<{
  'update:prompt': [value: string]
  'update:selectedGroupId': [value: number]
  'update:model': [value: string]
  'update:imageResolution': [value: MediaStudioImageResolution]
  'update:imageAspectRatio': [value: MediaStudioImageAspectRatio]
  addCustomImageAspectRatio: [value: string]
  'update:quality': [value: string]
  'update:count': [value: number]
  'update:resolution': [value: MediaStudioVideoResolution]
  'update:duration': [value: number]
  selectMode: [id: MediaStudioModeId]
  reloadGroups: []
  'update:image-attachments': [value: MediaStudioImageAttachment[]]
  reloadModels: []
  submit: []
  retry: [message: MediaStudioMessage]
  clear: []
}>()

const { t, locale } = useI18n()
const typeMenuOpen = ref(false)
const typeMenuRef = ref<HTMLElement | null>(null)
const typeMenuTriggerRef = ref<HTMLButtonElement | null>(null)
const selectedImage = ref<MediaStudioGeneratedImagePreview | null>(null)
const customAspectRatioDialogOpen = ref(false)

const countOptions = [1, 2, 3, 4]
const imageResolutionOptions: MediaStudioImageResolution[] = ['1K', '2K', '4K']
const imageAspectRatioOptions: MediaStudioImageAspectRatio[] = ['1:1', '3:2', '2:3', '4:3', '3:4', '16:9', '9:16']
const resolutionOptions: MediaStudioVideoResolution[] = ['480p', '720p', '1080p']
const durationOptions = Array.from({ length: 15 }, (_, index) => index + 1)

function selectMode(id: MediaStudioModeId) {
  const mode = props.modes.find(item => item.id === id)
  if (!mode?.available) return
  emit('selectMode', id)
  typeMenuOpen.value = false
}

function modeButtonClass(mode: MediaStudioMode) {
  if (!mode.available) return 'cursor-not-allowed text-gray-400 dark:text-gray-600'
  if (mode.id === props.selectedModeId) return 'bg-gray-100 text-gray-950 dark:bg-dark-800 dark:text-white'
  return 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800'
}

function openImagePreview(image: MediaStudioGeneratedImagePreview) {
  selectedImage.value = image
}

function closeImagePreview() {
  selectedImage.value = null
}

function handleImageResolutionSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:imageResolution', value as MediaStudioImageResolution)
}

function handleAspectRatioSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (value === '__custom__') {
    customAspectRatioDialogOpen.value = true
    return
  }
  if (value.startsWith('custom:')) {
    emit('update:imageAspectRatio', value as MediaStudioImageAspectRatio)
    return
  }
  emit('update:imageAspectRatio', value as MediaStudioImageAspectRatio)
}

function handleCustomResolutionSave(value: string) {
  emit('addCustomImageAspectRatio', value)
  customAspectRatioDialogOpen.value = false
}

function formatTime(value: number): string {
  try {
    return new Intl.DateTimeFormat(locale.value, {
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(value))
  } catch {
    return ''
  }
}

function closeTypeMenuOnOutsidePointer(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (typeMenuRef.value?.contains(target) || typeMenuTriggerRef.value?.contains(target)) return
  typeMenuOpen.value = false
}

function closeTypeMenuOnEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  typeMenuOpen.value = false
  closeImagePreview()
}

onMounted(() => {
  document.addEventListener('pointerdown', closeTypeMenuOnOutsidePointer)
  document.addEventListener('keydown', closeTypeMenuOnEscape)
})

onUnmounted(() => {
  document.removeEventListener('pointerdown', closeTypeMenuOnOutsidePointer)
  document.removeEventListener('keydown', closeTypeMenuOnEscape)
})
</script>

<style scoped>
.composer-chip {
  @apply inline-flex h-10 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800;
}

.composer-select {
  @apply inline-flex h-10 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 shadow-sm transition-colors focus-within:border-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:focus-within:border-dark-500;
}

.select-inner {
  @apply border-0 bg-transparent text-sm font-medium text-gray-700 outline-none placeholder:text-gray-400 dark:text-gray-200 dark:placeholder:text-gray-500;
  color-scheme: light dark;
}

.select-inner option {
  @apply bg-white text-gray-800 dark:bg-dark-900 dark:text-gray-100;
}

.meta-pill {
  @apply inline-flex h-6 items-center rounded-full border border-gray-200 bg-gray-50 px-2 dark:border-dark-700 dark:bg-dark-800;
}
</style>
