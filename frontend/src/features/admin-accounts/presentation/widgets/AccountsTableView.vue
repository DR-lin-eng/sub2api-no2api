<template>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <AccountTableFilters
            v-model:searchQuery="params.search"
            :filters="params"
            :groups="groups"
            @update:filters="(newFilters) => Object.assign(params, newFilters)"
            @change="debouncedReload"
            @update:searchQuery="debouncedReload"
          />
          <AccountTableActions
            :loading="loading"
            @refresh="handleManualRefresh"
            @create="showCreate = true"
          >
            <template #after>
              <!-- Auto Refresh Dropdown -->
              <div class="relative" ref="autoRefreshDropdownRef">
                <button
                  @click="
                    showAutoRefreshDropdown = !showAutoRefreshDropdown;
                    showAccountToolsDropdown = false
                  "
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.accounts.autoRefresh')"
                >
                  <Icon name="refresh" size="sm" :class="[autoRefreshEnabled ? 'animate-spin' : '']" />
                  <span class="hidden md:inline">
                    {{
                      autoRefreshEnabled
                        ? t('admin.accounts.autoRefreshCountdown', { seconds: autoRefreshCountdown })
                        : t('admin.accounts.autoRefresh')
                    }}
                  </span>
                </button>
                <div
                  v-if="showAutoRefreshDropdown"
                  class="absolute right-0 z-50 mt-2 w-56 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
                >
                  <div class="p-2">
                    <button
                      @click="setAutoRefreshEnabled(!autoRefreshEnabled)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ t('admin.accounts.enableAutoRefresh') }}</span>
                      <Icon v-if="autoRefreshEnabled" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>
                    <button
                      v-for="sec in autoRefreshIntervals"
                      :key="sec"
                      @click="setAutoRefreshInterval(sec)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ autoRefreshIntervalLabel(sec) }}</span>
                      <Icon v-if="autoRefreshIntervalSeconds === sec" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- More Tools Dropdown -->
              <div class="relative" ref="accountToolsDropdownRef">
                <button
                  ref="accountToolsTriggerRef"
                  @click="toggleAccountToolsDropdown"
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.accounts.moreActions')"
                  :aria-expanded="showAccountToolsDropdown"
                >
                  <Icon name="more" size="sm" class="md:mr-1.5" />
                  <span class="hidden md:inline">{{ t('admin.accounts.moreActions') }}</span>
                  <Icon name="chevronDown" size="xs" class="ml-1 hidden md:inline" />
                </button>
                <Teleport to="body">
                  <div
                    v-if="showAccountToolsDropdown"
                    class="fixed z-[9999] origin-top-right overflow-hidden rounded-lg border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
                    :style="accountToolsDropdownStyle"
                    @click.stop
                  >
                    <div class="overflow-y-auto p-2" :style="{ maxHeight: `${accountToolsDropdownPosition.maxHeight}px` }">
                      <div class="px-2 py-2">
                        <div class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                          {{ t('admin.accounts.dataActions') }}
                        </div>
                      </div>
                      <button class="account-tools-menu-item" @click="openSyncFromCrs">
                        <span class="account-tools-menu-icon bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
                          <Icon name="sync" size="sm" />
                        </span>
                        <span class="flex-1 text-left">{{ t('admin.accounts.syncFromCrs') }}</span>
                      </button>
                      <button class="account-tools-menu-item" @click="openImportData">
                        <span class="account-tools-menu-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300">
                          <Icon name="upload" size="sm" />
                        </span>
                        <span class="flex-1 text-left">{{ t('admin.accounts.dataImport') }}</span>
                      </button>
                      <button class="account-tools-menu-item" @click="openExportDataDialogFromMenu">
                        <span class="account-tools-menu-icon bg-violet-50 text-violet-600 dark:bg-violet-900/30 dark:text-violet-300">
                          <Icon name="download" size="sm" />
                        </span>
                        <span class="flex-1 text-left">
                          {{ selIds.length ? t('admin.accounts.dataExportSelected') : t('admin.accounts.dataExport') }}
                        </span>
                        <span
                          v-if="selIds.length"
                          class="rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
                        >
                          {{ t('admin.accounts.selectedCount', { count: selIds.length }) }}
                        </span>
                      </button>

                      <div class="my-2 border-t border-gray-100 dark:border-dark-700"></div>
                      <div class="px-2 py-2">
                        <div class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                          {{ t('admin.accounts.toolActions') }}
                        </div>
                      </div>
                      <button class="account-tools-menu-item" @click="openErrorPassthrough">
                        <span class="account-tools-menu-icon bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300">
                          <Icon name="shield" size="sm" />
                        </span>
                        <span class="flex-1 text-left">{{ t('admin.errorPassthrough.title') }}</span>
                      </button>
                      <button class="account-tools-menu-item" @click="openTLSFingerprintProfiles">
                        <span class="account-tools-menu-icon bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-200">
                          <Icon name="lock" size="sm" />
                        </span>
                        <span class="flex-1 text-left">{{ t('admin.tlsFingerprintProfiles.title') }}</span>
                      </button>

                      <div class="my-2 border-t border-gray-100 dark:border-dark-700"></div>
                      <div class="px-2 py-2">
                        <div class="flex items-center justify-between gap-3">
                          <span class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                            {{ t('admin.accounts.viewColumns') }}
                          </span>
                          <Icon name="grid" size="sm" class="text-gray-400" />
                        </div>
                      </div>
                      <div class="grid grid-cols-1 gap-1">
                        <button
                          v-for="col in toggleableColumns"
                          :key="col.key"
                          @click="toggleColumn(col.key)"
                          class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                        >
                          <span class="truncate">{{ col.label }}</span>
                          <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                        </button>
                      </div>
                    </div>
                  </div>
                </Teleport>
              </div>
            </template>
          </AccountTableActions>
        </div>
        <div
          v-if="hasPendingListSync"
          class="mt-2 flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-700/40 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <span>{{ t('admin.accounts.listPendingSyncHint') }}</span>
          <button
            class="btn btn-secondary px-2 py-1 text-xs"
            @click="syncPendingListChanges"
          >
            {{ t('admin.accounts.listPendingSyncAction') }}
          </button>
        </div>
      </template>
      <template #table>
        <AccountBulkActionsBar
          :selected-ids="selIds"
          :total-results="pagination.total"
          :selecting-all="selectingAllResults"
          :all-results-selected="allResultsSelected"
          :querying-upstream-quota="bulkQueryingUpstreamQuota"
          :querying-openai-quota="bulkQueryingOpenAIQuota"
          @delete="handleBulkDelete"
          @reset-status="handleBulkResetStatus"
          @refresh-token="handleBulkRefreshToken"
          @query-upstream-quota="handleBulkQueryUpstreamQuota"
          @query-openai-quota="handleBulkQueryOpenAIQuota"
          @probe-upstream-billing="handleBulkProbeUpstreamBilling"
          @edit-selected="openBulkEditSelected"
          @edit-filtered="openBulkEditFiltered"
          @clear="clearSelection"
          @select-page="selectPage"
          @select-all-results="handleSelectAllResults"
          @toggle-schedulable="handleBulkToggleSchedulable"
        />
        <div ref="accountTableRef" class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <DataTable
          ref="dataTableRef"
          :columns="cols"
          :data="accounts"
          :loading="loading"
          row-key="id"
          :server-side-sort="true"
          @sort="handleSort"
          default-sort-key="name"
          default-sort-order="asc"
          :sort-storage-key="ACCOUNT_SORT_STORAGE_KEY"
          :estimate-row-height="156"
          :overscan="5"
          :virtualize-threshold="50"
        >
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allVisibleSelected"
              @click.stop
              @change="toggleSelectAllVisible($event)"
            />
          </template>
          <template #cell-select="{ row }">
            <input type="checkbox" :checked="isSelected(row.id)" @change="toggleSel(row.id)" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </template>
          <template #cell-id="{ value }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ value }}</span>
          </template>
          <template #cell-name="{ row, value }">
            <div class="flex flex-col">
              <HelpTooltip
                v-if="accountHomepageUrl(row)"
                :content="accountHomepageUrl(row)"
                width-class="w-max max-w-sm break-all"
                class="-ml-1 self-start"
              >
                <template #trigger>
                  <a
                    :href="accountHomepageUrl(row)"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="border-b border-dotted border-gray-300 font-medium text-gray-900 dark:border-dark-600 dark:text-white"
                  >
                    {{ value }}
                  </a>
                </template>
              </HelpTooltip>
              <span v-else class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <span
                v-if="accountDisplayEmail(row)"
                class="text-xs text-gray-500 dark:text-gray-400 truncate max-w-[200px]"
                :title="accountDisplayEmail(row) + (row.parent_chatgpt_account_id ? ' · ' + row.parent_chatgpt_account_id : '')"
              >
                {{ accountDisplayEmail(row) }}
              </span>
            </div>
          </template>
          <template #cell-notes="{ value }">
            <span v-if="value" :title="value" class="block max-w-xs truncate text-sm text-gray-600 dark:text-gray-300">{{ value }}</span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-platform_type="{ row }">
            <div class="flex min-w-0 flex-col gap-1">
              <div class="flex flex-wrap items-center gap-1">
                <PlatformTypeBadge :platform="row.platform" :type="row.type"
                  :auth-mode="getOpenAIAuthMode(row)"
                  :plan-type="getAccountPlanType(row)"
                  :privacy-mode="row.extra?.privacy_mode || row.parent_privacy_mode"
                  :subscription-expires-at="row.credentials?.subscription_expires_at || row.parent_subscription_expires_at" />
                <span
                  v-if="getAntigravityTierLabel(row)"
                  :class="['inline-block rounded px-1.5 py-0.5 text-[10px] font-medium', getAntigravityTierClass(row)]"
                >
                  {{ getAntigravityTierLabel(row) }}
                </span>
              </div>
              <div
                v-if="getOpenAICompactMeta(row)"
                :class="[
                  'inline-flex items-center gap-1.5 pl-0.5 text-[11px] font-medium leading-4',
                  getOpenAICompactMeta(row)?.className
                ]"
                :title="getOpenAICompactTitle(row)"
              >
                <span :class="['h-1.5 w-1.5 rounded-full', getOpenAICompactMeta(row)?.dotClass]" />
                <span>{{ getOpenAICompactMeta(row)?.label }}</span>
              </div>
            </div>
          </template>
          <template #cell-capacity="{ row }">
            <AccountCapacityCell :account="row" />
          </template>
          <template #cell-status="{ row }">
            <div class="flex flex-wrap items-center gap-1.5">
              <AccountStatusIndicator :account="row" @show-temp-unsched="handleShowTempUnsched" />
              <div
                v-if="row.stream_degraded"
                class="flex flex-col items-start gap-0.5"
                :title="t('admin.accounts.streamDegradedTip', { level: row.stream_degradation_level ?? 1, nextProbe: row.stream_next_probe_at ? formatDateTime(row.stream_next_probe_at) : '-' })"
              >
                <span class="inline-flex items-center rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
                  {{ t('admin.accounts.streamDegraded', { level: row.stream_degradation_level ?? 1 }) }}
                </span>
                <span class="text-[10px] leading-4 text-amber-700 dark:text-amber-300">
                  {{ t('admin.accounts.streamNextProbe', { nextProbe: row.stream_next_probe_at ? formatDateTime(row.stream_next_probe_at) : '-' }) }}
                </span>
              </div>
            </div>
          </template>
          <template #cell-schedulable="{ row }">
            <button @click="handleToggleSchedulable(row)" :disabled="togglingSchedulable === row.id" class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus:ring-offset-dark-800" :class="[row.schedulable ? 'bg-primary-500 hover:bg-primary-600' : 'bg-gray-200 hover:bg-gray-300 dark:bg-dark-600 dark:hover:bg-dark-500']" :title="row.schedulable ? t('admin.accounts.schedulableEnabled') : t('admin.accounts.schedulableDisabled')">
              <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out" :class="[row.schedulable ? 'translate-x-4' : 'translate-x-0']" />
            </button>
          </template>
          <template #cell-today_stats="{ row }">
            <AccountTodayStatsCell
              :stats="todayStatsByAccountId[String(row.id)] ?? null"
              :loading="todayStatsLoading"
              :error="todayStatsError"
            />
          </template>
          <template #header-hourly_usage="{ column }">
            <div class="flex items-center">
              <span>{{ column.label }}</span>
              <HelpTooltip :content="t('admin.accounts.hourlyUsageHint')" width-class="w-80" />
            </div>
          </template>
          <template #cell-hourly_usage="{ row }">
            <AccountHourlyUsageCell :stats="row.hourly_usage" />
          </template>
          <template #cell-groups="{ row }">
            <AccountGroupsCell :groups="row.groups" :max-display="4" />
          </template>
          <template #header-usage="{ column }">
            <div class="flex items-center">
              <span>{{ column.label }}</span>
              <HelpTooltip :content="t('admin.accounts.usageWindowsHint')" width-class="w-72" />
            </div>
          </template>
          <template #cell-usage="{ row }">
            <AccountUsageCell
              :account="row"
              :today-stats="todayStatsByAccountId[String(row.id)] ?? null"
              :today-stats-loading="todayStatsLoading"
              :manual-refresh-token="usageManualRefreshToken"
              :bulk-openai-quota-result="bulkOpenAIQuotaResults.get(row.id)"
              :upstream-quota-result="upstreamQuotaResults.get(row.id)"
              :now="upstreamBillingNow"
              @account-updated="handleAccountUpdated"
            />
          </template>
          <template #cell-proxy="{ row }">
            <div class="flex flex-col gap-1">
              <div v-if="row.proxy" class="flex items-center gap-2">
                <span class="text-sm text-gray-700 dark:text-gray-300">{{ row.proxy.name }}</span>
                <span v-if="row.proxy.country_code" class="text-xs text-gray-500 dark:text-gray-400">
                  ({{ row.proxy.country_code }})
                </span>
              </div>
              <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
              <div v-if="row.proxy && row.proxy.expires_at" class="flex items-center gap-2 text-xs">
                <span class="text-gray-600 dark:text-gray-300">{{ formatDateTime(row.proxy.expires_at) }}</span>
                <span :class="proxyExpiryBadge(row.proxy)">{{ proxyExpiryText(row.proxy) }}</span>
              </div>
              <div v-if="row.proxy_fallback_origin_id" class="flex items-center gap-1">
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200" :title="t('admin.accounts.fallbackActiveTip', { origin: row.proxy_fallback_origin_name })">
                  {{ t('admin.accounts.fallbackActive') }}
                </span>
                <button class="text-xs px-1.5 py-0.5 rounded border border-gray-300 dark:border-dark-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-dark-700" @click="onRevertFallback(row)">{{ t('admin.accounts.revertProxy') }}</button>
              </div>
            </div>
          </template>
          <template #cell-rate_multiplier="{ row }">
            <span class="inline-flex items-center gap-1 text-sm font-mono text-gray-700 dark:text-gray-300">
              <span>{{ formatMultiplier(row.rate_multiplier ?? 1) }}x</span>
              <span
                v-if="row.extra?.upstream_billing_rate_sync_enabled === true"
                class="inline-flex cursor-help text-emerald-600 dark:text-emerald-400"
                :aria-label="t('admin.accounts.upstreamBilling.syncedRateTooltip')"
                :title="t('admin.accounts.upstreamBilling.syncedRateTooltip')"
                data-testid="account-rate-sync-indicator"
              >
                <Icon name="sync" size="xs" />
              </span>
            </span>
          </template>
          <template #header-upstream_billing_rate="{ column }">
            <div class="flex items-center gap-1">
              <span>{{ column.label }}</span>
              <span @click.stop>
                <HelpTooltip :content="t('admin.accounts.upstreamBilling.trustWarning')" width-class="w-80" />
              </span>
            </div>
          </template>
          <template #cell-upstream_billing_rate="{ row }">
            <UpstreamBillingRateCell
              :account="row"
              :global-probe-enabled="upstreamBillingProbeGloballyEnabled"
              :now="upstreamBillingNow"
              :probing="probingUpstreamBilling.has(row.id)"
              :quota-result="upstreamQuotaResults.get(row.id)"
              :quota-error="upstreamQuotaErrors.get(row.id)"
              :quota-loading="queryingUpstreamQuota.has(row.id)"
              :rate-feedback="upstreamBillingFeedback.get(row.id)"
              :quota-feedback="upstreamQuotaFeedback.get(row.id)"
              @probe="handleProbeUpstreamBilling(row)"
              @query-quota="handleQueryUpstreamQuota(row)"
            />
          </template>
          <template #cell-priority="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value }}</span>
          </template>
          <template #header-scheduler_score="{ column }">
            <div class="flex items-center">
              <span>{{ column.label }}</span>
              <HelpTooltip :content="t('admin.accounts.schedulerScore.hint')" width-class="w-80" />
            </div>
          </template>
          <template #cell-scheduler_score="{ row }">
            <div v-if="getSchedulerScoreRows(row).length" class="flex min-w-[7rem] flex-col gap-0.5 font-mono text-[11px] leading-4">
              <div
                v-for="score in getSchedulerScoreRows(row)"
                :key="String(score.group_id)"
                class="flex items-center gap-1 whitespace-nowrap text-gray-700 dark:text-gray-300"
                :title="`${formatSchedulerScoreGroup(score)} / ${formatSchedulerScore(score.base_score)} / ${formatStickySchedulerScore(score)}`"
              >
                <span class="max-w-[4.75rem] truncate text-gray-500 dark:text-dark-400">{{ formatSchedulerScoreGroup(score) }}</span>
                <span class="text-gray-300 dark:text-gray-600">/</span>
                <span>{{ formatSchedulerScore(score.base_score) }}</span>
                <span class="text-gray-300 dark:text-gray-600">/</span>
                <span class="text-primary-700 dark:text-primary-300">{{ formatStickySchedulerScore(score) }}</span>
              </div>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-last_used_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatRelativeTime(value) }}</span>
          </template>
          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>
          <template #cell-expires_at="{ row, value }">
            <div class="flex flex-col items-start gap-1">
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatExpiresAt(value) }}</span>
              <div v-if="isExpired(value) || (row.auto_pause_on_expired && value)" class="flex items-center gap-1">
                <span
                  v-if="isExpired(value)"
                  class="inline-flex items-center rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                >
                  {{ t('admin.accounts.expired') }}
                </span>
                <span
                  v-if="row.auto_pause_on_expired && value"
                  class="inline-flex items-center rounded-md bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                >
                  {{ t('admin.accounts.autoPauseOnExpired') }}
                </span>
              </div>
            </div>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button @click="handleEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" /></svg>
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <button @click="handleDelete(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" /></svg>
                <span class="text-xs">{{ t('common.delete') }}</span>
              </button>
              <button @click="openMenu(row, $event)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM12.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM18.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0z" /></svg>
                <span class="text-xs">{{ t('common.more') }}</span>
              </button>
            </div>
          </template>
        </DataTable>
        </div>
      </template>
      <template #pagination><Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" /></template>
    </TablePageLayout>

