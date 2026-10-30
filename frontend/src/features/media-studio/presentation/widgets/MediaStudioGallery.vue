<template>
  <section class="mx-auto w-full max-w-6xl px-1 pb-24 pt-2">
    <header class="flex flex-wrap items-end justify-between gap-3 pb-4">
      <div>
        <h2 class="text-lg font-semibold text-gray-950 dark:text-white">
          {{ t('mediaStudio.gallery.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('mediaStudio.gallery.subtitle') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('mediaStudio.gallery.filters.count', { count: items.length }) }}
        </span>
        <button v-if="items.length > 0" type="button" class="gallery-chip danger" @click="clearAll">
          <Icon name="trash" size="sm" />
          {{ t('mediaStudio.gallery.clear') }}
        </button>
      </div>
    </header>

    <p v-if="storageUnavailable" class="gallery-empty">
      {{ t('mediaStudio.gallery.storageUnavailable') }}
    </p>

    <div v-else class="flex flex-wrap items-center gap-2 pb-4">
      <div class="gallery-segment">
        <button
          v-for="option in modeOptions"
          :key="option.value"
          type="button"
          class="gallery-chip"
          :class="{ 'is-active': modeFilter === option.value }"
          @click="modeFilter = option.value"
        >
          {{ option.label }}
        </button>
      </div>
      <button
        type="button"
        class="gallery-chip"
        :class="{ 'is-active': favoriteOnly }"
        @click="favoriteOnly = !favoriteOnly"
      >
        <Icon name="badge" size="sm" />
        {{ t('mediaStudio.gallery.filters.favoriteOnly') }}
      </button>
      <label class="gallery-search">
        <Icon name="search" size="sm" />
        <input
          v-model="keyword"
          type="search"
          :placeholder="t('mediaStudio.gallery.filters.search')"
        />
      </label>
    </div>

    <p v-if="loading" class="gallery-empty">{{ t('mediaStudio.gallery.loading') }}</p>
    <p v-else-if="filteredItems.length === 0" class="gallery-empty">
      {{ items.length === 0 ? t('mediaStudio.gallery.empty') : t('mediaStudio.gallery.emptyFiltered') }}
    </p>

    <div v-else class="gallery-grid">
      <article v-for="item in filteredItems" :key="item.id" class="gallery-card">
        <div class="gallery-media">
          <img
            v-if="item.mode === 'image' && sources[item.id]"
            :src="sources[item.id]"
            :alt="item.prompt"
            loading="lazy"
          />
          <video
            v-else-if="item.mode === 'video' && sources[item.id]"
            :src="sources[item.id]"
            muted
            loop
            playsinline
            preload="metadata"
            @mouseenter="playPreview"
            @mouseleave="pausePreview"
          />
          <span v-else class="gallery-placeholder">{{ t('mediaStudio.gallery.mediaMissing') }}</span>
          <span class="gallery-badge">{{ modeLabel(item) }}</span>
          <Icon v-if="item.favorite" name="badge" size="sm" class="gallery-favorite-mark" />
        </div>

        <div class="gallery-body">
          <p class="gallery-prompt" :title="item.prompt">{{ item.prompt || t('mediaStudio.gallery.untitled') }}</p>
          <p class="gallery-meta" :title="savedAt(item)">{{ metaLine(item) }}</p>
          <div class="gallery-actions">
            <button
              type="button"
              class="gallery-action"
              :title="t('mediaStudio.gallery.actions.reuse')"
              @click="emit('reuse', item)"
            >
              <Icon name="refresh" size="sm" />
              <span>{{ t('mediaStudio.gallery.actions.reuse') }}</span>
            </button>
            <button
              v-if="item.mode === 'image'"
              type="button"
              class="gallery-action"
              :title="t('mediaStudio.gallery.actions.edit')"
              @click="emit('edit', item)"
            >
              <Icon name="edit" size="sm" />
              <span>{{ t('mediaStudio.gallery.actions.edit') }}</span>
            </button>
            <a
              v-if="sources[item.id]"
              class="gallery-action"
              :href="sources[item.id]"
              :download="downloadName(item)"
              :title="t('mediaStudio.gallery.actions.download')"
            >
              <Icon name="download" size="sm" />
              <span>{{ t('mediaStudio.gallery.actions.download') }}</span>
            </a>
            <button
              type="button"
              class="gallery-action"
              :title="item.favorite ? t('mediaStudio.gallery.actions.unfavorite') : t('mediaStudio.gallery.actions.favorite')"
              @click="emit('favorite', item)"
            >
              <Icon name="badge" size="sm" />
              <span>{{ item.favorite ? t('mediaStudio.gallery.actions.unfavorite') : t('mediaStudio.gallery.actions.favorite') }}</span>
            </button>
            <button
              type="button"
              class="gallery-action danger"
              :title="t('mediaStudio.gallery.actions.remove')"
              @click="removeItem(item)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { MediaStudioGalleryItem } from '@/features/media-studio/data/datasources/mediaStudioGalleryStore'

// 布尔 prop 缺省时 Vue 会强制转为 false，必须显式给默认值，否则未传属性会被误判为存储不可用。
const props = withDefaults(
  defineProps<{
    items: MediaStudioGalleryItem[]
    sources: Record<string, string>
    loading: boolean
    storageAvailable?: boolean
  }>(),
  { storageAvailable: true },
)

const emit = defineEmits<{
  (e: 'favorite', item: MediaStudioGalleryItem): void
  (e: 'remove', item: MediaStudioGalleryItem): void
  (e: 'reuse', item: MediaStudioGalleryItem): void
  (e: 'edit', item: MediaStudioGalleryItem): void
  (e: 'clear'): void
}>()

const { t, locale } = useI18n()

type GalleryModeFilter = 'all' | 'image' | 'video'

const modeFilter = ref<GalleryModeFilter>('all')
const favoriteOnly = ref(false)
const keyword = ref('')

// 未传入时默认视作可用，避免父组件漏传属性把整个作品库误判为不可用。
const storageUnavailable = computed(() => props.storageAvailable === false)

const modeOptions = computed<Array<{ value: GalleryModeFilter; label: string }>>(() => [
  { value: 'all', label: t('mediaStudio.gallery.filters.all') },
  { value: 'image', label: t('mediaStudio.gallery.filters.image') },
  { value: 'video', label: t('mediaStudio.gallery.filters.video') },
])

const filteredItems = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return props.items.filter((item) => {
    if (modeFilter.value !== 'all' && item.mode !== modeFilter.value) return false
    if (favoriteOnly.value && !item.favorite) return false
    if (!search) return true
    return item.prompt.toLowerCase().includes(search) || (item.model || '').toLowerCase().includes(search)
  })
})

function modeLabel(item: MediaStudioGalleryItem) {
  return item.mode === 'video'
    ? t('mediaStudio.gallery.filters.video')
    : t('mediaStudio.gallery.filters.image')
}

function metaLine(item: MediaStudioGalleryItem) {
  const parts = item.mode === 'video'
    ? [item.resolution, item.aspectRatio, item.duration ? `${item.duration}s` : '']
    : [item.imageResolution, item.size, item.quality]
  return parts.filter(Boolean).join(' · ')
}

function savedAt(item: MediaStudioGalleryItem) {
  return new Date(item.createdAt).toLocaleString(locale.value)
}

function downloadName(item: MediaStudioGalleryItem) {
  const extension = item.mode === 'video' ? 'mp4' : 'png'
  return `media-studio-${item.id}.${extension}`
}

function playPreview(event: MouseEvent) {
  const video = event.currentTarget as HTMLVideoElement | null
  if (video && typeof video.play === 'function') void video.play().catch(() => undefined)
}

function pausePreview(event: MouseEvent) {
  const video = event.currentTarget as HTMLVideoElement | null
  if (video && typeof video.pause === 'function') video.pause()
}

function removeItem(item: MediaStudioGalleryItem) {
  if (!window.confirm(t('mediaStudio.gallery.removeConfirm'))) return
  emit('remove', item)
}

function clearAll() {
  if (!window.confirm(t('mediaStudio.gallery.clearConfirm'))) return
  emit('clear')
}
</script>

<style scoped>
.gallery-grid {
  display: grid;
  gap: 0.875rem;
  grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
}

.gallery-card {
  @apply flex flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white;
  @apply dark:border-dark-700 dark:bg-dark-900;
  transition: box-shadow 150ms ease, transform 150ms ease, border-color 150ms ease;
}

.gallery-card:hover {
  transform: translateY(-1px);
  @apply border-gray-300 shadow-lg shadow-gray-900/5 dark:border-dark-600 dark:shadow-black/30;
}

.gallery-media {
  position: relative;
  aspect-ratio: 1 / 1;
  @apply bg-gray-100 dark:bg-dark-800;
}

.gallery-media img,
.gallery-media video {
  height: 100%;
  width: 100%;
  object-fit: cover;
}

.gallery-badge {
  position: absolute;
  top: 0.5rem;
  left: 0.5rem;
  @apply rounded-full bg-gray-900/70 px-2 py-0.5 text-[11px] text-white dark:bg-black/60;
}

.gallery-favorite-mark {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  @apply text-amber-500;
}

.gallery-placeholder {
  @apply flex h-full items-center justify-center text-xs text-gray-500 dark:text-gray-400;
}

.gallery-body {
  @apply flex flex-1 flex-col gap-1.5 px-3 pb-3 pt-2.5;
}

.gallery-prompt {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  @apply text-[13px] leading-snug text-gray-900 dark:text-gray-100;
}

.gallery-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  @apply text-[11px] text-gray-500 dark:text-gray-400;
}

