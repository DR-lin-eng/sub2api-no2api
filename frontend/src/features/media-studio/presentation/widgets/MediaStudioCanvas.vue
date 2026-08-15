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
                <span class="meta-pill">{{ sizeLabel(message.size || size) }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.countValue', { count: message.count || 1 }) }}</span>
              </template>
              <template v-else>
                <span class="meta-pill">{{ message.resolution || resolution }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.durationValue', { count: message.duration || duration }) }}</span>
              </template>
              <span v-if="message.taskId" class="meta-pill font-mono">{{ message.taskId }}</span>
            </div>

            <div
              v-if="message.status === 'processing' || message.status === 'queued'"
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
              <a
                v-for="image in message.images"
                :key="image.id"
                :href="image.url || image.src"
                target="_blank"
                rel="noopener noreferrer"
                class="group overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
              >
                <img :src="image.src" :alt="image.revisedPrompt || message.prompt" referrerpolicy="no-referrer" class="aspect-square w-full object-cover transition duration-200 group-hover:scale-[1.02]" />
              </a>
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
            <div v-if="selectedModeId === 'batch'" class="rounded-[1.1rem] bg-gray-50 px-4 py-5 dark:bg-dark-800/70">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('mediaStudio.batch.title') }}</h3>
              <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('mediaStudio.batch.description') }}</p>
              <button type="button" class="btn btn-primary mt-4" @click="emit('openBatch')">
                {{ t('mediaStudio.batch.open') }}
              </button>
            </div>

            <textarea
              v-else
              :value="prompt"
              rows="3"
              class="min-h-24 w-full resize-none rounded-[1.1rem] border-0 bg-transparent px-2 py-2 text-base leading-7 text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
              :placeholder="selectedModeId === 'video' ? t('mediaStudio.composer.videoPlaceholder') : t('mediaStudio.composer.placeholder')"
              @input="emit('update:prompt', ($event.target as HTMLTextAreaElement).value)"
              @keydown.meta.enter.prevent="emit('submit')"
              @keydown.ctrl.enter.prevent="emit('submit')"
            />

            <div v-if="apiKeyLoadError || modelLoadError || submitError" class="mb-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
              {{ submitError || apiKeyLoadError || modelLoadError }}
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                class="composer-chip bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white"
                :aria-expanded="typeMenuOpen"
                @click="typeMenuOpen = !typeMenuOpen"
              >
                <Icon :name="selectedMode.iconName" size="sm" />
                <span>{{ t(`mediaStudio.modeItems.${selectedMode.id}.title`) }}</span>
                <Icon :name="typeMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="text-gray-400" />
              </button>

              <label v-if="selectedModeId !== 'batch'" class="composer-select">
                <Icon name="key" size="sm" />
                <select
                  :value="selectedApiKeyId"
                  class="select-inner min-w-[150px]"
                  :disabled="loadingKeys"
                  @change="emit('update:selectedApiKeyId', Number(($event.target as HTMLSelectElement).value))"
                >
                  <option :value="0">{{ loadingKeys ? t('mediaStudio.composer.loadingKeys') : t('mediaStudio.composer.selectKey') }}</option>
                  <option v-for="key in apiKeys" :key="key.id" :value="key.id">
                    {{ key.name }}
                  </option>
                </select>
              </label>

              <label v-if="selectedModeId !== 'batch'" class="composer-select">
                <Icon name="cube" size="sm" />
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
              </label>

              <label v-if="selectedModeId === 'image'" class="composer-select">
                <Icon name="grid" size="sm" />
                <select :value="size" class="select-inner w-28" @change="emit('update:size', ($event.target as HTMLSelectElement).value)">
                  <option v-for="option in sizeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                </select>
              </label>

              <label v-if="selectedModeId === 'image'" class="composer-select">
                <Icon name="sparkles" size="sm" />
                <select :value="quality" class="select-inner w-24" @change="emit('update:quality', ($event.target as HTMLSelectElement).value)">
                  <option v-for="option in qualityOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
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
                v-if="selectedModeId !== 'batch'"
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

            <div v-if="selectedModeId !== 'batch' && !loadingKeys && apiKeys.length === 0" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ t('mediaStudio.composer.noKeys') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadKeys')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
            <div v-else-if="selectedModeId !== 'batch' && modelOptions.length === 0 && !loadingModels" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ t('mediaStudio.composer.manualModelHint') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadModels')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
          </div>

          <p v-if="!hasMessages" class="mx-auto mt-4 max-w-2xl text-center text-xs leading-5 text-gray-400 dark:text-gray-500">
            {{ selectedModeId === 'batch' ? t('mediaStudio.composer.batchHint') : t('mediaStudio.composer.shortHint') }}
          </p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ApiKey } from '@/types'
import type { MediaStudioVideoResolution } from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type { MediaStudioMode, MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'
import type { MediaStudioConversation, MediaStudioMessage } from '@/features/media-studio/presentation/composables/useMediaStudioController'

const props = defineProps<{
  modes: MediaStudioMode[]
  selectedMode: MediaStudioMode
  selectedModeId: MediaStudioModeId
  prompt: string
  selectedApiKeyId: number
  model: string
  size: string
  quality: string
  count: number
  resolution: MediaStudioVideoResolution
  duration: number
  apiKeys: ApiKey[]
  loadingKeys: boolean
  apiKeyLoadError: string
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
  'update:selectedApiKeyId': [value: number]
  'update:model': [value: string]
  'update:size': [value: string]
  'update:quality': [value: string]
  'update:count': [value: number]
  'update:resolution': [value: MediaStudioVideoResolution]
  'update:duration': [value: number]
  selectMode: [id: MediaStudioModeId]
  reloadKeys: []
  reloadModels: []
  submit: []
  retry: [message: MediaStudioMessage]
  clear: []
  openBatch: []
}>()

const { t, locale } = useI18n()
const typeMenuOpen = ref(false)

const sizeOptions = [
  { value: '1024x1024', label: '1:1' },
  { value: '1536x1024', label: '3:2' },
  { value: '1024x1536', label: '2:3' },
  { value: '2048x2048', label: '2K' },
]

const qualityOptions = [
  { value: 'auto', label: 'Auto' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Med' },
  { value: 'high', label: 'High' },
]

const countOptions = [1, 2, 3, 4]
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

function sizeLabel(value: string): string {
  return sizeOptions.find(option => option.value === value)?.label || value
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
}

.meta-pill {
  @apply inline-flex h-6 items-center rounded-full border border-gray-200 bg-gray-50 px-2 dark:border-dark-700 dark:bg-dark-800;
}
</style>
