<template>
  <div class="command-bar">
    <div class="relative inline-block">
      <button
        ref="menuTriggerRef"
        type="button"
        class="command-bar__trigger"
        :aria-expanded="menuOpen"
        aria-haspopup="menu"
        @click="toggleMenu"
      >
        <Icon name="menu" size="sm" />
        <span>{{ currentViewLabel }}</span>
        <span class="command-bar__badge">{{ sessions.length }}</span>
        <Icon name="chevronDown" size="xs" />
      </button>

      <div v-if="menuOpen" ref="menuRef" class="command-bar__panel" role="menu">
        <button
          v-for="option in viewOptions"
          :key="option.value"
          type="button"
          role="menuitemradio"
          :aria-checked="option.value === modelValue"
          class="command-bar__item"
          :class="{ 'is-active': option.value === modelValue }"
          @click="handleSelectView(option.value)"
        >
          <Icon :name="option.icon" size="sm" />
          <span class="command-bar__item-label">{{ option.label }}</span>
          <span v-if="option.value === 'gallery' && galleryCount > 0" class="command-bar__badge">
            {{ galleryCount }}
          </span>
          <Icon v-else-if="option.value === modelValue" name="check" size="xs" />
        </button>

        <div class="command-bar__divider" role="presentation"></div>

        <MediaStudioSessionList
          :sessions="sessions"
          :active-id="activeSessionId"
          @select="handleSelect"
          @remove="handleRemove"
        />

        <div class="command-bar__divider" role="presentation"></div>

        <button type="button" class="command-bar__item is-primary" @click="handleNewConversation">
          <Icon name="plus" size="sm" />
          <span class="command-bar__item-label">{{ t('mediaStudio.session.new') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import MediaStudioSessionList from '@/features/media-studio/presentation/widgets/MediaStudioSessionList.vue'
import type { MediaStudioSessionSummary } from '@/features/media-studio/presentation/composables/useMediaStudioController'

type MediaStudioView = 'studio' | 'gallery'

const props = withDefaults(defineProps<{
  modelValue: MediaStudioView
  galleryCount?: number
  sessions?: MediaStudioSessionSummary[]
  activeSessionId: string
}>(), {
  galleryCount: 0,
  sessions: () => [],
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: MediaStudioView): void
  (e: 'newConversation'): void
  (e: 'selectSession', id: string): void
  (e: 'removeSession', id: string): void
}>()

const { t } = useI18n()
const menuOpen = ref(false)
const menuTriggerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)

const viewOptions = computed<{ value: MediaStudioView; label: string; icon: 'sparkles' | 'grid' }[]>(() => [
  { value: 'studio', label: t('mediaStudio.tabs.studio'), icon: 'sparkles' },
  { value: 'gallery', label: t('mediaStudio.tabs.gallery'), icon: 'grid' },
])
const currentViewLabel = computed(
  () => viewOptions.value.find(option => option.value === props.modelValue)?.label ?? '',
)

function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

function handleSelectView(value: MediaStudioView) {
  menuOpen.value = false
  if (value !== props.modelValue) emit('update:modelValue', value)
}

function handleSelect(id: string) {
  menuOpen.value = false
  emit('selectSession', id)
}

function handleRemove(id: string) {
  emit('removeSession', id)
}

function handleNewConversation() {
  menuOpen.value = false
  emit('newConversation')
}

function closeOnOutsidePointer(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (menuRef.value?.contains(target) || menuTriggerRef.value?.contains(target)) return
  menuOpen.value = false
}

function closeOnEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  menuOpen.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', closeOnOutsidePointer)
  document.addEventListener('keydown', closeOnEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', closeOnOutsidePointer)
  document.removeEventListener('keydown', closeOnEscape)
})
</script>

<style scoped>
.command-bar {
  @apply sticky top-16 z-20 mx-auto mb-3 w-full max-w-5xl px-1;
}

.command-bar__trigger {
  @apply inline-flex h-9 items-center gap-2 rounded-xl border border-gray-200 bg-white/85 px-3 text-xs font-medium text-gray-700 shadow-sm backdrop-blur transition-colors;
  @apply hover:bg-gray-100 hover:text-gray-900;
  @apply dark:border-dark-700 dark:bg-dark-900/85 dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white;
}

/* 下拉面板必须纵向排列：创作 / 作品库 / 分隔线 / 历史对话 / 新建对话。 */
.command-bar__panel {
  @apply absolute left-0 top-full z-30 mt-2 flex max-h-96 w-64 flex-col gap-0.5 overflow-y-auto rounded-2xl border border-gray-200 bg-white p-1.5 shadow-xl shadow-gray-900/10;
  @apply dark:border-dark-700 dark:bg-dark-900 dark:shadow-black/30;
}

.command-bar__item {
  @apply flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-gray-700 transition-colors;
  @apply hover:bg-gray-100;
  @apply dark:text-gray-300 dark:hover:bg-dark-800;
}

.command-bar__item.is-active {
  @apply bg-primary-50 font-medium text-primary-600;
  @apply dark:bg-primary-500/15 dark:text-primary-300;
}

.command-bar__item.is-primary {
  @apply text-primary-600;
  @apply dark:text-primary-300;
}

.command-bar__item-label {
  @apply min-w-0 flex-1 truncate;
}

.command-bar__divider {
  @apply my-1 h-px w-full bg-gray-200 dark:bg-dark-700;
}

.command-bar__badge {
  @apply inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-gray-100 px-1.5 text-[11px] font-medium text-gray-500;
  @apply dark:bg-dark-800 dark:text-gray-400;
}
</style>
