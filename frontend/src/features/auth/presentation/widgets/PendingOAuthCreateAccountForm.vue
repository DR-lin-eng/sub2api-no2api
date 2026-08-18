<template>
  <form class="space-y-3" @submit.prevent="handleSubmit">
    <input
      v-model="email"
      :data-testid="`${testIdPrefix}-create-account-email`"
      type="email"
      class="input w-full"
      :placeholder="t('auth.emailPlaceholder')"
      :disabled="isSubmitting || isSendingCode"
    />
    <input
      v-model="password"
      :data-testid="`${testIdPrefix}-create-account-password`"
      type="password"
      class="input w-full"
      :placeholder="t('auth.passwordPlaceholder')"
      :disabled="isSubmitting"
    />
    <div v-if="externalHumanVerificationReady" class="space-y-2">
      <HumanVerificationWidget
        ref="turnstileRef"
        :provider="humanVerificationProvider"
        :site-key="turnstileSiteKey"
        :api-endpoint="humanVerificationAPIEndpoint"
        :tencent-region="tencentCaptchaRegion"
        :aliyun-scene-id="aliyunCaptchaSceneId"
        :aliyun-prefix="aliyunCaptchaPrefix"
        :aliyun-region="aliyunCaptchaRegion"
        @verify="onTurnstileVerify"
        @expire="onTurnstileExpire"
        @error="onTurnstileError"
      />
    </div>
    <LocalCaptchaWidget
      v-else-if="localCaptchaEnabled"
      ref="localCaptchaRef"
      v-model:captcha-id="localCaptchaId"
      v-model:captcha-code="localCaptchaCode"
      :disabled="isSubmitting || isSendingCode"
      :input-id="`${testIdPrefix}-local-captcha`"
    />
    <div v-if="emailVerifyEnabled" class="flex gap-3">
      <input
        v-model="verifyCode"
        :data-testid="`${testIdPrefix}-create-account-verify-code`"
        type="text"
        inputmode="numeric"
        maxlength="6"
        class="input min-w-0 flex-1"
        placeholder="123456"
        :disabled="isSubmitting"
      />
      <button
        :data-testid="`${testIdPrefix}-create-account-send-code`"
        type="button"
        class="btn btn-secondary shrink-0"
        :disabled="isSubmitting || isSendingCode || countdown > 0 || !email.trim() || (inlineHumanVerificationRequired && !turnstileToken) || (localCaptchaEnabled && (!localCaptchaId || !localCaptchaCode))"
        @click="handleSendCode"
      >
        {{
          isSendingCode
            ? t('auth.sendingCode')
            : countdown > 0
              ? t('auth.resendCountdown', { countdown })
              : t('auth.sendCode')
        }}
      </button>
    </div>
    <p v-if="emailVerifyEnabled && sendCodeSuccess" class="text-sm text-green-600 dark:text-green-400">
      {{ t('auth.codeSentSuccess') }}
    </p>
    <p v-else-if="emailVerifyEnabled" class="text-xs text-gray-500 dark:text-dark-400">
      {{ t('auth.verificationCodeHint') }}
    </p>
    <input
      v-if="invitationCodeEnabled"
      v-model="invitationCode"
      :data-testid="`${testIdPrefix}-create-account-invitation-code`"
      type="text"
      class="input w-full"
      :placeholder="t('auth.invitationCodePlaceholder')"
      :disabled="isSubmitting"
    />
    <button
      :data-testid="`${testIdPrefix}-create-account-submit`"
      type="button"
      class="btn btn-primary w-full"
      :disabled="isSubmitting || !email.trim() || password.length < 6 || (invitationCodeEnabled && !invitationCode.trim())"
      @click="handleSubmit"
    >
      {{ isSubmitting ? t('common.processing') : t('auth.createAccount') }}
    </button>
    <button
      type="button"
      class="btn btn-secondary w-full"
      :disabled="isSubmitting"
      @click="emitSwitchToBind"
    >
      {{ t('auth.alreadyHaveAccount') }}
    </button>
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import HumanVerificationWidget from '@/features/auth/presentation/widgets/HumanVerificationWidget.vue'
import LocalCaptchaWidget from '@/features/auth/presentation/widgets/LocalCaptchaWidget.vue'
import { getPublicSettings } from '@/features/auth/data/datasources/authQueries'
import { sendPendingOAuthVerifyCode } from '@/features/auth/data/datasources/authOAuthActions'
import { useAppStore } from '@/core/stores/appStore'
import {
  resolveHumanVerification,
  type AliyunCaptchaRegion,
  type ExternalHumanVerificationProvider,
  type TencentCaptchaRegion,
  type TencentCaptchaProof
} from '@/core/services/humanVerification'

