<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div
          class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start"
        >
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.groups.searchGroups')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
            <Select
              v-model="filters.platform"
              :options="platformFilterOptions"
              :placeholder="t('admin.groups.allPlatforms')"
              class="w-44"
              @change="loadGroups"
            />
            <Select
              v-model="filters.status"
              :options="statusOptions"
              :placeholder="t('admin.groups.allStatus')"
              class="w-40"
              @change="loadGroups"
            />
            <Select
              v-model="filters.is_exclusive"
              :options="exclusiveOptions"
              :placeholder="t('admin.groups.allGroups')"
              class="w-44"
              @change="loadGroups"
            />
          </div>

          <div
            class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto"
          >
            <button
              @click="loadGroups"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon
                name="refresh"
                size="md"
                :class="loading ? 'animate-spin' : ''"
              />
            </button>
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary"
                :title="t('admin.groups.columnSettings')"
              >
                <Icon name="grid" size="md" class="mr-2" />
                <span class="hidden md:inline">{{
                  t("admin.groups.columnSettings")
                }}</span>
              </button>
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
              >
                <button
                  v-for="col in toggleableColumns"
                  :key="col.key"
                  @click="toggleColumn(col.key)"
                  class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
                >
                  <span>{{ col.label }}</span>
                  <Icon
                    v-if="isColumnVisible(col.key)"
                    name="check"
                    size="sm"
                    class="text-primary-500"
                    :stroke-width="2"
                  />
                </button>
              </div>
            </div>
            <button
              @click="openSortModal"
              class="btn btn-secondary"
              :title="t('admin.groups.sortOrder')"
            >
              <Icon name="arrowsUpDown" size="md" class="mr-2" />
              {{ t("admin.groups.sortOrder") }}
            </button>
            <button
              @click="openCreateModal"
              class="btn btn-primary"
              data-tour="groups-create-btn"
            >
              <Icon name="plus" size="md" class="mr-2" />
              {{ t("admin.groups.createGroup") }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="groups"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="sort_order"
          default-sort-order="asc"
          @sort="handleSort"
        >
          <template #cell-name="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{
              value
            }}</span>
          </template>

          <template #cell-id="{ value }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400"
              >#{{ value }}</span
            >
          </template>

          <template #cell-platform="{ value }">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : value === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : value === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : value === 'grok'
                        ? 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'
                        : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
              ]"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ t("admin.groups.platforms." + value) }}
            </span>
          </template>

          <template #cell-billing_type="{ row }">
            <div class="space-y-1">
              <span
                :class="[
                  'inline-block rounded-full px-2 py-0.5 text-xs font-medium',
                  row.subscription_type === 'subscription'
                    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
                ]"
              >
                {{
                  row.subscription_type === "subscription"
                    ? t("admin.groups.subscription.subscription")
                    : t("admin.groups.subscription.standard")
                }}
              </span>
              <div
                v-if="row.subscription_type === 'subscription'"
                class="space-y-0.5 text-xs text-gray-500 dark:text-gray-400"
              >
                <div
                  v-if="
                    row.daily_limit_usd ||
                    row.weekly_limit_usd ||
                    row.monthly_limit_usd
                  "
                  class="flex flex-wrap items-center gap-x-1 gap-y-0.5"
                >
                  <span v-if="row.daily_limit_usd" class="whitespace-nowrap">
                    <span
                      v-if="usageLoading"
                      class="font-medium text-gray-400 dark:text-gray-500"
                      >—</span
                    >
                    <span
                      v-else
                      :class="
                        getQuotaUsageClass(
                          usageMap.get(row.id)?.today_cost ?? 0,
                          row.daily_limit_usd
                        )
                      "
                      >{{
                        formatUsd(usageMap.get(row.id)?.today_cost ?? 0)
                      }}</span
                    >
                    <span class="text-gray-400 dark:text-gray-500">
                      / {{ formatUsd(row.daily_limit_usd) }}/{{
                        t("admin.groups.limitDay")
                      }}</span
                    >
                  </span>
                  <span
                    v-if="
                      row.daily_limit_usd &&
                      (row.weekly_limit_usd || row.monthly_limit_usd)
                    "
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.weekly_limit_usd" class="whitespace-nowrap"
                    >{{ formatUsd(row.weekly_limit_usd) }}/{{
                      t("admin.groups.limitWeek")
                    }}</span
                  >
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.monthly_limit_usd" class="whitespace-nowrap"
                    >{{ formatUsd(row.monthly_limit_usd) }}/{{
                      t("admin.groups.limitMonth")
                    }}</span
                  >
                </div>
                <span v-else class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.subscription.noLimit")
                }}</span>
                <div class="text-gray-400 dark:text-gray-500">
                  {{ t("admin.groups.usageTotal") }}
                  <span class="ml-1 font-medium text-gray-600 dark:text-gray-300"
                    >{{
                      usageLoading
                        ? "—"
                        : formatUsd(usageMap.get(row.id)?.total_cost ?? 0)
                    }}</span
                  >
                </div>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300"
              >{{ value }}x</span
            >
          </template>

          <template #cell-is_exclusive="{ value }">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
              {{
                value ? t("admin.groups.exclusive") : t("admin.groups.public")
              }}
            </span>
          </template>

          <template #cell-account_count="{ row }">
            <div class="space-y-0.5 text-xs">
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsAvailable")
                }}</span>
                <span
                  class="ml-1 font-medium text-emerald-600 dark:text-emerald-400"
                  >{{ row.active_account_count || 0 }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
              <div v-if="row.rate_limited_account_count">
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsRateLimited")
                }}</span>
                <span
                  class="ml-1 font-medium text-amber-600 dark:text-amber-400"
                  >{{ row.rate_limited_account_count }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsTotal")
                }}</span>
                <span
                  class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >{{ row.account_count || 0 }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
            </div>
          </template>

          <template #cell-capacity="{ row }">
            <GroupCapacityBadge
              v-if="capacityMap.get(row.id)"
              :concurrency-used="capacityMap.get(row.id)!.concurrencyUsed"
              :concurrency-max="capacityMap.get(row.id)!.concurrencyMax"
              :sessions-used="capacityMap.get(row.id)!.sessionsUsed"
              :sessions-max="capacityMap.get(row.id)!.sessionsMax"
              :rpm-used="capacityMap.get(row.id)!.rpmUsed"
              :rpm-max="capacityMap.get(row.id)!.rpmMax"
            />
            <span v-else class="text-xs text-gray-400">—</span>
          </template>

          <template #cell-usage="{ row }">
            <div v-if="usageLoading" class="text-xs text-gray-400">—</div>
            <div v-else class="space-y-0.5 text-xs">
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.usageToday")
                }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >${{
                    formatCost(usageMap.get(row.id)?.today_cost ?? 0)
                  }}</span
                >
              </div>
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.usageYesterday")
                }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >${{
                    formatCost(usageMap.get(row.id)?.yesterday_cost ?? 0)
                  }}</span
                >
              </div>
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.usageTotal")
                }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >${{
                    formatCost(usageMap.get(row.id)?.total_cost ?? 0)
                  }}</span
                >
              </div>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active' ? 'badge-success' : 'badge-danger',
              ]"
            >
              {{ t("admin.accounts.status." + value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t("common.edit") }}</span>
              </button>
              <button
                data-testid="group-duplicate"
                :title="
                  duplicatingGroupIds.has(row.id)
                    ? t('admin.groups.duplicating')
                    : t('admin.groups.duplicate')
                "
                :disabled="duplicatingGroupIds.has(row.id)"
                @click="handleDuplicate(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="copy" size="sm" />
                <span class="text-xs">
                  {{
                    duplicatingGroupIds.has(row.id)
                      ? t("admin.groups.duplicating")
                      : t("admin.groups.duplicate")
                  }}
                </span>
              </button>
              <button
                v-if="row.platform === 'composite'"
                @click="handleCompositeRoutes(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-cyan-600 dark:hover:bg-dark-700 dark:hover:text-cyan-400"
              >
                <Icon name="swap" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.compositeRoutes.action")
                }}</span>
              </button>
              <button
                @click="handleRateMultipliers(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-purple-600 dark:hover:bg-dark-700 dark:hover:text-purple-400"
              >
                <Icon name="dollar" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rateMultipliers")
                }}</span>
              </button>
              <button
                @click="handleRPMOverrides(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-orange-600 dark:hover:bg-dark-700 dark:hover:text-orange-400"
              >
                <Icon name="bolt" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rpmOverrides")
                }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t("common.delete") }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.groups.noGroupsYet')"
              :description="t('admin.groups.createFirstGroup')"
              :action-text="t('admin.groups.createGroup')"
              @action="openCreateModal"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <CreateGroupDialog :context="createDialogContext" />
    <EditGroupDialog :context="editDialogContext" />
    <GroupPageDialogs
      :show-delete="showDeleteDialog"
      :delete-confirm-message="deleteConfirmMessage"
      :show-unsupported-live="showUnsupportedLiveConfirm"
      :show-sort="showSortModal"
      :sortable-groups="sortableGroups"
      :sort-submitting="sortSubmitting"
      :show-composite-routes="showCompositeRoutesModal"
      :composite-routes-group="compositeRoutesGroup"
      :show-rate-multipliers="showRateMultipliersModal"
      :rate-multipliers-group="rateMultipliersGroup"
      :show-rpm-overrides="showRPMOverridesModal"
      :rpm-overrides-group="rpmOverridesGroup"
      @confirm-delete="confirmDelete"
      @close-delete="showDeleteDialog = false"
      @confirm-unsupported-live="confirmUnsupportedLive"
      @cancel-unsupported-live="cancelUnsupportedLive"
      @update:sortable-groups="sortableGroups = $event"
      @close-sort="closeSortModal"
      @save-sort="saveSortOrder"
      @close-composite-routes="closeCompositeRoutesModal"
      @close-rate-multipliers="showRateMultipliersModal = false"
      @close-rpm-overrides="showRPMOverridesModal = false"
      @reload="loadGroups"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import AppLayout from "@/common/widgets/layout/AppLayout.vue";
