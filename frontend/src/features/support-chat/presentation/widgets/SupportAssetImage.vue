<template>
  <div
    ref="root"
    class="relative overflow-hidden bg-gray-100 dark:bg-dark-900"
    :class="[containerClass, previewable && src ? 'cursor-zoom-in' : '']"
    :role="previewable && src ? 'button' : undefined"
    :tabindex="previewable && src ? 0 : undefined"
    @click="openPreview"
    @keydown.enter.prevent="openPreview"
    @keydown.space.prevent="openPreview"
  >
    <img
      v-if="src"
      :src="src"
      :alt="alt"
      class="h-full w-full object-cover"
      loading="lazy"
      decoding="async"
      referrerpolicy="no-referrer"
    />
    <div v-else-if="loading" class="flex h-full min-h-20 items-center justify-center text-xs text-gray-500 dark:text-dark-400">
      {{ t('common.loading') }}
    </div>
    <button
      v-else
      type="button"
      class="flex h-full min-h-20 w-full items-center justify-center px-3 text-xs text-red-600 dark:text-red-300"
      @click="load"
    >
      {{ t('supportChat.assets.retry') }}
    </button>
  </div>

  <Teleport to="body">
    <div
      v-if="previewOpen && src"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/90 p-4"
      role="dialog"
      aria-modal="true"
      :aria-label="alt || t('supportChat.assets.image')"
      @click="closePreview"
    >
      <button
        type="button"
        class="absolute right-4 top-4 inline-flex h-10 w-10 items-center justify-center rounded-full bg-white/15 text-2xl text-white hover:bg-white/25"
        :aria-label="t('common.close')"
        @click="closePreview"
      >
        ×
      </button>
      <img :src="src" :alt="alt" class="max-h-[90vh] max-w-[94vw] object-contain" @click.stop />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getChatAssetBlob } from '@/features/support-chat/data/datasources/supportChatDatasource'

const props = withDefaults(defineProps<{
  assetId: number
  scope: 'user' | 'admin'
  alt?: string
  containerClass?: string
  lazy?: boolean
  previewable?: boolean
}>(), {
  alt: '',
  containerClass: '',
  lazy: false,
  previewable: false,
})

const { t } = useI18n()
const src = ref('')
const loading = ref(false)
const root = ref<HTMLElement | null>(null)
const previewOpen = ref(false)
let requestSequence = 0
let mounted = false
let observer: IntersectionObserver | null = null

function revoke() {
  previewOpen.value = false
  if (!src.value) return
  URL.revokeObjectURL(src.value)
  src.value = ''
}

function openPreview() {
  if (props.previewable && src.value) previewOpen.value = true
}

function closePreview() {
  previewOpen.value = false
}

function handleEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') closePreview()
}

async function load() {
  const sequence = ++requestSequence
  loading.value = true
  revoke()
  try {
    const blob = await getChatAssetBlob(props.scope, props.assetId)
    if (sequence !== requestSequence) return
    src.value = URL.createObjectURL(blob)
  } catch {
    // The retry control intentionally avoids exposing backend details.
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

function scheduleLoad() {
  observer?.disconnect()
  observer = null
  if (loading.value || src.value) return
  if (!props.lazy || typeof IntersectionObserver === 'undefined') {
    void load()
    return
  }
  if (!mounted || !root.value) return
  observer = new IntersectionObserver((entries) => {
    if (!entries.some(entry => entry.isIntersecting)) return
    observer?.disconnect()
    observer = null
    void load()
  }, { rootMargin: '160px' })
  observer.observe(root.value)
}

watch(() => [props.assetId, props.scope, props.lazy] as const, () => {
  requestSequence += 1
  loading.value = false
  revoke()
  scheduleLoad()
}, { immediate: true })

onMounted(() => {
  mounted = true
  document.addEventListener('keydown', handleEscape)
  scheduleLoad()
})

onBeforeUnmount(() => {
  requestSequence += 1
  observer?.disconnect()
  observer = null
  document.removeEventListener('keydown', handleEscape)
  revoke()
})
</script>