export type PendingOAuthCreateAccountPayload = {
  email: string
  password: string
  verifyCode: string
  invitationCode?: string
  captchaToken?: string
  tencentCaptchaTicket?: string
  tencentCaptchaRandstr?: string
  captchaId?: string
  captchaCode?: string
}

const props = defineProps<{
  initialEmail: string
  testIdPrefix: string
  isSubmitting: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  submit: [payload: PendingOAuthCreateAccountPayload]
  switchToBind: [email: string]
}>()

const { t } = useI18n()
const appStore = useAppStore()

const email = ref('')
const password = ref('')
const verifyCode = ref('')
const invitationCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const invitationCodeEnabled = ref(false)
const emailVerifyEnabled = ref(true)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const humanVerificationProvider = ref<ExternalHumanVerificationProvider>('turnstile')
const humanVerificationAPIEndpoint = ref('')
const tencentCaptchaRegion = ref<TencentCaptchaRegion>('cn')
const aliyunCaptchaSceneId = ref('')
const aliyunCaptchaPrefix = ref('')
const aliyunCaptchaRegion = ref<AliyunCaptchaRegion>('cn')
const turnstileToken = ref('')
const turnstileRef = ref<InstanceType<typeof HumanVerificationWidget> | null>(null)
const localCaptchaEnabled = ref(false)
const localCaptchaId = ref('')
const localCaptchaCode = ref('')
const localCaptchaRef = ref<InstanceType<typeof LocalCaptchaWidget> | null>(null)
const tencentCaptchaEnabled = computed(
  () => turnstileEnabled.value && humanVerificationProvider.value === 'tencent'
)
const inlineHumanVerificationRequired = computed(
  () => turnstileEnabled.value && humanVerificationProvider.value !== 'tencent'
)
const externalHumanVerificationReady = computed(() => {
  if (!turnstileEnabled.value) return false
  if (humanVerificationProvider.value === 'cap') return Boolean(humanVerificationAPIEndpoint.value)
  if (humanVerificationProvider.value === 'aliyun') {
    return Boolean(aliyunCaptchaSceneId.value && aliyunCaptchaPrefix.value)
  }
  return Boolean(turnstileSiteKey.value)
})

let countdownTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => props.initialEmail,
  value => {
    email.value = value || ''
  },
  { immediate: true }
)

watch(sendCodeError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(
  () => props.errorMessage,
  value => {
    if (value) {
      appStore.showError(value)
    }
  }
)

function clearCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function startCountdown(seconds: number) {
  clearCountdown()
  countdown.value = Math.max(0, seconds)

  if (countdown.value <= 0) {
    return
  }

  countdownTimer = setInterval(() => {
    if (countdown.value <= 1) {
      countdown.value = 0
      clearCountdown()
      return
    }

    countdown.value -= 1
  }, 1000)
}

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string } } }
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
}

function resetTurnstile() {
  turnstileToken.value = ''
  turnstileRef.value?.reset()
}

function onTurnstileVerify(token: string) {
  turnstileToken.value = token
  sendCodeError.value = ''
}

function onTurnstileExpire() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileExpired')
}

function onTurnstileError() {
  turnstileToken.value = ''
  sendCodeError.value = t('auth.turnstileFailed')
}

async function verifyTencentForAction(): Promise<TencentCaptchaProof | null> {
  try {
    return (await turnstileRef.value?.verifyTencent()) || null
  } catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.turnstileFailed'))
    return null
  }
}

