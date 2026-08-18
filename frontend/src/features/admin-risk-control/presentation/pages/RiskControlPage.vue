<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="statusLoading" @click="loadStatus(false)">
              <Icon name="refresh" size="sm" :class="statusLoading ? 'animate-spin' : ''" />
              {{ t('admin.riskControl.refreshStatus') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="openSettings">
              <Icon name="cog" size="sm" />
              {{ t('admin.riskControl.openSettings') }}
            </button>
          </div>
        </div>

        <RiskControlOverviewCards :items="overviewItems" />
        <RiskControlRuntimeCards
          :status="status"
          :mode="configForm.mode"
          :worker-count="configForm.worker_count"
          :queue-size="configForm.queue_size"
        />

        <RiskControlRecordsPanel
          :logs="logs"
          :loading="logsLoading"
          :filters="filters"
          :result-options="resultOptions"
          :group-filter-options="groupFilterOptions"
          :endpoint-options="endpointOptions"
          :model-filter-summary="modelFilterSummary"
          :model-filter-preview-models="modelFilterPreviewModels"
          :hidden-model-filter-model-count="hiddenModelFilterModelCount"
          :pagination="pagination"
          :unbanning-user-id="unbanningUserID"
          @refresh="loadLogs"
          @reload="reloadLogsFromFirstPage"
          @page-change="onPageChange"
          @page-size-change="onPageSizeChange"
          @unban="unbanUser"
          @update-filter="updateLogFilter"
        />
      </template>

      <BaseDialog :show="settingsOpen" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="settingsOpen = false">
        <div class="space-y-6">
          <div class="flex gap-2 overflow-x-auto border-b border-gray-100 pb-3 dark:border-dark-700">
            <button
              v-for="tab in settingsTabs"
              :key="tab.id"
              type="button"
              class="inline-flex whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="activeSettingsTab === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
              @click="activeSettingsTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>

          <div v-if="activeSettingsTab === 'basic'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.enabled') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.enabledHint') }}</p>
                </div>
                <Toggle v-model="configForm.enabled" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.mode') }}</label>
                <Select v-model="configForm.mode" :options="modeOptions" />
                <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ modeDescription(configForm.mode) }}</p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.baseUrl') }}</label>
                <input v-model.trim="configForm.base_url" type="url" class="input" placeholder="https://api.openai.com" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.model') }}</label>
                <input v-model.trim="configForm.model" type="text" class="input" placeholder="omni-moderation-latest" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.proxy') }}</label>
                <ProxySelector v-model="configForm.proxy_id" :proxies="proxies" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.timeoutMs') }}</label>
                <input v-model.number="configForm.timeout_ms" type="number" min="500" max="30000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.retryCount') }}</label>
                <input v-model.number="configForm.retry_count" type="number" min="0" max="5" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.sampleRate') }}</label>
                <div class="relative">
                  <input v-model.number="configForm.sample_rate" type="number" min="0" max="100" step="1" class="input pr-8" />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
              </div>
            </div>

            <div class="overflow-hidden rounded-xl border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex flex-col gap-4 border-b border-gray-100 bg-gray-50 px-4 py-4 dark:border-dark-700 dark:bg-dark-800/60 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeys') }}</label>
                    <p class="mt-1 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.apiKeysHint', { count: configForm.api_key_count }) }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || inputApiKeyCount === 0 || configForm.clear_api_key"
                    @click="testApiKeys(true)"
                  >
                    <Icon name="beaker" size="sm" :class="apiKeyTesting ? 'animate-pulse' : ''" />
                    {{ apiKeyTesting ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || effectiveStoredApiKeyCount === 0 || pendingDeletedApiKeyCount > 0 || configForm.clear_api_key || configForm.api_keys_mode === 'replace'"
                    @click="testApiKeys(false)"
                  >
                    <Icon name="shield" size="sm" />
                    {{ storedApiKeyTestButtonText }}
                  </button>
                  <button
                    v-if="configForm.api_key_configured"
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    @click="toggleClearApiKey"
                  >
                    <Icon :name="configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
                    {{ configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') }}
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
                <div class="space-y-3">
                  <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
                    <div class="text-xs leading-5 text-gray-500 dark:text-gray-400">
                      <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeysWriteMode') }}</span>
                      <span class="ml-2">{{ apiKeysModeHint }}</span>
                    </div>
                    <div class="inline-flex rounded-lg bg-white p-1 shadow-sm dark:bg-dark-800">
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'append' ? 'bg-primary-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('append')"
                      >
                        {{ t('admin.riskControl.apiKeysModeAppend') }}
                      </button>
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'replace' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('replace')"
                      >
                        {{ t('admin.riskControl.apiKeysModeReplace') }}
                      </button>
                    </div>
                  </div>
                  <textarea
                    v-model="configForm.api_keys_text"
                    class="input min-h-44 resize-y font-mono text-sm"
                    :placeholder="apiKeysPlaceholder"
                    autocomplete="new-password"
                    :disabled="configForm.clear_api_key"
                  ></textarea>
                  <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.inputApiKeyCount', { count: inputApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_key_configured" class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.storedApiKeyCount', { count: configForm.api_key_count }) }}
                    </span>
                    <span v-if="configForm.clear_api_key" class="inline-flex rounded-md bg-red-50 px-2 py-1 text-red-700 dark:bg-red-900/20 dark:text-red-300">
                      {{ t('admin.riskControl.apiKeyWillClear') }}
                    </span>
                    <span v-else-if="pendingDeletedApiKeyCount > 0" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: pendingDeletedApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_keys_mode === 'replace'" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeysReplaceWarning') }}
                    </span>
                  </div>

                  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30" @paste="handleModerationImagePaste">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestInput') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestInputHint') }}</p>
                      </div>
                      <button
                        v-if="moderationTestPrompt || moderationTestImages.length > 0 || moderationTestResult"
                        type="button"
                        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                        @click="clearModerationTestInput"
                      >
                        <Icon name="x" size="xs" />
                        {{ t('admin.riskControl.clearAuditTest') }}
                      </button>
                    </div>
                    <textarea
                      v-model="moderationTestPrompt"
                      class="input min-h-24 resize-y text-sm"
                      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
                    ></textarea>
                    <div
                      class="mt-3 rounded-lg border border-dashed border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
                      @dragover.prevent
                      @drop.prevent="handleModerationImageDrop"
                    >
                      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="flex items-start gap-2">
                          <Icon name="upload" size="md" class="mt-0.5 text-gray-400" />
                          <div>
                            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.riskControl.auditTestImages') }}</p>
                            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestImagesHint') }}</p>
                          </div>
                        </div>
                        <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
                          <Icon name="plus" size="sm" />
                          {{ t('admin.riskControl.addAuditTestImage') }}
                          <input type="file" accept="image/*" multiple class="sr-only" @change="handleModerationImageUpload" />
                        </label>
                      </div>
                      <div v-if="moderationTestImages.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <div
                          v-for="(image, index) in moderationTestImages"
                          :key="image.slice(0, 64) + index"
                          class="group relative aspect-square overflow-hidden rounded-lg border border-gray-100 bg-gray-100 dark:border-dark-700 dark:bg-dark-700"
                        >
                          <img :src="image" alt="" class="h-full w-full object-cover" />
                          <button
                            type="button"
                            class="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"
                            @click="removeModerationTestImage(index)"
                          >
                            <Icon name="x" size="xs" :stroke-width="2" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="mb-3 flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeyHealth') }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyFreezeRule') }}</p>
                    </div>
                    <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-medium leading-5 text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                      {{ t('admin.riskControl.apiKeyRows', { count: apiKeyRows.length }) }}
                    </span>
                  </div>

                  <div v-if="apiKeyRows.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 bg-white px-4 py-6 text-center dark:border-dark-700 dark:bg-dark-800">
                    <Icon name="infoCircle" size="lg" class="text-gray-300 dark:text-dark-500" />
                    <p class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeyHealthEmpty') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyHealthEmptyHint') }}</p>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="space-y-2" :class="apiKeyRowsExpanded ? 'max-h-72 overflow-y-auto pr-1' : ''">
                      <div
                        v-for="(row, index) in visibleApiKeyRows"
                        :key="apiKeyRowKey(row, index)"
                        class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-dark-800"
                        :class="isStoredApiKeyPendingDelete(row) ? 'border-amber-200 opacity-70 dark:border-amber-800/60' : 'border-gray-100 dark:border-dark-700'"
                      >
                        <div class="flex items-start justify-between gap-2">
                          <div class="min-w-0">
                            <div class="flex min-w-0 flex-wrap items-center gap-2">
                              <span class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.masked || '-' }}</span>
                              <span
                                class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                                :class="row.configured ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'"
                              >
                                {{ isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') }}
                              </span>
                            </div>
                            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ apiKeyStatusMeta(row) }}</p>
                          </div>
                          <div class="flex flex-shrink-0 items-center gap-1.5">
                            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium" :class="apiKeyStatusBadgeClass(row.status)">
                              <span class="h-1.5 w-1.5 rounded-full" :class="apiKeyStatusDotClass(row.status)"></span>
                              {{ apiKeyStatusLabel(row.status) }}
                            </span>
                            <button
                              v-if="row.configured && !configForm.clear_api_key"
                              type="button"
                              class="inline-flex h-7 w-7 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                              :title="isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
                              @click="toggleDeleteStoredApiKey(row)"
                            >
                              <Icon :name="isStoredApiKeyPendingDelete(row) ? 'refresh' : 'trash'" size="xs" />
                            </button>
                          </div>
                        </div>
                        <p v-if="row.last_error" class="mt-1.5 rounded-md bg-amber-50 px-2 py-1.5 text-xs leading-5 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                          {{ row.last_error }}
                        </p>
                      </div>
                    </div>

                    <div v-if="canToggleApiKeyRows" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                      <span class="min-w-0 truncate">
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.apiKeyRowsExpanded', { count: apiKeyRows.length }) : t('admin.riskControl.apiKeyRowsCollapsed', { count: hiddenApiKeyRowCount }) }}
                      </span>
                      <button
                        type="button"
                        class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-300 dark:hover:bg-primary-900/20"
                        @click="apiKeyRowsExpanded = !apiKeyRowsExpanded"
                      >
                        <Icon :name="apiKeyRowsExpanded ? 'chevronUp' : 'chevronDown'" size="xs" />
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') }}
                      </button>
                    </div>
                  </div>

                  <div v-if="moderationTestResult" class="mt-4 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
                    <div class="flex items-start justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestResult') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                          {{ t('admin.riskControl.auditTestHighest', { category: moderationTestResult.highest_category || '-', score: percent(moderationTestResult.highest_score) }) }}
                        </p>
                      </div>
                      <span class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="moderationTestResult.flagged ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">
                        {{ moderationTestResult.flagged ? t('admin.riskControl.auditTestFlagged') : t('admin.riskControl.auditTestPassed') }}
                      </span>
                    </div>
                    <div class="mt-3">
                      <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                        <span>{{ t('admin.riskControl.auditTestComposite') }}</span>
                        <span class="font-semibold text-gray-900 dark:text-white">{{ percent(moderationTestResult.composite_score) }}</span>
                      </div>
                      <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="moderationTestResult.flagged ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: percentWidth(moderationTestResult.composite_score) }"></div>
                      </div>
                    </div>
                    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
                      <div v-for="score in moderationScoreRows" :key="score.category">
                        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
                          <span class="truncate text-gray-600 dark:text-gray-300">{{ score.category }}</span>
                          <span class="font-mono text-gray-500 dark:text-gray-400">{{ percent(score.score) }} / {{ percent(score.threshold) }}</span>
                        </div>
                        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                          <div class="h-full rounded-full" :class="score.hit ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: percentWidth(score.score) }"></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'scope'" class="space-y-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.groupScope') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.groupScopeHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = true"
                >
                  {{ t('admin.riskControl.allGroups') }}
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="!configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = false"
                >
                  {{ t('admin.riskControl.selectedGroups') }}
                </button>
              </div>
            </div>

            <div v-if="!configForm.all_groups" class="space-y-4">
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="groupSearch" type="search" class="input pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
              </div>
              <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
                <button
                  v-for="group in filteredGroups"
                  :key="group.id"
                  type="button"
                  class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
                  :class="isGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="toggleGroup(group.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                    <span class="mt-1 inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ group.platform }}</span>
                  </span>
                  <span
                    class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
                    :class="isGroupSelected(group.id) ? 'border-primary-500 bg-primary-500 text-white' : 'border-gray-300 text-transparent dark:border-dark-500'"
                  >
                    <Icon name="check" size="xs" :stroke-width="2" />
                  </span>
                </button>
                <p v-if="filteredGroups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.noGroups') }}</p>
              </div>
            </div>

            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
              <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.modelFilter') }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.modelFilterHint') }}</p>
                </div>
                <span class="inline-flex w-fit rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modelFilterSummary }}
                </span>
              </div>

              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <button
                  v-for="option in modelFilterOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.model_filter_type === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="setModelFilterType(option.value)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.model_filter_type === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
                </button>
              </div>

              <div v-if="configForm.model_filter_type !== 'all'" class="space-y-2">
                <label class="input-label">{{ t('admin.riskControl.modelFilterModels') }}</label>
                <ModelWhitelistSelector v-model="configForm.model_filter_models" />
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.riskControl.modelFilterModelCount', { count: modelFilterModelCount }) }}
                </p>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'runtime'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.workerCount') }}</label>
              <input v-model.number="configForm.worker_count" type="number" min="1" max="32" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.queueSize') }}</label>
              <input v-model.number="configForm.queue_size" type="number" min="100" max="100000" class="input" />
            </div>
            <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.recordNonHits') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordNonHitsHint') }}</p>
              </div>
              <Toggle v-model="configForm.record_non_hits" />
            </div>
            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.preHashCheck') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preHashCheckHint') }}</p>
                </div>
                <Toggle v-model="configForm.pre_hash_check_enabled" />
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('admin.riskControl.flaggedHashCount', { count: formatNumber(status?.flagged_hash_count ?? 0) }) }}
                    </p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.flaggedHashHint') }}</p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2 text-red-600 hover:text-red-700 dark:text-red-300"
                    :disabled="hashActionLoading || (status?.flagged_hash_count ?? 0) === 0"
                    @click="clearFlaggedHashes"
                  >
                    <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
                    {{ t('admin.riskControl.clearFlaggedHashes') }}
                  </button>
                </div>
                <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    v-model.trim="flaggedHashInput"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2"
                    :disabled="hashActionLoading || !isFlaggedHashInputValid"
                    @click="deleteFlaggedHash"
                  >
                    <Icon name="trash" size="sm" />
                    {{ t('admin.riskControl.deleteFlaggedHash') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'response'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
                <input v-model.number="configForm.block_status" type="number" min="400" max="599" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
                <input v-model.trim="configForm.block_message" type="text" class="input" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.emailOnHit') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emailOnHitHint') }}</p>
                </div>
                <Toggle v-model="configForm.email_on_hit" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.autoBan') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.autoBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.auto_ban_enabled" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.cyberPolicyExcludeBan') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.cyberPolicyExcludeBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.cyber_policy_exclude_from_ban_count" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.banThreshold') }}</label>
                <input v-model.number="configForm.ban_threshold" type="number" min="1" max="1000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.violationWindowHours') }}</label>
                <input v-model.number="configForm.violation_window_hours" type="number" min="1" max="8760" class="input" />
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'riskThresholds'" class="space-y-5">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.riskThresholds') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.riskThresholdsHint') }}</p>
              </div>
              <button
                type="button"
                class="btn btn-secondary inline-flex items-center justify-center gap-2"
                @click="resetRiskThresholds"
              >
                <Icon name="refresh" size="sm" />
                {{ t('admin.riskControl.riskThresholdReset') }}
              </button>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="row in riskThresholdRows"
                :key="row.category"
                class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <label class="block truncate text-sm font-semibold text-gray-900 dark:text-white" :for="`risk-threshold-${row.category}`">
                      {{ row.category }}
                    </label>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.riskThresholdDefault', { value: formatThresholdPercent(row.defaultValue) }) }}
                    </p>
                  </div>
                  <span class="inline-flex shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs font-medium text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                    {{ formatThresholdPercent(row.value) }}
                  </span>
                </div>
                <div class="mt-3">
                  <label class="sr-only" :for="`risk-threshold-${row.category}`">
                    {{ t('admin.riskControl.riskThresholdPercent') }}
                  </label>
                  <div class="relative">
                    <input
                      :id="`risk-threshold-${row.category}`"
                      v-model.number="configForm.thresholds[row.category]"
                      :data-test="`risk-threshold-${row.category}`"
                      type="number"
                      min="0"
                      max="100"
                      step="0.1"
                      class="input pr-8 font-mono"
                    />
                    <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'keywords'" class="space-y-5">
            <div
              class="flex items-start gap-3 rounded-lg border p-4"
              :class="keywordNotice.toneClass"
            >
              <Icon
                :name="keywordNotice.icon"
                size="md"
                :class="keywordNotice.iconClass"
              />
              <div class="text-sm leading-6">
                <p class="font-medium" :class="keywordNotice.titleClass">{{ keywordNotice.title }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ keywordNotice.description }}</p>
              </div>
            </div>

            <div class="space-y-2">
              <label class="input-label">{{ t('admin.riskControl.keywordBlockingMode') }}</label>
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <button
                  v-for="option in keywordBlockingModeOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.keyword_blocking_mode === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="configForm.keyword_blocking_mode = option.value"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.keyword_blocking_mode === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
                </button>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <label class="input-label mb-0">{{ t('admin.riskControl.blockedKeywords') }}</label>
                <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                  {{ t('admin.riskControl.blockedKeywordCount', { count: blockedKeywordCount }) }}
                </span>
              </div>
              <textarea
                v-model="configForm.blocked_keywords_text"
                class="input min-h-52 resize-y font-mono text-sm"
                :placeholder="t('admin.riskControl.blockedKeywordsPlaceholder')"
                :disabled="configForm.keyword_blocking_mode === 'api_only'"
              ></textarea>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.riskControl.blockedKeywordsLimit', { max: blockedKeywordMax }) }}
              </p>
            </div>
          </div>

          <div v-else class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
              <input v-model.number="configForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.nonHitRetentionDays') }}</label>
              <input v-model.number="configForm.non_hit_retention_days" type="number" min="1" max="3" class="input" />
            </div>
            <div class="rounded-lg border border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400 lg:col-span-2">
              <div class="flex flex-wrap items-center gap-3">
                <Icon name="database" size="md" class="text-gray-400" />
                <span>{{ t('admin.riskControl.cleanupStats', { hit: status?.last_cleanup_deleted_hit ?? 0, nonHit: status?.last_cleanup_deleted_non_hit ?? 0 }) }}</span>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="settingsOpen = false">{{ t('common.cancel') }}</button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              {{ saving ? t('common.saving') : t('admin.riskControl.saveConfig') }}
            </button>
          </div>
        </template>
      </BaseDialog>


    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import ProxySelector from '@/common/widgets/data/ProxySelector.vue'
