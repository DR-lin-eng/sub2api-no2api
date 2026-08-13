import { computed, reactive, ref, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useAppStore } from "@/core/stores/appStore";
import { createStableObjectKeyResolver } from "@/core/utils/stableObjectKey";
import * as groupsAPI from "@/features/admin-groups/data/datasources/adminGroupsDatasource";
import type { AdminGroup } from "@/types";
import {
  messagesDispatchConfigToFormState,
  messagesDispatchFormStateToConfig,
  resetMessagesDispatchFormState,
  type MessagesDispatchMappingRow,
} from "../groupsMessagesDispatchResolver";
import {
  buildModelsListConfig,
  createModelsListState,
  moveModelsListItem,
} from "../groupsModelsListResolver";
import { normalizeSupportedModelScopesForPlatform } from "../groupsSupportedModelScopesResolver";
import {
  isProfitControlPlatform,
  profitDecimalToPercent,
  profitPercentToDecimal,
  validateProfitControlFormState,
} from "../groupsProfitControl";
import {
  normalizeReasoningEffortForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  supportsReasoningEffortPolicyPlatform,
} from "../groupsReasoningEffort";
import type {
  EditGroupDialogContext,
  GroupEditorRoutingRule,
  GroupReasoningEffortFieldsExpose,
} from "../groupEditorContext";
import {
  createGroupPricingEntry,
  groupPricingFromAPI,
  groupPricingToAPI,
  updateGroupPricingModels,
  type GroupPricingFormEntry,
} from "../groupsModelPricing";
import {
  buildImageFinalPricePreview,
  buildVideoFinalPricePreview,
  buildWebSearchFinalPricePreview,
  convertRoutingRulesToApiFormat,
  createEditGroupFormState,
  normalizeOptionalLimit,
  normalizeRateMultiplier,
  resetDisabledBatchImagePricing,
  resetModelsListState,
} from "./groupEditorFormSupport";
import type { GroupEditorRuntime } from "./useGroupEditorRuntime";

interface EditGroupControllerOptions {
  groups: Ref<AdminGroup[]>;
  loadGroups: () => void | Promise<void>;
  runtime: GroupEditorRuntime;
}

