<template>
  <section class="card overflow-hidden">
    <header class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
      <div class="flex min-w-0 items-start gap-3">
        <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-teal-50 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300">
          <Icon name="shield" size="md" />
        </span>
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.oauth2Provider.title') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.oauth2Provider.description') }}
          </p>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm h-9 w-9 p-0"
        :disabled="loading || operating"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="load"
      >
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
      </button>
    </header>

    <div v-if="loading && !config" class="flex min-h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <div v-else class="space-y-6 p-5 sm:p-6">
      <div v-if="loadError" role="alert" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
        <span>{{ loadError }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="load">
          {{ t('common.retry') }}
        </button>
      </div>

      <div v-if="config" class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_12rem_auto] lg:items-end">
        <label class="block min-w-0">
          <span class="input-label">{{ t('admin.settings.oauth2Provider.issuer') }}</span>
          <input
            v-model.trim="configForm.issuer"
            class="input mt-1.5 w-full font-mono text-sm"
            type="url"
            maxlength="512"
            autocomplete="url"
            placeholder="https://api.example.com"
          />
        </label>
        <label class="block">
          <span class="input-label">{{ t('admin.settings.oauth2Provider.tokenLifetime') }}</span>
          <input
            v-model.number="configForm.access_token_ttl_seconds"
            class="input mt-1.5 w-full"
            type="number"
            min="60"
            max="86400"
            step="60"
          />
        </label>
        <div class="flex min-h-10 items-center justify-between gap-3 lg:justify-end">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.oauth2Provider.enabled') }}
          </span>
          <Toggle v-model="configForm.enabled" />
        </div>
      </div>

      <div v-if="config" class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <div class="flex items-center gap-2 text-sm">
          <span class="h-2.5 w-2.5 rounded-full" :class="config.enabled ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600'" />
          <span :class="config.enabled ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-gray-400'">
            {{ config.enabled ? t('common.enabled') : t('common.disabled') }}
          </span>
        </div>
        <button type="button" class="btn btn-primary btn-sm inline-flex items-center gap-2" :disabled="operating" @click="saveConfig">
          <Icon name="check" size="sm" />
          {{ t('common.save') }}
        </button>
      </div>

      <div v-if="oneTimeSecret" class="rounded-lg border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/60 dark:bg-emerald-950/30">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-sm font-semibold text-emerald-800 dark:text-emerald-200">
              {{ t('admin.settings.oauth2Provider.secretOnce') }}
            </p>
            <p class="mt-1 break-all font-mono text-xs text-emerald-700 dark:text-emerald-300">
              {{ oneTimeSecret }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm h-9 w-9 shrink-0 p-0" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copySecret">
            <Icon name="copy" size="sm" />
          </button>
        </div>
      </div>

      <div v-if="config" class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.oauth2Provider.clients') }}
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.oauth2Provider.clientCount', { count: config.clients.length }) }}
            </p>
          </div>
          <button type="button" class="btn btn-primary btn-sm inline-flex items-center gap-2" :disabled="operating" @click="openCreateDialog">
            <Icon name="plus" size="sm" />
            {{ t('admin.settings.oauth2Provider.addClient') }}
          </button>
        </div>

        <div v-if="config.clients.length === 0" class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.settings.oauth2Provider.noClients') }}
        </div>

        <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[760px] text-left text-sm">
            <thead class="bg-gray-50 text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-4 py-3">{{ t('common.name') }}</th>
                <th class="px-4 py-3">{{ t('admin.settings.oauth2Provider.redirects') }}</th>
                <th class="px-4 py-3">{{ t('admin.settings.oauth2Provider.scopes') }}</th>
                <th class="px-4 py-3">{{ t('common.status') }}</th>
                <th class="px-4 py-3 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="client in config.clients" :key="client.client_id" class="bg-white align-top dark:bg-dark-900">
                <td class="px-4 py-3">
                  <p class="font-medium text-gray-900 dark:text-white">{{ client.name }}</p>
                  <code class="mt-1 block max-w-56 break-all text-[11px] text-gray-500 dark:text-gray-400">{{ client.client_id }}</code>
                  <span class="mt-1 inline-flex rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ clientTypeLabel(client.client_type) }}
                  </span>
                </td>
                <td class="max-w-72 px-4 py-3">
                  <code v-for="uri in client.redirect_uris" :key="uri" class="mb-1 block break-all text-xs text-gray-600 last:mb-0 dark:text-gray-300">{{ uri }}</code>
                </td>
                <td class="px-4 py-3">
                  <div class="flex max-w-48 flex-wrap gap-1">
                    <span v-for="scope in client.allowed_scopes" :key="scope" class="rounded bg-teal-50 px-1.5 py-0.5 text-[11px] text-teal-700 dark:bg-teal-900/30 dark:text-teal-300">{{ scope }}</span>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span :class="client.enabled ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-500 dark:text-gray-400'">
                    {{ client.enabled ? t('common.enabled') : t('common.disabled') }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <button type="button" class="btn btn-secondary btn-sm h-8 w-8 p-0" :disabled="operating" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openEditDialog(client)">
                      <Icon name="edit" size="sm" />
                    </button>
                    <button v-if="client.client_type === 'confidential'" type="button" class="btn btn-secondary btn-sm h-8 w-8 p-0" :disabled="operating" :title="t('admin.settings.oauth2Provider.rotateSecret')" :aria-label="t('admin.settings.oauth2Provider.rotateSecret')" @click="rotateSecret(client)">
                      <Icon name="refresh" size="sm" />
                    </button>
                    <button type="button" class="btn btn-secondary btn-sm h-8 w-8 p-0 text-red-600 dark:text-red-400" :disabled="operating" :title="t('common.delete')" :aria-label="t('common.delete')" @click="removeClient(client)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <BaseDialog :show="clientDialogOpen" :title="editingClientId ? t('admin.settings.oauth2Provider.editClient') : t('admin.settings.oauth2Provider.addClient')" width="wide" @close="closeClientDialog">
      <form class="space-y-5" @submit.prevent="saveClient">
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block">
            <span class="input-label">{{ t('common.name') }}</span>
            <input v-model.trim="clientForm.name" class="input mt-1.5 w-full" type="text" maxlength="100" required />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.settings.oauth2Provider.clientType') }}</span>
            <select v-model="clientForm.client_type" class="input mt-1.5 w-full" :disabled="Boolean(editingClientId)">
              <option value="confidential">{{ t('admin.settings.oauth2Provider.types.confidential') }}</option>
              <option value="public">{{ t('admin.settings.oauth2Provider.types.public') }}</option>
            </select>
          </label>
        </div>

        <label class="block">
          <span class="input-label">{{ t('admin.settings.oauth2Provider.redirects') }}</span>
          <textarea v-model="clientForm.redirect_uris" class="input mt-1.5 min-h-28 w-full resize-y font-mono text-sm" required placeholder="https://app.example.com/oauth/callback"></textarea>
        </label>

        <fieldset>
          <legend class="input-label">{{ t('admin.settings.oauth2Provider.scopes') }}</legend>
          <div class="mt-2 grid gap-2 sm:grid-cols-2">
            <label v-for="scope in config?.scopes || []" :key="scope.name" class="flex items-start gap-2 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
              <input v-model="clientForm.allowed_scopes" class="mt-0.5" type="checkbox" :value="scope.name" />
              <span class="min-w-0">
                <strong class="block text-sm text-gray-800 dark:text-gray-100">{{ scope.name }}</strong>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ translatedScopeDescription(scope.name, scope.description) }}</span>
              </span>
            </label>
          </div>
        </fieldset>

        <div class="flex items-center justify-between rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-800">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.oauth2Provider.clientEnabled') }}</span>
          <Toggle v-model="clientForm.enabled" />
        </div>

        <div class="flex justify-end gap-2 border-t border-gray-100 pt-4 dark:border-dark-700">
          <button type="button" class="btn btn-secondary" :disabled="operating" @click="closeClientDialog">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary inline-flex items-center gap-2" :disabled="operating || !canSaveClient">
            <Icon name="check" size="sm" />
            {{ t('common.save') }}
          </button>
        </div>
      </form>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/common/composables/useStepUp'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { createOAuth2ProviderClient, deleteOAuth2ProviderClient, rotateOAuth2ProviderClientSecret, updateOAuth2ProviderClient, updateOAuth2ProviderConfig } from '@/features/admin-settings/data/datasources/oauth2ProviderActions'
