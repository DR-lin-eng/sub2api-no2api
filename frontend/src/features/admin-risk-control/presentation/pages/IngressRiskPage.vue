<template>
  <AppLayout>
    <div class="space-y-6 pb-10" data-test="ingress-risk-view">
      <header class="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div class="min-w-0">
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.title') }}
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.description') }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-3">
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ lastUpdatedLabel }}
          </span>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="refreshing"
            data-test="refresh"
            @click="refreshAll"
          >
            <Icon name="refresh" size="sm" :class="refreshing && 'animate-spin'" />
            {{ t('admin.ingressRisk.actions.refresh') }}
          </button>
        </div>
      </header>

      <div
        v-if="healthError"
        class="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
        role="alert"
      >
        <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0" />
        <span>{{ healthError }}</span>
      </div>

      <section
        class="rounded-lg border p-4 sm:p-5"
        :class="healthBandClass"
        data-test="health-band"
      >
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex min-w-0 items-start gap-3">
            <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg" :class="healthIconClass">
              <Icon :name="healthIcon" size="md" :stroke-width="2" />
            </div>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.ingressRisk.health.title') }}
                </h3>
                <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="healthBadgeClass">
                  {{ t(`admin.ingressRisk.health.${overallHealth}`) }}
                </span>
              </div>
              <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">
                {{ t(`admin.ingressRisk.health.${overallHealth}Description`) }}
              </p>
              <p v-if="healthLastError" class="mt-1 break-all font-mono text-xs text-red-700 dark:text-red-300">
                {{ healthLastError }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:min-w-[540px]">
            <div
              v-for="signal in healthSignals"
              :key="signal.key"
              class="rounded-lg border border-black/5 bg-white/60 px-3 py-2.5 dark:border-white/10 dark:bg-dark-900/45"
            >
              <div class="flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">
                <span class="h-2 w-2 rounded-full" :class="signalDotClass(signal.level)"></span>
                {{ signal.label }}
              </div>
              <div class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">
                {{ signal.value }}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section>
        <div class="mb-3 flex flex-col justify-between gap-1 sm:flex-row sm:items-end">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.metrics.title') }}
          </h3>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.metrics.cumulativeHint') }}
          </p>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <div v-for="metric in metricCards" :key="metric.key" class="card p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
                <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white" :data-test="`metric-${metric.key}`">
                  {{ metric.value }}
                </p>
                <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ metric.hint }}</p>
              </div>
              <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg" :class="metric.iconClass">
                <Icon :name="metric.icon" size="md" :stroke-width="2" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="card overflow-hidden" data-test="cloudflare-edge">
        <div class="flex flex-col justify-between gap-3 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:px-5">
          <div class="flex min-w-0 items-start gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-orange-100 text-orange-700 dark:bg-orange-900/35 dark:text-orange-300">
              <Icon name="cloud" size="md" :stroke-width="2" />
            </div>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.ingressRisk.cloudflare.title') }}
                </h3>
                <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="cloudflareBadgeClass">
                  {{ cloudflareStatusLabel(t, cloudflareStatus) }}
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
                {{ cloudflareStatusDescription(t, cloudflareStatus) }}
              </p>
            </div>
          </div>
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ cloudflareLastSuccess }}
          </span>
        </div>

        <div class="grid grid-cols-2 divide-x divide-y divide-gray-200 dark:divide-dark-700 sm:grid-cols-3 xl:grid-cols-6 xl:divide-y-0">
          <div v-for="metric in cloudflareMetrics" :key="metric.key" class="min-w-0 px-4 py-4 sm:px-5">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
            <p class="mt-2 truncate text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="metric.value">
              {{ metric.value }}
            </p>
          </div>
        </div>

        <div
          v-if="cloudflareMode === 'waf_custom_rules' && cloudflareWAFHealth"
          class="border-t border-gray-200 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-900/30"
          data-test="cloudflare-waf-analytics"
        >
          <div class="flex flex-col justify-between gap-1 px-4 pt-4 sm:flex-row sm:items-end sm:px-5">
            <div>
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.ingressRisk.cloudflare.waf.title') }}
              </h4>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.ingressRisk.cloudflare.waf.description') }}
              </p>
            </div>
            <span v-if="cloudflareWAFHealth.last_synced_at" class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.ingressRisk.cloudflare.waf.lastSynced', { time: formatDateTime(cloudflareWAFHealth.last_synced_at) }) }}
            </span>
          </div>
          <div class="mt-3 grid grid-cols-2 divide-x divide-y divide-gray-200 border-t border-gray-200 dark:divide-dark-700 dark:border-dark-700 sm:grid-cols-3 xl:grid-cols-6 xl:divide-y-0">
            <div v-for="metric in wafAnalyticsMetrics" :key="metric.key" class="min-w-0 px-4 py-3 sm:px-5">
              <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
              <p class="mt-1.5 truncate text-base font-semibold tabular-nums text-gray-900 dark:text-white" :title="metric.value">
                {{ metric.value }}
              </p>
            </div>
          </div>
          <div
            v-if="cloudflareWAFHostnameStats.length > 1"
            class="border-t border-gray-200 px-4 py-3 dark:border-dark-700 sm:px-5"
            data-test="cloudflare-waf-hostname-stats"
          >
            <div class="grid grid-cols-[minmax(0,1fr)_auto_auto] gap-x-4 border-b border-gray-200 pb-2 text-xs font-medium text-gray-500 dark:border-dark-700 dark:text-dark-400">
              <span>{{ t('admin.ingressRisk.cloudflare.waf.hostname') }}</span>
              <span class="text-right">{{ t('admin.ingressRisk.cloudflare.waf.requests') }}</span>
              <span class="text-right">{{ t('admin.ingressRisk.cloudflare.waf.blocks') }}</span>
            </div>
            <div
              v-for="item in cloudflareWAFHostnameStats"
              :key="item.hostname"
              class="grid grid-cols-[minmax(0,1fr)_auto_auto] gap-x-4 border-b border-gray-100 py-2 text-sm last:border-b-0 dark:border-dark-800"
            >
              <span class="truncate font-mono text-gray-700 dark:text-dark-200" :title="item.hostname">{{ item.hostname }}</span>
              <span class="min-w-20 text-right tabular-nums text-gray-900 dark:text-white">{{ formatNumber(item.requests_24h) }}</span>
              <span class="min-w-20 text-right tabular-nums text-gray-900 dark:text-white">{{ formatNumber(item.blocked_requests_24h) }}</span>
            </div>
          </div>
          <div
            v-if="cloudflareWAFHealth.analytics_error"
            class="border-t border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-300 sm:px-5"
            role="status"
          >
            {{ cloudflareWAFHealth.analytics_error }}
          </div>
        </div>

        <div
          v-if="cloudflareHealth?.last_error"
          class="border-t border-red-200 bg-red-50 px-4 py-3 font-mono text-xs text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300 sm:px-5"
          role="alert"
        >
          {{ cloudflareHealth.last_error }}
        </div>

        <div class="border-t border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-5" data-test="cloudflare-settings">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.ingressRisk.cloudflare.settings.title') }}
              </h4>
              <p class="mt-1 truncate text-xs text-gray-500 dark:text-dark-400" :title="cloudflareSettingsSummary">
                {{ cloudflareSettingsSummary }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">
                {{ t('admin.ingressRisk.cloudflare.settings.enabled') }}
              </span>
              <Toggle
                v-model="cloudflareForm.enabled"
                :disabled="cloudflareSettingsLoading || cloudflareSettingsSaving || !cloudflareSettings"
                data-test="cloudflare-enabled"
                @update:model-value="cloudflareSettingsExpanded = true"
              />
              <button
                type="button"
                class="btn btn-secondary ml-1 h-9 px-3"
                :aria-expanded="cloudflareSettingsExpanded"
                data-test="toggle-cloudflare-settings"
                @click="cloudflareSettingsExpanded = !cloudflareSettingsExpanded"
              >
                <Icon
                  name="chevronDown"
                  size="sm"
                  class="transition-transform"
                  :class="cloudflareSettingsExpanded && 'rotate-180'"
                />
                {{ t(cloudflareSettingsExpanded
                  ? 'admin.ingressRisk.cloudflare.settings.hideSettings'
                  : 'admin.ingressRisk.cloudflare.settings.showSettings') }}
              </button>
            </div>
          </div>

          <div v-if="cloudflareSettingsExpanded && cloudflareSettingsLoading" class="mt-4 flex items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
            <Icon name="refresh" size="sm" class="animate-spin" />
            {{ t('admin.ingressRisk.cloudflare.settings.loading') }}
          </div>

          <template v-else-if="cloudflareSettingsExpanded && cloudflareSettings">
            <div
              v-if="cloudflareSettingsError || cloudflareSettingsSaved"
              class="mt-4 rounded-md border px-3 py-2 text-sm"
              :class="cloudflareSettingsError
                ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300'
                : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/25 dark:text-emerald-300'"
              role="status"
            >
              {{ cloudflareSettingsError || cloudflareSettingsSaved }}
            </div>

            <div
              v-if="cloudflareForm.enabled && authHealth && !authHealth.invalid_abuse.enabled"
              class="mt-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-300"
              role="alert"
            >
              {{ t('admin.ingressRisk.cloudflare.settings.localLimiterRequired') }}
            </div>

            <fieldset class="mt-4" :disabled="cloudflareCredentialsLocked || cloudflareSettingsSaving">
              <legend class="input-label">
                {{ t('admin.ingressRisk.cloudflare.settings.mode') }}
              </legend>
              <div class="grid grid-cols-2 gap-2" role="radiogroup">
                <label
                  v-for="option in cloudflareModeOptions"
                  :key="option.value"
                  class="cursor-pointer rounded-md border px-3 py-2.5 text-center transition-colors"
                  :class="cloudflareForm.mode === option.value
                    ? 'border-primary-500 bg-primary-50 text-primary-800 dark:border-primary-500 dark:bg-primary-950/30 dark:text-primary-200'
                    : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200 dark:hover:border-dark-600'"
                >
                  <input
                    v-model="cloudflareForm.mode"
                    type="radio"
                    name="cloudflare-ingress-mode"
                    :value="option.value"
                    class="sr-only"
                    :data-test="`cloudflare-mode-${option.value}`"
                  />
                  <span class="block text-sm font-semibold">{{ option.label }}</span>
                </label>
              </div>
            </fieldset>

            <div class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
              <div>
                <label class="input-label" for="cloudflare-zone-id">
                  {{ t('admin.ingressRisk.cloudflare.settings.zoneId') }}
                </label>
                <input
                  id="cloudflare-zone-id"
                  v-model.trim="cloudflareForm.zone_id"
                  type="text"
                  maxlength="32"
                  class="input font-mono"
                  :disabled="cloudflareCredentialsLocked || cloudflareSettingsSaving"
                  :placeholder="t('admin.ingressRisk.cloudflare.settings.zoneIdPlaceholder')"
                  data-test="cloudflare-zone-id"
                />
              </div>

              <div>
                <div class="mb-1.5 flex flex-wrap items-center justify-between gap-2">
                  <label class="text-sm font-medium text-gray-700 dark:text-dark-200" for="cloudflare-api-token">
                    {{ t('admin.ingressRisk.cloudflare.settings.apiToken') }}
                  </label>
                  <span
                    class="rounded-full px-2 py-0.5 text-xs font-semibold"
                    :class="cloudflareSettings.api_token_configured
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/35 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'"
                  >
                    {{ cloudflareTokenStatusLabel(t, cloudflareSettings.api_token_configured) }}
                  </span>
                </div>
                <input
                  id="cloudflare-api-token"
                  v-model="cloudflareForm.api_token"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono"
                  :disabled="cloudflareCredentialsLocked || cloudflareSettingsSaving"
                  :placeholder="t('admin.ingressRisk.cloudflare.settings.apiTokenPlaceholder')"
                  data-test="cloudflare-api-token"
                />
              </div>
            </div>

            <div
              v-if="cloudflareForm.mode === 'waf_custom_rules'"
              class="mt-4 grid grid-cols-1 gap-4 border-t border-gray-100 pt-4 dark:border-dark-700 lg:grid-cols-2"
            >
              <div>
                <label class="input-label" for="cloudflare-waf-hostname">
                  {{ t('admin.ingressRisk.cloudflare.settings.wafHostname') }}
                </label>
                <textarea
                  id="cloudflare-waf-hostname"
                  v-model="cloudflareForm.waf_hostnames_text"
                  rows="3"
                  class="input min-h-20 resize-y font-mono"
                  :disabled="cloudflareCredentialsLocked || cloudflareSettingsSaving"
                  :placeholder="t('admin.ingressRisk.cloudflare.settings.wafHostnamePlaceholder')"
                  data-test="cloudflare-waf-hostname"
                ></textarea>
              </div>
              <div>
                <label class="input-label" for="cloudflare-waf-rule-ids">
                  {{ t('admin.ingressRisk.cloudflare.settings.wafRuleIds') }}
                </label>
                <textarea
                  id="cloudflare-waf-rule-ids"
                  v-model="cloudflareForm.waf_rule_ids_text"
                  rows="4"
                  class="input min-h-24 resize-y font-mono"
                  :disabled="cloudflareCredentialsLocked || cloudflareSettingsSaving"
                  :placeholder="t('admin.ingressRisk.cloudflare.settings.wafRuleIdsPlaceholder')"
                  data-test="cloudflare-waf-rule-ids"
                ></textarea>
              </div>
            </div>

            <p v-if="cloudflareCredentialsLocked" class="mt-2 text-xs leading-5 text-amber-700 dark:text-amber-300">
              {{ t('admin.ingressRisk.cloudflare.settings.credentialsLocked') }}
            </p>

            <button
              type="button"
              class="mt-4 flex w-full items-center justify-between border-t border-gray-100 pt-3 text-sm font-medium text-gray-700 dark:border-dark-700 dark:text-dark-200"
              :aria-expanded="cloudflareAdvancedExpanded"
              data-test="toggle-cloudflare-advanced"
              @click="cloudflareAdvancedExpanded = !cloudflareAdvancedExpanded"
            >
              <span>{{ t('admin.ingressRisk.cloudflare.settings.advanced') }}</span>
              <Icon
                name="chevronDown"
                size="sm"
                class="transition-transform"
                :class="cloudflareAdvancedExpanded && 'rotate-180'"
              />
            </button>

            <div v-if="cloudflareAdvancedExpanded" class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
              <div>
                <label class="input-label" for="cloudflare-timeout">
                  {{ t('admin.ingressRisk.cloudflare.settings.requestTimeout') }}
                </label>
                <input
                  id="cloudflare-timeout"
                  v-model.number="cloudflareForm.request_timeout_seconds"
                  type="number"
                  min="1"
                  max="30"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
              <div v-if="cloudflareForm.mode === 'waf_custom_rules'">
                <label class="input-label" for="cloudflare-waf-sync-interval">
                  {{ t('admin.ingressRisk.cloudflare.settings.wafSyncInterval') }}
                </label>
                <input
                  id="cloudflare-waf-sync-interval"
                  v-model.number="cloudflareForm.waf_sync_interval_seconds"
                  type="number"
                  min="5"
                  max="300"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
              <div v-if="cloudflareForm.mode === 'waf_custom_rules'">
                <label class="input-label" for="cloudflare-analytics-interval">
                  {{ t('admin.ingressRisk.cloudflare.settings.analyticsInterval') }}
                </label>
                <input
                  id="cloudflare-analytics-interval"
                  v-model.number="cloudflareForm.analytics_interval_seconds"
                  type="number"
                  min="60"
                  max="3600"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
              <div>
                <label class="input-label" for="cloudflare-queue-capacity">
                  {{ t('admin.ingressRisk.cloudflare.settings.queueCapacity') }}
                </label>
                <input
                  id="cloudflare-queue-capacity"
                  v-model.number="cloudflareForm.queue_capacity"
                  type="number"
                  min="16"
                  max="100000"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
              <div>
                <label class="input-label" for="cloudflare-rule-limit">
                  {{ t(cloudflareForm.mode === 'waf_custom_rules'
                    ? 'admin.ingressRisk.cloudflare.settings.maxActiveEntries'
                    : 'admin.ingressRisk.cloudflare.settings.maxActiveRules') }}
                </label>
                <input
                  id="cloudflare-rule-limit"
                  v-model.number="cloudflareForm.max_active_rules"
                  type="number"
                  min="1"
                  max="50000"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
              <div>
                <label class="input-label" for="cloudflare-reconcile-interval">
                  {{ t('admin.ingressRisk.cloudflare.settings.reconcileInterval') }}
                </label>
                <input
                  id="cloudflare-reconcile-interval"
                  v-model.number="cloudflareForm.reconcile_interval_seconds"
                  type="number"
                  min="30"
                  max="3600"
                  class="input"
                  :disabled="cloudflareSettingsSaving"
                />
              </div>
            </div>

            <div class="mt-4 flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="cloudflareSettingsSaving || Boolean(cloudflareForm.enabled && authHealth && !authHealth.invalid_abuse.enabled)"
                data-test="save-cloudflare-settings"
                @click="saveCloudflareSettings"
              >
                <Icon :name="cloudflareSettingsSaving ? 'refresh' : 'check'" size="sm" :class="cloudflareSettingsSaving && 'animate-spin'" />
                {{ t(cloudflareSettingsSaving ? 'admin.ingressRisk.cloudflare.settings.saving' : 'admin.ingressRisk.cloudflare.settings.save') }}
              </button>
            </div>
          </template>

          <div
            v-else-if="cloudflareSettingsError"
            class="mt-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
            role="alert"
          >
            {{ cloudflareSettingsError }}
          </div>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-5">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.runtime.title') }}
          </h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.runtime.currentHint') }}
          </p>
        </div>
        <div class="grid grid-cols-1 divide-y divide-gray-200 dark:divide-dark-700 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-3">
          <div v-for="indicator in runtimeIndicators" :key="indicator.key" class="min-w-0 px-4 py-4 sm:px-5">
            <div class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-dark-400">
              <Icon :name="indicator.icon" size="sm" />
              {{ indicator.label }}
            </div>
            <div class="mt-2 truncate text-lg font-semibold text-gray-900 dark:text-white" :title="indicator.value">
              {{ indicator.value }}
            </div>
            <div class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
              {{ indicator.detail }}
            </div>
          </div>
        </div>
      </section>

      <section class="card p-4 sm:p-5">
        <div class="mb-4 flex items-center gap-2">
          <Icon name="filter" size="sm" class="text-gray-500 dark:text-dark-400" />
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.filters.title') }}
          </h3>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7">
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.timeRange') }}</label>
            <Select v-model="filters.time_range" :options="timeRangeOptions" :searchable="false" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.reason') }}</label>
            <Select v-model="filters.reason" :options="reasonOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.routeFamily') }}</label>
            <Select v-model="filters.route_family" :options="routeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.protocol') }}</label>
            <Select v-model="filters.protocol" :options="protocolOptions" :searchable="false" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.clientIp') }}</label>
            <input
              v-model.trim="filters.client_ip"
              type="text"
              class="input font-mono"
              :placeholder="t('admin.ingressRisk.filters.clientIpPlaceholder')"
              @keyup.enter="search"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.userId') }}</label>
            <input v-model="filters.user_id" type="number" min="1" class="input" @keyup.enter="search" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.apiKeyId') }}</label>
            <input v-model="filters.api_key_id" type="number" min="1" class="input" @keyup.enter="search" />
          </div>
        </div>
        <div class="mt-4 flex flex-wrap justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="recordsLoading" @click="resetFilters">
            {{ t('admin.ingressRisk.actions.reset') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="recordsLoading" data-test="search" @click="search">
            <Icon name="search" size="sm" />
            {{ t('admin.ingressRisk.actions.search') }}
          </button>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex flex-col justify-between gap-2 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:flex-row sm:items-end sm:px-5">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.ingressRisk.table.title') }}
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.ingressRisk.table.summary', { total: formatNumber(total), requests: formatNumber(currentPageRequests) }) }}
            </p>
          </div>
          <span class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300">
            {{ t(`admin.ingressRisk.timeRanges.${filters.time_range}`) }}
          </span>
        </div>

        <div
          v-if="recordsError"
          class="border-b border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
          role="alert"
        >
          {{ recordsError }}
        </div>

        <DataTable :columns="columns" :data="records" :loading="recordsLoading" row-key="id" :sticky-first-column="false">
          <template #cell-bucket_start="{ value }">
            <span class="whitespace-nowrap text-gray-600 dark:text-dark-300">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-reject_reason="{ value }">
            <span class="inline-flex rounded-md px-2 py-1 text-xs font-semibold" :class="reasonBadgeClass(value)" :title="value">
              {{ reasonLabel(value) }}
            </span>
          </template>

          <template #cell-route="{ row }">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">{{ routeLabel(row.route_family) }}</div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ protocolLabel(row.protocol) }}</div>
            </div>
          </template>

          <template #cell-client_ip="{ value }">
            <div class="flex items-center gap-2">
              <span class="font-mono text-gray-700 dark:text-dark-200">{{ value }}</span>
              <button
                type="button"
                class="btn-ghost btn-icon h-7 w-7 shrink-0"
                :title="t('admin.ingressRisk.actions.filterIp', { ip: value })"
                :aria-label="t('admin.ingressRisk.actions.filterIp', { ip: value })"
                @click.stop="filterByIp(value)"
              >
                <Icon name="filter" size="xs" />
              </button>
            </div>
          </template>

          <template #cell-request_count="{ value }">
            <span class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatNumber(value) }}</span>
          </template>

          <template #cell-seen="{ row }">
            <div class="space-y-0.5 text-xs text-gray-500 dark:text-dark-400">
              <div>{{ t('admin.ingressRisk.table.first', { time: formatDateTime(row.first_seen) }) }}</div>
              <div>{{ t('admin.ingressRisk.table.last', { time: formatDateTime(row.last_seen) }) }}</div>
            </div>
          </template>

          <template #cell-subject="{ row }">
            <div v-if="row.user_id || row.api_key_id" class="space-y-0.5 text-xs">
              <div v-if="row.user_id">{{ t('admin.ingressRisk.table.user', { id: row.user_id }) }}</div>
              <div v-if="row.api_key_id">{{ t('admin.ingressRisk.table.apiKey', { id: row.api_key_id }) }}</div>
            </div>
            <span v-else class="text-gray-400">—</span>
          </template>

          <template #empty>
            <div class="flex flex-col items-center py-8">
              <Icon name="shield" size="xl" class="mb-3 h-10 w-10 text-gray-300 dark:text-dark-600" />
              <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.ingressRisk.table.empty') }}
              </p>
            </div>
          </template>
        </DataTable>

        <Pagination
          v-if="total > 0"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="onPageChange"
          @update:page-size="onPageSizeChange"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import DataTable from '@/common/widgets/data/DataTable.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { Column } from '@/common/types/uiTypes'
import {
  ingressRiskAPI,
  type AuthCacheHealth,
  type CloudflareIngressMode,
  type CloudflareIngressSettings,
  type IngressCollectorHealth,
  type IngressRejection,
  type IngressRejectionQuery,
  type IngressRiskTimeRange,
} from '@/features/admin-risk-control/data/datasources/ingressRiskDatasource'
import { formatDateTime, formatNumber } from '@/core/utils/format'
import {
  cloudflareStatusDescription,
  cloudflareStatusLabel,
  cloudflareTokenStatusLabel,
  ingressRiskProtocolLabel,
  ingressRiskReasonLabel,
  ingressRiskRouteLabel,
} from '@/features/admin-risk-control/presentation/ingressRiskLocale'

const { t } = useI18n()

type HealthLevel = 'healthy' | 'warning' | 'critical' | 'unknown'
type CloudflareStatus = 'disabled' | 'cleanup' | 'healthy' | 'warning' | 'stopped' | 'unknown'
type DisplayIcon = 'key' | 'ban' | 'shield' | 'globe' | 'database' | 'server' | 'sync' | 'clock'

const REASONS = [
  'query_api_key_deprecated', 'api_key_required', 'invalid_api_key', 'invalid_auth_rate_limited',
  'api_key_auth_overloaded', 'api_key_disabled', 'ip_restricted', 'user_inactive', 'group_deleted',
  'group_disabled', 'group_not_allowed', 'group_unassigned', 'other',
] as const
const ROUTES = [
  'antigravity', 'gemini', 'codex', 'messages', 'responses', 'chat_completions',
  'images', 'videos', 'embeddings', 'models', 'other',
] as const
const PROTOCOLS = ['google', 'anthropic', 'openai', 'gateway', 'other'] as const
const TIME_RANGES: IngressRiskTimeRange[] = ['5m', '30m', '1h', '6h', '24h', '7d', '30d']

const filters = reactive({
  time_range: '1h' as IngressRiskTimeRange,
  reason: '',
  route_family: '',
  protocol: '',
  client_ip: '',
  user_id: '',
  api_key_id: '',
})

const records = ref<IngressRejection[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const recordsLoading = ref(false)
const healthLoading = ref(false)
const recordsError = ref('')
const healthError = ref('')
const collectorHealth = ref<IngressCollectorHealth | null>(null)
const authHealth = ref<AuthCacheHealth | null>(null)
const lastUpdated = ref<Date | null>(null)
const cloudflareSettings = ref<CloudflareIngressSettings | null>(null)
const cloudflareSettingsLoading = ref(false)
const cloudflareSettingsSaving = ref(false)
const cloudflareSettingsError = ref('')
const cloudflareSettingsSaved = ref('')
const cloudflareSettingsExpanded = ref(false)
const cloudflareAdvancedExpanded = ref(false)
const cloudflareForm = reactive({
  enabled: false,
  mode: 'zone_access_rules' as CloudflareIngressMode,
  zone_id: '',
  api_token: '',
  waf_hostnames_text: '',
  waf_rule_ids_text: '',
  waf_sync_interval_seconds: 15,
  analytics_interval_seconds: 300,
  request_timeout_seconds: 5,
  queue_capacity: 1024,
  max_active_rules: 1000,
  reconcile_interval_seconds: 300,
})

const refreshing = computed(() => recordsLoading.value || healthLoading.value)
const cloudflareHealth = computed(() => authHealth.value?.invalid_abuse.cloudflare)
const cloudflareWAFHealth = computed(() => cloudflareHealth.value?.waf)
const cloudflareWAFHostnameStats = computed(() => cloudflareWAFHealth.value?.hostname_stats ?? [])
const cloudflareMode = computed<CloudflareIngressMode>(() => cloudflareHealth.value?.mode
  ?? cloudflareSettings.value?.mode
  ?? 'zone_access_rules')
const cloudflareCredentialsLocked = computed(() => Boolean(
  cloudflareSettings.value?.enabled ||
  (cloudflareHealth.value?.active_rules ?? 0) > 0 ||
  (cloudflareHealth.value?.queue_depth ?? 0) > 0 ||
  (cloudflareHealth.value?.waf?.synced_entries ?? 0) > 0 ||
  (cloudflareHealth.value?.waf?.overflow_entries ?? 0) > 0,
))
const cloudflareSettingsSummary = computed(() => {
  const settings = cloudflareSettings.value
  if (!settings) return t('admin.ingressRisk.cloudflare.settings.loading')
  if (settings.mode === 'waf_custom_rules') {
    const hostCount = settings.waf_hostnames?.length || (settings.waf_hostname ? 1 : 0)
    return t('admin.ingressRisk.cloudflare.settings.summaryWaf', {
      hosts: formatNumber(hostCount),
      rules: formatNumber(settings.waf_rule_ids?.length ?? 0),
      interval: formatNumber(settings.waf_sync_interval_seconds ?? 15),
    })
  }
  return t('admin.ingressRisk.cloudflare.settings.summaryAccess', {
    limit: formatNumber(settings.max_active_rules),
  })
})
const currentPageRequests = computed(() => records.value.reduce((sum, row) => sum + row.request_count, 0))
const lastUpdatedLabel = computed(() => lastUpdated.value
  ? t('admin.ingressRisk.updatedAt', { time: formatDateTime(lastUpdated.value) })
  : t('admin.ingressRisk.neverUpdated'))

const timeRangeOptions = computed(() => TIME_RANGES.map((value) => ({
  value,
  label: t(`admin.ingressRisk.timeRanges.${value}`),
})))
const reasonOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...REASONS.map((value) => ({ value, label: reasonLabel(value) })),
])
const routeOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...ROUTES.map((value) => ({ value, label: routeLabel(value) })),
])
const protocolOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...PROTOCOLS.map((value) => ({ value, label: protocolLabel(value) })),
])
const cloudflareModeOptions = computed<Array<{ value: CloudflareIngressMode; label: string }>>(() => [
  {
    value: 'zone_access_rules',
    label: t('admin.ingressRisk.cloudflare.settings.modes.zoneAccessRules'),
  },
  {
    value: 'waf_custom_rules',
    label: t('admin.ingressRisk.cloudflare.settings.modes.wafCustomRules'),
  },
])

