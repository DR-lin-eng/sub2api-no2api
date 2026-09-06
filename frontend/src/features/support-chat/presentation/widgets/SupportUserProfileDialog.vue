<template>
  <BaseDialog
    :show="show"
    :title="t('supportChat.userProfile.title')"
    width="normal"
    initial-focus="dialog"
    @close="$emit('close')"
  >
    <div v-if="loading" class="flex items-center justify-center py-10">
      <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </div>

    <div v-else-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-2xl bg-gray-50 p-4 dark:bg-dark-800">
        <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-primary-100 text-lg font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
          {{ userInitial }}
        </div>
        <div class="min-w-0">
          <p class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="truncate text-sm text-gray-500 dark:text-dark-400">
            {{ user.username || t('supportChat.userProfile.noUsername') }}
          </p>
        </div>
        <span
          class="ml-auto shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold"
          :class="user.status === 'active'
            ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200'
            : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-200'"
        >
          {{ statusLabel(user.status) }}
        </span>
      </div>

      <dl class="grid gap-3 sm:grid-cols-2">
        <InfoItem :label="t('supportChat.userProfile.userId')" :value="`#${user.id}`" />
        <InfoItem :label="t('supportChat.userProfile.role')" :value="roleLabel(user.role)" />
        <InfoItem :label="t('supportChat.userProfile.balance')" :value="formatCurrency(user.balance)" />
        <InfoItem :label="t('supportChat.userProfile.availableBalance')" :value="formatCurrency(user.available_balance ?? user.balance)" />
        <InfoItem :label="t('supportChat.userProfile.concurrency')" :value="String(user.concurrency)" />
        <InfoItem :label="t('supportChat.userProfile.rpmLimit')" :value="rpmLimitLabel(user.rpm_limit)" />
        <InfoItem :label="t('supportChat.userProfile.createdAt')" :value="formatNullableDate(user.created_at)" />
        <InfoItem :label="t('supportChat.userProfile.lastActiveAt')" :value="formatNullableDate(user.last_active_at)" />
        <InfoItem :label="t('supportChat.userProfile.lastUsedAt')" :value="formatNullableDate(user.last_used_at)" />
        <InfoItem :label="t('supportChat.userProfile.schedulingTier')" :value="schedulingTierLabel(user.scheduling_tier)" />
      </dl>

      <div v-if="user.notes" class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
        <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('supportChat.userProfile.notes') }}</dt>
        <dd class="mt-1 whitespace-pre-wrap text-sm text-gray-800 dark:text-dark-100">{{ user.notes }}</dd>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import { formatCurrency, formatDateTime } from '@/core/utils/format'
import type { BasicUser } from '@/features/admin-users/data/datasources/adminUsersDatasource'
import type { RequestSchedulingTier } from '@/types'

const props = defineProps<{
  show: boolean
  loading: boolean
  user: BasicUser | null
}>()

defineEmits<{
  close: []
}>()

const { t } = useI18n()

const userInitial = computed(() => {
  const source = props.user?.username || props.user?.email || '#'
  return source.charAt(0).toUpperCase()
})

const InfoItem = {
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(itemProps: { label: string; value: string }) {
    return () => h('div', { class: 'rounded-xl border border-gray-200 p-3 dark:border-dark-700' }, [
      h('dt', { class: 'text-xs font-medium text-gray-500 dark:text-dark-400' }, itemProps.label),
      h('dd', { class: 'mt-1 break-words text-sm font-medium text-gray-900 dark:text-white' }, itemProps.value || '-'),
    ])
  },
}

function statusLabel(status: BasicUser['status']): string {
  return status === 'active' ? t('supportChat.userProfile.active') : t('supportChat.userProfile.disabled')
}

function roleLabel(role: BasicUser['role']): string {
  return role === 'admin' ? t('supportChat.userProfile.admin') : role === 'user' ? t('supportChat.userProfile.normalUser') : role
}

function rpmLimitLabel(value: number | undefined): string {
  if (!value) return t('supportChat.userProfile.unlimited')
  return String(value)
}

function schedulingTierLabel(value: RequestSchedulingTier): string {
  if (value === 0) return t('supportChat.userProfile.tierPriority')
  if (value === 2) return t('supportChat.userProfile.tierLow')
  return t('supportChat.userProfile.tierNormal')
}

function formatNullableDate(value: string | null | undefined): string {
  return value ? formatDateTime(value) || '-' : '-'
}
</script>