import ModelWhitelistSelector from '@/features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue'
import RiskControlOverviewCards from '@/features/admin-risk-control/presentation/widgets/RiskControlOverviewCards.vue'
import RiskControlRuntimeCards from '@/features/admin-risk-control/presentation/widgets/RiskControlRuntimeCards.vue'
import RiskControlRecordsPanel from '@/features/admin-risk-control/presentation/widgets/RiskControlRecordsPanel.vue'
import { useRiskControlPolling } from '@/features/admin-risk-control/presentation/composables/useRiskControlPolling'
import {
  useRiskControlOptions,
  type RiskControlSettingsTab,
} from '@/features/admin-risk-control/presentation/composables/useRiskControlOptions'
import {
  buildModelFilterPayload,
  buildRiskThresholdPayload,
  formatThresholdPercent,
  normalizeDateTimeLocal,
  normalizeKeywordBlockingMode,
  normalizeModelFilter,
  parseApiKeys,
  parseBlockedKeywords,
  riskThresholdCategories,
  riskThresholdDefaults,
  riskThresholdsFromConfig,
} from '@/features/admin-risk-control/presentation/composables/riskControlFormResolvers'
import { adminAPI } from '@/api/admin'
import { getAll as getAllAdminGroups } from '@/features/admin-groups/data/datasources/adminGroupQueries'
import type {
  ContentModerationAPIKeyStatus,
  ContentModerationConfig,
  ContentModerationLog,
  ContentModerationModelFilterType,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  KeywordBlockingMode,
  ModerationMode,
  UpdateContentModerationConfig,
} from '@/features/admin-risk-control/data/datasources/adminRiskControlDatasource'
import type { AdminGroup, Proxy } from '@/types'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/core/utils/format'