const columns = computed<Column[]>(() => [
  { key: 'bucket_start', label: t('admin.ingressRisk.table.bucket') },
  { key: 'reject_reason', label: t('admin.ingressRisk.table.reason') },
  { key: 'route', label: t('admin.ingressRisk.table.route') },
  { key: 'client_ip', label: t('admin.ingressRisk.table.clientIp') },
  { key: 'request_count', label: t('admin.ingressRisk.table.requests'), class: 'text-right' },
  { key: 'seen', label: t('admin.ingressRisk.table.seen') },
  { key: 'subject', label: t('admin.ingressRisk.table.subject') },
])

const overallHealth = computed<HealthLevel>(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  if (!collector || !auth) return 'unknown'
  const cloudflare = auth.invalid_abuse.cloudflare
  if (!collector.accepting || !auth.subscriber.connected || !auth.outbox.running || (cloudflare?.enabled && !cloudflare.running)) return 'critical'
  if (
    collector.flush_failure_count > 0 || collector.dropped_count > 0 || collector.overflowed_count > 0 ||
    collector.pending_batches > 0 || auth.lookup.rejected > 0 || auth.outbox.failures > 0 ||
    auth.outbox.pending > 0 || auth.invalid_abuse.overflowed > 0 || auth.invalid_abuse.global_blocked > 0 ||
    (cloudflare?.enabled && (
      cloudflare.queue_depth > 0 || cloudflare.dropped > 0 || Boolean(cloudflare.last_error) ||
      (cloudflare.waf?.overflow_entries ?? 0) > 0 || Boolean(cloudflare.waf?.analytics_error)
    ))
  ) return 'warning'
  return 'healthy'
})

