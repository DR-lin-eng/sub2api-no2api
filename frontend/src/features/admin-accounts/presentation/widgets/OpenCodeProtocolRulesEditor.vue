<template>
  <div class="space-y-2" data-testid="opencode-protocol-rules">
    <div class="flex items-start justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.opencode.protocolRules') }}</label>
        <p class="input-hint">{{ t('admin.accounts.opencode.protocolRulesHint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm shrink-0 whitespace-nowrap" @click="restoreDefaults">
        {{ t('admin.accounts.opencode.restoreProtocolRules') }}
      </button>
    </div>

    <div
      v-for="(row, index) in rows"
      :key="getRowKey(row)"
      class="grid grid-cols-[minmax(0,1fr)_2.25rem] gap-2 sm:grid-cols-[minmax(0,1fr)_12rem_2.25rem]"
    >
      <input
        v-model="row.pattern"
        type="text"
        class="input col-start-1 row-start-1 min-w-0 font-mono"
        :placeholder="t('admin.accounts.opencode.protocolPatternPlaceholder')"
        :data-testid="`opencode-protocol-pattern-${index}`"
      />
      <select
        v-model="row.protocol"
        class="input col-start-1 row-start-2 sm:col-start-2 sm:row-start-1"
        :data-testid="`opencode-protocol-value-${index}`"
      >
        <option value="chat_completions">{{ t('admin.accounts.cnProvider.chatCompletions') }}</option>
        <option value="anthropic">{{ t('admin.accounts.cnProvider.anthropic') }}</option>
        <option value="responses">{{ t('admin.accounts.cnProvider.responses') }}</option>
      </select>
      <button
        type="button"
        class="col-start-2 row-start-1 inline-flex h-9 w-9 items-center justify-center self-center text-red-600 hover:text-red-700 sm:col-start-3"
        :aria-label="t('admin.accounts.opencode.removeProtocolRule')"
        @click="removeRow(index)"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>

    <p class="input-hint font-mono">{{ t('admin.accounts.opencode.protocolFallback') }}</p>
    <button type="button" class="btn btn-secondary btn-sm" data-testid="opencode-protocol-add" @click="addRow">
      <Icon name="plus" size="sm" class="mr-1" />
      {{ t('admin.accounts.opencode.addProtocolRule') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import { createStableObjectKeyResolver } from '@/core/utils/stableObjectKey'
import {
  defaultOpenCodeProtocolRules,
  type OpenCodeAccountMode,
  type OpenCodeProtocolRule,
} from '@/core/constants/account'

const props = defineProps<{
  rows: OpenCodeProtocolRule[]
  plan: OpenCodeAccountMode
}>()

const emit = defineEmits<{
  'update:rows': [rows: OpenCodeProtocolRule[]]
}>()

const { t } = useI18n()
const getRowKey = createStableObjectKeyResolver<OpenCodeProtocolRule>('opencode-protocol-rule')

const addRow = () => {
  emit('update:rows', [...props.rows, { pattern: '', protocol: 'chat_completions' }])
}

const removeRow = (index: number) => {
  emit('update:rows', props.rows.filter((_, rowIndex) => rowIndex !== index))
}

const restoreDefaults = () => {
  emit('update:rows', defaultOpenCodeProtocolRules(props.plan))
}
</script>
