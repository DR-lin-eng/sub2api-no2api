import { computed, reactive, ref, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useAppStore } from "@/core/stores/appStore";
import { useOnboardingStore } from "@/core/stores/onboardingStore";
import { createStableObjectKeyResolver } from "@/core/utils/stableObjectKey";
import { create } from "@/features/admin-groups/data/datasources/adminGroupActions";
import type { AdminGroup } from "@/features/admin-groups/data/dtos/adminGroupDtos";
import {
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
import { groupPlatformLabel } from "../groupsLocale";
import {
  isProfitControlPlatform,
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
  GroupEditorDialogContext,
  GroupEditorRoutingRule,
  GroupReasoningEffortFieldsExpose,
} from "../groupEditorContext";
import {
  createGroupPricingEntry,
  groupPricingToAPI,
  updateGroupPricingModels,
  type GroupPricingFormEntry,
} from "../groupsModelPricing";
import {
  buildImageFinalPricePreview,
  buildVideoFinalPricePreview,
  buildWebSearchFinalPricePreview,
  convertRoutingRulesToApiFormat,
  createCreateGroupFormState,
  normalizeOptionalLimit,
  normalizeRateMultiplier,
  resetDisabledBatchImagePricing,
  resetModelsListState,
} from "./groupEditorFormSupport";
import type { GroupEditorRuntime } from "./useGroupEditorRuntime";

interface CreateGroupControllerOptions {
  groups: Ref<AdminGroup[]>;
  loadGroups: () => void | Promise<void>;
  runtime: GroupEditorRuntime;
}

export function useCreateGroupController({
  groups,
  loadGroups,
  runtime,
}: CreateGroupControllerOptions) {
  const { t } = useI18n();
  const appStore = useAppStore();
  const onboardingStore = useOnboardingStore();
  const showCreateModal = ref(false);
  const createForm = reactive(createCreateGroupFormState());
  const modelsListState = reactive(createModelsListState());
  const modelsListLoading = ref(false);
  const modelsListSelectedCount = computed(
    () => modelsListState.items.filter((item) => item.selected).length,
  );
  const reasoningEffortPolicyRef =
    ref<GroupReasoningEffortFieldsExpose | null>(null);
  const modelRoutingRules = ref<GroupEditorRoutingRule[]>([]);
  const resolveRuleKey =
    createStableObjectKeyResolver<GroupEditorRoutingRule>("create-rule");
  const resolveMessagesDispatchRowKey =
    createStableObjectKeyResolver<MessagesDispatchMappingRow>(
      "create-messages-dispatch-row",
    );
  const getRuleRenderKey = (rule: GroupEditorRoutingRule) =>
    resolveRuleKey(rule);
  const getRuleSearchKey = (rule: GroupEditorRoutingRule) =>
    `create-${resolveRuleKey(rule)}`;
  const getMessagesDispatchRowKey = (row: MessagesDispatchMappingRow) =>
    resolveMessagesDispatchRowKey(row);

  const fallbackOptions = computed(() => {
    const options: { value: number | null; label: string }[] = [
      { value: null, label: t("admin.groups.claudeCode.noFallback") },
    ];
    groups.value
      .filter(
        (group) =>
          group.platform === "anthropic" &&
          !group.claude_code_only &&
          group.status === "active",
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
          group.fallback_group_id_on_invalid_request === null,
      )
      .forEach((group) => options.push({ value: group.id, label: group.name }));
    return options;
  });
  const copyAccountsOptions = computed(() =>
    groups.value
      .filter(
        (group) =>
          (createForm.platform === "composite" ||
            group.platform === createForm.platform) &&
          (group.account_count || 0) > 0,
      )
      .map((group) => ({
        value: group.id,
        label: `${group.name} - ${groupPlatformLabel(t, group.platform)} (${t("admin.groups.accountsCount", {
          count: group.account_count || 0,
        })})`,
      })),
  );
  const imageFinalPricePreview = computed(() =>
    buildImageFinalPricePreview(
      createForm,
      t("admin.groups.imagePricing.notConfigured"),
    ),
  );
  const videoFinalPricePreview = computed(() =>
    buildVideoFinalPricePreview(
      createForm,
      t("admin.groups.videoPricing.notConfigured"),
    ),
  );
  const webSearchFinalPricePreview = computed(() =>
    buildWebSearchFinalPricePreview(
      createForm,
      t("admin.groups.imagePricing.notConfigured"),
    ),
  );

  const loadModelsListCandidates = () =>
    runtime.loadModelsListCandidates(
      "create",
      0,
      createForm.platform,
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
    const index = createForm.supported_model_scopes.indexOf(scope);
    if (index === -1) createForm.supported_model_scopes.push(scope);
    else createForm.supported_model_scopes.splice(index, 1);
  };
  const addMessagesDispatchMapping = () => {
    createForm.exact_model_mappings.push({
      claude_model: "",
      target_model: "",
    });
  };
  const removeMessagesDispatchMapping = (row: MessagesDispatchMappingRow) => {
    const index = createForm.exact_model_mappings.indexOf(row);
    if (index !== -1) createForm.exact_model_mappings.splice(index, 1);
  };
  const addModelPricing = () => {
    createForm.model_pricing.push(createGroupPricingEntry());
  };
  const removeModelPricing = (index: number) => {
    createForm.model_pricing.splice(index, 1);
  };
  const updateModelPricing = (
    index: number,
    entry: GroupPricingFormEntry,
  ) => {
    createForm.model_pricing[index] = entry;
  };
  const updateModelPricingEntryModels = (index: number, models: string[]) =>
    updateGroupPricingModels(
      createForm.model_pricing,
      index,
      models,
      runtime.loadModelDefaultPricing,
    );

  const openCreateModal = () => {
    showCreateModal.value = true;
    void loadModelsListCandidates();
  };
  const closeCreateModal = () => {
    showCreateModal.value = false;
    modelRoutingRules.value.forEach((rule) => {
      runtime.clearRuleSearch(getRuleSearchKey(rule));
    });
    runtime.clearAllAccountSearchState();
    createForm.name = "";
    createForm.description = "";
    createForm.platform = "anthropic";
    createForm.rate_multiplier = 1.0;
    createForm.is_exclusive = false;
    createForm.subscription_type = "standard";
    createForm.daily_limit_usd = null;
    createForm.weekly_limit_usd = null;
    createForm.monthly_limit_usd = null;
    createForm.long_context_pricing_enabled = true;
    createForm.model_pricing = [];
    createForm.allow_image_generation = false;
    createForm.openai_force_image_tool = false;
    createForm.allow_batch_image_generation = false;
    createForm.image_rate_independent = false;
    createForm.image_rate_multiplier = 1;
    createForm.batch_image_discount_multiplier = 0.5;
    createForm.batch_image_hold_multiplier = 0.6;
    createForm.image_price_1k = null;
    createForm.image_price_2k = null;
    createForm.image_price_4k = null;
    createForm.video_rate_independent = false;
    createForm.video_rate_multiplier = 1;
    createForm.video_price_480p = null;
    createForm.video_price_720p = null;
    createForm.video_price_1080p = null;
    createForm.web_search_price_per_call = null;
    createForm.peak_rate_enabled = false;
    createForm.peak_start = "";
    createForm.peak_end = "";
    createForm.peak_rate_multiplier = 1.0;
    createForm.profit_control_enabled = false;
    createForm.profit_min_margin_percent = 0;
    createForm.profit_safety_buffer_percent = 0;
    createForm.claude_code_only = false;
    createForm.fallback_group_id = null;
    createForm.fallback_group_id_on_invalid_request = null;
    resetMessagesDispatchFormState(createForm);
    createForm.allow_live = false;
    createForm.require_oauth_only = false;
    createForm.require_privacy_set = false;
    createForm.supported_model_scopes = [
      "claude",
      "gemini_text",
      "gemini_image",
    ];
    createForm.mcp_xml_inject = true;
    createForm.copy_accounts_from_group_ids = [];
    createForm.rpm_limit = 0;
    createForm.max_reasoning_effort = "";
    createForm.reasoning_effort_mappings = [];
    reasoningEffortPolicyRef.value?.resetValidation();
    resetModelsListState(modelsListState);
    modelRoutingRules.value = [];
  };

  const handleCreateGroup = async () => {
    if (!createForm.name.trim()) {
      appStore.showError(t("admin.groups.nameRequired"));
      return;
    }
    const profitControlError = validateProfitControlFormState(createForm);
    if (profitControlError) {
      appStore.showError(
        t(`admin.groups.profitControl.${profitControlError}`),
      );
      return;
    }
    if (
      supportsReasoningEffortPolicyPlatform(createForm.platform) &&
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
      } = createForm;
      const profitControlEnabled =
        isProfitControlPlatform(createForm.platform) &&
        createForm.profit_control_enabled;
      const requestData = {
        ...formData,
        profit_control_enabled: profitControlEnabled,
        profit_min_margin: profitControlEnabled
          ? profitPercentToDecimal(profitMinMarginPercent)
          : 0,
        profit_safety_buffer: profitControlEnabled
          ? profitPercentToDecimal(profitSafetyBufferPercent)
          : 0,
        daily_limit_usd: normalizeOptionalLimit(createForm.daily_limit_usd),
        weekly_limit_usd: normalizeOptionalLimit(createForm.weekly_limit_usd),
        monthly_limit_usd: normalizeOptionalLimit(createForm.monthly_limit_usd),
        model_pricing: groupPricingToAPI(
          createForm.model_pricing,
          createForm.platform,
        ),
        model_routing: convertRoutingRulesToApiFormat(modelRoutingRules.value),
        models_list_config: buildModelsListConfig(modelsListState),
        supported_model_scopes: normalizeSupportedModelScopesForPlatform(
          createForm.platform,
          createForm.supported_model_scopes,
        ),
        messages_dispatch_model_config:
          createForm.platform === "openai"
            ? messagesDispatchFormStateToConfig(createForm)
            : undefined,
        reasoning_effort_mappings: reasoningEffortMappingsToAPI(
          createForm.reasoning_effort_mappings,
        ),
      };
      const emptyToNull = (value: any) => (value === "" ? null : value);
      requestData.daily_limit_usd = emptyToNull(requestData.daily_limit_usd);
      requestData.weekly_limit_usd = emptyToNull(requestData.weekly_limit_usd);
      requestData.monthly_limit_usd = emptyToNull(requestData.monthly_limit_usd);
      requestData.image_rate_multiplier = normalizeRateMultiplier(
        requestData.image_rate_multiplier,
      );
      resetDisabledBatchImagePricing(requestData);
      requestData.batch_image_discount_multiplier = normalizeRateMultiplier(
        requestData.batch_image_discount_multiplier,
      );
      requestData.batch_image_hold_multiplier = normalizeRateMultiplier(
        requestData.batch_image_hold_multiplier,
      );
      requestData.video_rate_multiplier = normalizeRateMultiplier(
        requestData.video_rate_multiplier,
      );
      requestData.image_price_1k = emptyToNull(requestData.image_price_1k);
      requestData.image_price_2k = emptyToNull(requestData.image_price_2k);
      requestData.image_price_4k = emptyToNull(requestData.image_price_4k);
      requestData.video_price_480p = emptyToNull(requestData.video_price_480p);
      requestData.video_price_720p = emptyToNull(requestData.video_price_720p);
      requestData.video_price_1080p = emptyToNull(
        requestData.video_price_1080p,
      );
      requestData.web_search_price_per_call = emptyToNull(
        requestData.web_search_price_per_call,
      );
      requestData.peak_rate_enabled = createForm.peak_rate_enabled;
      requestData.peak_start = createForm.peak_start;
      requestData.peak_end = createForm.peak_end;
      requestData.peak_rate_multiplier = normalizeRateMultiplier(
        createForm.peak_rate_multiplier,
      );
      await create(requestData);
      appStore.showSuccess(t("admin.groups.groupCreated"));
      closeCreateModal();
      loadGroups();
      if (onboardingStore.isCurrentStep('[data-tour="group-form-submit"]')) {
        onboardingStore.nextStep(500);
      }
    } catch (error: any) {
      appStore.showError(
        error.response?.data?.detail || t("admin.groups.failedToCreate"),
      );
      console.error("Error creating group:", error);
    } finally {
      runtime.submitting.value = false;
    }
  };

  watch(
    () => createForm.subscription_type,
    (newValue) => {
      if (newValue === "subscription") {
        createForm.is_exclusive = true;
        createForm.fallback_group_id_on_invalid_request = null;
      } else {
        createForm.peak_rate_enabled = false;
        createForm.peak_start = "";
        createForm.peak_end = "";
        createForm.peak_rate_multiplier = 1.0;
      }
    },
  );
  watch(
    () => createForm.platform,
    (newValue) => {
      if (!["anthropic", "antigravity"].includes(newValue)) {
        createForm.fallback_group_id_on_invalid_request = null;
      }
      if (newValue !== "openai") {
        resetMessagesDispatchFormState(createForm);
        createForm.allow_live = false;
      }
      if (!["openai", "composite"].includes(newValue)) {
        createForm.openai_force_image_tool = false;
      }
      createForm.max_reasoning_effort = normalizeReasoningEffortForPlatform(
        newValue,
        createForm.max_reasoning_effort,
      );
      createForm.reasoning_effort_mappings = reasoningEffortMappingsToRows(
        reasoningEffortMappingsToAPI(createForm.reasoning_effort_mappings),
        newValue,
      );
      reasoningEffortPolicyRef.value?.resetValidation();
      if (
        !["openai", "antigravity", "anthropic", "gemini"].includes(newValue)
      ) {
        createForm.require_oauth_only = false;
        createForm.require_privacy_set = false;
      }
      if (!isProfitControlPlatform(newValue)) {
        createForm.profit_control_enabled = false;
        createForm.profit_min_margin_percent = 0;
        createForm.profit_safety_buffer_percent = 0;
      }
      resetDisabledBatchImagePricing(createForm);
      resetModelsListState(modelsListState);
      void loadModelsListCandidates();
    },
  );
  watch(
    () => createForm.allow_image_generation,
    (enabled) => {
      if (!enabled) createForm.openai_force_image_tool = false;
      resetDisabledBatchImagePricing(createForm);
    },
  );
  watch(
    () => createForm.openai_force_image_tool,
    (enabled) => {
      if (enabled) createForm.allow_image_generation = true;
    },
  );
  watch(
    () => createForm.allow_batch_image_generation,
    () => resetDisabledBatchImagePricing(createForm),
  );

  const dialogContext: GroupEditorDialogContext = {
    show: showCreateModal,
    form: createForm,
    close: closeCreateModal,
    submit: handleCreateGroup,
    submitting: runtime.submitting,
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
    toggleLive: () => runtime.toggleLive("create", createForm),
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
    form: createForm,
    showCreateModal,
    openCreateModal,
    dialogContext,
    loadModelsListCandidates,
  };
}
