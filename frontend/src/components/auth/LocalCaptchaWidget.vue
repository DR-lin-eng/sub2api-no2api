<template>
  <div>
    <label :for="inputId" class="input-label">
      {{ t('auth.localCaptchaLabel') }}
    </label>
    <div class="flex items-stretch gap-2">
      <input
        :id="inputId"
        :value="captchaCode"
        type="text"
        required
        maxlength="8"
        autocomplete="off"
        autocapitalize="characters"
        spellcheck="false"
        :disabled="disabled || loading"
        class="input min-w-0 flex-1 font-mono uppercase"
        :placeholder="t('auth.localCaptchaPlaceholder')"
        @input="handleInput"
      />
      <button
        type="button"
        class="relative h-11 w-36 flex-none overflow-hidden rounded-md border border-gray-200 bg-gray-50 transition-colors hover:border-primary-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800"
        :disabled="disabled || loading"
        :title="t('auth.localCaptchaRefresh')"
        :aria-label="t('auth.localCaptchaRefresh')"
        @click="reload"
      >
        <img
          v-if="imageData"
          :src="imageData"
          :alt="t('auth.localCaptchaAlt')"
          class="h-full w-full object-contain"
          draggable="false"
        />
        <Icon
          v-else
          name="refresh"
          size="md"
          class="mx-auto text-gray-400"
          :class="{ 'animate-spin': loading }"
        />
      </button>
    </div>
    <p class="mt-1.5 text-xs" :class="errorMessage ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'">
      {{ errorMessage || t('auth.localCaptchaHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalCaptcha } from '@/api/auth'
import Icon from '@/components/icons/Icon.vue'

withDefaults(defineProps<{
  captchaId: string
  captchaCode: string
  disabled?: boolean
  inputId?: string
}>(), {
  disabled: false,
  inputId: 'local-captcha'
})

const emit = defineEmits<{
  'update:captchaId': [value: string]
  'update:captchaCode': [value: string]
}>()

const { t } = useI18n()
const imageData = ref('')
const loading = ref(false)
const errorMessage = ref('')

function handleInput(event: Event): void {
  const target = event.target as HTMLInputElement
  emit('update:captchaCode', target.value.toUpperCase().replace(/\s+/g, ''))
}

async function reload(): Promise<void> {
  if (loading.value) return

  loading.value = true
  errorMessage.value = ''
  imageData.value = ''
  emit('update:captchaId', '')
  emit('update:captchaCode', '')
  try {
    const challenge = await getLocalCaptcha()
    imageData.value = challenge.image_data
    emit('update:captchaId', challenge.captcha_id)
  } catch {
    errorMessage.value = t('auth.localCaptchaLoadFailed')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void reload()
})

defineExpose({ reset: reload })
</script>
