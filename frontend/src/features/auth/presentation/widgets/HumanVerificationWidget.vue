<template>
  <TurnstileWidget
    v-if="provider === 'turnstile'"
    ref="turnstileRef"
    :site-key="siteKey || ''"
    @verify="emit('verify', $event)"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <AliyunCaptchaWidget
    v-else-if="provider === 'aliyun'"
    ref="aliyunRef"
    :scene-id="aliyunSceneId || ''"
    :prefix="aliyunPrefix || ''"
    :region="aliyunRegion"
    @verify="emit('verify', $event)"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <div v-else-if="provider !== 'tencent'" class="human-verification-wrapper">
    <div ref="containerRef" class="human-verification-container"></div>
  </div>
  <div
    v-else-if="isTencentInternational"
    ref="tencentInternationalContainerRef"
    data-testid="tencent-captcha-international-container"
    :class="
      tencentInternationalContainerVisible
        ? 'flex min-h-[60px] w-full justify-center'
        : 'pointer-events-none fixed left-1/2 top-0 flex h-[60px] w-[302px] -translate-x-1/2 scale-[0.01] opacity-0'
    "
  />
  <span v-else class="hidden" aria-hidden="true"></span>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import TurnstileWidget from '@/features/auth/presentation/widgets/TurnstileWidget.vue'
import AliyunCaptchaWidget from '@/features/auth/presentation/widgets/AliyunCaptchaWidget.vue'
import type { ActionCaptchaRequestProof } from '@/types'
import {
  loadTencentCaptcha,
  normalizeTencentCaptchaRegion,
  type AliyunCaptchaRegion,
  type ExternalHumanVerificationProvider,
  type TencentCaptchaInstance,
  type TencentCaptchaProof,
  type TencentCaptchaRegion,
  type TencentCaptchaResult
} from '@/core/services/humanVerification'

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
  tencentRegion?: TencentCaptchaRegion
  aliyunSceneId?: string
  aliyunPrefix?: string
  aliyunRegion?: AliyunCaptchaRegion
}>()

const emit = defineEmits<{
  verify: [token: string]
  expire: []
  error: []
}>()

const { locale } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const aliyunRef = ref<InstanceType<typeof AliyunCaptchaWidget> | null>(null)
const recaptchaWidgetId = ref<number | null>(null)
const isTencentInternational = computed(
  () => props.provider === 'tencent' && normalizeTencentCaptchaRegion(props.tencentRegion) === 'intl'
)
const tencentInternationalContainerRef = ref<HTMLDivElement | null>(null)
const tencentInternationalContainerVisible = ref(isTencentInternational.value)
let tencentInstance: TencentCaptchaInstance | null = null
let tencentPending: Promise<TencentCaptchaProof | null> | null = null
let cancelTencentPending: (() => void) | null = null
let tencentCachedProof: TencentCaptchaProof | null = null
let tencentPreloading = false
let isMounted = false

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

async function renderCap(): Promise<void> {
  if (!props.apiEndpoint || !containerRef.value) return
  await import('cap-widget')
  if (!containerRef.value || props.provider !== 'cap') return
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
    if (props.provider === 'cap') await renderCap()
  } catch (error) {
    console.error('Failed to initialize human verification:', error)
    emit('error')
  }
}

function cancelTencent(): void {
  tencentInstance?.destroy()
  tencentInstance = null
  cancelTencentPending?.()
  cancelTencentPending = null
  tencentPending = null
  tencentCachedProof = null
  tencentPreloading = false
  tencentInternationalContainerVisible.value = isTencentInternational.value
}

function createTencentVerification(
  revealInternational: boolean = true
): Promise<TencentCaptchaProof | null> {
  return new Promise((resolve, reject) => {
    let settled = false

    const finish = (
      callback: () => void,
      keepInternationalContainer: boolean = false
    ): void => {
      if (settled) return
      settled = true
      if (cancelTencentPending === cancel) cancelTencentPending = null
      if (!keepInternationalContainer) {
        tencentInstance?.destroy()
        tencentInstance = null
        tencentInternationalContainerVisible.value = false
      }
      callback()
    }
    const cancel = (): void => finish(() => resolve(null))
    cancelTencentPending = cancel

    const region = normalizeTencentCaptchaRegion(props.tencentRegion)
    if (region === 'intl' && revealInternational) {
      tencentInternationalContainerVisible.value = true
    }

    void nextTick()
      .then(() => loadTencentCaptcha(region))
      .then((TencentCaptcha) => {
        if (cancelTencentPending !== cancel || props.provider !== 'tencent') return
        const userLanguage = locale.value.toLowerCase().startsWith('zh') ? 'zh-cn' : 'en'
        const handleResult = (result: TencentCaptchaResult): void => {
          if (result.ret === 2) {
            finish(() => resolve(null))
            return
          }
          const ticket = result.ticket?.trim() || ''
          const randstr = result.randstr?.trim() || ''
          if (!ticket || !randstr || ticket.startsWith('trerror_') || result.errorCode !== undefined) {
            finish(() => reject(new Error('Tencent Captcha verification failed')))
            return
          }
          finish(() => resolve({ ticket, randstr }), region === 'intl')
        }
        if (region === 'intl') {
          const container = tencentInternationalContainerRef.value
          if (!container) {
            throw new Error('Tencent Captcha international container is unavailable')
          }
          tencentInstance = new TencentCaptcha(container, props.siteKey || '', handleResult, {
            enableAutoCheck: false,
            userLanguage,
            type: 'popup'
          })
        } else {
          tencentInstance = new TencentCaptcha(props.siteKey || '', handleResult, { userLanguage })
        }
        tencentInstance.show()
      })
      .catch((error: unknown) => finish(() => reject(error)))
  })
}