type APIKeysWriteMode = 'append' | 'replace'
type OverviewIcon = 'shield' | 'key' | 'users' | 'document'
type OverviewItem = {
  key: string
  label: string
  value: string
  meta: string
  icon: OverviewIcon
  iconClass: string
  badge?: string
  badgeClass?: string
}
type ModerationScoreRow = {
  category: string
  score: number
  threshold: number
  hit: boolean
}
type RiskThresholdRow = {
  category: string
  value: number
  defaultValue: number
}

const maxModerationTestImages = 1
const maxModerationTestImageSize = 8 * 1024 * 1024
const maxVisibleApiKeyRows: number = 3
const blockedKeywordMax = 10000

const { t } = useI18n()
const appStore = useAppStore()
const defaultBlockMessage = () => t('admin.riskControl.defaultBlockMessage')

const loading = ref(true)
const saving = ref(false)
const logsLoading = ref(false)
const statusLoading = ref(false)
const apiKeyTesting = ref(false)
const hashActionLoading = ref(false)
const unbanningUserID = ref<number | null>(null)
const settingsOpen = ref(false)
const activeSettingsTab = ref<RiskControlSettingsTab>('basic')
const groupSearch = ref('')
const flaggedHashInput = ref('')
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const logs = ref<ContentModerationLog[]>([])
const status = ref<ContentModerationRuntimeStatus | null>(null)
const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
const pendingDeleteApiKeyHashes = ref<string[]>([])
const apiKeyRowsExpanded = ref<boolean>(false)
const moderationTestPrompt = ref('')
const moderationTestImages = ref<string[]>([])
const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)

