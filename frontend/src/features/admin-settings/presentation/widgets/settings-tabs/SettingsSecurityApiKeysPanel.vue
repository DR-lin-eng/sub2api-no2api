<template>
        <div class="space-y-6">
          <!-- Scoped Admin API Key permission panel -->
          <div class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.adminApiKey.scopedTitle") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.adminApiKey.scopedDescription") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="grid gap-3 md:grid-cols-[1fr_1fr_auto]">
                <input v-model="adminApiKeyForm.name" class="input" type="text" maxlength="100" :placeholder="t('admin.settings.adminApiKey.namePlaceholder')" />
                <input v-model="adminApiKeyForm.expires_at" class="input" type="datetime-local" :min="adminApiKeyMinExpiry" />
                <button type="button" class="btn btn-primary" :disabled="adminApiKeyPanelOperating || !adminApiKeyForm.name.trim()" @click="createScopedAdminApiKey">
                  {{ editingAdminApiKeyId ? t("admin.settings.adminApiKey.saveScoped") : t("admin.settings.adminApiKey.createScoped") }}
                </button>
                <button v-if="editingAdminApiKeyId" type="button" class="btn btn-secondary" @click="cancelEditScopedAdminApiKey">{{ t("admin.settings.adminApiKey.cancel") }}</button>
              </div>
              <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                <label v-for="scope in adminApiKeyScopeOptions" :key="scope.value" class="flex items-start gap-2 rounded border border-gray-200 p-2 text-xs dark:border-dark-600">
                  <input v-model="adminApiKeyForm.scopes" type="checkbox" :value="scope.value" class="mt-0.5" />
                  <span><strong class="block text-gray-800 dark:text-gray-100">{{ scope.label }}</strong><span class="text-gray-500 dark:text-gray-400">{{ scope.value }}</span></span>
                </label>
              </div>
              <div v-if="adminApiKeyPanelSecret" class="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
                <p class="text-sm font-medium text-green-700 dark:text-green-300">{{ t("admin.settings.adminApiKey.secretOnce") }}</p>
                <div class="mt-2 flex gap-2"><code class="flex-1 select-all break-all rounded bg-white px-3 py-2 font-mono text-sm dark:bg-dark-800">{{ adminApiKeyPanelSecret }}</code><button type="button" class="btn btn-primary btn-sm" @click="copyScopedAdminApiKey">{{ t("admin.settings.adminApiKey.copyKey") }}</button></div>
              </div>
              <div v-if="adminApiKeyPanelLoading" class="text-sm text-gray-500">{{ t("common.loading") }}</div>
              <div v-else class="overflow-x-auto">
                <table class="w-full text-left text-sm">
                  <thead><tr class="border-b border-gray-200 text-xs text-gray-500 dark:border-dark-700"><th class="py-2">{{ t("admin.settings.adminApiKey.name") }}</th><th>{{ t("admin.settings.adminApiKey.scopes") }}</th><th>{{ t("admin.settings.adminApiKey.status") }}</th><th>{{ t("admin.settings.adminApiKey.lastUsed") }}</th><th class="text-right">{{ t("admin.settings.adminApiKey.actions") }}</th></tr></thead>
                  <tbody><tr v-for="key in scopedAdminApiKeys" :key="key.id" class="border-b border-gray-100 dark:border-dark-800"><td class="py-3"><div class="font-medium">{{ key.name }}</div><code class="text-xs text-gray-500">{{ key.key_prefix }}...{{ key.last_four }}</code></td><td><div class="flex max-w-md flex-wrap gap-1"><span v-for="scope in key.scopes" :key="scope" class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] dark:bg-dark-700">{{ scope }}</span></div></td><td><span :class="key.status === 'active' ? 'text-green-600' : 'text-red-500'">{{ key.status }}</span><span v-if="key.expires_at" class="ml-1 text-xs text-gray-500">{{ formatAdminApiKeyDate(key.expires_at) }}</span></td><td class="text-xs text-gray-500">{{ key.last_used_at ? formatAdminApiKeyDate(key.last_used_at) : '—' }}</td><td class="whitespace-nowrap text-right"><button v-if="key.status === 'active'" type="button" class="btn btn-secondary btn-sm mr-1" :disabled="adminApiKeyPanelOperating" @click="editScopedAdminApiKey(key)">{{ t("admin.settings.adminApiKey.edit") }}</button><button v-if="key.id !== 'legacy' && key.status === 'active'" type="button" class="btn btn-secondary btn-sm mr-1" :disabled="adminApiKeyPanelOperating" @click="rotateScopedAdminApiKey(key.id)">{{ t("admin.settings.adminApiKey.rotate") }}</button><button v-if="key.id !== 'legacy' && key.status === 'active'" type="button" class="btn btn-secondary btn-sm text-red-600" :disabled="adminApiKeyPanelOperating" @click="revokeScopedAdminApiKey(key.id)">{{ t("admin.settings.adminApiKey.revoke") }}</button></td></tr></tbody>
                </table>
                <p v-if="!scopedAdminApiKeys.length" class="py-5 text-center text-sm text-gray-500">{{ t("admin.settings.adminApiKey.noScopedKeys") }}</p>
              </div>
            </div>
          </div>

          <!-- Admin API Key Settings -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.adminApiKey.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.adminApiKey.description") }}
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Security Warning -->
              <div
                class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20"
              >
                <div class="flex items-start">
                  <Icon
                    name="exclamationTriangle"
                    size="md"
                    class="mt-0.5 flex-shrink-0 text-amber-500"
                  />
                  <p class="ml-3 text-sm text-amber-700 dark:text-amber-300">
                    {{ t("admin.settings.adminApiKey.securityWarning") }}
                  </p>
                </div>
              </div>

              <!-- Loading State -->
              <div
                v-if="adminApiKeyLoading"
                class="flex items-center gap-2 text-gray-500"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
                ></div>
                {{ t("common.loading") }}
              </div>

              <!-- No Key Configured -->
              <div
                v-else-if="!adminApiKeyExists"
                class="flex items-center justify-between"
              >
                <span class="text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.adminApiKey.notConfigured") }}
                </span>
                <button
                  type="button"
                  @click="createAdminApiKey"
                  :disabled="adminApiKeyOperating"
                  class="btn btn-primary btn-sm"
                >
                  <svg
                    v-if="adminApiKeyOperating"
                    class="mr-1 h-4 w-4 animate-spin"
                    fill="none"
                    viewBox="0 0 24 24"
                  >
                    <circle
                      class="opacity-25"
                      cx="12"
                      cy="12"
                      r="10"
                      stroke="currentColor"
                      stroke-width="4"
                    ></circle>
                    <path
                      class="opacity-75"
                      fill="currentColor"
                      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                    ></path>
                  </svg>
                  {{
                    adminApiKeyOperating
                      ? t("admin.settings.adminApiKey.creating")
                      : t("admin.settings.adminApiKey.create")
                  }}
                </button>
              </div>

              <!-- Key Exists -->
              <div v-else class="space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <label
                      class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t("admin.settings.adminApiKey.currentKey") }}
                    </label>
                    <code
                      class="rounded bg-gray-100 px-2 py-1 font-mono text-sm text-gray-900 dark:bg-dark-700 dark:text-gray-100"
                    >
                      {{ adminApiKeyMasked }}
                    </code>
                  </div>
                  <div class="flex gap-2">
                    <button
                      type="button"
                      @click="regenerateAdminApiKey"
                      :disabled="adminApiKeyOperating"
                      class="btn btn-secondary btn-sm"
                    >
                      {{
                        adminApiKeyOperating
                          ? t("admin.settings.adminApiKey.regenerating")
                          : t("admin.settings.adminApiKey.regenerate")
                      }}
                    </button>
                    <button
                      type="button"
                      @click="deleteAdminApiKey"
                      :disabled="adminApiKeyOperating"
                      class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                    >
                      {{ t("admin.settings.adminApiKey.delete") }}
                    </button>
                  </div>
                </div>

                <!-- Newly Generated Key Display -->
                <div
                  v-if="newAdminApiKey"
                  class="space-y-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20"
                >
                  <p
                    class="text-sm font-medium text-green-700 dark:text-green-300"
                  >
                    {{ t("admin.settings.adminApiKey.keyWarning") }}
                  </p>
                  <div class="flex items-center gap-2">
                    <code
                      class="flex-1 select-all break-all rounded border border-green-300 bg-white px-3 py-2 font-mono text-sm dark:border-green-700 dark:bg-dark-800"
                    >
                      {{ newAdminApiKey }}
                    </code>
                    <button
                      type="button"
                      @click="copyNewKey"
                      class="btn btn-primary btn-sm flex-shrink-0"
                    >
                      {{ t("admin.settings.adminApiKey.copyKey") }}
                    </button>
                  </div>
                  <p class="text-xs text-green-600 dark:text-green-400">
                    {{ t("admin.settings.adminApiKey.usage") }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
</template>

<script setup lang="ts">
import Icon from '@/common/widgets/icons/Icon.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { adminApiKeyExists, adminApiKeyForm, adminApiKeyLoading, adminApiKeyMasked, adminApiKeyMinExpiry, adminApiKeyOperating, adminApiKeyPanelLoading, adminApiKeyPanelOperating, adminApiKeyPanelSecret, adminApiKeyScopeOptions, cancelEditScopedAdminApiKey, copyNewKey, copyScopedAdminApiKey, createAdminApiKey, createScopedAdminApiKey, deleteAdminApiKey, editScopedAdminApiKey, editingAdminApiKeyId, formatAdminApiKeyDate, newAdminApiKey, regenerateAdminApiKey, revokeScopedAdminApiKey, rotateScopedAdminApiKey, scopedAdminApiKeys, t } = useSettingsPageContext()
</script>
