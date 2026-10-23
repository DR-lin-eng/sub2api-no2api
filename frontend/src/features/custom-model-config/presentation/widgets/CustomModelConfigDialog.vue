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

      <div v-if="form.capabilities.includes('video')">
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.modal.videoApiType.label') }}
        </label>
        <select v-model="form.video_api_type" class="input w-full">
          <option :value="null">{{ t('admin.customModelConfig.modal.videoApiType.autoDetect') }}</option>
          <option value="grok">{{ t('admin.customModelConfig.modal.videoApiType.grok') }}</option>
          <option value="agnes">{{ t('admin.customModelConfig.modal.videoApiType.agnes') }}</option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.modal.videoApiTypeHint') }}
        </p>
      </div>

      <div
        v-if="form.capabilities.length > 0"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      >
        <div class="mb-3">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.modal.mediaLimits.title') }}
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.modal.mediaLimits.hint') }}
          </p>
        </div>

        <label class="flex cursor-pointer items-start gap-3">
          <input
            v-model="form.media_limits.allow_upload"
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-dark-700"
          />
          <span>
            <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.customModelConfig.modal.mediaLimits.allowUpload') }}
            </span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.customModelConfig.modal.mediaLimits.allowUploadHint') }}
            </span>
          </span>
        </label>

        <div class="mt-4 space-y-2">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.modal.mediaLimits.fileTypes') }}
          </div>
          <div
            v-for="kind in mediaFileKinds"
            :key="kind"
            class="rounded-md border border-gray-200 p-2 dark:border-dark-600"
          >
            <label class="flex cursor-pointer items-center gap-2">
              <input
                v-model="form.media_limits.file_types"
                type="checkbox"
                :value="kind"
                :disabled="!form.media_limits.allow_upload"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed dark:border-gray-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">
                {{ t(`admin.customModelConfig.modal.mediaLimits.kinds.${kind}`) }}
              </span>
            </label>
            <div class="mt-2 grid grid-cols-2 gap-2">
              <label class="block">
                <span class="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.modal.mediaLimits.maxFiles') }}
                </span>
                <input
                  v-model.number="form.media_limits.max_files[kind]"
                  type="number"
                  min="0"
                  max="32"
                  :disabled="!isMediaKindEnabled(kind)"
                  :placeholder="t('admin.customModelConfig.modal.mediaLimits.unlimited')"
                  class="input w-full"
                />
              </label>
              <label class="block">
                <span class="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.modal.mediaLimits.maxFileSize') }}
                </span>
                <input
                  v-model.number="form.media_limits.max_file_size_mb[kind]"
                  type="number"
                  min="1"
                  max="256"
                  :disabled="!isMediaKindEnabled(kind)"
                  :placeholder="t('admin.customModelConfig.modal.mediaLimits.unlimited')"
                  class="input w-full"
                />
              </label>
            </div>
          </div>
        </div>

        <div v-if="form.capabilities.includes('video')" class="mt-4">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.modal.mediaLimits.duration') }}
          </div>
          <div class="mt-2 grid grid-cols-2 gap-2">
            <label class="block">
              <span class="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.customModelConfig.modal.mediaLimits.durationMin') }}
              </span>
              <input
                v-model.number="form.media_limits.duration_min"
                type="number"
                min="1"
                max="600"
                :placeholder="t('admin.customModelConfig.modal.mediaLimits.unlimited')"
                class="input w-full"
              />
            </label>
            <label class="block">
              <span class="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.customModelConfig.modal.mediaLimits.durationMax') }}
              </span>
              <input
                v-model.number="form.media_limits.duration_max"
                type="number"
                min="1"
                max="600"
                :placeholder="t('admin.customModelConfig.modal.mediaLimits.unlimited')"
                class="input w-full"
              />
            </label>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.modal.mediaLimits.durationHint') }}
          </p>
          <p v-if="!isDurationValid" class="mt-1 text-xs text-red-500">
            {{ t('admin.customModelConfig.modal.mediaLimits.durationInvalid') }}
          </p>
        </div>
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
  CustomModelMediaLimits,
  CustomModelRequestTemplate,
  MediaFileKind,
  ModelCapability,
  VideoApiType,
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
const mediaFileKinds: MediaFileKind[] = ['image', 'video', 'audio'];
const capabilityLabels = computed<Record<ModelCapability, string>>(() => ({
  image: t('admin.customModelConfig.capabilities.image'),
  video: t('admin.customModelConfig.capabilities.video'),
  audio: t('admin.customModelConfig.capabilities.audio'),
}));

interface MediaLimitsForm {
  allow_upload: boolean;
  file_types: MediaFileKind[];
  max_files: Record<MediaFileKind, number | null>;
  max_file_size_mb: Record<MediaFileKind, number | null>;
  duration_min: number | null;
  duration_max: number | null;
}

const submitting = ref(false);
const form = ref({
  model_name: '',
  prefix_match: false,
  capabilities: [] as ModelCapability[],
  video_api_type: null as VideoApiType | null,
  template_id: null as number | null,
  media_limits: createDefaultMediaLimits(),
});