const healthSignals = computed(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  return [
    {
      key: 'collector',
      label: t('admin.ingressRisk.health.collector'),
      value: collector ? t(`admin.ingressRisk.health.${collector.accepting ? 'running' : 'stopped'}`) : '—',
      level: !collector ? 'unknown' : !collector.accepting ? 'critical' : collector.flush_failure_count > 0 || collector.dropped_count > 0 || collector.overflowed_count > 0 ? 'warning' : 'healthy',
    },
    {
      key: 'subscriber',
      label: t('admin.ingressRisk.health.subscriber'),
      value: auth ? t(`admin.ingressRisk.health.${auth.subscriber.connected ? 'connected' : 'disconnected'}`) : '—',
      level: !auth ? 'unknown' : auth.subscriber.connected ? auth.subscriber.failures > 0 ? 'warning' : 'healthy' : 'critical',
    },
    {
      key: 'lookup',
      label: t('admin.ingressRisk.health.lookup'),
      value: auth ? t('admin.ingressRisk.health.rejected', { count: formatNumber(auth.lookup.rejected) }) : '—',
      level: !auth ? 'unknown' : auth.lookup.rejected > 0 ? 'warning' : 'healthy',
    },
    {
      key: 'outbox',
      label: t('admin.ingressRisk.health.outbox'),
      value: auth ? t('admin.ingressRisk.health.pending', { count: formatNumber(auth.outbox.pending) }) : '—',
      level: !auth ? 'unknown' : !auth.outbox.running ? 'critical' : auth.outbox.pending > 0 || auth.outbox.failures > 0 ? 'warning' : 'healthy',
    },
  ] as Array<{ key: string; label: string; value: string; level: HealthLevel }>
})

