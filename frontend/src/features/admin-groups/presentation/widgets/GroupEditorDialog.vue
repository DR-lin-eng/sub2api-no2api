<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import BaseDialog from "@/common/widgets/feedback/BaseDialog.vue";
import GroupEditorAccountRoutingFields from "./GroupEditorAccountRoutingFields.vue";
import GroupEditorAntigravityFields from "./GroupEditorAntigravityFields.vue";
import GroupEditorCoreFields from "./GroupEditorCoreFields.vue";
import GroupEditorProviderFields from "./GroupEditorProviderFields.vue";
import GroupEditorProfitControlFields from "./GroupEditorProfitControlFields.vue";
import GroupEditorModelPricingFields from "./GroupEditorModelPricingFields.vue";
import type {
  EditGroupDialogContext,
  GroupEditorDialogContext,
} from "../groupEditorContext";

type GroupEditorDialogProps =
  | { mode: "create"; context: GroupEditorDialogContext }
  | { mode: "edit"; context: EditGroupDialogContext };

const props = defineProps<GroupEditorDialogProps>();
const { t } = useI18n();
const isEdit = props.mode === "edit";
const editorContext = props.context;
const editingGroup = computed(() =>
  props.mode === "edit" ? props.context.editingGroup.value : null,
);
const statusOptions = computed(() =>
  props.mode === "edit" ? props.context.statusOptions.value : [],
);
const {
  close,
  show,
  submit,
  submitting,
} = editorContext;
</script>

<template>
<BaseDialog
  :show="show"
  :title="t(isEdit ? 'admin.groups.editGroup' : 'admin.groups.createGroup')"
  width="wide"
  @close="close"
>
  <form
    v-if="!isEdit || editingGroup"
    :id="isEdit ? 'edit-group-form' : 'create-group-form'"
    @submit.prevent="submit"
    class="space-y-5"
  >
    <GroupEditorCoreFields
      :context="editorContext"
      :is-edit="isEdit"
      :status-options="statusOptions"
    />
    <GroupEditorModelPricingFields :context="editorContext" />
    <GroupEditorAntigravityFields :context="editorContext" />
    <GroupEditorProviderFields :context="editorContext" />
    <GroupEditorProfitControlFields :context="editorContext" />
    <GroupEditorAccountRoutingFields :context="editorContext" />
  </form>

  <template #footer>
    <div class="flex justify-end gap-3 pt-4">
      <button
        @click="close"
        type="button"
        class="btn btn-secondary"
      >
        {{ t("common.cancel") }}
      </button>
      <button
        type="submit"
        :form="isEdit ? 'edit-group-form' : 'create-group-form'"
        :disabled="submitting"
        class="btn btn-primary"
        data-tour="group-form-submit"
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
        {{
          submitting
            ? t(isEdit ? "admin.groups.updating" : "admin.groups.creating")
            : t(isEdit ? "common.update" : "common.create")
        }}
      </button>
    </div>
  </template>
</BaseDialog>
</template>