const configForm = reactive({
  enabled: false,
  mode: 'pre_block' as ModerationMode,
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null as number | null,
  api_keys_text: '',
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [] as string[],
  api_key_statuses: [] as ContentModerationAPIKeyStatus[],
  api_keys_mode: 'append' as APIKeysWriteMode,
  clear_api_key: false,
  timeout_ms: 3000,
  retry_count: 2,
  sample_rate: 100,
  all_groups: true,
  group_ids: [] as number[],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: defaultBlockMessage(),
  email_on_hit: true,
  auto_ban_enabled: true,
  cyber_policy_exclude_from_ban_count: false,
  ban_threshold: 10,
  violation_window_hours: 720,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  thresholds: { ...riskThresholdDefaults } as Record<string, number>,
  blocked_keywords_text: '',
  keyword_blocking_mode: 'keyword_and_api' as KeywordBlockingMode,
  model_filter_type: 'all' as ContentModerationModelFilterType,
  model_filter_models: [] as string[],
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
})

const filters = reactive({
  result: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
})

const {
  endpointOptions,
  groupFilterOptions,
  keywordBlockingModeOptions,
  keywordNotice,
  modeOptions,
  modelFilterOptions,
  resultOptions,
  settingsTabs,
} = useRiskControlOptions(t, groups, configForm)

