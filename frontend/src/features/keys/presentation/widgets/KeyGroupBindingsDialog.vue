<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ApiKeyGroupBinding } from '@/types'
import type { GroupOption } from '../keysPageContext'
import KeyGroupBindingsEditor from './KeyGroupBindingsEditor.vue'

const props = defineProps<{
  show: boolean
  apiKeyName: string
  modelValue: ApiKeyGroupBinding[]
  groupOptions: GroupOption[]
  saving: boolean
}>()

const emit = defineEmits<{
  close: []
  save: []
  'update:modelValue': [value: ApiKeyGroupBinding[]]
}>()

const { t } = useI18n()

const bindings = computed({
  get: () => props.modelValue,
  set: (value: ApiKeyGroupBinding[]) => emit('update:modelValue', value)
})

const primaryName = computed(() => {
  const primaryID = bindings.value[0]?.group_id
  return props.groupOptions.find(option => option.value === primaryID)?.label ?? (primaryID ? `#${primaryID}` : '')
})
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('keys.groupBindings.dialogTitle')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-5" data-test="key-group-bindings-dialog">
      <div class="rounded-xl border border-primary-100 bg-primary-50/70 p-4 dark:border-primary-900/50 dark:bg-primary-950/25">
        <div class="flex items-start gap-3">
          <span class="mt-0.5 rounded-lg bg-white p-2 text-primary-600 shadow-sm dark:bg-dark-800 dark:text-primary-400">
            <Icon name="swap" size="md" :stroke-width="2" />
          </span>
          <div class="min-w-0">
            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">
              {{ t('keys.groupBindings.dialogDescription', { name: apiKeyName }) }}
            </p>
            <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
              {{ t('keys.groupBindings.orderHint') }}
            </p>
            <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
              <span class="rounded-full bg-white px-2 py-1 font-medium text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                {{ t('keys.groupBindings.groupCount', { count: bindings.length }) }}
              </span>
              <span v-if="primaryName" class="text-gray-500 dark:text-gray-400">
                {{ t('keys.groupBindings.currentPrimary', { group: primaryName }) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <KeyGroupBindingsEditor
        v-model="bindings"
        :group-options="groupOptions"
      />
    </div>

    <template #footer>
      <div class="flex w-full flex-col-reverse gap-2 sm:flex-row sm:justify-end sm:gap-3">
        <button type="button" class="btn btn-secondary w-full sm:w-auto" :disabled="saving" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary w-full sm:w-auto"
          :disabled="saving"
          data-test="key-group-bindings-save"
          @click="emit('save')"
        >
          <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
          {{ saving ? t('common.saving') : t('keys.groupBindings.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
