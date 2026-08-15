import { computed, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import type { Column } from "@/common/types/uiTypes";
import { getPersistedPageSize } from "@/common/composables/usePersistedPageSize";
import { extractApiErrorMessage } from "@/core/utils/apiError";
import { useAppStore } from "@/core/stores/appStore";
import * as groupsAPI from "@/features/admin-groups/data/datasources/adminGroupsDatasource";
import type { AdminGroup, GroupPlatform } from "@/types";

const ALWAYS_VISIBLE_COLUMNS = new Set(["name", "actions"]);
const DEFAULT_HIDDEN_COLUMNS = ["id"];
const HIDDEN_COLUMNS_KEY = "group-hidden-columns";
const COLUMN_SETTINGS_VERSION_KEY = "group-column-settings-version";
const COLUMN_SETTINGS_VERSION = 2;
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = { 2: ["id"] };

type GroupUsageSummary = {
  today_cost: number;
  yesterday_cost: number;
  total_cost: number;
};

type GroupCapacitySummary = {
  concurrencyUsed: number;
  concurrencyMax: number;
  sessionsUsed: number;
  sessionsMax: number;
  rpmUsed: number;
  rpmMax: number;
};

export function useGroupsListController() {
  const { t } = useI18n();
  const appStore = useAppStore();

  const allColumns = computed<Column[]>(() => [
    { key: "name", label: t("admin.groups.columns.name"), sortable: true },
    { key: "id", label: t("admin.groups.columns.id"), sortable: true },
    {
      key: "platform",
      label: t("admin.groups.columns.platform"),
      sortable: true,
    },
    {
      key: "billing_type",
      label: t("admin.groups.columns.billingType"),
      sortable: true,
    },
    {
      key: "rate_multiplier",
      label: t("admin.groups.columns.rateMultiplier"),
      sortable: true,
    },
    {
      key: "is_exclusive",
      label: t("admin.groups.columns.type"),
      sortable: true,
    },
    {
      key: "account_count",
      label: t("admin.groups.columns.accounts"),
      sortable: true,
    },
    {
      key: "capacity",
      label: t("admin.groups.columns.capacity"),
      sortable: false,
    },
    { key: "usage", label: t("admin.groups.columns.usage"), sortable: false },
    { key: "status", label: t("admin.groups.columns.status"), sortable: true },
    {
      key: "actions",
      label: t("admin.groups.columns.actions"),
      sortable: false,
    },
  ]);
  const toggleableColumns = computed(() =>
    allColumns.value.filter((column) => !ALWAYS_VISIBLE_COLUMNS.has(column.key)),
  );
  const hiddenColumns = reactive<Set<string>>(new Set());
  const showColumnDropdown = ref(false);
  const columnDropdownRef = ref<HTMLElement | null>(null);
  const getValidHiddenColumnKeys = () =>
    new Set(toggleableColumns.value.map((column) => column.key));

  const saveColumnsToStorage = () => {
    try {
      const validKeys = getValidHiddenColumnKeys();
      const keys = [...hiddenColumns].filter((key) => validKeys.has(key));
      localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify(keys));
      localStorage.setItem(
        COLUMN_SETTINGS_VERSION_KEY,
        String(COLUMN_SETTINGS_VERSION),
      );
    } catch (error) {
      console.error("Failed to save group column settings:", error);
    }
  };
  const loadSavedColumns = () => {
    hiddenColumns.clear();
    try {
      const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY);
      const validKeys = getValidHiddenColumnKeys();
      if (saved) {
        const parsed: unknown = JSON.parse(saved);
        if (Array.isArray(parsed)) {
          parsed
            .filter(
              (key): key is string =>
                typeof key === "string" && validKeys.has(key),
            )
            .forEach((key) => hiddenColumns.add(key));
        }
        const storedVersion = Number(
          localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? "1",
        );
        if (storedVersion < COLUMN_SETTINGS_VERSION) {
          let mutated = false;
          for (
            let version = storedVersion + 1;
            version <= COLUMN_SETTINGS_VERSION;
            version++
          ) {
            for (const key of VERSION_NEW_HIDDEN_COLUMNS[version] ?? []) {
              if (validKeys.has(key) && !hiddenColumns.has(key)) {
                hiddenColumns.add(key);
                mutated = true;
              }
            }
          }
          if (mutated) {
            saveColumnsToStorage();
          } else {
            localStorage.setItem(
              COLUMN_SETTINGS_VERSION_KEY,
              String(COLUMN_SETTINGS_VERSION),
            );
          }
        }
      } else {
        DEFAULT_HIDDEN_COLUMNS.forEach((key) => {
          if (validKeys.has(key)) hiddenColumns.add(key);
        });
        saveColumnsToStorage();
      }
    } catch (error) {
      console.error("Failed to load group column settings:", error);
      DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key));
    }
  };

  const isColumnVisible = (key: string) => !hiddenColumns.has(key);
  const hasVisibleUsageSummaryConsumer = computed(
    () => isColumnVisible("usage") || isColumnVisible("billing_type"),
  );
  const hasVisibleCapacityColumn = computed(() => isColumnVisible("capacity"));
  const columns = computed<Column[]>(() =>
    allColumns.value.filter(
      (column) =>
        ALWAYS_VISIBLE_COLUMNS.has(column.key) ||
        !hiddenColumns.has(column.key),
    ),
  );

  const statusOptions = computed(() => [
    { value: "", label: t("admin.groups.allStatus") },
    { value: "active", label: t("admin.accounts.status.active") },
    { value: "inactive", label: t("admin.accounts.status.inactive") },
  ]);
  const exclusiveOptions = computed(() => [
    { value: "", label: t("admin.groups.allGroups") },
    { value: "true", label: t("admin.groups.exclusive") },
    { value: "false", label: t("admin.groups.nonExclusive") },
  ]);
  const platformFilterOptions = computed(() => [
    { value: "", label: t("admin.groups.allPlatforms") },
    { value: "anthropic", label: "Anthropic" },
    { value: "openai", label: "OpenAI" },
    { value: "gemini", label: "Gemini" },
    { value: "antigravity", label: "Antigravity" },
    { value: "grok", label: "Grok" },
    { value: "composite", label: "Composite" },
  ]);

  const groups = ref<AdminGroup[]>([]);
  const loading = ref(false);
  const usageMap = ref<Map<number, GroupUsageSummary>>(new Map());
  const usageLoading = ref(false);
  const capacityMap = ref<Map<number, GroupCapacitySummary>>(new Map());
  const searchQuery = ref("");
  const filters = reactive({ platform: "", status: "", is_exclusive: "" });
  const pagination = reactive({
    page: 1,
    page_size: getPersistedPageSize(),
    total: 0,
    pages: 0,
  });
  const sortState = reactive({
    sort_by: "sort_order",
    sort_order: "asc" as "asc" | "desc",
  });
  let abortController: AbortController | null = null;

  const loadUsageSummary = async () => {
    if (!hasVisibleUsageSummaryConsumer.value) {
      usageLoading.value = false;
      return;
    }
    usageLoading.value = true;
    try {
      const data = await groupsAPI.getUsageSummary();
      const map = new Map<number, GroupUsageSummary>();
      for (const item of data) {
        map.set(item.group_id, {
          today_cost: item.today_cost,
          yesterday_cost: item.yesterday_cost,
          total_cost: item.total_cost,
        });
      }
      usageMap.value = map;
    } catch (error) {
      console.error("Error loading group usage summary:", error);
    } finally {
      usageLoading.value = false;
    }
  };
  const loadCapacitySummary = async () => {
    if (!hasVisibleCapacityColumn.value) return;
    try {
      const data = await groupsAPI.getCapacitySummary();
      const map = new Map<number, GroupCapacitySummary>();
      for (const item of data) {
        map.set(item.group_id, {
          concurrencyUsed: item.concurrency_used,
          concurrencyMax: item.concurrency_max,
          sessionsUsed: item.sessions_used,
          sessionsMax: item.sessions_max,
          rpmUsed: item.rpm_used,
          rpmMax: item.rpm_max,
        });
      }
      capacityMap.value = map;
    } catch (error) {
      console.error("Error loading group capacity summary:", error);
    }
  };
  const loadGroups = async () => {
    abortController?.abort();
    const currentController = new AbortController();
    abortController = currentController;
    const { signal } = currentController;
    loading.value = true;
    try {
      const response = await groupsAPI.list(
        pagination.page,
        pagination.page_size,
        {
          platform: (filters.platform as GroupPlatform) || undefined,
          status: filters.status as "active" | "inactive",
          is_exclusive: filters.is_exclusive
            ? filters.is_exclusive === "true"
            : undefined,
          search: searchQuery.value.trim() || undefined,
          sort_by: sortState.sort_by,
          sort_order: sortState.sort_order,
        },
        { signal },
      );
      if (signal.aborted) return;
      groups.value = response.items;
      pagination.total = response.total;
      pagination.pages = response.pages;
      if (hasVisibleUsageSummaryConsumer.value) void loadUsageSummary();
      else usageLoading.value = false;
      if (hasVisibleCapacityColumn.value) void loadCapacitySummary();
    } catch (error: unknown) {
      const requestError = error as { name?: string; code?: string };
      if (
        signal.aborted ||
        requestError.name === "AbortError" ||
        requestError.code === "ERR_CANCELED"
      ) {
        return;
      }
      appStore.showError(t("admin.groups.failedToLoad"));
      console.error("Error loading groups:", error);
    } finally {
      if (abortController === currentController && !signal.aborted) {
        loading.value = false;
      }
    }
  };

  const toggleColumn = (key: string) => {
    const validKeys = getValidHiddenColumnKeys();
    if (!validKeys.has(key)) return;
    const wasHidden = hiddenColumns.has(key);
    if (wasHidden) hiddenColumns.delete(key);
    else hiddenColumns.add(key);
    saveColumnsToStorage();
    if (wasHidden && (key === "usage" || key === "billing_type")) {
      void loadUsageSummary();
    }
    if (wasHidden && key === "capacity") void loadCapacitySummary();
  };

  const formatCost = (cost: number): string => {
    if (cost >= 1000) return cost.toFixed(0);
    if (cost >= 100) return cost.toFixed(1);
    return cost.toFixed(2);
  };
  const formatUsd = (cost: number | null | undefined): string =>
    `$${formatCost(cost ?? 0)}`;
  const getQuotaUsageClass = (
    used: number,
    limit: number | null | undefined,
  ): string => {
    if (!limit || limit <= 0) {
      return "font-medium text-gray-700 dark:text-gray-300";
    }
    const ratio = used / limit;
    if (ratio >= 1) return "font-semibold text-red-600 dark:text-red-400";
    if (ratio >= 0.8) {
      return "font-semibold text-amber-600 dark:text-amber-400";
    }
    return "font-medium text-gray-700 dark:text-gray-300";
  };

  let searchTimeout: ReturnType<typeof setTimeout>;
  const handleSearch = () => {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      pagination.page = 1;
      void loadGroups();
    }, 300);
  };
  const handlePageChange = (page: number) => {
    pagination.page = page;
    void loadGroups();
  };
  const handlePageSizeChange = (pageSize: number) => {
    pagination.page_size = pageSize;
    pagination.page = 1;
    void loadGroups();
  };
  const handleSort = (key: string, order: "asc" | "desc") => {
    sortState.sort_by = key;
    sortState.sort_order = order;
    pagination.page = 1;
    void loadGroups();
  };

  const showDeleteDialog = ref(false);
  const deletingGroup = ref<AdminGroup | null>(null);
  const duplicatingGroupIds = reactive(new Set<number>());
  const showSortModal = ref(false);
  const sortSubmitting = ref(false);
  const sortableGroups = ref<AdminGroup[]>([]);
  const showRateMultipliersModal = ref(false);
  const rateMultipliersGroup = ref<AdminGroup | null>(null);
  const showRPMOverridesModal = ref(false);
  const rpmOverridesGroup = ref<AdminGroup | null>(null);
  const showCompositeRoutesModal = ref(false);
  const compositeRoutesGroup = ref<AdminGroup | null>(null);
  const deleteConfirmMessage = computed(() => {
    if (!deletingGroup.value) return "";
    if (deletingGroup.value.subscription_type === "subscription") {
      return t("admin.groups.deleteConfirmSubscription", {
        name: deletingGroup.value.name,
      });
    }
    return t("admin.groups.deleteConfirm", { name: deletingGroup.value.name });
  });

  const handleRateMultipliers = (group: AdminGroup) => {
    rateMultipliersGroup.value = group;
    showRateMultipliersModal.value = true;
  };
  const handleRPMOverrides = (group: AdminGroup) => {
    rpmOverridesGroup.value = group;
    showRPMOverridesModal.value = true;
  };
  const handleDuplicate = async (group: AdminGroup) => {
    if (duplicatingGroupIds.has(group.id)) return;
    duplicatingGroupIds.add(group.id);
    try {
      const duplicate = await groupsAPI.duplicate(group.id);
      appStore.showSuccess(
        t("admin.groups.duplicateSuccess", { name: duplicate.name }),
      );
      await loadGroups();
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.groups.duplicateFailed")),
      );
    } finally {
      duplicatingGroupIds.delete(group.id);
    }
  };
  const handleCompositeRoutes = (group: AdminGroup) => {
    compositeRoutesGroup.value = group;
    showCompositeRoutesModal.value = true;
  };
  const closeCompositeRoutesModal = () => {
    showCompositeRoutesModal.value = false;
    compositeRoutesGroup.value = null;
  };
  const handleDelete = (group: AdminGroup) => {
    deletingGroup.value = group;
    showDeleteDialog.value = true;
  };
  const confirmDelete = async () => {
    if (!deletingGroup.value) return;
    try {
      await groupsAPI.deleteGroup(deletingGroup.value.id);
      appStore.showSuccess(t("admin.groups.groupDeleted"));
      showDeleteDialog.value = false;
      deletingGroup.value = null;
      void loadGroups();
    } catch (error: unknown) {
      const requestError = error as { response?: { data?: { detail?: string } } };
      appStore.showError(
        requestError.response?.data?.detail ||
          t("admin.groups.failedToDelete"),
      );
      console.error("Error deleting group:", error);
    }
  };
  const openSortModal = async () => {
    try {
      const allGroups = await groupsAPI.getAll();
      sortableGroups.value = [...allGroups].sort(
        (left, right) => left.sort_order - right.sort_order,
      );
      showSortModal.value = true;
    } catch (error) {
      appStore.showError(t("admin.groups.failedToLoad"));
      console.error("Error loading groups for sorting:", error);
    }
  };
  const closeSortModal = () => {
    showSortModal.value = false;
    sortableGroups.value = [];
  };
  const saveSortOrder = async () => {
    sortSubmitting.value = true;
    try {
      const updates = sortableGroups.value.map((group, index) => ({
        id: group.id,
        sort_order: index * 10,
      }));
      await groupsAPI.updateSortOrder(updates);
      appStore.showSuccess(t("admin.groups.sortOrderUpdated"));
      closeSortModal();
      void loadGroups();
    } catch (error: unknown) {
      const requestError = error as { response?: { data?: { detail?: string } } };
      appStore.showError(
        requestError.response?.data?.detail ||
          t("admin.groups.failedToUpdateSortOrder"),
      );
      console.error("Error updating group sort order:", error);
    } finally {
      sortSubmitting.value = false;
    }
  };
  const handleDocumentClick = (event: MouseEvent) => {
    const target = event.target as HTMLElement;
    if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
      showColumnDropdown.value = false;
    }
  };

  if (typeof window !== "undefined") loadSavedColumns();

  return {
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
    handleDocumentClick,
  };
}
