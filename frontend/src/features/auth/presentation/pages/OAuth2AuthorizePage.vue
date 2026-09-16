<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <span class="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-teal-50 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300">
          <Icon name="shield" size="lg" />
        </span>
        <h2 class="mt-4 text-xl font-semibold text-gray-900 dark:text-white">
          {{ t('oauth2Consent.title') }}
        </h2>
      </div>

      <div v-if="loading" class="flex min-h-36 items-center justify-center gap-2 text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('common.loading') }}
      </div>

      <div v-else-if="errorMessage" role="alert" class="space-y-4 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
        <p>{{ errorMessage }}</p>
        <button type="button" class="btn btn-secondary w-full" @click="loadPreview">
          {{ t('common.retry') }}
        </button>
      </div>

      <template v-else-if="preview">
        <div class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-center dark:border-dark-700 dark:bg-dark-800">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('oauth2Consent.requestedBy') }}</p>
          <p class="mt-1 break-words text-base font-semibold text-gray-900 dark:text-white">{{ preview.client_name }}</p>
          <p class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">
            {{ t('oauth2Consent.redirectTo') }}: {{ redirectOrigin }}
          </p>
        </div>

        <div>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('oauth2Consent.permissions') }}</h3>
          <ul class="mt-3 space-y-2">
            <li v-for="scope in preview.scopes" :key="scope.name" class="flex items-start gap-3 rounded-lg border border-gray-200 px-3 py-3 dark:border-dark-700">
              <span class="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
                <Icon name="check" size="xs" />
              </span>
              <span class="min-w-0">
                <strong class="block text-sm text-gray-800 dark:text-gray-100">{{ scopeLabel(scope.name) }}</strong>
                <span class="mt-0.5 block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ scopeDescription(scope.name, scope.description) }}</span>
              </span>
            </li>
          </ul>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <button type="button" class="btn btn-secondary" :disabled="submitting" @click="submit(false)">
            {{ t('oauth2Consent.deny') }}
          </button>
          <button type="button" class="btn btn-primary inline-flex items-center justify-center gap-2" :disabled="submitting" @click="submit(true)">
            <Icon v-if="submitting" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="check" size="sm" />
            {{ t('oauth2Consent.allow') }}
          </button>
        </div>
      </template>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AuthLayout from '@/common/widgets/layout/AuthLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { getOAuth2AuthorizationPreview, submitOAuth2Authorization, type OAuth2AuthorizationParameters, type OAuth2AuthorizationPreview } from '@/features/auth/data/datasources/oauth2ProviderDatasource'

const route = useRoute()
const { t } = useI18n()
const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const preview = ref<OAuth2AuthorizationPreview | null>(null)
const redirectOrigin = computed(() => {
  if (!preview.value?.redirect_uri) return ''
  try {
    return new URL(preview.value.redirect_uri).origin
  } catch {
    return preview.value.redirect_uri
  }
})

function queryString(name: string): string {
  const value = route.query[name]
  return typeof value === 'string' ? value : ''
}

function parameters(): OAuth2AuthorizationParameters {
  return {
    client_id: queryString('client_id'),
    redirect_uri: queryString('redirect_uri'),
    response_type: queryString('response_type'),
    scope: queryString('scope'),
    state: queryString('state'),
    code_challenge: queryString('code_challenge'),
    code_challenge_method: queryString('code_challenge_method'),
  }
}

async function loadPreview() {
  loading.value = true
  errorMessage.value = ''
  preview.value = null
  try {
    preview.value = await getOAuth2AuthorizationPreview(parameters())
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('oauth2Consent.invalidRequest'))
  } finally {
    loading.value = false
  }
}

async function submit(approved: boolean) {
  submitting.value = true
  errorMessage.value = ''
  try {
    const result = await submitOAuth2Authorization(parameters(), approved)
    const target = new URL(result.redirect_url)
    if (target.protocol !== 'https:' && target.protocol !== 'http:') throw new Error(t('oauth2Consent.invalidRedirect'))
    window.location.assign(target.href)
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('oauth2Consent.submitFailed'))
  } finally {
    submitting.value = false
  }
}

function scopeLabel(name: string): string {
  const key = `oauth2Consent.scopes.${name}.label`
  const translated = t(key)
  return translated === key ? name : translated
}

function scopeDescription(name: string, fallback: string): string {
  const key = `oauth2Consent.scopes.${name}.description`
  const translated = t(key)
  return translated === key ? fallback : translated
}

onMounted(loadPreview)
</script>
