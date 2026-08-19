<template>
  <BaseDialog :show="visible" :title="dialogTitle" width="wide" @close="handleClose">
    <form @submit.prevent="handleSubmit" class="space-y-4">
      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.modal.modelName') }}
          <span class="text-red-500">*</span>
        </label>
        <input
          v-model="form.model_name"
          type="text"
          :placeholder="t('admin.customModelConfig.modal.modelNamePlaceholder')"
          class="input w-full"
          :disabled="isEdit"
          required
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{
            form.prefix_match
              ? t('admin.customModelConfig.modal.modelNamePrefixHint')
              : t('admin.customModelConfig.modal.modelNameHint')
          }}
        </p>
      </div>

      <label
        class="flex cursor-pointer items-start gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      >
        <input
          v-model="form.prefix_match"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-dark-700"
        />
        <span>
          <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.modal.prefixMatch') }}
          </span>
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.modal.prefixMatchHint') }}
          </span>
        </span>
      </label>

      <div>
        <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.modal.capabilities') }}
          <span class="text-red-500">*</span>
        </label>
        <div class="space-y-2">
          <label
            v-for="cap in availableCapabilities"
            :key="cap"
            class="flex cursor-pointer items-center gap-2"
          >
            <input
              type="checkbox"
              :value="cap"
              v-model="form.capabilities"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-dark-700"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ getCapabilityLabel(cap) }}</span>
          </label>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.modal.capabilitiesHint') }}
        </p>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.modal.requestTemplate') }}
        </label>
        <select v-model="form.template_id" class="input w-full">
          <option :value="null">{{ t('admin.customModelConfig.modal.noTemplate') }}</option>
          <option v-for="template in templates" :key="template.id" :value="template.id">
            {{ template.name }}
          </option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.modal.requestTemplateHint') }}
        </p>
      </div>

      <div class="flex justify-end gap-3 pt-4">
        <button type="button" @click="handleClose" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" :disabled="!isFormValid || submitting" class="btn btn-primary">
          <Icon v-if="submitting" name="refresh" size="md" class="mr-2 animate-spin" />
          {{ isEdit ? t('common.save') : t('common.create') }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue';
import Icon from '@/common/widgets/icons/Icon.vue';
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource';
import type {
  CustomModelConfig,
  CustomModelRequestTemplate,
  ModelCapability,
} from '../../domain/entities/customModelConfig';

const { t } = useI18n();

interface Props {
  visible: boolean;
  config?: CustomModelConfig | null;
  templates: CustomModelRequestTemplate[];
}

const props = defineProps<Props>();
const emit = defineEmits<{
  close: [];
  saved: [];
}>();

const availableCapabilities: ModelCapability[] = ['image', 'video', 'audio'];

const submitting = ref(false);
const form = ref({
  model_name: '',
  prefix_match: false,
  capabilities: [] as ModelCapability[],
  template_id: null as number | null,
});

const isEdit = computed(() => !!props.config);
const dialogTitle = computed(() =>
  isEdit.value
    ? t('admin.customModelConfig.modal.editTitle')
    : t('admin.customModelConfig.modal.createTitle')
);

const isFormValid = computed(() => form.value.model_name.trim().length > 0 && form.value.capabilities.length > 0);

watch(
  () => props.config,
  (config) => {
    if (config) {
      form.value = {
        model_name: config.model_name,
        prefix_match: config.prefix_match,
        capabilities: [...config.capabilities],
        template_id: config.template_id ?? null,
      };
    } else {
      resetForm();
    }
  },
  { immediate: true }
);

function resetForm() {
  form.value = {
    model_name: '',
    prefix_match: false,
    capabilities: [],
    template_id: null,
  };
}

function getCapabilityLabel(cap: ModelCapability): string {
  return t(`admin.customModelConfig.capabilities.${cap}`);
}

function handleClose() {
  emit('close');
}

async function handleSubmit() {
  if (!isFormValid.value || submitting.value) return;

  submitting.value = true;
  try {
    if (isEdit.value && props.config) {
      await customModelConfigDatasource.update(props.config.id, {
        prefix_match: form.value.prefix_match,
        capabilities: form.value.capabilities,
        template_id: form.value.template_id,
      });
    } else {
      await customModelConfigDatasource.create({
        model_name: form.value.model_name.trim(),
        prefix_match: form.value.prefix_match,
        capabilities: form.value.capabilities,
        template_id: form.value.template_id,
      });
    }
    emit('saved');
  } catch (error) {
    console.error('Failed to save model config:', error);
    alert(t('admin.customModelConfig.modal.saveFailed'));
  } finally {
    submitting.value = false;
  }
}
</script>
