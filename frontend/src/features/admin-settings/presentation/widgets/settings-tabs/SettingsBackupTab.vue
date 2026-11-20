<template>
  <div class="space-y-5">
    <RouterLink v-if="auth.isAdmin" to="/admin/site-migration" class="btn btn-secondary">{{ t('nav.siteMigration') }}</RouterLink>
    <BackupSettings />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/features/auth'
import BackupSettings from '@/features/admin-backup/presentation/pages/BackupPage.vue'

const auth = useAuthStore()
const { t } = useI18n()
</script>