const healthLastError = computed(() => collectorHealth.value?.last_error || authHealth.value?.outbox.last_error || authHealth.value?.outbox.stats_error || cloudflareHealth.value?.last_error || '')
const healthIcon = computed(() => overallHealth.value === 'healthy' ? 'checkCircle' : overallHealth.value === 'unknown' ? 'clock' : 'exclamationTriangle')
const healthBandClass = computed(() => ({
  healthy: 'border-emerald-200 bg-emerald-50/70 dark:border-emerald-900/60 dark:bg-emerald-950/20',
  warning: 'border-amber-200 bg-amber-50/70 dark:border-amber-900/60 dark:bg-amber-950/20',
  critical: 'border-red-200 bg-red-50/70 dark:border-red-900/60 dark:bg-red-950/20',
  unknown: 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/60',
})[overallHealth.value])
const healthIconClass = computed(() => ({
  healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  unknown: 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
})[overallHealth.value])
const healthBadgeClass = computed(() => ({
  healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  unknown: 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
})[overallHealth.value])

const cloudflareStatus = computed<CloudflareStatus>(() => {
  const health = cloudflareHealth.value
  if (!authHealth.value) return 'unknown'
  if (!health?.enabled) return health?.running && health.active_rules > 0 ? 'cleanup' : 'disabled'
  if (!health.running) return 'stopped'
  if (
    health.queue_depth > 0 || health.dropped > 0 || health.last_error ||
    (health.waf?.overflow_entries ?? 0) > 0 || health.waf?.analytics_error
  ) return 'warning'
  return 'healthy'
})
const cloudflareBadgeClass = computed(() => ({
  healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  cleanup: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  stopped: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  disabled: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
  unknown: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
})[cloudflareStatus.value])
const cloudflareLastSuccess = computed(() => cloudflareHealth.value?.last_success_at
  ? t('admin.ingressRisk.cloudflare.lastSuccess', { time: formatDateTime(cloudflareHealth.value.last_success_at) })
  : t('admin.ingressRisk.cloudflare.noSuccess'))