import TablePageLayout from "@/common/widgets/layout/TablePageLayout.vue";
import DataTable from "@/common/widgets/data/DataTable.vue";
import Pagination from "@/common/widgets/data/Pagination.vue";
import EmptyState from "@/common/widgets/feedback/EmptyState.vue";
import Select from "@/common/widgets/forms/Select.vue";
import PlatformIcon from "@/common/widgets/icons/PlatformIcon.vue";
import Icon from "@/common/widgets/icons/Icon.vue";
import GroupCapacityBadge from "@/common/widgets/data/GroupCapacityBadge.vue";
import { useCreateGroupController } from "../composables/useCreateGroupController";
import { useEditGroupController } from "../composables/useEditGroupController";
import { useGroupEditorRuntime } from "../composables/useGroupEditorRuntime";
import { useGroupsListController } from "../composables/useGroupsListController";
import CreateGroupDialog from "../widgets/CreateGroupDialog.vue";
import EditGroupDialog from "../widgets/EditGroupDialog.vue";
import GroupPageDialogs from "../widgets/GroupPageDialogs.vue";

const { t } = useI18n();
const listController = useGroupsListController();
const editorRuntime = useGroupEditorRuntime();
const createController = useCreateGroupController({
  groups: listController.groups,
  loadGroups: listController.loadGroups,
  runtime: editorRuntime,
});
const editController = useEditGroupController({
  groups: listController.groups,
  loadGroups: listController.loadGroups,
  runtime: editorRuntime,
});