export function useEditGroupController({
  groups,
  loadGroups,
  runtime,
}: EditGroupControllerOptions) {
  const { t } = useI18n();
  const appStore = useAppStore();
  const showEditModal = ref(false);
  const editingGroup = ref<AdminGroup | null>(null);
  const editForm = reactive(createEditGroupFormState());
  const modelsListState = reactive(createModelsListState());
  const modelsListLoading = ref(false);
  const modelsListSelectedCount = computed(
    () => modelsListState.items.filter((item) => item.selected).length,
  );
  const reasoningEffortPolicyRef =
    ref<GroupReasoningEffortFieldsExpose | null>(null);
  const modelRoutingRules = ref<GroupEditorRoutingRule[]>([]);
  const resolveRuleKey =
    createStableObjectKeyResolver<GroupEditorRoutingRule>("edit-rule");
  const resolveMessagesDispatchRowKey =
    createStableObjectKeyResolver<MessagesDispatchMappingRow>(
      "edit-messages-dispatch-row",
    );
  const getRuleRenderKey = (rule: GroupEditorRoutingRule) =>
    resolveRuleKey(rule);
  const getRuleSearchKey = (rule: GroupEditorRoutingRule) =>
    `edit-${resolveRuleKey(rule)}`;
  const getMessagesDispatchRowKey = (row: MessagesDispatchMappingRow) =>
    resolveMessagesDispatchRowKey(row);

  const statusOptions = computed(() => [
    { value: "active", label: t("admin.accounts.status.active") },
    { value: "inactive", label: t("admin.accounts.status.inactive") },
  ]);
  const fallbackOptions = computed(() => {
    const options: { value: number | null; label: string }[] = [
      { value: null, label: t("admin.groups.claudeCode.noFallback") },
    ];
    groups.value
      .filter(
        (group) =>
          group.platform === "anthropic" &&
          !group.claude_code_only &&
          group.status === "active" &&
          group.id !== editingGroup.value?.id,
      )
      .forEach((group) => options.push({ value: group.id, label: group.name }));
    return options;
  });
  const invalidRequestFallbackOptions = computed(() => {
    const options: { value: number | null; label: string }[] = [
      {
        value: null,
        label: t("admin.groups.invalidRequestFallback.noFallback"),
      },
    ];
    groups.value
      .filter(
        (group) =>
          group.platform === "anthropic" &&
          group.status === "active" &&
          group.subscription_type !== "subscription" &&
          group.fallback_group_id_on_invalid_request === null &&
          group.id !== editingGroup.value?.id,
      )
      .forEach((group) => options.push({ value: group.id, label: group.name }));
    return options;
  });
  const copyAccountsOptions = computed(() =>
    groups.value
      .filter(
        (group) =>
          (editForm.platform === "composite" ||
            group.platform === editForm.platform) &&
          (group.account_count || 0) > 0 &&
          group.id !== editingGroup.value?.id,
      )
      .map((group) => ({
        value: group.id,
        label: `${group.name} - ${t(
          `admin.groups.platforms.${group.platform}`,
        )} (${t("admin.groups.accountsCount", {
          count: group.account_count || 0,
        })})`,
      })),
  );
  const imageFinalPricePreview = computed(() =>
    buildImageFinalPricePreview(
      editForm,
      t("admin.groups.imagePricing.notConfigured"),
    ),
  );
  const videoFinalPricePreview = computed(() =>
    buildVideoFinalPricePreview(
      editForm,
      t("admin.groups.videoPricing.notConfigured"),
    ),
  );
  const webSearchFinalPricePreview = computed(() =>
    buildWebSearchFinalPricePreview(
      editForm,
      t("admin.groups.imagePricing.notConfigured"),
    ),
  );

  const loadModelsListCandidates = (groupID: number, platform = editForm.platform) =>
    runtime.loadModelsListCandidates(
      "edit",
      groupID,
      platform,
      modelsListState,
      modelsListLoading,
    );
  const moveModelsListItemByIndex = (fromIndex: number, toIndex: number) => {
    moveModelsListItem(modelsListState, fromIndex, toIndex);
  };
  const addRoutingRule = () => {
    modelRoutingRules.value.push({ pattern: "", accounts: [] });
  };
  const removeRoutingRule = (rule: GroupEditorRoutingRule) => {
    const index = modelRoutingRules.value.indexOf(rule);
    if (index === -1) return;
    runtime.clearRuleSearch(getRuleSearchKey(rule));
    modelRoutingRules.value.splice(index, 1);
  };
  const toggleScope = (scope: string) => {
    const index = editForm.supported_model_scopes.indexOf(scope);
    if (index === -1) editForm.supported_model_scopes.push(scope);
    else editForm.supported_model_scopes.splice(index, 1);
  };
  const addMessagesDispatchMapping = () => {
    editForm.exact_model_mappings.push({
      claude_model: "",
      target_model: "",
    });
  };
  const removeMessagesDispatchMapping = (row: MessagesDispatchMappingRow) => {
    const index = editForm.exact_model_mappings.indexOf(row);
    if (index !== -1) editForm.exact_model_mappings.splice(index, 1);
  };
  const addModelPricing = () => {
    editForm.model_pricing.push(createGroupPricingEntry());
  };
  const removeModelPricing = (index: number) => {
    editForm.model_pricing.splice(index, 1);
  };
  const updateModelPricing = (
    index: number,
    entry: GroupPricingFormEntry,
  ) => {
    editForm.model_pricing[index] = entry;
  };
  const updateModelPricingEntryModels = (index: number, models: string[]) =>
    updateGroupPricingModels(
      editForm.model_pricing,
      index,
      models,
      runtime.loadModelDefaultPricing,
    );

  const handleEdit = async (group: AdminGroup) => {
    editingGroup.value = group;
    editForm.name = group.name;
    editForm.description = group.description || "";
    editForm.platform = group.platform;
    editForm.rate_multiplier = group.rate_multiplier;
    editForm.is_exclusive = group.is_exclusive;
    editForm.status = group.status;
    editForm.subscription_type = group.subscription_type || "standard";
    editForm.daily_limit_usd = group.daily_limit_usd;
    editForm.weekly_limit_usd = group.weekly_limit_usd;
    editForm.monthly_limit_usd = group.monthly_limit_usd;
    editForm.long_context_pricing_enabled =
      group.long_context_pricing_enabled ?? true;
    editForm.model_pricing = groupPricingFromAPI(group.model_pricing);
    editForm.allow_image_generation = group.allow_image_generation ?? false;
    editForm.openai_force_image_tool = group.openai_force_image_tool ?? false;
    editForm.allow_batch_image_generation =
      group.allow_batch_image_generation ?? false;
    editForm.image_rate_independent = group.image_rate_independent ?? false;
    editForm.image_rate_multiplier = group.image_rate_multiplier ?? 1;
    editForm.batch_image_discount_multiplier =
      group.batch_image_discount_multiplier ?? 0.5;
    editForm.batch_image_hold_multiplier =
      group.batch_image_hold_multiplier ?? 0.6;
    editForm.image_price_1k = group.image_price_1k;
    editForm.image_price_2k = group.image_price_2k;
    editForm.image_price_4k = group.image_price_4k;
    editForm.video_rate_independent = group.video_rate_independent ?? false;
    editForm.video_rate_multiplier = group.video_rate_multiplier ?? 1;
    editForm.video_price_480p = group.video_price_480p;
    editForm.video_price_720p = group.video_price_720p;
    editForm.video_price_1080p = group.video_price_1080p;
    editForm.web_search_price_per_call =
      group.web_search_price_per_call ?? null;
    editForm.peak_rate_enabled = group.peak_rate_enabled ?? false;
    editForm.peak_start = group.peak_start ?? "";
    editForm.peak_end = group.peak_end ?? "";
    editForm.peak_rate_multiplier = group.peak_rate_multiplier ?? 1.0;
    editForm.profit_control_enabled = group.profit_control_enabled ?? false;
    editForm.profit_min_margin_percent = profitDecimalToPercent(
      group.profit_min_margin,
    );
    editForm.profit_safety_buffer_percent = profitDecimalToPercent(
      group.profit_safety_buffer,
    );
    editForm.claude_code_only = group.claude_code_only || false;
    editForm.fallback_group_id = group.fallback_group_id;
    editForm.fallback_group_id_on_invalid_request =
      group.fallback_group_id_on_invalid_request;
    const messagesState = messagesDispatchConfigToFormState(
      group.messages_dispatch_model_config,
    );
    editForm.allow_messages_dispatch =
      group.allow_messages_dispatch || messagesState.allow_messages_dispatch;
    editForm.allow_live = group.allow_live ?? false;
    editForm.opus_mapped_model = messagesState.opus_mapped_model;
    editForm.sonnet_mapped_model = messagesState.sonnet_mapped_model;
    editForm.haiku_mapped_model = messagesState.haiku_mapped_model;
    editForm.exact_model_mappings = messagesState.exact_model_mappings;
    editForm.require_oauth_only = group.require_oauth_only ?? false;
    editForm.require_privacy_set = group.require_privacy_set ?? false;
    editForm.model_routing_enabled = group.model_routing_enabled || false;
    editForm.supported_model_scopes = group.supported_model_scopes || [
      "claude",
      "gemini_text",
      "gemini_image",
    ];
    editForm.mcp_xml_inject = group.mcp_xml_inject ?? true;
    editForm.copy_accounts_from_group_ids = [];
    editForm.rpm_limit = group.rpm_limit ?? 0;
    editForm.max_reasoning_effort = normalizeReasoningEffortForPlatform(
      group.platform,
      group.max_reasoning_effort,
    );
    editForm.reasoning_effort_mappings = reasoningEffortMappingsToRows(
      group.reasoning_effort_mappings,
      group.platform,
    );
    resetModelsListState(modelsListState, group.models_list_config);
    modelRoutingRules.value = await runtime.convertApiFormatToRoutingRules(
      group.model_routing,
    );
    void loadModelsListCandidates(group.id, group.platform);
    showEditModal.value = true;
  };

  const closeEditModal = () => {
    modelRoutingRules.value.forEach((rule) => {
      runtime.clearRuleSearch(getRuleSearchKey(rule));
    });
    runtime.clearAllAccountSearchState();
    showEditModal.value = false;
    editingGroup.value = null;
    editForm.max_reasoning_effort = "";
    editForm.reasoning_effort_mappings = [];
    reasoningEffortPolicyRef.value?.resetValidation();
    modelRoutingRules.value = [];
    editForm.copy_accounts_from_group_ids = [];
    editForm.peak_rate_enabled = false;
    editForm.peak_start = "";
    editForm.peak_end = "";
    editForm.peak_rate_multiplier = 1.0;
    editForm.profit_control_enabled = false;
    editForm.profit_min_margin_percent = 0;
    editForm.profit_safety_buffer_percent = 0;
    editForm.video_rate_independent = false;
    editForm.video_rate_multiplier = 1;
    editForm.video_price_480p = null;
    editForm.video_price_720p = null;
    editForm.video_price_1080p = null;
    editForm.web_search_price_per_call = null;
    editForm.long_context_pricing_enabled = true;
    editForm.model_pricing = [];
    resetMessagesDispatchFormState(editForm);
    editForm.allow_live = false;
    resetModelsListState(modelsListState);
  };

  const handleUpdateGroup = async () => {
    if (!editingGroup.value) return;
    if (!editForm.name.trim()) {
      appStore.showError(t("admin.groups.nameRequired"));
      return;
    }
    const profitControlError = validateProfitControlFormState(editForm);
    if (profitControlError) {
      appStore.showError(
        t(`admin.groups.profitControl.${profitControlError}`),
      );
      return;
    }
    if (
      supportsReasoningEffortPolicyPlatform(editForm.platform) &&
      reasoningEffortPolicyRef.value &&
      !reasoningEffortPolicyRef.value.validate()
    ) {
      return;
    }
    runtime.submitting.value = true;
    try {
      const {
        profit_min_margin_percent: profitMinMarginPercent,
        profit_safety_buffer_percent: profitSafetyBufferPercent,
        ...formData
      } = editForm;
      const profitControlEnabled =
        isProfitControlPlatform(editForm.platform) &&
        editForm.profit_control_enabled;
      const payload = {
        ...formData,
        profit_control_enabled: profitControlEnabled,
        profit_min_margin: profitControlEnabled
          ? profitPercentToDecimal(profitMinMarginPercent)
          : 0,
        profit_safety_buffer: profitControlEnabled
          ? profitPercentToDecimal(profitSafetyBufferPercent)
          : 0,
        daily_limit_usd: normalizeOptionalLimit(editForm.daily_limit_usd),
        weekly_limit_usd: normalizeOptionalLimit(editForm.weekly_limit_usd),
        monthly_limit_usd: normalizeOptionalLimit(editForm.monthly_limit_usd),
        model_pricing: groupPricingToAPI(
          editForm.model_pricing,
          editForm.platform,
        ),
        fallback_group_id:
          editForm.fallback_group_id === null
            ? 0
            : editForm.fallback_group_id,
        fallback_group_id_on_invalid_request:
          editForm.fallback_group_id_on_invalid_request === null
            ? 0
            : editForm.fallback_group_id_on_invalid_request,
        model_routing: convertRoutingRulesToApiFormat(modelRoutingRules.value),
        models_list_config: buildModelsListConfig(modelsListState),
        supported_model_scopes: normalizeSupportedModelScopesForPlatform(
          editForm.platform,
          editForm.supported_model_scopes,
        ),
        messages_dispatch_model_config:
          editForm.platform === "openai"
            ? messagesDispatchFormStateToConfig(editForm)
            : undefined,
        reasoning_effort_mappings: reasoningEffortMappingsToAPI(
          editForm.reasoning_effort_mappings,
        ),
      };
      const emptyToNull = (value: any) => (value === "" ? null : value);
      payload.daily_limit_usd = emptyToNull(payload.daily_limit_usd);
      payload.weekly_limit_usd = emptyToNull(payload.weekly_limit_usd);
      payload.monthly_limit_usd = emptyToNull(payload.monthly_limit_usd);
      payload.image_rate_multiplier = normalizeRateMultiplier(
        payload.image_rate_multiplier,
      );
      resetDisabledBatchImagePricing(payload);
      payload.batch_image_discount_multiplier = normalizeRateMultiplier(
        payload.batch_image_discount_multiplier,
      );
      payload.batch_image_hold_multiplier = normalizeRateMultiplier(
        payload.batch_image_hold_multiplier,
      );
      payload.video_rate_multiplier = normalizeRateMultiplier(
        payload.video_rate_multiplier,
      );
      const emptyPriceToClear = (value: any) =>
        value === "" || value === null ? -1 : value;
      payload.image_price_1k = emptyPriceToClear(payload.image_price_1k);
      payload.image_price_2k = emptyPriceToClear(payload.image_price_2k);
      payload.image_price_4k = emptyPriceToClear(payload.image_price_4k);
      payload.video_price_480p = emptyPriceToClear(payload.video_price_480p);
      payload.video_price_720p = emptyPriceToClear(payload.video_price_720p);
      payload.video_price_1080p = emptyPriceToClear(payload.video_price_1080p);
      payload.web_search_price_per_call = emptyPriceToClear(
        payload.web_search_price_per_call,
      );
      payload.peak_rate_enabled = editForm.peak_rate_enabled;
      payload.peak_start = editForm.peak_start;
      payload.peak_end = editForm.peak_end;
      payload.peak_rate_multiplier = normalizeRateMultiplier(
        editForm.peak_rate_multiplier,
      );
      await groupsAPI.update(editingGroup.value.id, payload);
      appStore.showSuccess(t("admin.groups.groupUpdated"));
      closeEditModal();
      loadGroups();
    } catch (error: any) {
      appStore.showError(
        error.response?.data?.detail || t("admin.groups.failedToUpdate"),
      );
      console.error("Error updating group:", error);
    } finally {
      runtime.submitting.value = false;
    }
  };

  watch(
    () => editForm.subscription_type,
    (newValue) => {
      if (newValue !== "subscription") {
        editForm.peak_rate_enabled = false;
        editForm.peak_start = "";
        editForm.peak_end = "";
        editForm.peak_rate_multiplier = 1.0;
      }
    },
  );
  watch(
    () => editForm.platform,
    (newValue) => {
      if (!["anthropic", "antigravity"].includes(newValue)) {
        editForm.fallback_group_id_on_invalid_request = null;
      }
      if (newValue !== "openai") {
        resetMessagesDispatchFormState(editForm);
        editForm.allow_live = false;
        editForm.default_mapped_model = "";
      }
      if (!["openai", "composite"].includes(newValue)) {
        editForm.openai_force_image_tool = false;
      }
      editForm.max_reasoning_effort = normalizeReasoningEffortForPlatform(
        newValue,
        editForm.max_reasoning_effort,
      );
      editForm.reasoning_effort_mappings = reasoningEffortMappingsToRows(
        reasoningEffortMappingsToAPI(editForm.reasoning_effort_mappings),
        newValue,
      );
      reasoningEffortPolicyRef.value?.resetValidation();
      if (
        !["openai", "antigravity", "anthropic", "gemini"].includes(newValue)
      ) {
        editForm.require_oauth_only = false;
        editForm.require_privacy_set = false;
      }
      if (!isProfitControlPlatform(newValue)) {
        editForm.profit_control_enabled = false;
        editForm.profit_min_margin_percent = 0;
        editForm.profit_safety_buffer_percent = 0;
      }
      resetDisabledBatchImagePricing(editForm);
      if (editingGroup.value) {
        resetModelsListState(
          modelsListState,
          editForm.platform === editingGroup.value.platform
            ? editingGroup.value.models_list_config
            : undefined,
        );
        void loadModelsListCandidates(editingGroup.value.id, newValue);
      }
    },
  );
  watch(
    () => editForm.allow_image_generation,
    (enabled) => {
      if (!enabled) editForm.openai_force_image_tool = false;
      resetDisabledBatchImagePricing(editForm);
    },
  );
  watch(
    () => editForm.openai_force_image_tool,
    (enabled) => {
      if (enabled) editForm.allow_image_generation = true;
    },
  );
  watch(
    () => editForm.allow_batch_image_generation,
    () => resetDisabledBatchImagePricing(editForm),
  );

  const dialogContext: EditGroupDialogContext = {
    show: showEditModal,
    form: editForm,
    close: closeEditModal,
    submit: handleUpdateGroup,
    submitting: runtime.submitting,
    editingGroup,
    statusOptions,
    platformOptions: runtime.platformOptions,
    subscriptionTypeOptions: runtime.subscriptionTypeOptions,
    copyAccountsOptions,
    fallbackOptions,
    invalidRequestFallbackOptions,
    imageFinalPricePreview,
    videoFinalPricePreview,
    webSearchFinalPricePreview,
    modelsListState,
    modelsListLoading,
    modelsListSelectedCount,
    moveModelsListItem: moveModelsListItemByIndex,
    modelRoutingRules,
    addRoutingRule,
    removeRoutingRule,
    getRuleRenderKey,
    getRuleSearchKey,
    accountSearchKeyword: runtime.accountSearchKeyword,
    accountSearchResults: runtime.accountSearchResults,
    showAccountDropdown: runtime.showAccountDropdown,
    searchAccountsByRule: (rule) =>
      runtime.searchAccounts(getRuleSearchKey(rule)),
    selectAccount: (rule, account) =>
      runtime.selectAccount(rule, account, getRuleSearchKey(rule)),
    removeSelectedAccount: runtime.removeSelectedAccount,
    onAccountSearchFocus: (rule) =>
      runtime.onAccountSearchFocus(getRuleSearchKey(rule)),
    toggleScope,
    toggleLive: () => runtime.toggleLive("edit", editForm),
    addMessagesDispatchMapping,
    removeMessagesDispatchMapping,
    getMessagesDispatchRowKey,
    reasoningEffortPolicyRef,
    addModelPricing,
    removeModelPricing,
    updateModelPricing,
    updateModelPricingModels: updateModelPricingEntryModels,
  };

  return {
    form: editForm,
    showEditModal,
    editingGroup,
    handleEdit,
    dialogContext,
  };
}