</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue'
import DataTable from '@/common/widgets/data/DataTable.vue'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import AccountTableActions from '@/features/admin-accounts/presentation/widgets/AccountTableActions.vue'
import AccountTableFilters from '@/features/admin-accounts/presentation/widgets/AccountTableFilters.vue'
import AccountBulkActionsBar from '@/features/admin-accounts/presentation/widgets/AccountBulkActionsBar.vue'
import AccountStatusIndicator from '@/features/admin-accounts/presentation/widgets/AccountStatusIndicator.vue'
import AccountUsageCell from '@/features/admin-accounts/presentation/widgets/AccountUsageCell.vue'
import AccountTodayStatsCell from '@/features/admin-accounts/presentation/widgets/AccountTodayStatsCell.vue'
import AccountHourlyUsageCell from '@/features/admin-accounts/presentation/widgets/AccountHourlyUsageCell.vue'
import AccountGroupsCell from '@/features/admin-accounts/presentation/widgets/AccountGroupsCell.vue'
import AccountCapacityCell from '@/features/admin-accounts/presentation/widgets/AccountCapacityCell.vue'
import UpstreamBillingRateCell from '@/features/admin-accounts/presentation/widgets/UpstreamBillingRateCell.vue'
import PlatformTypeBadge from '@/common/widgets/icons/PlatformTypeBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { formatDateTime, formatRelativeTime } from '@/core/utils/format'
import { formatMultiplier } from '@/core/utils/formatters'
import type { AccountTableViewContext } from '@/features/admin-accounts/presentation/accountTableViewContext'