const isEdit = computed(() => !!props.config);
const dialogTitle = computed(() =>
  isEdit.value
    ? t('admin.customModelConfig.modal.editTitle')
    : t('admin.customModelConfig.modal.createTitle')
);

const isDurationValid = computed(() => {
  const min = toLimitNumber(form.value.media_limits.duration_min);
  const max = toLimitNumber(form.value.media_limits.duration_max);
  if (min === null && max === null) return true;
  if (min === null || max === null) return false;
  return min >= 1 && max >= min && max <= 600;
});

const isFormValid = computed(
  () =>
    form.value.model_name.trim().length > 0 &&
    form.value.capabilities.length > 0 &&
    isDurationValid.value
);

function createDefaultMediaLimits(): MediaLimitsForm {
  return {
    allow_upload: true,
    file_types: [...mediaFileKinds],
    max_files: { image: null, video: null, audio: null },
    max_file_size_mb: { image: null, video: null, audio: null },
    duration_min: null,
    duration_max: null,
  };
}

function toLimitNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.trunc(parsed) : null;
}

function toMediaLimitsForm(limits: CustomModelMediaLimits | null | undefined): MediaLimitsForm {
  const defaults = createDefaultMediaLimits();
  if (!limits) return defaults;
  const fileTypes = (limits.file_types ?? []).filter((kind): kind is MediaFileKind =>
    mediaFileKinds.includes(kind)
  );
  return {
    allow_upload: limits.allow_upload !== false,
    file_types: fileTypes.length > 0 ? fileTypes : [...mediaFileKinds],
    max_files: {
      image: toLimitNumber(limits.max_files?.image),
      video: toLimitNumber(limits.max_files?.video),
      audio: toLimitNumber(limits.max_files?.audio),
    },
    max_file_size_mb: {
      image: toLimitNumber(limits.max_file_size_mb?.image),
      video: toLimitNumber(limits.max_file_size_mb?.video),
      audio: toLimitNumber(limits.max_file_size_mb?.audio),
    },
    duration_min: toLimitNumber(limits.duration?.min),
    duration_max: toLimitNumber(limits.duration?.max),
  };
}

/** 把表单还原成接口结构；全部保持默认时返回 null 表示清空限制。 */
function serializeMediaLimits(): CustomModelMediaLimits | null {
  const source = form.value.media_limits;
  const maxFiles: Partial<Record<MediaFileKind, number>> = {};
  const maxFileSize: Partial<Record<MediaFileKind, number>> = {};
  for (const kind of mediaFileKinds) {
    const count = toLimitNumber(source.max_files[kind]);
    if (count !== null) maxFiles[kind] = count;
    const size = toLimitNumber(source.max_file_size_mb[kind]);
    if (size !== null) maxFileSize[kind] = size;
  }
  const durationMin = toLimitNumber(source.duration_min);
  const durationMax = toLimitNumber(source.duration_max);
  const hasCounts = Object.keys(maxFiles).length > 0;
  const hasSizes = Object.keys(maxFileSize).length > 0;
  const hasDuration = durationMin !== null && durationMax !== null;
  const restricted =
    !source.allow_upload ||
    source.file_types.length < mediaFileKinds.length ||
    hasCounts ||
    hasSizes ||
    hasDuration;
  if (!restricted) return null;

  const payload: CustomModelMediaLimits = {
    allow_upload: source.allow_upload,
    file_types: [...source.file_types],
  };
  if (hasCounts) payload.max_files = maxFiles;
  if (hasSizes) payload.max_file_size_mb = maxFileSize;
  if (hasDuration) payload.duration = { min: durationMin, max: durationMax };
  return payload;
}

function isMediaKindEnabled(kind: MediaFileKind): boolean {
  return form.value.media_limits.allow_upload && form.value.media_limits.file_types.includes(kind);
}

watch(
  () => props.config,
  (config) => {
    if (config) {
      form.value = {
        model_name: config.model_name,
        prefix_match: config.prefix_match,
        capabilities: [...config.capabilities],
        video_api_type: config.video_api_type ?? null,
        template_id: config.template_id ?? null,
        media_limits: toMediaLimitsForm(config.media_limits),
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
    video_api_type: null,
    template_id: null,
    media_limits: createDefaultMediaLimits(),
  };
}

function getCapabilityLabel(cap: ModelCapability): string {
  return capabilityLabels.value[cap];
}

function handleClose() {
  emit('close');
}

async function handleSubmit() {
  if (!isFormValid.value || submitting.value) return;

  submitting.value = true;
  try {
    const mediaLimits = serializeMediaLimits();
    if (isEdit.value && props.config) {
      await customModelConfigDatasource.update(props.config.id, {
        prefix_match: form.value.prefix_match,
        capabilities: form.value.capabilities,
        video_api_type: form.value.video_api_type,
        template_id: form.value.template_id,
        media_limits: mediaLimits,
      });
    } else {
      await customModelConfigDatasource.create({
        model_name: form.value.model_name.trim(),
        prefix_match: form.value.prefix_match,
        capabilities: form.value.capabilities,
        video_api_type: form.value.video_api_type,
        template_id: form.value.template_id,
        media_limits: mediaLimits,
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
