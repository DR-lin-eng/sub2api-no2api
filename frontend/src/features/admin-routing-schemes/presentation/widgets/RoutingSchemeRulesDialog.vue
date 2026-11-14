<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import type { RoutingSchemeRule } from '../../data/dtos/routingSchemeDtos'
import RoutingSchemeRuleEditor from './RoutingSchemeRuleEditor.vue'

const props = withDefaults(defineProps<{
  show: boolean
  groupName: string
  rules?: RoutingSchemeRule[]
}>(), {
  rules: () => [],
})

const emit = defineEmits<{
  close: []
  save: [rules: RoutingSchemeRule[]]
}>()

const { t } = useI18n()
const draft = ref<RoutingSchemeRule[]>([])

const cloneRules = (rules: RoutingSchemeRule[]): RoutingSchemeRule[] => rules.map(rule => ({
  ...rule,
  trigger: { ...rule.trigger } as RoutingSchemeRule['trigger'],
  action: { ...rule.action },
}))

watch(() => props.show, (show) => {
  if (show) draft.value = cloneRules(props.rules)
}, { immediate: true })

const save = () => {
  emit('save', cloneRules(draft.value))
  emit('close')
}
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('admin.routingSchemes.policy.dialogTitle', { name: groupName })"
    width="wide"
    :z-index="60"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">
        {{ t('admin.routingSchemes.policy.dialogHint') }}
      </p>
      <RoutingSchemeRuleEditor
        :rules="draft"
        @update:rules="draft = $event"
      />
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" @click="save">
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
