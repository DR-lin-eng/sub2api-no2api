<script setup lang="ts">
import { useI18n } from "vue-i18n";
import ConfirmDialog from "@/common/widgets/feedback/ConfirmDialog.vue";
import type { AdminGroup } from "@/features/admin-groups/data/dtos/adminGroupDtos";
import CompositeRoutesDialog from "./CompositeRoutesDialog.vue";
import GroupRateMultipliersDialog from "./GroupRateMultipliersDialog.vue";
import GroupRPMOverridesDialog from "./GroupRPMOverridesDialog.vue";
import GroupSortOrderDialog from "./GroupSortOrderDialog.vue";

defineProps<{
  showDelete: boolean;
  deleteConfirmMessage: string;
  showUnsupportedLive: boolean;
  showSort: boolean;
  sortableGroups: AdminGroup[];
  sortSubmitting: boolean;
  showCompositeRoutes: boolean;
  compositeRoutesGroup: AdminGroup | null;
  showRateMultipliers: boolean;
  rateMultipliersGroup: AdminGroup | null;
  showRpmOverrides: boolean;
  rpmOverridesGroup: AdminGroup | null;
}>();

const emit = defineEmits<{
  (event: "confirmDelete"): void;
  (event: "closeDelete"): void;
  (event: "confirmUnsupportedLive"): void;
  (event: "cancelUnsupportedLive"): void;
  (event: "update:sortableGroups", value: AdminGroup[]): void;
  (event: "closeSort"): void;
  (event: "saveSort"): void;
  (event: "closeCompositeRoutes"): void;
  (event: "closeRateMultipliers"): void;
  (event: "closeRpmOverrides"): void;
  (event: "reload"): void;
}>();

const { t } = useI18n();
</script>

<template>
  <ConfirmDialog
    :show="showDelete"
    :title="t('admin.groups.deleteGroup')"
    :message="deleteConfirmMessage"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="emit('confirmDelete')"
    @cancel="emit('closeDelete')"
  />

  <ConfirmDialog
    :show="showUnsupportedLive"
    :title="t('admin.groups.openaiLive.unsupportedTitle')"
    :message="t('admin.groups.openaiLive.unsupportedMessage')"
    :confirm-text="t('admin.groups.openaiLive.enableAnyway')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="emit('confirmUnsupportedLive')"
    @cancel="emit('cancelUnsupportedLive')"
  />

  <GroupSortOrderDialog
    :model-value="sortableGroups"
    :show="showSort"
    :submitting="sortSubmitting"
    @update:model-value="emit('update:sortableGroups', $event)"
    @close="emit('closeSort')"
    @save="emit('saveSort')"
  />

  <CompositeRoutesDialog
    :show="showCompositeRoutes"
    :group="compositeRoutesGroup"
    @close="emit('closeCompositeRoutes')"
  />
  <GroupRateMultipliersDialog
    :show="showRateMultipliers"
    :group="rateMultipliersGroup"
    @close="emit('closeRateMultipliers')"
    @success="emit('reload')"
  />
  <GroupRPMOverridesDialog
    :show="showRpmOverrides"
    :group="rpmOverridesGroup"
    @close="emit('closeRpmOverrides')"
    @success="emit('reload')"
  />
</template>