const selectedGroupCount = computed(() => String(configForm.group_ids.length))

const modelFilterModelCount = computed(() => configForm.model_filter_models.length)

const modelFilterSummary = computed(() => {
  if (configForm.model_filter_type === 'include') {
    return t('admin.riskControl.modelFilterIncludeSummary', { count: modelFilterModelCount.value })
  }
  if (configForm.model_filter_type === 'exclude') {
    return t('admin.riskControl.modelFilterExcludeSummary', { count: modelFilterModelCount.value })
  }
  return t('admin.riskControl.modelFilterAllSummary')
})

const modelFilterPreviewModels = computed(() => configForm.model_filter_models.slice(0, 6))

const hiddenModelFilterModelCount = computed(() => Math.max(0, configForm.model_filter_models.length - modelFilterPreviewModels.value.length))

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter((group) => {
    return group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword)
  })
})

const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)

const blockedKeywordList = computed(() => parseBlockedKeywords(configForm.blocked_keywords_text))

const blockedKeywordCount = computed(() => blockedKeywordList.value.length)

const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)

const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

const apiKeysPlaceholder = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysPlaceholderReplace')
    : t('admin.riskControl.apiKeysPlaceholder')
))

const apiKeysModeHint = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysModeReplaceHint')
    : t('admin.riskControl.apiKeysModeAppendHint')
))

const hasModerationAuditInput = computed(() => {
  return moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0
})

const isFlaggedHashInputValid = computed(() => /^[a-fA-F0-9]{64}$/.test(flaggedHashInput.value.trim()))

const storedApiKeyTestButtonText = computed(() => {
  if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
  if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
  return t('admin.riskControl.testStoredApiKeys')
})

const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  const rows = status.value?.api_key_statuses?.length
    ? status.value.api_key_statuses
    : configForm.api_key_statuses
  return Array.isArray(rows) ? rows : []
})

