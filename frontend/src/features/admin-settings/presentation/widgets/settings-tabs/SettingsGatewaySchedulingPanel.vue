<template>
<div class="space-y-6">
  <!-- Claude Code Settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.claudeCode.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.claudeCode.description") }}
      </p>
    </div>
    <div class="p-6">
      <div>
        <label
          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t("admin.settings.claudeCode.minVersion") }}
        </label>
        <input
          v-model="form.min_claude_code_version"
          type="text"
          class="input max-w-xs font-mono text-sm"
          :placeholder="
            t('admin.settings.claudeCode.minVersionPlaceholder')
          "
        />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.claudeCode.minVersionHint") }}
        </p>
      </div>
      <div class="mt-4">
        <label
          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t("admin.settings.claudeCode.maxVersion") }}
        </label>
        <input
          v-model="form.max_claude_code_version"
          type="text"
          class="input max-w-xs font-mono text-sm"
          :placeholder="
            t('admin.settings.claudeCode.maxVersionPlaceholder')
          "
        />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.claudeCode.maxVersionHint") }}
        </p>
      </div>
    </div>
  </div>

  <!-- Codex Settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.gatewayForwarding.codexHardeningTitle") }}
      </h2>
    </div>
    <div class="p-6 space-y-4">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t("admin.settings.gatewayForwarding.codexClientRestrictionTitle") }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.gatewayForwarding.codexHardeningDesc") }}
          </p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.gatewayForwarding.minCodexVersion") }}
            </label>
            <input
              v-model="form.min_codex_version"
              type="text"
              class="input w-full font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.minCodexVersionPlaceholder',
                )
              "
            />
          </div>
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.gatewayForwarding.maxCodexVersion") }}
            </label>
            <input
              v-model="form.max_codex_version"
              type="text"
              class="input w-full font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.maxCodexVersionPlaceholder',
                )
              "
            />
          </div>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.gatewayForwarding.codexVersionHint") }}
        </p>

        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.gatewayForwarding.codexFingerprintSignals") }}
          </label>
          <p class="mb-2 mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.gatewayForwarding.codexFingerprintSignalsDesc") }}
          </p>
          <div
            v-for="(row, i) in codexFingerprintRows"
            :key="`codex-fp-${i}`"
            class="mb-2 flex items-center gap-2"
          >
            <select v-model="row.type" class="input w-32 text-sm">
              <option value="header_exact">{{ t("admin.settings.gatewayForwarding.codexFpTypeHeaderExact") }}</option>
              <option value="header_prefix">{{ t("admin.settings.gatewayForwarding.codexFpTypeHeaderPrefix") }}</option>
              <option value="body_path">{{ t("admin.settings.gatewayForwarding.codexFpTypeBodyPath") }}</option>
            </select>
            <input
              v-model="row.match"
              type="text"
              class="input flex-1 font-mono text-sm"
              :placeholder="t('admin.settings.gatewayForwarding.codexFpMatchPlaceholder')"
            />
            <label class="flex shrink-0 items-center gap-1 text-xs text-gray-600 dark:text-gray-400">
              <input v-model="row.required" type="checkbox" />
              {{ t("admin.settings.gatewayForwarding.codexFpRequired") }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
              @click="removeCodexFingerprintRow(i)"
            >
              {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
            </button>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="addCodexFingerprintRow">
            {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
          </button>
          <p
            v-if="codexFingerprintNoRequired"
            class="mt-2 text-xs text-amber-600 dark:text-amber-500"
          >
            {{ t("admin.settings.gatewayForwarding.codexFingerprintNoRequiredWarn") }}
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div class="pr-4">
            <label
              class="block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t("admin.settings.gatewayForwarding.codexAllowAppServer")
              }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.codexAllowAppServerDesc",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.codex_cli_only_allow_app_server_clients"
          />
        </div>

        <div>
          <label
            class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.gatewayForwarding.codexBlacklist") }}
          </label>
          <p class="mb-2 mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.gatewayForwarding.codexBlacklistDesc") }}
          </p>
          <div
            v-for="(row, i) in codexBlacklistRows"
            :key="`codex-bl-${i}`"
            class="mb-2 flex gap-2"
          >
            <input
              v-model="row.originator"
              type="text"
              class="input w-1/3 font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                )
              "
            />
            <input
              v-model="row.uaContains"
              type="text"
              class="input flex-1 font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                )
              "
            />
            <button
              type="button"
              class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
              @click="removeCodexBlacklistRow(i)"
            >
              {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
            </button>
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="addCodexBlacklistRow"
          >
            {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
          </button>
        </div>

        <div>
          <label
            class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.gatewayForwarding.codexWhitelist") }}
          </label>
          <p class="mb-2 mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.gatewayForwarding.codexWhitelistDesc") }}
          </p>
          <div
            v-for="(row, i) in codexWhitelistRows"
            :key="`codex-wl-${i}`"
            class="mb-2 flex gap-2"
          >
            <input
              v-model="row.originator"
              type="text"
              class="input w-1/3 font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                )
              "
            />
            <input
              v-model="row.uaContains"
              type="text"
              class="input flex-1 font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                )
              "
            />
            <label
              class="flex shrink-0 items-center gap-1 text-xs text-gray-600 dark:text-gray-400"
              :title="
                t(
                  'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprintTooltip',
                )
              "
            >
              <input
                v-model="row.skipEngineFingerprint"
                type="checkbox"
              />
              {{
                t(
                  'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprint',
                )
              }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
              @click="removeCodexWhitelistRow(i)"
            >
              {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
            </button>
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="addCodexWhitelistRow"
          >
            {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
          </button>
        </div>
    </div>
  </div>

  <!-- Upstream Billing Probe Settings -->
  <div class="card" data-testid="upstream-billing-probe-settings">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.upstreamBillingProbe.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.upstreamBillingProbe.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div
        v-if="upstreamBillingProbeLoading"
        class="flex items-center gap-2 text-gray-500"
      >
        <div
          class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
        ></div>
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.upstreamBillingProbe.enabled") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.upstreamBillingProbe.enabledHint") }}
            </p>
          </div>
          <Toggle
            v-model="upstreamBillingProbeForm.enabled"
            :aria-label="t('admin.settings.upstreamBillingProbe.enabled')"
            data-testid="upstream-billing-probe-enabled"
          />
        </div>

        <div
          v-if="upstreamBillingProbeForm.enabled"
          class="border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            for="upstream-billing-probe-interval"
          >
            {{ t("admin.settings.upstreamBillingProbe.intervalMinutes") }}
          </label>
          <input
            id="upstream-billing-probe-interval"
            v-model.number="upstreamBillingProbeForm.interval_minutes"
            type="number"
            min="5"
            max="1440"
            class="input w-32"
            data-testid="upstream-billing-probe-interval"
            @keydown.enter.prevent="saveUpstreamBillingProbeSettings"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.upstreamBillingProbe.intervalHint") }}
          </p>
        </div>

        <div
          class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="upstreamBillingProbeSaving"
            data-testid="upstream-billing-probe-save"
            @click="saveUpstreamBillingProbeSettings"
          >
            {{
              upstreamBillingProbeSaving
                ? t("common.saving")
                : t("common.save")
            }}
          </button>
        </div>
      </template>
    </div>
  </div>

  <!-- Ollama Cloud Usage Settings -->
  <div class="card" data-testid="ollama-cloud-usage-global-settings">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.ollamaCloudUsage.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.ollamaCloudUsage.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div v-if="ollamaCloudUsageLoading" class="flex items-center gap-2 text-gray-500">
        <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
        {{ t("common.loading") }}
      </div>
      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.ollamaCloudUsage.enabled") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.ollamaCloudUsage.enabledHint") }}
            </p>
          </div>
          <Toggle
            v-model="ollamaCloudUsageForm.enabled"
            :aria-label="t('admin.settings.ollamaCloudUsage.enabled')"
            data-testid="ollama-cloud-usage-global-enabled"
          />
        </div>
        <div v-if="ollamaCloudUsageForm.enabled" class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300" for="ollama-cloud-usage-debounce">
              {{ t("admin.settings.ollamaCloudUsage.debounceMinutes") }}
            </label>
            <input
              id="ollama-cloud-usage-debounce"
              v-model.number="ollamaCloudUsageForm.debounce_minutes"
              type="number"
              min="1"
              max="60"
              class="input w-32"
              data-testid="ollama-cloud-usage-global-debounce"
              @keydown.enter.prevent="saveOllamaCloudUsageSettings"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.ollamaCloudUsage.debounceHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300" for="ollama-cloud-usage-interval">
              {{ t("admin.settings.ollamaCloudUsage.intervalMinutes") }}
            </label>
            <input
              id="ollama-cloud-usage-interval"
              v-model.number="ollamaCloudUsageForm.interval_minutes"
              type="number"
              min="15"
              max="1440"
              class="input w-32"
              data-testid="ollama-cloud-usage-global-interval"
              @keydown.enter.prevent="saveOllamaCloudUsageSettings"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.ollamaCloudUsage.intervalHint") }}
            </p>
          </div>
        </div>
        <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="ollamaCloudUsageSaving"
            data-testid="ollama-cloud-usage-global-save"
            @click="saveOllamaCloudUsageSettings"
          >
            {{ ollamaCloudUsageSaving ? t("common.saving") : t("common.save") }}
          </button>
        </div>
      </template>
    </div>
  </div>

  <!-- Gateway Scheduling Settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.scheduling.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.scheduling.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between">
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.scheduling.allowUngroupedKey") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.scheduling.allowUngroupedKeyHint") }}
          </p>
        </div>
        <Toggle v-model="form.allow_ungrouped_key_scheduling" />
      </div>

      <div
        class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.scheduling.contentSessionBurstBalance") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.scheduling.contentSessionBurstBalanceHint") }}
          </p>
        </div>
        <Toggle
          v-model="form.openai_content_session_burst_balance_enabled"
          data-testid="openai-content-session-burst-balance-toggle"
        />
      </div>

      <div class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.scheduling.sessionIDRateLimit") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.scheduling.sessionIDRateLimitHint") }}
          </p>
        </div>
        <Toggle v-model="form.openai_session_id_rate_limit_enabled" data-testid="openai-session-id-rate-limit-toggle" />
      </div>

      <div v-if="form.openai_session_id_rate_limit_enabled" class="flex flex-col gap-2 border-t border-gray-100 pt-5 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
        <label for="openai-session-id-rate-limit-per-minute" class="text-sm text-gray-700 dark:text-gray-300">
          {{ t("admin.settings.scheduling.sessionIDRateLimitPerMinute") }}
        </label>
        <input id="openai-session-id-rate-limit-per-minute" v-model.number="form.openai_session_id_rate_limit_per_minute" class="input w-full sm:w-32" type="number" min="0" max="1000000" step="1" data-testid="openai-session-id-rate-limit-per-minute" />
      </div>

      <div
        v-if="!form.openai_advanced_scheduler_enabled"
        class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.lowRatePriorityTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.openaiExperimentalScheduler.lowRatePriorityDescription")
            }}
          </p>
        </div>
        <Toggle
          v-model="form.openai_low_upstream_rate_priority_enabled"
          data-testid="openai-low-rate-priority-toggle"
        />
      </div>

      <div
        v-if="!form.openai_advanced_scheduler_enabled && form.openai_low_upstream_rate_priority_enabled"
        class="flex flex-col items-stretch gap-3 border-t border-gray-100 pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6 dark:border-dark-700"
      >
        <div class="min-w-0">
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
            for="openai-oauth-scheduling-rate-multiplier"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.openaiExperimentalScheduler.oauthRatePriorityDescription") }}
          </p>
        </div>
        <div class="relative w-full shrink-0 sm:w-32">
          <input
            id="openai-oauth-scheduling-rate-multiplier"
            v-model.number="form.openai_oauth_scheduling_rate_multiplier"
            class="input pr-8"
            data-testid="openai-oauth-scheduling-rate-multiplier"
            min="0"
            required
            step="0.01"
            type="number"
          />
          <span
            class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-400"
          >x</span>
        </div>
      </div>

      <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
        <div class="flex items-start justify-between">
          <div class="min-w-0 pr-4">
            <div class="flex flex-wrap items-center gap-2">
              <label
                class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {{ t("admin.settings.schedulerV2.title") }}
              </label>
              <span class="text-xs font-medium" :class="schedulerV2StatusClass">
                {{ schedulerV2StatusLabel }}
              </span>
            </div>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.schedulerV2.description") }}
            </p>
            <p
              v-if="form.scheduler_v2_error"
              class="mt-1 break-words text-xs text-red-600 dark:text-red-400"
            >
              {{ form.scheduler_v2_error }}
            </p>
          </div>
          <Toggle
            v-model="form.scheduler_v2_enabled"
            data-testid="scheduler-v2-toggle"
          />
        </div>
        <div
          v-if="form.scheduler_v2_enabled"
          class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
        >
          <label class="block">
            <span class="input-label">{{
              t("admin.settings.schedulerV2.candidateLimit")
            }}</span>
            <input
              v-model.number="form.scheduler_v2_candidate_limit"
              type="number"
              min="1"
              max="4096"
              required
              class="input"
              data-testid="scheduler-v2-candidate-limit"
            />
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.schedulerV2.candidateLimitHelp") }}
            </span>
          </label>
          <label class="block">
            <span class="input-label">{{
              t("admin.settings.schedulerV2.scanLimit")
            }}</span>
            <input
              v-model.number="form.scheduler_v2_scan_limit"
              type="number"
              :min="Math.max(1, Number(form.scheduler_v2_candidate_limit) || 1)"
              max="65536"
              required
              class="input"
              data-testid="scheduler-v2-scan-limit"
            />
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.schedulerV2.scanLimitHelp") }}
            </span>
          </label>
        </div>
      </div>

      <div
        class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.title") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.openaiExperimentalScheduler.description")
            }}
          </p>
        </div>
        <Toggle
          v-model="form.openai_advanced_scheduler_enabled"
          data-testid="openai-advanced-scheduler-toggle"
        />
      </div>

      <div
        v-if="form.openai_advanced_scheduler_enabled"
        class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.stickyWeightedTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.openaiExperimentalScheduler.stickyWeightedDescription")
            }}
          </p>
        </div>
        <Toggle v-model="form.openai_advanced_scheduler_sticky_weighted_enabled" />
      </div>

      <div
        v-if="form.openai_advanced_scheduler_enabled"
        class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityDescription")
            }}
          </p>
        </div>
        <Toggle v-model="form.openai_advanced_scheduler_subscription_priority_enabled" />
      </div>

      <div
        v-if="form.openai_advanced_scheduler_enabled"
        class="flex flex-col items-stretch gap-3 border-t border-gray-100 pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6 dark:border-dark-700"
      >
        <div class="min-w-0">
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
            for="openai-oauth-scheduling-rate-multiplier"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.openaiExperimentalScheduler.oauthRateWeightedDescription") }}
          </p>
        </div>
        <div class="relative w-full shrink-0 sm:w-32">
          <input
            id="openai-oauth-scheduling-rate-multiplier"
            v-model.number="form.openai_oauth_scheduling_rate_multiplier"
            class="input pr-8"
            data-testid="openai-oauth-scheduling-rate-multiplier"
            min="0"
            required
            step="0.01"
            type="number"
          />
          <span
            class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-400"
          >x</span>
        </div>
      </div>

      <div
        v-if="form.openai_advanced_scheduler_enabled"
        class="border-t border-gray-100 pt-5 dark:border-dark-700"
      >
        <div>
          <label
            class="text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.openaiExperimentalScheduler.weightsTitle") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.openaiExperimentalScheduler.weightsDescription")
            }}
          </p>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
          <label
            v-for="field in openAIAdvancedSchedulerWeightFields"
            :key="field.key"
            class="block"
          >
            <span class="text-xs font-medium text-gray-600 dark:text-gray-400">
              {{ field.label }}
            </span>
            <input
              v-model="form[field.key]"
              class="input mt-1"
              inputmode="decimal"
              :placeholder="field.placeholder"
              type="text"
            />
          </label>
        </div>
      </div>
    </div>
  </div>
</div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { addCodexBlacklistRow, addCodexFingerprintRow, addCodexWhitelistRow, codexBlacklistRows, codexFingerprintNoRequired, codexFingerprintRows, codexWhitelistRows, form, ollamaCloudUsageForm, ollamaCloudUsageLoading, ollamaCloudUsageSaving, openAIAdvancedSchedulerWeightFields, removeCodexBlacklistRow, removeCodexFingerprintRow, removeCodexWhitelistRow, saveOllamaCloudUsageSettings, saveUpstreamBillingProbeSettings, schedulerV2StatusClass, schedulerV2StatusLabel, t, upstreamBillingProbeForm, upstreamBillingProbeLoading, upstreamBillingProbeSaving } = useSettingsPageContext()
</script>