import { getOAuth2ProviderConfig } from '@/features/admin-settings/data/datasources/oauth2ProviderQueries'
import type { OAuth2ClientType, OAuth2ProviderClient, OAuth2ProviderConfig } from '@/features/admin-settings/data/dtos/oauth2ProviderDtos'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { t } = useI18n()
const appStore = useAppStore()
const { settingsStepUp } = useSettingsPageContext()
const config = ref<OAuth2ProviderConfig | null>(null)
const loading = ref(false)
const operating = ref(false)
const loadError = ref('')
const oneTimeSecret = ref('')
const clientDialogOpen = ref(false)
const editingClientId = ref('')

const configForm = reactive({
  enabled: false,
  issuer: '',
  access_token_ttl_seconds: 3600,
})

const clientForm = reactive({
  name: '',
  client_type: 'confidential' as OAuth2ClientType,
  redirect_uris: '',
  allowed_scopes: ['profile'] as string[],
  enabled: true,
})

const parsedRedirectURIs = computed(() => clientForm.redirect_uris.split(/\r?\n/).map((value) => value.trim()).filter(Boolean))
const canSaveClient = computed(() => clientForm.name.length > 0 && parsedRedirectURIs.value.length > 0 && clientForm.allowed_scopes.length > 0)

function applyConfig(value: OAuth2ProviderConfig) {
  config.value = value
  configForm.enabled = value.enabled
  configForm.issuer = value.issuer || window.location.origin
  configForm.access_token_ttl_seconds = value.access_token_ttl_seconds || 3600
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    applyConfig(await getOAuth2ProviderConfig())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.settings.oauth2Provider.loadFailed'))
  } finally {
    loading.value = false
  }
}