const { t } = useI18n()
const props = defineProps<{ context: AccountTableViewContext }>()
const {
  params,
  groups,
  loading,
  debouncedReload,
  handleManualRefresh,
  showCreate,
  autoRefreshDropdownRef,
  showAutoRefreshDropdown,
  showAccountToolsDropdown,
  autoRefreshEnabled,
  autoRefreshCountdown,
  autoRefreshIntervals,
  autoRefreshIntervalSeconds,
  autoRefreshIntervalLabel,
  setAutoRefreshEnabled,
  setAutoRefreshInterval,
  accountToolsDropdownRef,
  accountToolsTriggerRef,
  accountToolsDropdownStyle,
  accountToolsDropdownPosition,
  toggleAccountToolsDropdown,
  openSyncFromCrs,
  openImportData,
  openExportDataDialogFromMenu,
  openErrorPassthrough,
  openTLSFingerprintProfiles,
  toggleableColumns,
  toggleColumn,
  isColumnVisible,
  hasPendingListSync,
  syncPendingListChanges,
  selIds,
  selectingAllResults,
  allResultsSelected,
  bulkQueryingUpstreamQuota,
  bulkQueryingOpenAIQuota,
  handleBulkDelete,
  handleBulkResetStatus,
  handleBulkRefreshToken,
  handleBulkQueryUpstreamQuota,
  handleBulkQueryOpenAIQuota,
  handleBulkProbeUpstreamBilling,
  openBulkEditSelected,
  openBulkEditFiltered,
  clearSelection,
  selectPage,
  handleSelectAllResults,
  handleBulkToggleSchedulable,
  accountTableRef,
  dataTableRef,
  cols,
  accounts,
  accountSortStorageKey: ACCOUNT_SORT_STORAGE_KEY,
  handleSort,
  pagination,
  handlePageChange,
  handlePageSizeChange,
  allVisibleSelected,
  toggleSelectAllVisible,
  isSelected,
  toggleSel,
  accountHomepageUrl,
  accountDisplayEmail,
  getOpenAIAuthMode,
  getAccountPlanType,
  getAntigravityTierLabel,
  getAntigravityTierClass,
  getOpenAICompactMeta,
  getOpenAICompactTitle,
  handleShowTempUnsched,
  togglingSchedulable,
  handleToggleSchedulable,
  handleAccountUpdated,
  todayStatsByAccountId,
  todayStatsLoading,
  todayStatsError,
  usageManualRefreshToken,
  bulkOpenAIQuotaResults,
  upstreamQuotaResults,
  upstreamBillingNow,
  upstreamBillingProbeGloballyEnabled,
  probingUpstreamBilling,
  upstreamQuotaErrors,
  queryingUpstreamQuota,
  upstreamBillingFeedback,
  upstreamQuotaFeedback,
  handleProbeUpstreamBilling,
  handleQueryUpstreamQuota,
  proxyExpiryBadge,
  proxyExpiryText,
  onRevertFallback,
  getSchedulerScoreRows,
  formatSchedulerScoreGroup,
  formatSchedulerScore,
  formatStickySchedulerScore,
  formatExpiresAt,
  isExpired,
  handleEdit,
  handleDelete,
  openMenu
} = props.context
</script>

<style scoped>
.account-tools-menu-item {
  @apply flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700;
}

.account-tools-menu-icon {
  @apply inline-flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md;
}
</style>