const {
  columns,
  toggleableColumns,
  showColumnDropdown,
  columnDropdownRef,
  isColumnVisible,
  toggleColumn,
  statusOptions,
  exclusiveOptions,
  platformFilterOptions,
  groups,
  loading,
  usageMap,
  usageLoading,
  capacityMap,
  searchQuery,
  filters,
  pagination,
  loadGroups,
  formatCost,
  formatUsd,
  getQuotaUsageClass,
  handleSearch,
  handlePageChange,
  handlePageSizeChange,
  handleSort,
  showDeleteDialog,
  deleteConfirmMessage,
  duplicatingGroupIds,
  showSortModal,
  sortSubmitting,
  sortableGroups,
  showRateMultipliersModal,
  rateMultipliersGroup,
  showRPMOverridesModal,
  rpmOverridesGroup,
  showCompositeRoutesModal,
  compositeRoutesGroup,
  handleRateMultipliers,
  handleRPMOverrides,
  handleDuplicate,
  handleCompositeRoutes,
  closeCompositeRoutesModal,
  handleDelete,
  confirmDelete,
  openSortModal,
  closeSortModal,
  saveSortOrder,
} = listController;
const {
  form: createForm,
  openCreateModal,
  dialogContext: createDialogContext,
  loadModelsListCandidates: loadCreateModelsListCandidates,
} = createController;
const {
  form: editForm,
  handleEdit,
  dialogContext: editDialogContext,
} = editController;
const {
  showUnsupportedLiveConfirm,
  cancelUnsupportedLive,
} = editorRuntime;

const confirmUnsupportedLive = () => {
  editorRuntime.confirmUnsupportedLive(createForm, editForm);
};
const handleClickOutside = (event: MouseEvent) => {
  editorRuntime.handleDocumentClick(event);
  listController.handleDocumentClick(event);
};

onMounted(() => {
  loadGroups();
  void editorRuntime.loadLiveCapability();
  void loadCreateModelsListCandidates();
  document.addEventListener("click", handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener("click", handleClickOutside);
  editorRuntime.dispose();
});
</script>
