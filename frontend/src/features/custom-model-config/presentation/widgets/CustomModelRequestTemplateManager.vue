<template>
  <BaseDialog :show="visible" :title="t('admin.customModelConfig.template.managerTitle')" width="extra-wide" @close="emit('close')">
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="min-w-0 flex-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.managerHint') }}
        </p>
        <div class="flex shrink-0 items-center gap-2">
          <button class="btn btn-primary" @click="openCreate">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.customModelConfig.template.create') }}
          </button>
          <button class="btn btn-secondary" @click="importVisible = true">
            <Icon name="inbox" size="md" class="mr-2" />
            {{ t('admin.customModelConfig.template.import') }}
          </button>
        </div>
      </div>
      <div class="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div
          v-for="template in templates"
          :key="template.id"
          class="flex items-center justify-between gap-4 p-4"
        >
          <div class="min-w-0">
            <div class="font-medium text-gray-900 dark:text-white">{{ template.name }}</div>
            <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
              {{ template.description || t('admin.customModelConfig.template.noDescription') }}
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-3">
            <button
              class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              :title="t('admin.customModelConfig.template.export')"
              @click="copyTemplate(template)"
            >
              <Icon name="copy" size="md" />
            </button>
            <button class="text-gray-600 hover:text-primary-600 dark:text-gray-400" :title="t('common.edit')" @click="openEdit(template)">
              <Icon name="edit" size="md" />
            </button>
            <button class="text-gray-600 hover:text-red-600 dark:text-gray-400" :title="t('common.delete')" @click="remove(template)">
              <Icon name="trash" size="md" />
            </button>
          </div>
        </div>
        <div v-if="templates.length === 0" class="p-8 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.empty') }}
        </div>
      </div>
      <div class="flex justify-end pt-2">
        <button class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
      </div>
    </div>

    <CustomModelRequestTemplateDialog
      v-if="templateDialogVisible"
      :visible="templateDialogVisible"
      :template="editingTemplate"
      @close="closeTemplateDialog"
      @saved="handleTemplateSaved"
    />
    <CustomModelRequestTemplateImportDialog
      v-if="importVisible"
      :visible="importVisible"
      @close="importVisible = false"
      @saved="handleImported"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import CustomModelRequestTemplateDialog from './CustomModelRequestTemplateDialog.vue'
import CustomModelRequestTemplateImportDialog from './CustomModelRequestTemplateImportDialog.vue'
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource'
import type { CustomModelRequestTemplate } from '../../domain/entities/customModelConfig'

defineProps<{
  visible: boolean
  templates: CustomModelRequestTemplate[]
}>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()

const templateDialogVisible = ref(false)
const importVisible = ref(false)
const editingTemplate = ref<CustomModelRequestTemplate | null>(null)

function openCreate() {
  editingTemplate.value = null
  templateDialogVisible.value = true
}

function openEdit(template: CustomModelRequestTemplate) {
  editingTemplate.value = template
  templateDialogVisible.value = true
}

function closeTemplateDialog() {
  templateDialogVisible.value = false
  editingTemplate.value = null
}

function handleTemplateSaved() {
  closeTemplateDialog()
  emit('changed')
}

function handleImported() {
  importVisible.value = false
  emit('changed')
}

async function copyTemplate(template: CustomModelRequestTemplate) {
  const payload = {
    name: template.name,
    description: template.description,
    request_adapter: template.request_adapter ?? {},
  }
  const content = JSON.stringify(payload, null, 2)
  try {
    await navigator.clipboard.writeText(content)
    alert(t('admin.customModelConfig.template.copied'))
  } catch (error) {
    console.error('Failed to copy request template:', error)
    alert(t('admin.customModelConfig.template.copyFailed'))
  }
}

async function remove(template: CustomModelRequestTemplate) {
  if (!confirm(t('admin.customModelConfig.template.deleteConfirm'))) return
  try {
    await customModelConfigDatasource.deleteTemplate(template.id)
    emit('changed')
  } catch (error) {
    console.error('Failed to delete request template:', error)
    alert(t('admin.customModelConfig.template.deleteFailed'))
  }
}
</script>
