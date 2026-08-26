<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between gap-4">
      <div class="flex-1">
        <label
          id="bulk-edit-openai-tls-fingerprint-label"
          class="input-label mb-0"
          for="bulk-edit-openai-tls-fingerprint-enabled"
        >
          {{ t('admin.accounts.quotaControl.tlsFingerprint.label') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.tlsFingerprint.hint') }}
        </p>
      </div>
      <input
        v-model="enableUpdate"
        id="bulk-edit-openai-tls-fingerprint-enabled"
        type="checkbox"
        aria-controls="bulk-edit-openai-tls-fingerprint-body"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div
      id="bulk-edit-openai-tls-fingerprint-body"
      :class="!enableUpdate && 'pointer-events-none opacity-50'"
    >
      <div class="mb-3 flex justify-end">
        <Toggle
          v-model="enabled"
          data-testid="bulk-edit-openai-tls-fingerprint-toggle"
          :aria-label="t('admin.accounts.quotaControl.tlsFingerprint.label')"
        />
      </div>
      <select
        v-model="profileId"
        class="input"
        data-testid="bulk-edit-openai-tls-fingerprint-profile"
        aria-labelledby="bulk-edit-openai-tls-fingerprint-label"
        @focus="loadProfiles"
      >
        <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
        <option v-if="profiles.length > 0" :value="-1">
          {{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}
        </option>
        <option v-for="profile in profiles" :key="profile.id" :value="profile.id">
          {{ profile.name }}
        </option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { list as listTLSFingerprintProfiles } from '@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource'

const { t } = useI18n()
const enableUpdate = defineModel<boolean>('enableUpdate', { required: true })
const enabled = defineModel<boolean>('enabled', { required: true })
const profileId = defineModel<number | null>('profileId', { required: true })

const profiles = ref<Array<{ id: number; name: string }>>([])
const loading = ref(false)

const loadProfiles = async () => {
  if (loading.value || profiles.value.length > 0) return
  loading.value = true
  try {
    const result = await listTLSFingerprintProfiles()
    profiles.value = result.map(profile => ({ id: profile.id, name: profile.name }))
  } catch {
    profiles.value = []
  } finally {
    loading.value = false
  }
}
</script>