async function handleSendCode() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail) {
    return
  }

  if (inlineHumanVerificationRequired.value && !turnstileToken.value) {
    sendCodeError.value = t('auth.completeVerification')
    return
  }

  isSendingCode.value = true
  sendCodeError.value = ''
  sendCodeSuccess.value = false

  try {
    const tencentProof = tencentCaptchaEnabled.value ? await verifyTencentForAction() : null
    if (tencentCaptchaEnabled.value && !tencentProof) {
      return
    }
    const response = await sendPendingOAuthVerifyCode({
      email: trimmedEmail,
      ...(inlineHumanVerificationRequired.value ? { captcha_token: turnstileToken.value } : {}),
      ...(tencentProof
        ? {
            tencent_captcha_ticket: tencentProof.ticket,
            tencent_captcha_randstr: tencentProof.randstr
          }
        : {}),
      ...(localCaptchaEnabled.value
        ? { captcha_id: localCaptchaId.value, captcha_code: localCaptchaCode.value }
        : {})
    })
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
  } catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  } finally {
    if (turnstileEnabled.value) {
      resetTurnstile()
    }
    if (localCaptchaEnabled.value) {
      await localCaptchaRef.value?.reset()
    }
    isSendingCode.value = false
  }
}

async function handleSubmit() {
  const trimmedEmail = email.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  }

  let tencentProof: TencentCaptchaProof | null = null
  if (!emailVerifyEnabled.value) {
    if (inlineHumanVerificationRequired.value && !turnstileToken.value) {
      sendCodeError.value = t('auth.completeVerification')
      return
    }
    if (localCaptchaEnabled.value && (!localCaptchaId.value || !localCaptchaCode.value)) {
      sendCodeError.value = t('auth.completeVerification')
      return
    }
    if (tencentCaptchaEnabled.value) {
      tencentProof = await verifyTencentForAction()
      if (!tencentProof) {
        return
      }
    }
  }

  emit('submit', {
    email: trimmedEmail,
    password: password.value,
    verifyCode: emailVerifyEnabled.value ? verifyCode.value.trim() : '',
    invitationCode: invitationCode.value.trim() || undefined,
    ...(!emailVerifyEnabled.value && inlineHumanVerificationRequired.value
      ? { captchaToken: turnstileToken.value }
      : {}),
    ...(tencentProof
      ? {
          tencentCaptchaTicket: tencentProof.ticket,
          tencentCaptchaRandstr: tencentProof.randstr
        }
      : {}),
    ...(!emailVerifyEnabled.value && localCaptchaEnabled.value
      ? { captchaId: localCaptchaId.value, captchaCode: localCaptchaCode.value }
      : {})
  })

  if (!emailVerifyEnabled.value && turnstileEnabled.value) {
    resetTurnstile()
  }
  if (!emailVerifyEnabled.value && localCaptchaEnabled.value) {
    void localCaptchaRef.value?.reset()
  }
}

function emitSwitchToBind() {
  emit('switchToBind', email.value.trim())
}

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    invitationCodeEnabled.value = settings.invitation_code_enabled === true
    emailVerifyEnabled.value = settings.email_verify_enabled !== false
    const verification = resolveHumanVerification(settings)
    turnstileEnabled.value = verification.external
    turnstileSiteKey.value = verification.siteKey
    humanVerificationAPIEndpoint.value = verification.apiEndpoint
    humanVerificationProvider.value = verification.externalProvider
    tencentCaptchaRegion.value = verification.tencentRegion
    aliyunCaptchaSceneId.value = verification.aliyunSceneId
    aliyunCaptchaPrefix.value = verification.aliyunPrefix
    aliyunCaptchaRegion.value = verification.aliyunRegion
    localCaptchaEnabled.value = verification.provider === 'local'
  } catch {
    invitationCodeEnabled.value = false
    emailVerifyEnabled.value = true
    turnstileEnabled.value = false
    turnstileSiteKey.value = ''
    humanVerificationAPIEndpoint.value = ''
    tencentCaptchaRegion.value = 'cn'
    aliyunCaptchaSceneId.value = ''
    aliyunCaptchaPrefix.value = ''
    aliyunCaptchaRegion.value = 'cn'
    localCaptchaEnabled.value = false
  }
})

onUnmounted(() => {
  clearCountdown()
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