const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [
  ...savedApiKeyRows.value,
  ...testedApiKeyStatuses.value,
])

const visibleApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  if (apiKeyRowsExpanded.value) return apiKeyRows.value
  return apiKeyRows.value.slice(0, maxVisibleApiKeyRows)
})

const hiddenApiKeyRowCount = computed<number>(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))

const canToggleApiKeyRows = computed<boolean>(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

const activeSavedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => (
  savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row))
))

const apiKeyHealthBadges = computed<Array<{ status: ContentModerationAPIKeyStatus['status']; count: number }>>(() => {
  const counts: Record<ContentModerationAPIKeyStatus['status'], number> = {
    ok: 0,
    error: 0,
    frozen: 0,
    unknown: 0,
  }
  for (const row of activeSavedApiKeyRows.value) {
    counts[row.status] = (counts[row.status] ?? 0) + 1
  }
  if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) {
    counts.unknown = effectiveStoredApiKeyCount.value
  }
  return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
    .map((item) => ({ status: item, count: counts[item] }))
    .filter((item) => item.count > 0)
})

const apiKeyHealthSummary = computed(() => {
  if (!configForm.api_key_configured) return ''
  if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
  return apiKeyHealthBadges.value
    .map((badge) => `${apiKeyStatusLabel(badge.status)} ${badge.count}`)
    .join(' · ')
})

const overviewItems = computed<OverviewItem[]>(() => [
  {
    key: 'status',
    label: t('admin.riskControl.overview.status'),
    value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
    meta: modeLabel(configForm.mode),
    icon: 'shield',
    iconClass: configForm.enabled
      ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    badge: runtimeBadgeText.value,
    badgeClass: runtimeBadgeClass.value,
  },
  {
    key: 'api-key',
    label: t('admin.riskControl.overview.apiKey'),
    value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count }) : t('admin.riskControl.notConfigured'),
    meta: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
    icon: 'key',
    iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
  },
  {
    key: 'scope',
    label: t('admin.riskControl.overview.groupScope'),
    value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
    meta: modelFilterSummary.value,
    icon: 'users',
    iconClass: 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
  },
  {
    key: 'logs',
    label: t('admin.riskControl.overview.logs'),
    value: formatNumber(pagination.total),
    meta: t('admin.riskControl.overview.currentFilter'),
    icon: 'document',
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
  },
])

const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
  const result = moderationTestResult.value
  if (!result) return []
  return Object.entries(result.category_scores || {})
    .map(([category, score]) => {
      const threshold = result.thresholds?.[category] ?? 1
      return {
        category,
        score,
        threshold,
        hit: score >= threshold,
      }
    })
    .sort((a, b) => b.score - a.score)
})

const riskThresholdRows = computed<RiskThresholdRow[]>(() => (
  riskThresholdCategories.map((category) => ({
    category,
    value: configForm.thresholds[category] ?? riskThresholdDefaults[category],
    defaultValue: riskThresholdDefaults[category],
  }))
))

const runtimeBadgeText = computed(() => {
  if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
  if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
  return t('admin.riskControl.overview.enabled')
})