function handleOperationError(error: unknown, fallback: string) {
  if (isStepUpCancelled(error)) return
  if (isStepUpBlocked(error)) {
    appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
    return
  }
  appStore.showError(extractApiErrorMessage(error, fallback))
}

async function saveConfig() {
  operating.value = true
  try {
    const updated = await settingsStepUp.run(() => updateOAuth2ProviderConfig({
      enabled: configForm.enabled,
      issuer: configForm.issuer,
      access_token_ttl_seconds: Number(configForm.access_token_ttl_seconds),
    }))
    applyConfig(updated)
    appStore.showSuccess(t('admin.settings.oauth2Provider.saved'))
  } catch (error) {
    handleOperationError(error, t('admin.settings.oauth2Provider.saveFailed'))
  } finally {
    operating.value = false
  }
}

function resetClientForm() {
  clientForm.name = ''
  clientForm.client_type = 'confidential'
  clientForm.redirect_uris = ''
  clientForm.allowed_scopes = ['profile']
  clientForm.enabled = true
}

function openCreateDialog() {
  editingClientId.value = ''
  resetClientForm()
  clientDialogOpen.value = true
}

function openEditDialog(client: OAuth2ProviderClient) {
  editingClientId.value = client.client_id
  clientForm.name = client.name
  clientForm.client_type = client.client_type
  clientForm.redirect_uris = client.redirect_uris.join('\n')
  clientForm.allowed_scopes = [...client.allowed_scopes]
  clientForm.enabled = client.enabled
  clientDialogOpen.value = true
}

function closeClientDialog() {
  if (!operating.value) clientDialogOpen.value = false
}

async function saveClient() {
  if (!canSaveClient.value) return
  operating.value = true
  try {
    if (editingClientId.value) {
      await settingsStepUp.run(() => updateOAuth2ProviderClient(editingClientId.value, {
        name: clientForm.name,
        redirect_uris: parsedRedirectURIs.value,
        allowed_scopes: [...clientForm.allowed_scopes],
        enabled: clientForm.enabled,
      }))
      appStore.showSuccess(t('admin.settings.oauth2Provider.clientSaved'))
    } else {
      const result = await settingsStepUp.run(() => createOAuth2ProviderClient({
        name: clientForm.name,
        client_type: clientForm.client_type,
        redirect_uris: parsedRedirectURIs.value,
        allowed_scopes: [...clientForm.allowed_scopes],
        enabled: clientForm.enabled,
      }))
      oneTimeSecret.value = result.client_secret || ''
      appStore.showSuccess(t('admin.settings.oauth2Provider.clientCreated'))
    }
    clientDialogOpen.value = false
    await load()
  } catch (error) {
    handleOperationError(error, t('admin.settings.oauth2Provider.clientSaveFailed'))
  } finally {
    operating.value = false
  }
}

async function rotateSecret(client: OAuth2ProviderClient) {
  if (!window.confirm(t('admin.settings.oauth2Provider.rotateConfirm', { name: client.name }))) return
  operating.value = true
  try {
    const result = await settingsStepUp.run(() => rotateOAuth2ProviderClientSecret(client.client_id))
    oneTimeSecret.value = result.client_secret || ''
    appStore.showSuccess(t('admin.settings.oauth2Provider.secretRotated'))
    await load()
  } catch (error) {
    handleOperationError(error, t('admin.settings.oauth2Provider.rotateFailed'))
  } finally {
    operating.value = false
  }
}

async function removeClient(client: OAuth2ProviderClient) {
  if (!window.confirm(t('admin.settings.oauth2Provider.deleteConfirm', { name: client.name }))) return
  operating.value = true
  try {
    await settingsStepUp.run(() => deleteOAuth2ProviderClient(client.client_id))
    if (config.value) config.value.clients = config.value.clients.filter((item) => item.client_id !== client.client_id)
    appStore.showSuccess(t('admin.settings.oauth2Provider.clientDeleted'))
  } catch (error) {
    handleOperationError(error, t('admin.settings.oauth2Provider.deleteFailed'))
  } finally {
    operating.value = false
  }
}

async function copySecret() {
  try {
    await navigator.clipboard.writeText(oneTimeSecret.value)
    appStore.showSuccess(t('common.copied'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

function translatedScopeDescription(name: string, fallback: string): string {
  const key = `admin.settings.oauth2Provider.scopeDescriptions.${name}`
  const translated = t(key)
  return translated === key ? fallback : translated
}

function clientTypeLabel(clientType: OAuth2ClientType): string {
  return clientType === 'public'
    ? t('admin.settings.oauth2Provider.types.public')
    : t('admin.settings.oauth2Provider.types.confidential')
}

onMounted(load)
</script>
