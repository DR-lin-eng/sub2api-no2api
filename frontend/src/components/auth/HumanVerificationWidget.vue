<template>
  <TurnstileWidget
    v-if="provider === 'turnstile'"
    ref="turnstileRef"
    :site-key="siteKey || ''"
    @verify="emit('verify', $event)"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <div v-else class="human-verification-wrapper">
    <div ref="containerRef" class="human-verification-container"></div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import 'cap-widget'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import type { ExternalHumanVerificationProvider } from '@/utils/humanVerification'

interface RecaptchaAPI {
  render: (container: HTMLElement, options: {
    sitekey: string
    callback: (token: string) => void
    'expired-callback': () => void
    'error-callback': () => void
  }) => number
  reset: (widgetId?: number) => void
}

declare global {
  interface Window {
    grecaptcha?: RecaptchaAPI
    __onRecaptchaLoad?: () => void
    CAP_SCRIPT_NONCE?: string
    CAP_CSS_NONCE?: string
  }
}

const props = defineProps<{
  provider: ExternalHumanVerificationProvider
  siteKey?: string
  apiEndpoint?: string
}>()

const emit = defineEmits<{
  verify: [token: string]
  expire: []
  error: []
}>()

const containerRef = ref<HTMLElement | null>(null)
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const recaptchaWidgetId = ref<number | null>(null)

let recaptchaLoadPromise: Promise<void> | null = null

function loadRecaptcha(): Promise<void> {
  if (window.grecaptcha) return Promise.resolve()
  if (recaptchaLoadPromise) return recaptchaLoadPromise

  recaptchaLoadPromise = new Promise((resolve, reject) => {
    window.__onRecaptchaLoad = resolve
    const script = document.createElement('script')
    script.src = 'https://www.google.com/recaptcha/api.js?onload=__onRecaptchaLoad&render=explicit'
    script.async = true
    script.defer = true
    script.onerror = () => {
      recaptchaLoadPromise = null
      reject(new Error('Failed to load reCAPTCHA script'))
    }
    document.head.appendChild(script)
  })
  return recaptchaLoadPromise
}

function clearContainer(): void {
  recaptchaWidgetId.value = null
  if (containerRef.value) containerRef.value.replaceChildren()
}

async function renderRecaptcha(): Promise<void> {
  if (!props.siteKey || !containerRef.value) return
  await loadRecaptcha()
  if (!window.grecaptcha || !containerRef.value || props.provider !== 'recaptcha') return
  clearContainer()
  recaptchaWidgetId.value = window.grecaptcha.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: token => emit('verify', token),
    'expired-callback': () => emit('expire'),
    'error-callback': () => emit('error')
  })
}

function renderCap(): void {
  if (!props.apiEndpoint || !containerRef.value) return
  clearContainer()
  const nonce = document.querySelector<HTMLScriptElement>('script[nonce]')?.nonce
  if (nonce) {
    window.CAP_SCRIPT_NONCE = nonce
    window.CAP_CSS_NONCE = nonce
  }
  const widget = document.createElement('cap-widget')
  widget.setAttribute('data-cap-api-endpoint', props.apiEndpoint)
  widget.setAttribute('data-cap-i18n-initial-state', 'Verify you are human')
  widget.addEventListener('solve', event => {
    const token = (event as CustomEvent<{ token?: string }>).detail?.token
    if (token) emit('verify', token)
  })
  widget.addEventListener('reset', () => emit('expire'))
  widget.addEventListener('error', () => emit('error'))
  containerRef.value.appendChild(widget)
}

async function render(): Promise<void> {
  await nextTick()
  try {
    if (props.provider === 'recaptcha') await renderRecaptcha()
    if (props.provider === 'cap') renderCap()
  } catch (error) {
    console.error('Failed to initialize human verification:', error)
    emit('error')
  }
}

function reset(): void {
  if (props.provider === 'turnstile') {
    turnstileRef.value?.reset()
  } else if (props.provider === 'recaptcha' && window.grecaptcha && recaptchaWidgetId.value !== null) {
    window.grecaptcha.reset(recaptchaWidgetId.value)
  } else if (props.provider === 'cap') {
    renderCap()
  }
}

watch(() => [props.provider, props.siteKey, props.apiEndpoint], () => void render())
onMounted(() => void render())
onUnmounted(clearContainer)

defineExpose({ reset })
</script>

<style scoped>
.human-verification-wrapper,
.human-verification-container {
  width: 100%;
  min-height: 65px;
}

.human-verification-container :deep(iframe),
.human-verification-container :deep(cap-widget) {
  max-width: 100%;
}
</style>