const runtimeBadgeClass = computed(() => {
  if (!status.value?.risk_control_enabled || !configForm.enabled || configForm.mode === 'off') {
    return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
})

function applyConfig(config: ContentModerationConfig) {
  configForm.enabled = config.enabled
  configForm.mode = config.mode
  configForm.base_url = config.base_url || 'https://api.openai.com'
  configForm.model = config.model || 'omni-moderation-latest'
  configForm.proxy_id = config.proxy_id || null
  configForm.api_keys_text = ''
  configForm.api_key_configured = config.api_key_configured
  configForm.api_key_masked = config.api_key_masked || ''
  configForm.api_key_count = config.api_key_count || 0
  configForm.api_key_masks = Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : []
  configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
  configForm.api_keys_mode = 'append'
  configForm.clear_api_key = false
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  apiKeyRowsExpanded.value = false
  configForm.timeout_ms = config.timeout_ms || 3000
  configForm.retry_count = config.retry_count ?? 2
  configForm.sample_rate = config.sample_rate ?? 100
  configForm.all_groups = config.all_groups
  configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  configForm.record_non_hits = config.record_non_hits
  configForm.worker_count = config.worker_count || 4
  configForm.queue_size = config.queue_size || 32768
  configForm.block_status = config.block_status || 403
  configForm.block_message = config.block_message || defaultBlockMessage()
  configForm.email_on_hit = config.email_on_hit ?? true
  configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
  configForm.cyber_policy_exclude_from_ban_count = config.cyber_policy_exclude_from_ban_count ?? false
  configForm.ban_threshold = config.ban_threshold || 10
  configForm.violation_window_hours = config.violation_window_hours || 720
  configForm.hit_retention_days = config.hit_retention_days || 180
  configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
  configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
  configForm.thresholds = riskThresholdsFromConfig(config.thresholds)
  configForm.blocked_keywords_text = Array.isArray(config.blocked_keywords) ? config.blocked_keywords.join('\n') : ''
  configForm.keyword_blocking_mode = normalizeKeywordBlockingMode(config.keyword_blocking_mode)
  const modelFilter = normalizeModelFilter(config.model_filter)
  configForm.model_filter_type = modelFilter.type
  configForm.model_filter_models = modelFilter.models
}

async function loadAll() {
  loading.value = true
  try {
    const [config, groupItems, runtimeStatus, proxyItems] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      getAllAdminGroups(),
      adminAPI.riskControl.getStatus(),
      adminAPI.proxies.getAll().catch(() => [] as Proxy[]),
    ])
    applyConfig(config)
    groups.value = groupItems
    proxies.value = proxyItems
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadStatus(silent = true) {
  statusLoading.value = true
  try {
    const runtimeStatus = await adminAPI.riskControl.getStatus()
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
  } catch (err: unknown) {
    if (!silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    }
  } finally {
    statusLoading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const modelFilterPayload = buildModelFilterPayload(
      configForm.model_filter_type,
      configForm.model_filter_models,
    )
    if (modelFilterPayload.type !== 'all' && modelFilterPayload.models.length === 0) {
      appStore.showError(t('admin.riskControl.modelFilterModelsRequired'))
      return
    }
    const payload: UpdateContentModerationConfig = {
      enabled: configForm.enabled,
      mode: configForm.mode,
      base_url: configForm.base_url,
      model: configForm.model,
      proxy_id: configForm.proxy_id ?? 0,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      retry_count: Number(configForm.retry_count) || 0,
      sample_rate: Number(configForm.sample_rate) || 0,
      all_groups: configForm.all_groups,
      group_ids: configForm.all_groups ? [] : [...configForm.group_ids],
      record_non_hits: configForm.record_non_hits,
      clear_api_key: configForm.clear_api_key,
      worker_count: Number(configForm.worker_count) || 4,
      queue_size: Number(configForm.queue_size) || 32768,
      block_status: Number(configForm.block_status) || 403,
      block_message: configForm.block_message || defaultBlockMessage(),
      email_on_hit: configForm.email_on_hit,
      auto_ban_enabled: configForm.auto_ban_enabled,
      cyber_policy_exclude_from_ban_count: configForm.cyber_policy_exclude_from_ban_count,
      ban_threshold: Number(configForm.ban_threshold) || 10,
      violation_window_hours: Number(configForm.violation_window_hours) || 720,
      hit_retention_days: Number(configForm.hit_retention_days) || 180,
      non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
      pre_hash_check_enabled: configForm.pre_hash_check_enabled,
      thresholds: buildRiskThresholdPayload(configForm.thresholds),
      blocked_keywords: blockedKeywordList.value,
      keyword_blocking_mode: configForm.keyword_blocking_mode,
      model_filter: modelFilterPayload,
    }
    const keys = parseApiKeys(configForm.api_keys_text)
    if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
      appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
      return
    }
    if (keys.length > 0) {
      payload.api_keys = keys
      payload.api_keys_mode = configForm.api_keys_mode
      payload.clear_api_key = false
    }
    if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
      payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
    }

    const updated = await adminAPI.riskControl.updateConfig(payload)
    applyConfig(updated)
    settingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.saved'))
    await Promise.all([loadStatus(true), loadLogs()])
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      result: filters.result || undefined,
      group_id: filters.group_id || undefined,
      endpoint: filters.endpoint || undefined,
      search: filters.search || undefined,
      from: normalizeDateTimeLocal(filters.from),
      to: normalizeDateTimeLocal(filters.to),
    }
    const result = await adminAPI.riskControl.listLogs(params)
    logs.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.logsFailed')))
  } finally {
    logsLoading.value = false
  }
}

async function unbanUser(row: ContentModerationLog) {
  if (!row.user_id || unbanningUserID.value !== null) return
  unbanningUserID.value = row.user_id
  try {
    const result = await adminAPI.riskControl.unbanUser(row.user_id)
    logs.value = logs.value.map((item) => {
      if (item.user_id !== row.user_id) return item
      return { ...item, user_status: result.status }
    })
    appStore.showSuccess(t('admin.riskControl.unbanSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.unbanFailed')))
  } finally {
    unbanningUserID.value = null
  }
}

async function deleteFlaggedHash() {
  if (!isFlaggedHashInputValid.value || hashActionLoading.value) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.deleteFlaggedHash(flaggedHashInput.value)
    flaggedHashInput.value = ''
    await loadStatus(true)
    appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

async function clearFlaggedHashes() {
  if (hashActionLoading.value) return
  const confirmed = window.confirm(t('admin.riskControl.clearFlaggedHashesConfirm'))
  if (!confirmed) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.clearFlaggedHashes()
    await loadStatus(true)
    appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

function openSettings() {
  activeSettingsTab.value = 'basic'
  settingsOpen.value = true
}

function reloadLogsFromFirstPage() {
  pagination.page = 1
  void loadLogs()
}

function updateLogFilter(key: keyof typeof filters, value: string | number) {
  if (key === 'group_id') {
    filters.group_id = Number(value)
    return
  }
  filters[key] = String(value)
}

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
}

function onPageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadLogs()
}

function toggleClearApiKey() {
  configForm.clear_api_key = !configForm.clear_api_key
  if (configForm.clear_api_key) {
    configForm.api_keys_text = ''
    configForm.api_keys_mode = 'append'
    testedApiKeyStatuses.value = []
    pendingDeleteApiKeyHashes.value = []
  }
}

