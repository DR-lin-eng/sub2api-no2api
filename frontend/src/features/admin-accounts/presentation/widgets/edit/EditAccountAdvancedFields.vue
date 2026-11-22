<template>
  <!-- Temp Unschedulable Rules -->
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.tempUnschedulable.hint') }}
        </p>
      </div>
      <button
        type="button"
        @click="tempUnschedEnabled = !tempUnschedEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          tempUnschedEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            tempUnschedEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="tempUnschedEnabled" class="space-y-3">
      <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
        <p class="text-xs text-blue-700 dark:text-blue-400">
          <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
          {{ t('admin.accounts.tempUnschedulable.notice') }}
        </p>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          v-for="preset in tempUnschedPresets"
          :key="preset.label"
          type="button"
          @click="addTempUnschedRule(preset.rule)"
          class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          + {{ preset.label }}
        </button>
      </div>

      <div v-if="tempUnschedRules.length > 0" class="space-y-3">
        <div
          v-for="(rule, index) in tempUnschedRules"
          :key="getTempUnschedRuleKey(rule)"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="mb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 }) }}
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                :disabled="index === 0"
                @click="moveTempUnschedRule(index, -1)"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
              >
                <Icon name="chevronUp" size="sm" :stroke-width="2" />
              </button>
              <button
                type="button"
                :disabled="index === tempUnschedRules.length - 1"
                @click="moveTempUnschedRule(index, 1)"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              <button
                type="button"
                @click="removeTempUnschedRule(index)"
                class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.scope') }}</label>
              <select v-model="rule.scope" class="input">
                <option value="">{{ t('admin.accounts.tempUnschedulable.scopeAuto') }}</option>
                <option value="account">{{ t('admin.accounts.tempUnschedulable.scopeAccount') }}</option>
                <option value="model">{{ t('admin.accounts.tempUnschedulable.scopeModel') }}</option>
              </select>
              <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.scopeHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') }}</label>
              <input
                v-model.number="rule.error_code"
                type="number"
                min="100"
                max="599"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.errorCodePlaceholder')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') }}</label>
              <input
                v-model.number="rule.duration_minutes"
                type="number"
                min="1"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
              />
            </div>
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') }}</label>
              <input
                v-model="rule.keywords"
                type="text"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') }}</p>
            </div>
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') }}</label>
              <input
                v-model="rule.description"
                type="text"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.descriptionPlaceholder')"
              />
            </div>
          </div>
        </div>
      </div>

      <button
        type="button"
        @click="addTempUnschedRule()"
        class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
      >
        <svg
          class="mr-1 inline h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        {{ t('admin.accounts.tempUnschedulable.addRule') }}
      </button>
    </div>
  </div>

  <!-- Intercept Warmup Requests (Anthropic/Antigravity) -->
  <div
    v-if="account?.platform === 'anthropic' || account?.platform === 'antigravity'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{
          t('admin.accounts.interceptWarmupRequests')
        }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="interceptWarmupRequests = !interceptWarmupRequests"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>

  <div v-if="!isSparkShadow">
    <div class="mb-1 flex items-center gap-2">
      <label class="input-label mb-0">{{ t('admin.accounts.proxy') }}</label>
      <ProxyAdBanner />
    </div>
    <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
  </div>

  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div>
      <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
      <input v-model.number="form.concurrency" type="number" min="1" class="input"
        @input="form.concurrency = Math.max(1, form.concurrency || 1)" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
      <input v-model.number="form.load_factor" type="number" min="1"
        class="input" :placeholder="String(form.concurrency || 1)"
        @input="form.load_factor = (form.load_factor &amp;&amp; form.load_factor >= 1) ? form.load_factor : null" />
      <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.priority') }}</label>
      <input
        v-model.number="form.priority"
        type="number"
        min="1"
        class="input"
        data-tour="account-form-priority"
      />
      <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
      <input
        v-model.number="form.rate_multiplier"
        type="number"
        min="0"
        step="0.001"
        class="input disabled:cursor-not-allowed disabled:opacity-60"
        data-testid="account-rate-multiplier"
        :disabled="upstreamBillingRateSyncEnabled"
      />
      <p class="input-hint">
        {{
          t(
            upstreamBillingRateSyncEnabled
              ? 'admin.accounts.upstreamBilling.syncRateManagedHint'
              : 'admin.accounts.billingRateMultiplierHint'
          )
        }}
      </p>
      <div
        v-if="isUpstreamBillingProbeEligible(account.platform, account.type)"
        class="mt-3 flex items-center justify-between gap-3"
      >
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.accounts.upstreamBilling.syncRate') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.upstreamBilling.syncRateHint') }}
          </p>
        </div>
        <Toggle
          v-model="upstreamBillingRateSyncEnabled"
          data-testid="upstream-billing-rate-sync"
          :aria-label="t('admin.accounts.upstreamBilling.syncRate')"
        />
      </div>
    </div>
  </div>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
    <input v-model="expiresAtInput" type="datetime-local" class="input" />
    <div class="mt-2 flex flex-wrap gap-2">
      <button type="button" class="btn btn-secondary btn-sm" @click="form.expires_at = getAccountExpiryTimestamp(1)">
        {{ t('payment.oneMonth') }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" @click="form.expires_at = getAccountExpiryTimestamp(12)">
        {{ t('payment.oneYear') }}
      </button>
    </div>
    <p class="input-hint">
      {{ t('admin.accounts.expiresAtHint') }}
      {{ t('admin.accounts.expiresAtTimezoneHint', { timezone: browserTimeZone }) }}
    </p>
  </div>


  <EditResponsesSettings v-if="account.platform !== 'openai'" :context="context" />
  <OllamaCloudUsageSettings
    v-if="account?.ollama_cloud_usage?.eligible"
    :account="account"
    @updated="handleOllamaCloudUsageUpdated"
  />

  <!-- Anthropic API Key 自动透传开关 -->
  <div
    v-if="account?.platform === 'anthropic' && account?.type === 'apikey'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyPassthrough') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.anthropic.apiKeyPassthroughDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="anthropicPassthroughEnabled = !anthropicPassthroughEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          anthropicPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            anthropicPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>

  <div
    v-if="account?.platform === 'anthropic' && account?.type === 'apikey'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyAuthScheme') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.anthropic.apiKeyAuthSchemeDesc') }}
        </p>
      </div>
      <select v-model="anthropicAPIKeyAuthScheme" class="input w-52 text-sm">
        <option value="x_api_key">{{ t('admin.accounts.anthropic.apiKeyAuthSchemeXApiKey') }}</option>
        <option value="authorization_bearer">{{ t('admin.accounts.anthropic.apiKeyAuthSchemeBearer') }}</option>
      </select>
    </div>
  </div>

  <!-- Anthropic / OpenAI API Key: Web Search Emulation (hidden when global disabled) -->
  <div
    v-if="(account?.platform === 'anthropic' || account?.platform === 'openai') && account?.type === 'apikey' && webSearchGlobalEnabled"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.anthropic.webSearchEmulation') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.anthropic.webSearchEmulationDesc') }}
        </p>
      </div>
      <select v-model="webSearchEmulationMode" class="input w-24 text-sm">
        <option value="default">{{ t('admin.accounts.anthropic.webSearchDefault') }}</option>
        <option value="enabled">{{ t('admin.accounts.anthropic.webSearchEnabled') }}</option>
        <option value="disabled">{{ t('admin.accounts.anthropic.webSearchDisabled') }}</option>
      </select>
    </div>
  </div>

  <!-- 配额控制 (Anthropic apikey/bedrock: 配额限制 + 亲和) -->
  <div
    v-if="account?.platform === 'anthropic' && (account?.type === 'apikey' || account?.type === 'bedrock')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
  >
    <div class="mb-3">
      <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaControl.hint') }}
      </p>
    </div>
    <QuotaLimitCard
      :totalLimit="editQuotaLimit"
      :dailyLimit="editQuotaDailyLimit"
      :weeklyLimit="editQuotaWeeklyLimit"
      :dailyResetMode="editDailyResetMode"
      :dailyResetHour="editDailyResetHour"
      :weeklyResetMode="editWeeklyResetMode"
      :weeklyResetDay="editWeeklyResetDay"
      :weeklyResetHour="editWeeklyResetHour"
      :resetTimezone="editResetTimezone"
      :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
      :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled"
      :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold"
      :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
      :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled"
      :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold"
      :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
      :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled"
      :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold"
      :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
      @update:totalLimit="editQuotaLimit = $event"
      @update:dailyLimit="editQuotaDailyLimit = $event"
      @update:weeklyLimit="editQuotaWeeklyLimit = $event"
      @update:dailyResetMode="editDailyResetMode = $event"
      @update:dailyResetHour="editDailyResetHour = $event"
      @update:weeklyResetMode="editWeeklyResetMode = $event"
      @update:weeklyResetDay="editWeeklyResetDay = $event"
      @update:weeklyResetHour="editWeeklyResetHour = $event"
      @update:resetTimezone="editResetTimezone = $event"
      @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event"
      @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event"
      @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
      @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event"
      @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event"
      @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
      @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event"
      @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event"
      @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
    />
  </div>
  <!-- 配额控制 (非 Anthropic apikey/bedrock) -->
  <div
    v-else-if="supportsAccountQuota(account?.type)"
    class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
  >
    <div class="mb-3">
      <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaLimitHint') }}
      </p>
    </div>
    <QuotaLimitCard
      :totalLimit="editQuotaLimit"
      :dailyLimit="editQuotaDailyLimit"
      :weeklyLimit="editQuotaWeeklyLimit"
      :dailyResetMode="editDailyResetMode"
      :dailyResetHour="editDailyResetHour"
      :weeklyResetMode="editWeeklyResetMode"
      :weeklyResetDay="editWeeklyResetDay"
      :weeklyResetHour="editWeeklyResetHour"
      :resetTimezone="editResetTimezone"
      :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
      :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled"
      :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold"
      :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
      :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled"
      :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold"
      :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
      :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled"
      :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold"
      :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
      @update:totalLimit="editQuotaLimit = $event"
      @update:dailyLimit="editQuotaDailyLimit = $event"
      @update:weeklyLimit="editQuotaWeeklyLimit = $event"
      @update:dailyResetMode="editDailyResetMode = $event"
      @update:dailyResetHour="editDailyResetHour = $event"
      @update:weeklyResetMode="editWeeklyResetMode = $event"
      @update:weeklyResetDay="editWeeklyResetDay = $event"
      @update:weeklyResetHour="editWeeklyResetHour = $event"
      @update:resetTimezone="editResetTimezone = $event"
      @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event"
      @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event"
      @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
      @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event"
      @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event"
      @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
      @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event"
      @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event"
      @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
    />
  </div>