const cloudflareMetrics = computed(() => {
  const health = cloudflareHealth.value
  const value = (count?: number) => health ? formatNumber(count ?? 0) : '—'
  return [
    { key: 'active', label: t('admin.ingressRisk.cloudflare.metrics.active'), value: value(health?.active_rules) },
    { key: 'queue', label: t('admin.ingressRisk.cloudflare.metrics.queue'), value: health ? `${formatNumber(health.queue_depth)} / ${formatNumber(health.queue_capacity)}` : '—' },
    { key: 'enqueued', label: t('admin.ingressRisk.cloudflare.metrics.enqueued'), value: value(health?.enqueued) },
    { key: 'applied', label: t('admin.ingressRisk.cloudflare.metrics.applied'), value: value(health?.applied) },
    { key: 'released', label: t('admin.ingressRisk.cloudflare.metrics.released'), value: value(health?.released) },
    { key: 'failures', label: t('admin.ingressRisk.cloudflare.metrics.failures'), value: health ? `${formatNumber(health.failures)} / ${formatNumber(health.dropped)}` : '—' },
  ]
})
const wafAnalyticsMetrics = computed(() => {
  const waf = cloudflareWAFHealth.value
  if (!waf) return []
  return [
    {
      key: 'hostname',
      label: t('admin.ingressRisk.cloudflare.waf.hostname'),
      value: (waf.hostnames?.length ? waf.hostnames : [waf.hostname].filter(Boolean)).join(', ') || '—',
    },
    { key: 'requests', label: t('admin.ingressRisk.cloudflare.waf.hostnameRequests24h'), value: formatNumber(waf.hostname_requests_24h) },
    { key: 'blocked', label: t('admin.ingressRisk.cloudflare.waf.blockedRequests24h'), value: formatNumber(waf.blocked_requests_24h) },
    { key: 'rules', label: t('admin.ingressRisk.cloudflare.waf.rulesAndEntries'), value: `${formatNumber(waf.rule_count)} / ${formatNumber(waf.synced_entries)}` },
    { key: 'overflow', label: t('admin.ingressRisk.cloudflare.waf.overflow'), value: formatNumber(waf.overflow_entries) },
    {
      key: 'updated',
      label: t('admin.ingressRisk.cloudflare.waf.analyticsUpdated'),
      value: waf.analytics_updated_at ? formatDateTime(waf.analytics_updated_at) : '—',
    },
  ]
})

