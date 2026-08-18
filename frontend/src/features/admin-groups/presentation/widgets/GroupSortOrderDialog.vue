<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { VueDraggable } from "vue-draggable-plus";
import BaseDialog from "@/common/widgets/feedback/BaseDialog.vue";
import Icon from "@/common/widgets/icons/Icon.vue";
import type { AdminGroup } from "@/features/admin-groups/data/dtos/adminGroupDtos";
import { groupPlatformLabel } from "@/features/admin-groups/presentation/groupsLocale";

const props = defineProps<{
  show: boolean;
  modelValue: AdminGroup[];
  submitting: boolean;
}>();
const emit = defineEmits<{
  (event: "update:modelValue", value: AdminGroup[]): void;
  (event: "close"): void;
  (event: "save"): void;
}>();
const { t } = useI18n();
const groups = computed({
  get: () => props.modelValue,
  set: (value: AdminGroup[]) => emit("update:modelValue", value),
});
</script>

<template>
<!-- Sort Order Modal -->
<BaseDialog
  :show="show"
  :title="t('admin.groups.sortOrder')"
  width="normal"
  @close="emit('close')"
>
  <div class="space-y-4">
    <p class="text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.groups.sortOrderHint") }}
    </p>
    <VueDraggable
      v-model="groups"
      :animation="200"
      class="space-y-2"
    >
      <div
        v-for="group in groups"
        :key="group.id"
        class="flex cursor-grab items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 transition-shadow hover:shadow-md active:cursor-grabbing dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="text-gray-400">
          <Icon name="menu" size="md" />
        </div>
        <div class="flex-1">
          <div class="font-medium text-gray-900 dark:text-white">
            {{ group.name }}
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            <span
              :class="[
                'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
                group.platform === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : group.platform === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : group.platform === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : group.platform === 'grok'
                        ? 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'
                        : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
              ]"
            >
              {{ groupPlatformLabel(t, group.platform) }}
            </span>
          </div>
        </div>
        <div class="text-sm text-gray-400">#{{ group.id }}</div>
      </div>
    </VueDraggable>
  </div>

  <template #footer>
    <div class="flex justify-end gap-3 pt-4">
      <button
        @click="emit('close')"
        type="button"
        class="btn btn-secondary"
      >
        {{ t("common.cancel") }}
      </button>
      <button
        @click="emit('save')"
        :disabled="submitting"
        class="btn btn-primary"
      >
        <svg
          v-if="submitting"
          class="-ml-1 mr-2 h-4 w-4 animate-spin"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          ></circle>
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          ></path>
        </svg>
        {{ submitting ? t("common.saving") : t("common.save") }}
      </button>
    </div>
  </template>
</BaseDialog>
</template>
