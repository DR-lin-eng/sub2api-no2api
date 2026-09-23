<template>
  <div class="space-y-6">
    <SettingsSecurityApiKeysPanel />
    <SettingsSecurityAccessPanel />
    <SettingsSecurityIdentityProvidersPanel />
    <SettingsOAuth2ProviderPanel v-if="authStore.isAdmin" />
  </div>
</template>

<script setup lang="ts">
import SettingsSecurityApiKeysPanel from './SettingsSecurityApiKeysPanel.vue'
import SettingsSecurityAccessPanel from './SettingsSecurityAccessPanel.vue'
import SettingsSecurityIdentityProvidersPanel from './SettingsSecurityIdentityProvidersPanel.vue'
import SettingsOAuth2ProviderPanel from './SettingsOAuth2ProviderPanel.vue'
import { useAuthStore } from '@/features/auth'

const authStore = useAuthStore()
</script>