const metricCards = computed<Array<{ key: string; label: string; value: string; hint: string; icon: DisplayIcon; iconClass: string }>>(() => {
  const abuse = authHealth.value?.invalid_abuse
  const value = (count?: number) => abuse ? formatNumber(count ?? 0) : '—'
  return [
    { key: 'recorded', label: t('admin.ingressRisk.metrics.recorded'), value: value(abuse?.recorded), hint: t('admin.ingressRisk.metrics.recordedHint'), icon: 'key', iconClass: 'bg-blue-100 text-blue-700 dark:bg-blue-900/35 dark:text-blue-300' },
    { key: 'rejected', label: t('admin.ingressRisk.metrics.rejected'), value: value(abuse?.rejected), hint: t('admin.ingressRisk.metrics.rejectedHint'), icon: 'ban', iconClass: 'bg-red-100 text-red-700 dark:bg-red-900/35 dark:text-red-300' },
    { key: 'blocks', label: t('admin.ingressRisk.metrics.blocks'), value: value(abuse?.blocks), hint: t('admin.ingressRisk.metrics.blocksHint'), icon: 'shield', iconClass: 'bg-amber-100 text-amber-700 dark:bg-amber-900/35 dark:text-amber-300' },
    { key: 'global', label: t('admin.ingressRisk.metrics.globalBlocked'), value: value(abuse?.global_blocked), hint: t('admin.ingressRisk.metrics.globalBlockedHint'), icon: 'globe', iconClass: 'bg-violet-100 text-violet-700 dark:bg-violet-900/35 dark:text-violet-300' },
  ]
})