function setAPIKeysMode(mode: APIKeysWriteMode) {
  configForm.api_keys_mode = mode
  if (mode === 'replace') {
    pendingDeleteApiKeyHashes.value = []
  }
}

function setModelFilterType(type: ContentModerationModelFilterType) {
  configForm.model_filter_type = type
  if (type === 'all') {
    configForm.model_filter_models = []
  }
}

async function testApiKeys(useInputKeys: boolean) {
  const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
  if (useInputKeys && keys.length === 0) {
    appStore.showError(t('admin.riskControl.apiKeyTestNoInput'))
    return
  }
  apiKeyTesting.value = true
  try {
    const result = await adminAPI.riskControl.testAPIKeys({
      api_keys: keys,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      proxy_id: configForm.proxy_id ?? 0,
      prompt: moderationTestPrompt.value,
      images: moderationTestImages.value,
    })
    moderationTestResult.value = result.audit_result ?? null
    if (useInputKeys) {
      testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false }))
    } else {
      mergeConfiguredAPIKeyStatuses(result.items)
      testedApiKeyStatuses.value = []
      await loadStatus(true)
    }
    appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
  } finally {
    apiKeyTesting.value = false
  }
}

function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
  if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
    configForm.api_key_statuses = items
    return
  }
  const updates = new Map(items.map((item) => [item.key_hash, item]))
  configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
}

function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
  if (!row.configured || !row.key_hash) return
  const index = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
  if (index >= 0) {
    pendingDeleteApiKeyHashes.value.splice(index, 1)
    return
  }
  pendingDeleteApiKeyHashes.value.push(row.key_hash)
}

function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
  return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
}

function prunePendingDeleteAPIKeyHashes() {
  const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
  pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
}

function clearModerationTestInput() {
  moderationTestPrompt.value = ''
  moderationTestImages.value = []
  moderationTestResult.value = null
}

function removeModerationTestImage(index: number) {
  moderationTestImages.value.splice(index, 1)
}

async function handleModerationImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  await addModerationTestFiles(input.files)
  input.value = ''
}

async function handleModerationImageDrop(event: DragEvent) {
  await addModerationTestFiles(event.dataTransfer?.files ?? null)
}

async function handleModerationImagePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  if (files.length === 0) return
  event.preventDefault()
  await addModerationTestFiles(files)
}

async function addModerationTestFiles(files: FileList | File[] | null) {
  if (!files) return
  const items = Array.from(files).filter((file) => file.type.startsWith('image/'))
  for (const file of items) {
    if (moderationTestImages.value.length >= maxModerationTestImages) {
      appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages }))
      return
    }
    if (file.size > maxModerationTestImageSize) {
      appStore.showError(t('admin.riskControl.auditTestImageTooLarge'))
      continue
    }
    try {
      moderationTestImages.value.push(await fileToDataURL(file))
    } catch {
      appStore.showError(t('admin.riskControl.auditTestImageReadFailed'))
    }
  }
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function toggleGroup(groupID: number) {
  const index = configForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    configForm.group_ids.splice(index, 1)
  } else {
    configForm.group_ids.push(groupID)
  }
}

function isGroupSelected(groupID: number): boolean {
  return configForm.group_ids.includes(groupID)
}

function modeLabel(mode: ModerationMode): string {
  const found = modeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
}

function modeDescription(mode: ModerationMode): string {
  const descriptions: Record<ModerationMode, string> = {
    pre_block: t('admin.riskControl.modePreBlockDesc'),
    observe: t('admin.riskControl.modeObserveDesc'),
    off: t('admin.riskControl.modeOffDesc'),
  }
  return descriptions[mode] ?? ''
}

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function percentWidth(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
}

function apiKeyRowKey(row: ContentModerationAPIKeyStatus, index: number): string {
  return `${row.configured ? 'saved' : 'test'}-${row.key_hash || index}`
}

function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: t('admin.riskControl.apiKeyStatusOk'),
    error: t('admin.riskControl.apiKeyStatusError'),
    frozen: t('admin.riskControl.apiKeyStatusFrozen'),
    unknown: t('admin.riskControl.apiKeyStatusUnknown'),
  }
  return labels[statusValue] ?? labels.unknown
}

function apiKeyStatusBadgeClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
    error: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300',
    frozen: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300',
    unknown: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusDotClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-500',
    error: 'bg-amber-500',
    frozen: 'bg-red-500',
    unknown: 'bg-gray-400',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
  const parts: string[] = []
  parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 }))
  if (row.last_latency_ms > 0) {
    parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms }))
  }
  if (row.last_http_status > 0) {
    parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status }))
  }
  if (row.frozen_until) {
    parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) }))
  } else if (row.last_checked_at) {
    parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) }))
  } else {
    parts.push(t('admin.riskControl.apiKeyNotTested'))
  }
  return parts.join(' / ')
}

function resetRiskThresholds() {
  configForm.thresholds = { ...riskThresholdDefaults }
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

useRiskControlPolling({
  loadInitial: loadAll,
  refreshStatus: loadStatus,
})
</script>
