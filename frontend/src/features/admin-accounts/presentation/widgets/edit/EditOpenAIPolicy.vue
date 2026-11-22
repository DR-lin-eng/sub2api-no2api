<template>
  <!-- OpenAI 订阅档位手动覆盖（Plus/Pro/Free），仅 OAuth 非影子账号 -->
  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth' && !isSparkShadow"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between gap-4">
      <div class="min-w-0">
        <label class="input-label mb-0">{{ t('admin.accounts.openai.planType') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.planTypeDesc') }}
        </p>
      </div>
      <div class="w-44 flex-shrink-0">
        <Select v-model="editPlanType" :options="planTypeOptions" />
      </div>
    </div>
  </div>

  <div
    v-if="account?.platform === 'openai' && (account?.type === 'oauth' || account?.type === 'setup-token' || account?.type === 'apikey')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.compactModeDesc') }}
        </p>
      </div>
      <div class="w-44">
        <Select v-model="openAICompactMode" :options="openAICompactModeOptions" />
      </div>
    </div>
    <div class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
      <span class="font-medium">{{ t(openAICompactStatusKey) }}</span>
      <span
        v-if="account?.extra?.openai_compact_checked_at"
        class="ml-2 text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.openai.compactLastChecked') }}:
        {{ formatDateTime(new Date(String(account.extra.openai_compact_checked_at))) }}
      </span>
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') }}</label>
      <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') }}</p>
      <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
        <div
          v-for="(mapping, index) in openAICompactModelMappings"
          :key="getOpenAICompactModelMappingKey(mapping)"
          class="flex items-center gap-2"
        >
          <input
            v-model="mapping.from"
            type="text"
            class="input flex-1"
            :placeholder="t('admin.accounts.fromModel')"
          />
          <span class="text-gray-400">→</span>
          <input
            v-model="mapping.to"
            type="text"
            class="input flex-1"
            :placeholder="t('admin.accounts.toModel')"
          />
          <button type="button" @click="removeOpenAICompactModelMapping(index)" class="text-red-500 hover:text-red-700">
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
      <button type="button" @click="addOpenAICompactModelMapping" class="btn btn-secondary text-sm">
        + {{ t('admin.accounts.addMapping') }}
      </button>
    </div>
  </div>

  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth' && !isSparkShadow && !isOpenAIPersonalAccessTokenAccount && account?.credentials?.auth_mode !== 'agentIdentity'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.excelBpsEnabled') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.excelBpsEnabledDesc') }}</p>
      </div>
      <button
        type="button"
        data-test="excel-bps-toggle"
        @click="excelBpsEnabled = !excelBpsEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          excelBpsEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out', excelBpsEnabled ? 'translate-x-5' : 'translate-x-0']" />
      </button>
    </div>
  </div>
  <OpenAIQuotaPauseFields
    v-model:auto-pause5h-disabled="autoPause5hDisabled" v-model:auto-pause7d-disabled="autoPause7dDisabled"
    v-model:auto-pause5h-threshold="autoPause5hThreshold" v-model:auto-pause7d-threshold="autoPause7dThreshold"
  />

  <!-- OpenAI/Codex OAuth custom relay -->
  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.customBaseUrl.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.customBaseUrl.openaiHint') }}
          </p>
        </div>
        <button
          type="button"
          data-testid="edit-openai-custom-base-url-toggle"
          :aria-pressed="customBaseUrlEnabled"
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
          data-testid="edit-openai-custom-base-url"
          type="url"
          class="input"
          :placeholder="t('admin.accounts.quotaControl.customBaseUrl.openaiUrlHint')"
        />
      </div>
    </div>
  </div>

  <!-- OpenAI/Codex OAuth TLS profile -->
  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth' && !isSparkShadow"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
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
          data-testid="edit-openai-tls-fingerprint-toggle"
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
      <div v-if="tlsFingerprintEnabled" class="mt-3">
        <select v-model="tlsFingerprintProfileId" class="input">
          <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
          <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}</option>
          <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import OpenAIQuotaPauseFields from '../OpenAIQuotaPauseFields.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Select from '@/common/widgets/forms/Select.vue'
import type { EditAccountPolicyContext } from '../../accountEditorContext'

const props = defineProps<{ context: EditAccountPolicyContext }>()
const {
  account, addOpenAICompactModelMapping, autoPause5hDisabled, autoPause5hThreshold,
  autoPause7dDisabled, autoPause7dThreshold, customBaseUrl, customBaseUrlEnabled, editPlanType,
  excelBpsEnabled, formatDateTime, getOpenAICompactModelMappingKey,
  isOpenAIPersonalAccessTokenAccount, isSparkShadow, openAICompactMode, openAICompactModeOptions,
  openAICompactModelMappings, openAICompactStatusKey, planTypeOptions,
  removeOpenAICompactModelMapping, t, tlsFingerprintEnabled, tlsFingerprintProfileId,
  tlsFingerprintProfiles
} = props.context
</script>