function verifyTencent(): Promise<TencentCaptchaProof | null> {
  if (props.provider !== 'tencent' || !props.siteKey) return Promise.resolve(null)
  if (tencentCachedProof) {
    const proof = tencentCachedProof
    tencentCachedProof = null
    return Promise.resolve(proof)
  }
  if (tencentPending) {
    const verification = tencentPending
    tencentInternationalContainerVisible.value = true
    void nextTick().then(() => {
      if (tencentPending === verification) tencentInstance?.show()
    })
    if (!tencentPreloading) return verification
    return verification.then((proof) => {
      if (tencentCachedProof === proof) tencentCachedProof = null
      return proof
    })
  }

  const verification = createTencentVerification(true)
  tencentPending = verification
  tencentPreloading = false
  const clearPending = (): void => {
    if (tencentPending === verification) tencentPending = null
  }
  void verification.then(clearPending, clearPending)
  return verification
}

function preloadTencent(): void {
  if (!isTencentInternational.value || !props.siteKey || tencentPending || tencentCachedProof) return

  const verification = createTencentVerification(true)
  tencentPending = verification
  tencentPreloading = true
  void verification
    .then((proof) => {
      if (proof) tencentCachedProof = proof
    })
    .catch(() => undefined)
    .finally(() => {
      if (tencentPending === verification) {
        tencentPending = null
        tencentPreloading = false
      }
    })
}

function verifyAliyun(): Promise<string | null> {
  if (props.provider !== 'aliyun' || !props.aliyunSceneId || !props.aliyunPrefix) {
    return Promise.resolve(null)
  }
  return aliyunRef.value?.verifyAliyun() || Promise.resolve(null)
}

async function verifyAction(): Promise<ActionCaptchaRequestProof | null> {
  if (props.provider === 'tencent') {
    const proof = await verifyTencent()
    return proof
      ? {
          tencent_captcha_ticket: proof.ticket,
          tencent_captcha_randstr: proof.randstr
        }
      : null
  }
  if (props.provider === 'aliyun') {
    const token = await verifyAliyun()
    return token ? { captcha_token: token } : null
  }
  return null
}

function reset(): void {
  if (props.provider === 'turnstile') {
    turnstileRef.value?.reset()
  } else if (props.provider === 'recaptcha' && window.grecaptcha && recaptchaWidgetId.value !== null) {
    window.grecaptcha.reset(recaptchaWidgetId.value)
  } else if (props.provider === 'cap') {
    void renderCap()
  } else if (props.provider === 'tencent') {
    cancelTencent()
    if (isMounted && isTencentInternational.value) {
      void nextTick().then(() => {
        if (isMounted) preloadTencent()
      })
    }
  } else if (props.provider === 'aliyun') {
    aliyunRef.value?.reset()
  }
}

watch(
  () => [
    props.provider,
    props.siteKey,
    props.apiEndpoint,
    props.tencentRegion,
    props.aliyunSceneId,
    props.aliyunPrefix,
    props.aliyunRegion
  ] as const,
  (current, previous) => {
    if (
      previous?.[0] === 'tencent' &&
      (current[0] !== 'tencent' || current[1] !== previous[1] || current[3] !== previous[3])
    ) {
      cancelTencent()
    }
    void render()
    if (current[0] === 'tencent' && normalizeTencentCaptchaRegion(current[3]) === 'intl') {
      void nextTick().then(preloadTencent)
    }
  }
)
onMounted(() => {
  isMounted = true
  void render()
  preloadTencent()
})
onUnmounted(() => {
  isMounted = false
  clearContainer()
  cancelTencent()
})

defineExpose({ reset, verifyTencent, verifyAliyun, verifyAction })
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