.gallery-actions {
  @apply mt-auto flex flex-wrap items-center gap-1 pt-1;
}

.gallery-action,
.gallery-chip {
  @apply inline-flex items-center gap-1 rounded-full border border-gray-200 px-2 py-1 text-[11px] text-gray-700 transition-colors;
  @apply hover:bg-gray-50 hover:text-gray-900;
  @apply dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white;
}

.gallery-chip.is-active {
  @apply border-primary-500 bg-primary-50 text-primary-600;
  @apply dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300;
}

.gallery-action.danger,
.gallery-chip.danger {
  @apply text-red-600 dark:text-red-400;
}

.gallery-segment {
  @apply inline-flex gap-1;
}

.gallery-search {
  @apply inline-flex min-w-[10rem] max-w-[18rem] flex-1 items-center gap-1.5 rounded-full border border-gray-200 px-2.5 py-1 text-gray-500;
  @apply dark:border-dark-700 dark:text-gray-400;
}

.gallery-search input {
  @apply w-full border-none bg-transparent text-xs text-gray-900 outline-none placeholder:text-gray-400;
  @apply dark:text-gray-100 dark:placeholder:text-gray-500;
}

.gallery-empty {
  @apply rounded-2xl border border-dashed border-gray-300 px-4 py-12 text-center text-[13px] text-gray-500;
  @apply dark:border-dark-600 dark:text-gray-400;
}
</style>
