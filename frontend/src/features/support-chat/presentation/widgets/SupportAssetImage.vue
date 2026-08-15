<template>
  <div ref="root" class="relative overflow-hidden bg-gray-100 dark:bg-dark-900" :class="containerClass">
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
}>(), {
  alt: '',
  containerClass: '',
  lazy: false,
})

const { t } = useI18n()
const src = ref('')
const loading = ref(false)
const root = ref<HTMLElement | null>(null)
let requestSequence = 0
let mounted = false
let observer: IntersectionObserver | null = null

function revoke() {
  if (!src.value) return
  URL.revokeObjectURL(src.value)
  src.value = ''
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
  scheduleLoad()
})

onBeforeUnmount(() => {
  requestSequence += 1
  observer?.disconnect()
  observer = null
  revoke()
})
</script>
