<template>
  <section v-if="!unsupported" class="card">
    <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center gap-2">
        <Icon name="shield" size="md" class="text-primary-500" />
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.panelRateLimit.title') }}
        </h2>
      </div>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.panelRateLimit.description') }}
      </p>
    </header>

    <div class="space-y-5 p-6">
      <div v-if="loading" class="flex items-center gap-2 text-gray-500">
        <LoadingSpinner size="sm" />
        {{ t('common.loading') }}
      </div>

      <div v-else-if="!form" class="flex items-center justify-between gap-4">
        <p class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.settings.panelRateLimit.loadFailed') }}
        </p>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadSettings">
          {{ t('common.retry') }}
        </button>
      </div>

      <template v-else>
        <div class="rounded-lg border border-sky-200 bg-sky-50 p-4 dark:border-sky-800 dark:bg-sky-900/20">
          <div class="flex items-start">
            <Icon name="infoCircle" size="md" class="mt-0.5 flex-shrink-0 text-sky-500" />
            <p class="ml-3 text-sm text-sky-700 dark:text-sky-300">
              {{ t('admin.settings.panelRateLimit.proxySafeNote') }}
            </p>
          </div>
        </div>

        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t('admin.settings.panelRateLimit.enabled') }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.panelRateLimit.enabledHint') }}
            </p>
          </div>
          <Toggle v-model="form.enabled" data-testid="panel-rate-limit-enabled" />
        </div>

        <div
          v-if="form.enabled"
          class="space-y-5 border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <RateInput
              v-model="form.user_rpm"
              data-testid="panel-rate-limit-user-rpm"
              :label="t('admin.settings.panelRateLimit.userRpm')"
              :hint="t('admin.settings.panelRateLimit.userRpmHint')"
              :unit="t('admin.settings.panelRateLimit.perMinute')"
              @submit="saveSettings"
            />
            <RateInput
              v-model="form.heavy_rpm"
              data-testid="panel-rate-limit-heavy-rpm"
              :label="t('admin.settings.panelRateLimit.heavyRpm')"
              :hint="t('admin.settings.panelRateLimit.heavyRpmHint')"
              :unit="t('admin.settings.panelRateLimit.perMinute')"
              @submit="saveSettings"
            />
            <RateInput
              v-model="form.public_ip_rpm"
              data-testid="panel-rate-limit-public-ip-rpm"
              :label="t('admin.settings.panelRateLimit.publicIpRpm')"
              :hint="t('admin.settings.panelRateLimit.publicIpRpmHint')"
              :unit="t('admin.settings.panelRateLimit.perMinute')"
              @submit="saveSettings"
            />
          </div>

          <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">
                {{ t('admin.settings.panelRateLimit.exemptAdmin') }}
              </label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.panelRateLimit.exemptAdminHint') }}
              </p>
            </div>
            <Toggle v-model="form.exempt_admin" />
          </div>
        </div>

        <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
          <button
            type="button"
            data-testid="panel-rate-limit-save"
            class="btn btn-primary btn-sm"
            :disabled="saving"
            @click="saveSettings"
          >
            <LoadingSpinner v-if="saving" size="sm" class="mr-1" />
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  normalizePanelRateLimitSettings,
  type PanelRateLimitSettings,
} from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import { getPanelRateLimitSettings } from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import { updatePanelRateLimitSettings } from '@/features/admin-settings/data/datasources/adminSettingsActions'
import Icon from '@/common/widgets/icons/Icon.vue'
import LoadingSpinner from '@/common/widgets/feedback/LoadingSpinner.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import RateInput from '@/features/admin-settings/presentation/widgets/RateInput.vue'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const unsupported = ref(false)
const form = ref<PanelRateLimitSettings | null>(null)

function isNotFoundError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const value = error as {
    status?: unknown
    response?: { status?: unknown }
  }
  return value.status === 404 || value.response?.status === 404
}

async function loadSettings(): Promise<void> {
  loading.value = true
  try {
    const settings = await getPanelRateLimitSettings()
    form.value = normalizePanelRateLimitSettings(settings)
  } catch (error: unknown) {
    form.value = null
    if (isNotFoundError(error)) {
      unsupported.value = true
      return
    }
    appStore.showError(
      extractApiErrorMessage(error, t('admin.settings.panelRateLimit.loadFailed')),
    )
  } finally {
    loading.value = false
  }
}

async function saveSettings(): Promise<void> {
  if (!form.value || saving.value) return
  saving.value = true
  try {
    const updated = await updatePanelRateLimitSettings({ ...form.value })
    form.value = normalizePanelRateLimitSettings(updated)
    appStore.showSuccess(t('admin.settings.panelRateLimit.saved'))
  } catch (error: unknown) {
    if (isNotFoundError(error)) {
      unsupported.value = true
      form.value = null
      return
    }
    appStore.showError(
      extractApiErrorMessage(error, t('admin.settings.panelRateLimit.saveFailed')),
    )
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>
