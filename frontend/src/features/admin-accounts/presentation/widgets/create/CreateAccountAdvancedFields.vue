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
    v-if="form.platform === 'anthropic' || form.platform === 'antigravity'"
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

  <!-- 配额控制 (Anthropic OAuth/SetupToken: 亲和 + 窗口费用 + 会话 + RPM 等) -->
  <div
    v-if="form.platform === 'anthropic' && accountCategory === 'oauth-based'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
  >
    <div class="mb-3">
      <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaControl.hint') }}
      </p>
    </div>

    <!-- Window Cost Limit -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.windowCost.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.windowCost.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="windowCostEnabled = !windowCostEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            windowCostEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              windowCostEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

      <div v-if="windowCostEnabled" class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.limit') }}</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              v-model.number="windowCostLimit"
              type="number"
              min="0"
              step="1"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaControl.windowCost.limitPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.limitHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.windowCost.stickyReserve') }}</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              v-model.number="windowCostStickyReserve"
              type="number"
              min="0"
              step="1"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaControl.windowCost.stickyReservePlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.stickyReserveHint') }}</p>
        </div>
      </div>
    </div>

    <!-- Session Limit -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionLimit.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.sessionLimit.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="sessionLimitEnabled = !sessionLimitEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            sessionLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              sessionLimitEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

      <div v-if="sessionLimitEnabled" class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessions') }}</label>
          <input
            v-model.number="maxSessions"
            type="number"
            min="1"
            step="1"
            class="input"
            :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessionsPlaceholder')"
          />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessionsHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeout') }}</label>
          <div class="relative">
            <input
              v-model.number="sessionIdleTimeout"
              type="number"
              min="1"
              step="1"
              class="input pr-12"
              :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeoutPlaceholder')"
            />
            <span class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">{{ t('common.minutes') }}</span>
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeoutHint') }}</p>
        </div>
      </div>
    </div>

    <!-- RPM Limit -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.rpmLimit.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="rpmLimitEnabled = !rpmLimitEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            rpmLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              rpmLimitEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

      <div v-if="rpmLimitEnabled" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
          <input
            v-model.number="baseRpm"
            type="number"
            min="1"
            max="1000"
            step="1"
            class="input"
            :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')"
          />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') }}</p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
          <div class="flex gap-2">
            <button
              type="button"
              @click="rpmStrategy = 'tiered'"
              :class="[
                'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                rpmStrategy === 'tiered'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <div class="text-center">
                <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') }}</div>
                <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyTieredHint') }}</div>
              </div>
            </button>
            <button
              type="button"
              @click="rpmStrategy = 'sticky_exempt'"
              :class="[
                'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                rpmStrategy === 'sticky_exempt'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              <div class="text-center">
                <div>{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') }}</div>
                <div class="mt-0.5 text-[10px] opacity-70">{{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExemptHint') }}</div>
              </div>
            </button>
          </div>
        </div>

        <div v-if="rpmStrategy === 'tiered'">
          <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
          <input
            v-model.number="rpmStickyBuffer"
            type="number"
            min="1"
            step="1"
            class="input"
            :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
          />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
        </div>

      </div>

      <!-- 用户消息限速模式（独立于 RPM 开关，始终可见） -->
      <div class="mt-4">
        <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueue') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400 mb-2">
          {{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueueHint') }}
        </p>
        <div class="flex space-x-2">
          <button type="button" v-for="opt in umqModeOptions" :key="opt.value"
            @click="userMsgQueueMode = opt.value"
            :class="[
              'px-3 py-1.5 text-sm rounded-md border transition-colors',
              userMsgQueueMode === opt.value
                ? 'bg-primary-600 text-white border-primary-600'
                : 'bg-white dark:bg-dark-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-dark-500 hover:bg-gray-50 dark:hover:bg-dark-600'
            ]">
            {{ opt.label }}
          </button>
        </div>
      </div>
    </div>

    <!-- TLS Fingerprint -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.tlsFingerprint.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.tlsFingerprint.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="tlsFingerprintEnabled = !tlsFingerprintEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            tlsFingerprintEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              tlsFingerprintEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <!-- Profile selector -->
      <div v-if="tlsFingerprintEnabled" class="mt-3">
        <select v-model="tlsFingerprintProfileId" class="input">
          <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
          <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}</option>
          <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>

    <!-- Session ID Masking -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.sessionIdMasking.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.sessionIdMasking.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="sessionIdMaskingEnabled = !sessionIdMaskingEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            sessionIdMaskingEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              sessionIdMaskingEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
    </div>

    <!-- Cache TTL Override -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.cacheTTLOverride.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.cacheTTLOverride.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="cacheTTLOverrideEnabled = !cacheTTLOverrideEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            cacheTTLOverrideEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              cacheTTLOverrideEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <div v-if="cacheTTLOverrideEnabled" class="mt-3">
        <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.cacheTTLOverride.target') }}</label>
        <select
          v-model="cacheTTLOverrideTarget"
          class="mt-1 block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-700 dark:text-white"
        >
          <option value="5m">5m</option>
          <option value="1h">1h</option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.cacheTTLOverride.targetHint') }}
        </p>
      </div>
    </div>

    <!-- Custom Base URL Relay -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.customBaseUrl.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.customBaseUrl.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="customBaseUrlEnabled = !customBaseUrlEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            customBaseUrlEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              customBaseUrlEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <div v-if="customBaseUrlEnabled" class="mt-3">
        <input
          v-model="customBaseUrl"
          type="text"
          class="input"
          :placeholder="t('admin.accounts.quotaControl.customBaseUrl.urlHint')"
        />
      </div>
    </div>
  </div>


  <div>
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
      <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
      <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
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


  <!-- Anthropic API Key 自动透传开关 -->
  <div
    v-if="form.platform === 'anthropic' && accountCategory === 'apikey'"
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
    v-if="form.platform === 'anthropic' && accountCategory === 'apikey'"
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
    v-if="(form.platform === 'anthropic' || form.platform === 'openai') && accountCategory === 'apikey' && webSearchGlobalEnabled"
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


  <div>
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{
          t('admin.accounts.autoPauseOnExpired')
        }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.autoPauseOnExpiredDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="autoPauseOnExpired = !autoPauseOnExpired"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          autoPauseOnExpired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            autoPauseOnExpired ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>

  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <!-- Mixed Scheduling (only for antigravity accounts) -->
    <div v-if="form.platform === 'antigravity'" class="flex items-center gap-2">
      <label class="flex cursor-pointer items-center gap-2">
        <input
          type="checkbox"
          v-model="mixedScheduling"
          class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
        />
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.mixedScheduling') }}
        </span>
      </label>
      <div class="group relative">
        <span
          class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
        >
          ?
        </span>
        <!-- Tooltip（向下显示避免被弹窗裁剪） -->
        <div
          class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
        >
          {{ t('admin.accounts.mixedSchedulingTooltip') }}
          <div
            class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
          ></div>
        </div>
      </div>
    </div>
    <div v-if="form.platform === 'antigravity'" class="mt-3 flex items-center gap-2">
      <label class="flex cursor-pointer items-center gap-2">
        <input
          type="checkbox"
          v-model="allowOverages"
          class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
        />
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.allowOverages') }}
        </span>
      </label>
      <div class="group relative">
        <span
          class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
        >
          ?
        </span>
        <div
          class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
        >
          {{ t('admin.accounts.allowOveragesTooltip') }}
          <div
            class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
          ></div>
        </div>
      </div>
    </div>

    <!-- Group Selection - 仅标准模式显示 -->
    <GroupSelector
      v-if="!isSimpleMode"
      v-model="form.group_ids"
      :groups="groups"
      :platform="form.platform"
      :mixed-scheduling="mixedScheduling"
      data-tour="account-form-groups"
    />
  </div>
</template>

<script setup lang="ts">
import GroupSelector from '@/common/widgets/data/GroupSelector.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import ProxyAdBanner from '@/common/widgets/data/ProxyAdBanner.vue'
import ProxySelector from '@/common/widgets/data/ProxySelector.vue'
import { getBrowserTimeZone } from '@/core/utils/format'
import type { CreateAccountAdvancedContext } from '../../accountEditorContext'
import { getAccountExpiryTimestamp } from '../../accountExpiry'

const props = defineProps<{ context: CreateAccountAdvancedContext }>()
const browserTimeZone = getBrowserTimeZone()
const {
  accountCategory, addTempUnschedRule, allowOverages, anthropicAPIKeyAuthScheme,
  anthropicPassthroughEnabled, isSimpleMode, autoPauseOnExpired, baseRpm, cacheTTLOverrideEnabled,
  cacheTTLOverrideTarget, customBaseUrl, customBaseUrlEnabled, expiresAtInput, form,
  getTempUnschedRuleKey, groups, interceptWarmupRequests, maxSessions, mixedScheduling,
  moveTempUnschedRule, proxies, removeTempUnschedRule, rpmLimitEnabled, rpmStickyBuffer,
  rpmStrategy, sessionIdMaskingEnabled, sessionIdleTimeout, sessionLimitEnabled, t,
  tempUnschedEnabled, tempUnschedPresets, tempUnschedRules, tlsFingerprintEnabled,
  tlsFingerprintProfileId, tlsFingerprintProfiles, umqModeOptions, userMsgQueueMode,
  webSearchEmulationMode, webSearchGlobalEnabled, windowCostEnabled, windowCostLimit,
  windowCostStickyReserve
} = props.context
</script>
