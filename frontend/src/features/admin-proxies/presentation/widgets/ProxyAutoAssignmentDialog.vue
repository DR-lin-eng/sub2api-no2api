<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ProxyAutoAssignmentDialogContext } from '../proxyPageContext'

const props = defineProps<{
  context: ProxyAutoAssignmentDialogContext
}>()

const { t } = useI18n()
const {
  autoAssignmentForm,
  autoAssignmentLoading,
  autoAssignmentRebalancing,
  autoAssignmentSaving,
  closeAutoAssignmentDialog,
  rebalanceAutoAssignments,
  saveAutoAssignmentSettings,
  showAutoAssignmentDialog
} = props.context
</script>

<template>
  <BaseDialog
    :show="showAutoAssignmentDialog"
    :title="t('admin.proxies.autoAssignment.title')"
    width="normal"
    @close="closeAutoAssignmentDialog"
  >
    <div v-if="autoAssignmentLoading" class="flex min-h-48 items-center justify-center">
      <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
    </div>
    <form
      v-else
      id="proxy-auto-assignment-form"
      class="divide-y divide-gray-200 dark:divide-dark-600"
      @submit.prevent="saveAutoAssignmentSettings"
    >
      <div class="flex items-center justify-between gap-6 py-4 first:pt-0">
        <label class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.proxies.autoAssignment.enabled') }}
        </label>
        <Toggle v-model="autoAssignmentForm.enabled" />
      </div>

      <div class="flex items-center justify-between gap-6 py-4">
        <label class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.proxies.autoAssignment.healthCheckEnabled') }}
        </label>
        <Toggle
          v-model="autoAssignmentForm.health_check_enabled"
          :disabled="!autoAssignmentForm.enabled"
        />
      </div>

      <div class="grid grid-cols-1 gap-4 py-4 sm:grid-cols-2">
        <div>
          <label for="proxy-health-interval" class="input-label">
            {{ t('admin.proxies.autoAssignment.intervalMinutes') }}
          </label>
          <input
            id="proxy-health-interval"
            v-model.number="autoAssignmentForm.health_check_interval_minutes"
            type="number"
            min="1"
            max="1440"
            required
            class="input"
            :disabled="!autoAssignmentForm.enabled || !autoAssignmentForm.health_check_enabled"
          />
        </div>
        <div>
          <label for="proxy-health-failure-threshold" class="input-label">
            {{ t('admin.proxies.autoAssignment.failureThreshold') }}
          </label>
          <input
            id="proxy-health-failure-threshold"
            v-model.number="autoAssignmentForm.failure_threshold"
            type="number"
            min="1"
            max="10"
            required
            class="input"
            :disabled="!autoAssignmentForm.enabled || !autoAssignmentForm.health_check_enabled"
          />
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex w-full flex-wrap items-center justify-between gap-3">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="!autoAssignmentForm.enabled || autoAssignmentRebalancing || autoAssignmentSaving"
          @click="rebalanceAutoAssignments"
        >
          <Icon
            name="sync"
            size="sm"
            class="mr-2"
            :class="autoAssignmentRebalancing ? 'animate-spin' : ''"
          />
          {{ t('admin.proxies.autoAssignment.rebalance') }}
        </button>
        <div class="ml-auto flex gap-3">
          <button type="button" class="btn btn-secondary" @click="closeAutoAssignmentDialog">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="proxy-auto-assignment-form"
            class="btn btn-primary"
            :disabled="autoAssignmentLoading || autoAssignmentSaving || autoAssignmentRebalancing"
          >
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>