const runtimeIndicators = computed<Array<{ key: string; label: string; value: string; detail: string; icon: DisplayIcon }>>(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  return [
    {
      key: 'tracked', icon: 'shield', label: t('admin.ingressRisk.runtime.tracked'),
      value: auth ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(auth.invalid_abuse.tracked), capacity: formatNumber(auth.invalid_abuse.capacity) }) : '—',
      detail: auth ? t(`admin.ingressRisk.runtime.${auth.invalid_abuse.enabled ? 'enabled' : 'disabled'}`) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'lookup', icon: 'key', label: t('admin.ingressRisk.runtime.lookup'),
      value: auth ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(auth.lookup.in_flight), capacity: formatNumber(auth.lookup.capacity) }) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.lookupTotals', { total: formatNumber(auth.lookup.total), rejected: formatNumber(auth.lookup.rejected) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'collector', icon: 'database', label: t('admin.ingressRisk.runtime.collector'),
      value: collector ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(collector.cardinality), capacity: formatNumber(collector.capacity) }) : '—',
      detail: collector ? t('admin.ingressRisk.runtime.collectorPending', { rows: formatNumber(collector.pending_rows), batches: formatNumber(collector.pending_batches) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'delivery', icon: 'server', label: t('admin.ingressRisk.runtime.delivery'),
      value: collector ? formatNumber(collector.flushed_request_count) : '—',
      detail: collector ? t('admin.ingressRisk.runtime.deliveryTotals', { dropped: formatNumber(collector.dropped_count), overflowed: formatNumber(collector.overflowed_count), failed: formatNumber(collector.flush_failure_count) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'subscriber', icon: 'sync', label: t('admin.ingressRisk.runtime.subscriber'),
      value: auth ? t(`admin.ingressRisk.health.${auth.subscriber.connected ? 'connected' : 'disconnected'}`) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.subscriberFailures', { count: formatNumber(auth.subscriber.failures) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'outbox', icon: 'clock', label: t('admin.ingressRisk.runtime.outbox'),
      value: auth ? formatNumber(auth.outbox.pending) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.outboxTotals', { processed: formatNumber(auth.outbox.processed), failures: formatNumber(auth.outbox.failures) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
  ]
})

function signalDotClass(level: HealthLevel) {
  return {
    healthy: 'bg-emerald-500', warning: 'bg-amber-500', critical: 'bg-red-500', unknown: 'bg-gray-400',
  }[level]
}

function reasonLabel(reason: string) {
  return ingressRiskReasonLabel(t, reason)
}

function routeLabel(route: string) {
  return ingressRiskRouteLabel(t, route)
}

function protocolLabel(protocol: string) {
  return ingressRiskProtocolLabel(t, protocol)
}

function reasonBadgeClass(reason: string) {
  if (['invalid_api_key', 'invalid_auth_rate_limited', 'api_key_auth_overloaded'].includes(reason)) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/35 dark:text-red-300'
  }
  if (['query_api_key_deprecated', 'api_key_required', 'api_key_disabled', 'ip_restricted'].includes(reason)) {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-900/35 dark:text-amber-300'
  }
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200'
}

function positiveNumber(value: string): number | undefined {
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function buildQuery(): IngressRejectionQuery {
  return {
    time_range: filters.time_range,
    reason: filters.reason || undefined,
    route_family: filters.route_family || undefined,
    protocol: filters.protocol || undefined,
    client_ip: filters.client_ip || undefined,
    user_id: positiveNumber(filters.user_id),
    api_key_id: positiveNumber(filters.api_key_id),
    page: page.value,
    page_size: pageSize.value,
  }
}

function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}

async function loadRecords() {
  recordsLoading.value = true
  recordsError.value = ''
  try {
    const result = await ingressRiskAPI.listIngressRejections(buildQuery())
    records.value = result.items ?? []
    total.value = result.total ?? 0
    lastUpdated.value = new Date()
  } catch (error) {
    recordsError.value = errorMessage(error, t('admin.ingressRisk.errors.records'))
  } finally {
    recordsLoading.value = false
  }
}

