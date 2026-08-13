import { computed, ref, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useKeyedDebouncedSearch } from "@/common/composables/useKeyedDebouncedSearch";
import * as accountsAPI from "@/features/admin-accounts/data/datasources/adminAccountsDatasource";
import * as groupsAPI from "@/features/admin-groups/data/datasources/adminGroupsDatasource";
import type { GroupPlatform } from "@/types";
import { createModelsListCandidatesTracker } from "../groupsModelsListCandidatesResolver";
import { setModelsListCandidates, type ModelsListState } from "../groupsModelsListResolver";
import type {
  GroupEditorRoutingRule,
  GroupEditorSimpleAccount,
} from "../groupEditorContext";

type LiveForm = { allow_live: boolean };

export function useGroupEditorRuntime() {
  const { t } = useI18n();
  const submitting = ref(false);
  const platformOptions = computed(() => [
    { value: "anthropic", label: "Anthropic" },
    { value: "openai", label: "OpenAI" },
    { value: "gemini", label: "Gemini" },
    { value: "antigravity", label: "Antigravity" },
    { value: "grok", label: "Grok" },
    { value: "composite", label: "Composite" },
  ]);
  const subscriptionTypeOptions = computed(() => [
    { value: "standard", label: t("admin.groups.subscription.standard") },
    {
      value: "subscription",
      label: t("admin.groups.subscription.subscription"),
    },
  ]);
  const loadModelDefaultPricing = (model: string) =>
    groupsAPI.getModelDefaultPricing(model);

  const modelsListCandidatesTracker = createModelsListCandidatesTracker();
  const loadModelsListCandidates = async (
    mode: "create" | "edit",
    groupID: number,
    platform: GroupPlatform,
    state: ModelsListState,
    loading: Ref<boolean>,
  ) => {
    const request = { mode, groupID, platform };
    const requestID = modelsListCandidatesTracker.next(request);
    loading.value = true;
    try {
      const models = await groupsAPI.getModelsListCandidates(groupID, platform);
      if (!modelsListCandidatesTracker.isCurrent(requestID, request)) return;
      setModelsListCandidates(state, models);
    } catch (error) {
      if (!modelsListCandidatesTracker.isCurrent(requestID, request)) return;
      console.error("Error loading group models list candidates:", error);
    } finally {
      if (modelsListCandidatesTracker.isCurrent(requestID, request)) {
        loading.value = false;
      }
    }
  };

  const accountSearchKeyword = ref<Record<string, string>>({});
  const accountSearchResults = ref<Record<string, GroupEditorSimpleAccount[]>>(
    {},
  );
  const showAccountDropdown = ref<Record<string, boolean>>({});

  const clearAccountSearchStateByKey = (key: string) => {
    delete accountSearchKeyword.value[key];
    delete accountSearchResults.value[key];
    delete showAccountDropdown.value[key];
  };
  const clearAllAccountSearchState = () => {
    accountSearchKeyword.value = {};
    accountSearchResults.value = {};
    showAccountDropdown.value = {};
  };

  const accountSearchRunner = useKeyedDebouncedSearch<GroupEditorSimpleAccount[]>({
    delay: 300,
    search: async (keyword, { signal }) => {
      const response = await accountsAPI.list(
        1,
        20,
        { search: keyword, platform: "anthropic" },
        { signal },
      );
      return response.items.map((account) => ({
        id: account.id,
        name: account.name,
      }));
    },
    onSuccess: (key, result) => {
      accountSearchResults.value[key] = result;
    },
    onError: (key) => {
      accountSearchResults.value[key] = [];
    },
  });

  const searchAccounts = (key: string) => {
    accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || "");
  };
  const selectAccount = (
    rule: GroupEditorRoutingRule,
    account: GroupEditorSimpleAccount,
    key: string,
  ) => {
    if (!rule.accounts.some((selected) => selected.id === account.id)) {
      rule.accounts.push(account);
    }
    accountSearchKeyword.value[key] = "";
    showAccountDropdown.value[key] = false;
  };
  const removeSelectedAccount = (
    rule: GroupEditorRoutingRule,
    accountID: number,
  ) => {
    rule.accounts = rule.accounts.filter((account) => account.id !== accountID);
  };
  const onAccountSearchFocus = (key: string) => {
    showAccountDropdown.value[key] = true;
    if (!accountSearchResults.value[key]?.length) searchAccounts(key);
  };
  const clearRuleSearch = (key: string) => {
    accountSearchRunner.clearKey(key);
    clearAccountSearchStateByKey(key);
  };
  const handleDocumentClick = (event: MouseEvent) => {
    const target = event.target as HTMLElement;
    if (!target.closest(".account-search-container")) {
      Object.keys(showAccountDropdown.value).forEach((key) => {
        showAccountDropdown.value[key] = false;
      });
    }
  };

  const convertApiFormatToRoutingRules = async (
    apiFormat: Record<string, number[]> | null,
  ): Promise<GroupEditorRoutingRule[]> => {
    if (!apiFormat) return [];
    const rules: GroupEditorRoutingRule[] = [];
    for (const [pattern, accountIDs] of Object.entries(apiFormat)) {
      const accounts: GroupEditorSimpleAccount[] = [];
      for (const id of accountIDs) {
        try {
          const account = await accountsAPI.getById(id);
          accounts.push({ id: account.id, name: account.name });
        } catch {
          accounts.push({ id, name: `#${id}` });
        }
      }
      rules.push({ pattern, accounts });
    }
    return rules;
  };

  const pendingLiveForm = ref<"create" | "edit" | null>(null);
  const showUnsupportedLiveConfirm = computed(
    () => pendingLiveForm.value !== null,
  );
  const liveCapability = ref<{ supported: boolean; reason?: string } | null>(
    null,
  );
  let liveCapabilityRequest: Promise<{
    supported: boolean;
    reason?: string;
  }> | null = null;

  const loadLiveCapability = async () => {
    if (liveCapability.value) return liveCapability.value;
    if (!liveCapabilityRequest) {
      liveCapabilityRequest = groupsAPI
        .getLiveCapability()
        .catch(() => ({ supported: false }))
        .finally(() => {
          liveCapabilityRequest = null;
        });
    }
    liveCapability.value = await liveCapabilityRequest;
    return liveCapability.value ?? { supported: false };
  };
  const toggleLive = async (
    target: "create" | "edit",
    form: LiveForm,
  ) => {
    if (form.allow_live) {
      form.allow_live = false;
      return;
    }
    const capability = await loadLiveCapability();
    if (capability.supported) {
      form.allow_live = true;
      return;
    }
    pendingLiveForm.value = target;
  };
  const confirmUnsupportedLive = (
    createForm: LiveForm,
    editForm: LiveForm,
  ) => {
    if (pendingLiveForm.value === "create") createForm.allow_live = true;
    if (pendingLiveForm.value === "edit") editForm.allow_live = true;
    pendingLiveForm.value = null;
  };
  const cancelUnsupportedLive = () => {
    pendingLiveForm.value = null;
  };
  const dispose = () => {
    accountSearchRunner.clearAll();
    clearAllAccountSearchState();
  };

  return {
    submitting,
    platformOptions,
    subscriptionTypeOptions,
    loadModelDefaultPricing,
    loadModelsListCandidates,
    accountSearchKeyword,
    accountSearchResults,
    showAccountDropdown,
    searchAccounts,
    selectAccount,
    removeSelectedAccount,
    onAccountSearchFocus,
    clearRuleSearch,
    clearAllAccountSearchState,
    convertApiFormatToRoutingRules,
    showUnsupportedLiveConfirm,
    loadLiveCapability,
    toggleLive,
    confirmUnsupportedLive,
    cancelUnsupportedLive,
    handleDocumentClick,
    dispose,
  };
}

export type GroupEditorRuntime = ReturnType<typeof useGroupEditorRuntime>;