</template>

<script setup lang="ts">
import EditResponsesSettings from './EditResponsesSettings.vue'
import { supportsAccountQuota } from '@/features/admin-accounts/presentation/accountQuota'

import Icon from '@/common/widgets/icons/Icon.vue'
import OllamaCloudUsageSettings from '../OllamaCloudUsageSettings.vue'
import ProxyAdBanner from '@/common/widgets/data/ProxyAdBanner.vue'
import ProxySelector from '@/common/widgets/data/ProxySelector.vue'
import QuotaLimitCard from '../QuotaLimitCard.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { getBrowserTimeZone } from '@/core/utils/format'
import type { EditAccountAdvancedContext } from '../../accountEditorContext'
import { getAccountExpiryTimestamp } from '../../accountExpiry'
import { isUpstreamBillingProbeEligible } from '../../upstreamBillingProbeEligibility'

const props = defineProps<{ context: EditAccountAdvancedContext }>()
const browserTimeZone = getBrowserTimeZone()
const {
  account, addTempUnschedRule, anthropicAPIKeyAuthScheme, anthropicPassthroughEnabled,
  editDailyResetHour, editDailyResetMode, editQuotaDailyLimit, editQuotaLimit, editQuotaWeeklyLimit,
  editResetTimezone, editWeeklyResetDay, editWeeklyResetHour, editWeeklyResetMode, expiresAtInput,
  form, getTempUnschedRuleKey, handleOllamaCloudUsageUpdated, interceptWarmupRequests,
  isSparkShadow, moveTempUnschedRule, proxies, quotaNotifyGlobalEnabled, quotaNotifyState,
  removeTempUnschedRule, t, tempUnschedEnabled, tempUnschedPresets, tempUnschedRules,
  upstreamBillingRateSyncEnabled, webSearchEmulationMode, webSearchGlobalEnabled
} = props.context
</script>