async function loadHealth() {
  healthLoading.value = true
  healthError.value = ''
  const [collectorResult, authResult] = await Promise.allSettled([
    ingressRiskAPI.getIngressCollectorHealth(),
    ingressRiskAPI.getAuthCacheHealth(),
  ])
  if (collectorResult.status === 'fulfilled') collectorHealth.value = collectorResult.value
  if (authResult.status === 'fulfilled') authHealth.value = authResult.value
  if (collectorResult.status === 'rejected' || authResult.status === 'rejected') {
    const failure = collectorResult.status === 'rejected' ? collectorResult.reason : authResult.status === 'rejected' ? authResult.reason : null
    healthError.value = errorMessage(failure, t('admin.ingressRisk.errors.health'))
  } else {
    lastUpdated.value = new Date()
  }
  healthLoading.value = false
}

function applyCloudflareSettings(settings: CloudflareIngressSettings) {
  cloudflareSettings.value = settings
  cloudflareForm.enabled = settings.enabled
  cloudflareForm.mode = settings.mode ?? 'zone_access_rules'
  cloudflareForm.zone_id = settings.zone_id
  cloudflareForm.api_token = ''
  const hostnames = settings.waf_hostnames?.length
    ? settings.waf_hostnames
    : settings.waf_hostname ? [settings.waf_hostname] : []
  cloudflareForm.waf_hostnames_text = hostnames.join('\n')
  cloudflareForm.waf_rule_ids_text = (settings.waf_rule_ids ?? []).join('\n')
  cloudflareForm.waf_sync_interval_seconds = settings.waf_sync_interval_seconds ?? 15
  cloudflareForm.analytics_interval_seconds = settings.analytics_interval_seconds ?? 300
  cloudflareForm.request_timeout_seconds = settings.request_timeout_seconds
  cloudflareForm.queue_capacity = settings.queue_capacity
  cloudflareForm.max_active_rules = settings.max_active_rules
  cloudflareForm.reconcile_interval_seconds = settings.reconcile_interval_seconds
  if (!settings.enabled || !settings.api_token_configured) {
    cloudflareSettingsExpanded.value = true
  }
}

async function loadCloudflareSettings() {
  cloudflareSettingsLoading.value = true
  cloudflareSettingsError.value = ''
  cloudflareSettingsSaved.value = ''
  try {
    applyCloudflareSettings(await ingressRiskAPI.getCloudflareIngressSettings())
  } catch (error) {
    cloudflareSettingsError.value = errorMessage(error, t('admin.ingressRisk.cloudflare.settings.loadError'))
    cloudflareSettingsExpanded.value = true
  } finally {
    cloudflareSettingsLoading.value = false
  }
}

async function saveCloudflareSettings() {
  if (!cloudflareSettings.value || cloudflareSettingsSaving.value) return
  cloudflareSettingsError.value = ''
  cloudflareSettingsSaved.value = ''
  if (cloudflareForm.enabled && !cloudflareForm.zone_id.trim()) {
    cloudflareSettingsError.value = t('admin.ingressRisk.cloudflare.settings.zoneRequired')
    return
  }
  if (cloudflareForm.enabled && !cloudflareSettings.value.api_token_configured && !cloudflareForm.api_token.trim()) {
    cloudflareSettingsError.value = t('admin.ingressRisk.cloudflare.settings.tokenRequired')
    return
  }
  const wafRuleIDs = cloudflareForm.waf_rule_ids_text
    .split(/[\s,]+/)
    .map((value) => value.trim())
    .filter(Boolean)
  const wafHostnames = [...new Set(cloudflareForm.waf_hostnames_text
    .split(/[\s,]+/)
    .map((value) => value.trim().toLowerCase().replace(/\.$/, ''))
    .filter(Boolean))].sort()
  if (cloudflareForm.enabled && cloudflareForm.mode === 'waf_custom_rules' && wafHostnames.length === 0) {
    cloudflareSettingsError.value = t('admin.ingressRisk.cloudflare.settings.wafHostnameRequired')
    return
  }
  if (cloudflareForm.enabled && cloudflareForm.mode === 'waf_custom_rules' && wafRuleIDs.length === 0) {
    cloudflareSettingsError.value = t('admin.ingressRisk.cloudflare.settings.wafRuleIdsRequired')
    return
  }

  cloudflareSettingsSaving.value = true
  try {
    const updated = await ingressRiskAPI.updateCloudflareIngressSettings({
      enabled: cloudflareForm.enabled,
      mode: cloudflareForm.mode,
      zone_id: cloudflareForm.zone_id.trim(),
      api_token: cloudflareForm.api_token.trim(),
      waf_hostname: wafHostnames[0] ?? '',
      waf_hostnames: wafHostnames,
      waf_rule_ids: wafRuleIDs,
      waf_sync_interval_seconds: Number(cloudflareForm.waf_sync_interval_seconds),
      analytics_interval_seconds: Number(cloudflareForm.analytics_interval_seconds),
      request_timeout_seconds: Number(cloudflareForm.request_timeout_seconds),
      queue_capacity: Number(cloudflareForm.queue_capacity),
      max_active_rules: Number(cloudflareForm.max_active_rules),
      reconcile_interval_seconds: Number(cloudflareForm.reconcile_interval_seconds),
    })
    applyCloudflareSettings(updated)
    cloudflareSettingsSaved.value = t('admin.ingressRisk.cloudflare.settings.saved')
    await loadHealth()
  } catch (error) {
    cloudflareSettingsError.value = errorMessage(error, t('admin.ingressRisk.cloudflare.settings.saveError'))
  } finally {
    cloudflareSettingsSaving.value = false
  }
}

async function refreshAll() {
  await Promise.all([loadRecords(), loadHealth(), loadCloudflareSettings()])
}

function search() {
  page.value = 1
  void loadRecords()
}

function resetFilters() {
  filters.time_range = '1h'
  filters.reason = ''
  filters.route_family = ''
  filters.protocol = ''
  filters.client_ip = ''
  filters.user_id = ''
  filters.api_key_id = ''
  page.value = 1
  void loadRecords()
}

function filterByIp(ip: string) {
  filters.client_ip = ip
  page.value = 1
  void loadRecords()
}

function onPageChange(nextPage: number) {
  page.value = nextPage
  void loadRecords()
}

function onPageSizeChange(nextPageSize: number) {
  pageSize.value = nextPageSize
  page.value = 1
  void loadRecords()
}

onMounted(() => {
  void refreshAll()
})
</script>
