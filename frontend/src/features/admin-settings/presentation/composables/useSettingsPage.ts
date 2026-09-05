import { computed, onMounted, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { useClipboard } from "@/common/composables/useClipboard";
import {
  isStepUpBlocked,
  isStepUpCancelled,
  stepUpBlockReason,
  useStepUp,
} from "@/common/composables/useStepUp";
import { useAppStore } from "@/core/stores/appStore";
import { extractApiErrorMessage } from "@/core/utils/apiError";
import { normalizeRegistrationEmailSuffixDomains } from "@/core/utils/registrationEmailPolicy";
import {
  buildAuthSourceDefaultsState,
  defaultWeChatConnectScopesForMode,
  deriveWeChatConnectStoredMode,
  normalizeDefaultSubscriptionSettings,
  normalizePlatformQuotasMap,
  resolveWeChatConnectModeCapabilities,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import { getSettings } from "@/features/admin-settings/data/datasources/adminSettingsQueries";
import {
  sendTestEmail as sendTestEmailAction,
  testSmtpConnection as testSmtpConnectionAction,
  updateSettings,
} from "@/features/admin-settings/data/datasources/adminSettingsActions";
import { useAdminSettingsStore } from "@/features/admin-settings/presentation/stores/adminSettingsStore";
import {
  defaultLoginAgreementDocuments,
  loginAgreementRoutePath,
} from "./settingsAgreementResolver";
import {
  createSettingsForm,
  tablePageSizeMax,
  tablePageSizeMin,
} from "./settingsForm";
import { buildSettingsSavePayload } from "./settingsSavePayload";
import {
  prepareSettingsSave,
  type SettingsSaveValidationError,
} from "./settingsSavePreparation";
import { applySettingsSaveResponse } from "./settingsSaveResponse";
import { useSettingsAdminApiKeys } from "./useSettingsAdminApiKeys";
import { useSettingsAffiliate } from "./useSettingsAffiliate";
import { useSettingsClaudePromptBlocks } from "./useSettingsClaudePromptBlocks";
import { useSettingsGatewayPolicies } from "./useSettingsGatewayPolicies";
import { useSettingsIdentityAccess } from "./useSettingsIdentityAccess";
import { useSettingsPaymentProviders } from "./useSettingsPaymentProviders";
import { useSettingsRegistrationDefaults } from "./useSettingsRegistrationDefaults";
import { useSettingsStructuredEditors } from "./useSettingsStructuredEditors";
import { useSettingsWebSearch } from "./useSettingsWebSearch";
export function useSettingsPage() {
  const { t, locale } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const appStore = useAppStore();
  // 关闭 step-up 开关是敏感操作：后端返回 STEP_UP_REQUIRED 时弹 TOTP 码重试
  const settingsStepUp = useStepUp();
  const adminSettingsStore = useAdminSettingsStore();
  const isZhLocale = computed(() => locale.value.startsWith("zh"));

  function localText(zh: string, en: string): string {
    return isZhLocale.value ? zh : en;
  }

  const paymentGuideHref = computed(() =>
    locale.value.startsWith("zh")
      ? "https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/PAYMENT_CN.md"
      : "https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/PAYMENT.md",
  );

  const paymentMethodsHref = computed(() =>
    locale.value.startsWith("zh")
      ? "https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/PAYMENT_CN.md#支持的支付方式"
      : "https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/PAYMENT.md#supported-payment-methods",
  );

  const settingsTabKeys = [
    "general",
    "agreement",
    "features",
    "security",
    "users",
    "gateway",
    "performance",
    "payment",
    "email",
    "backup",
  ] as const;
  type SettingsTab = (typeof settingsTabKeys)[number];

  function resolveSettingsTab(value: unknown): SettingsTab {
    return typeof value === "string" &&
      settingsTabKeys.includes(value as SettingsTab)
      ? (value as SettingsTab)
      : "general";
  }

  const activeTab = ref<SettingsTab>(resolveSettingsTab(route.query.tab));
  const panelRateLimitSettingsMounted = ref(activeTab.value === "security");
  const settingsTabs = [
    { key: "general" as SettingsTab, icon: "home" as const },
    { key: "agreement" as SettingsTab, icon: "document" as const },
    { key: "features" as SettingsTab, icon: "bolt" as const },
    { key: "security" as SettingsTab, icon: "shield" as const },
    { key: "users" as SettingsTab, icon: "user" as const },
    { key: "gateway" as SettingsTab, icon: "server" as const },
    { key: "performance" as SettingsTab, icon: "bolt" as const },
    { key: "payment" as SettingsTab, icon: "creditCard" as const },
    { key: "email" as SettingsTab, icon: "mail" as const },
    { key: "backup" as SettingsTab, icon: "database" as const },
  ];

  const settingsTabKeyboardActions = {
    ArrowLeft: -1,
    ArrowUp: -1,
    ArrowRight: 1,
    ArrowDown: 1,
    Home: "first",
    End: "last",
  } as const;

  function selectSettingsTab(tab: SettingsTab): void {
    if (tab === "security") {
      panelRateLimitSettingsMounted.value = true;
    }
    activeTab.value = tab;
    if (route.query.tab !== tab) {
      void router.replace({
        query: {
          ...route.query,
          tab,
        },
      });
    }
  }

  watch(
    () => route.query.tab,
    (tab) => {
      const nextTab = resolveSettingsTab(tab);
      activeTab.value = nextTab;
      if (nextTab === "security") {
        panelRateLimitSettingsMounted.value = true;
      }
    },
  );

  function focusSettingsTab(tab: SettingsTab): void {
    window.requestAnimationFrame(() => {
      document.getElementById(`settings-tab-${tab}`)?.focus();
    });
  }

  function handleSettingsTabKeydown(event: KeyboardEvent, tab: SettingsTab): void {
    const action =
      settingsTabKeyboardActions[
        event.key as keyof typeof settingsTabKeyboardActions
      ];
    if (action === undefined) {
      return;
    }

    event.preventDefault();
    const currentIndex = settingsTabs.findIndex((item) => item.key === tab);
    let nextIndex = currentIndex < 0 ? 0 : currentIndex;

    if (action === "first") {
      nextIndex = 0;
    } else if (action === "last") {
      nextIndex = settingsTabs.length - 1;
    } else {
      nextIndex =
        (nextIndex + action + settingsTabs.length) % settingsTabs.length;
    }

    const nextTab = settingsTabs[nextIndex]?.key;
    if (!nextTab) {
      return;
    }

    selectSettingsTab(nextTab);
    focusSettingsTab(nextTab);
  }

  const { copyToClipboard } = useClipboard();

  const loading = ref(true);
  const loadFailed = ref(false);
  const saving = ref(false);
  const testingSmtp = ref(false);
  const sendingTestEmail = ref(false);
  const smtpPasswordManuallyEdited = ref(false);
  const testEmailAddress = ref("");
  const form = reactive(createSettingsForm(localText));

  const {
    addClaudeOAuthSystemPromptBlock,
    applyClaudeOAuthSystemPromptPreset,
    claudeOAuthSystemPromptBlocks,
    claudeOAuthSystemPromptBlockTypeOptions,
    claudeOAuthSystemPromptCacheTTLOptions,
    claudeOAuthSystemPromptPresetOptions,
    getClaudeOAuthPresetLabel,
    loadClaudeOAuthSystemPromptBlocks,
    markClaudeOAuthSystemPromptBlockCustom,
    moveClaudeOAuthSystemPromptBlock,
    removeClaudeOAuthSystemPromptBlock,
    resetClaudeOAuthSystemPromptBlocks,
    serializeClaudeOAuthSystemPromptBlocks,
    toggleClaudeOAuthSystemPromptBlock,
  } = useSettingsClaudePromptBlocks(form, t);

  const {
    aliyunCaptchaRegionOptions,
		tencentCaptchaRegionOptions,
    clientIPLastRefreshText,
    clientIPResolutionModeOptions,
    clientIPTrustedProxiesText,
    currentOrigin,
    githubOAuthRedirectUrlSuggestion,
    googleOAuthRedirectUrlSuggestion,
    handleWeChatMPEnabledChange,
    handleWeChatMobileEnabledChange,
    handleWeChatOpenEnabledChange,
    humanVerificationProviders,
    linuxdoRedirectUrlSuggestion,
    normalizeHumanVerificationProvider,
    oidcRedirectUrlSuggestion,
    parseClientIPTrustedProxies,
    setAndCopyEmailOAuthRedirectUrl,
    setAndCopyLinuxdoRedirectUrl,
    setAndCopyOIDCRedirectUrl,
    setAndCopyWeChatRedirectUrl,
    setHumanVerificationProvider,
    syncWeChatConnectMode,
    wechatRedirectUrlSuggestion,
  } = useSettingsIdentityAccess(form, t, localText, copyToClipboard);

  const {
    addAuthSourceDefaultSubscription,
    addDefaultSubscription,
    addQuotaNotifyEmail,
    authSourceDefaults,
    authSourceDefaultsMeta,
    commitRegistrationEmailSuffixWhitelistDraft,
    defaultSubscriptionGroupOptions,
    findDuplicateDefaultSubscription,
    handleRegistrationEmailSuffixWhitelistDraftInput,
    handleRegistrationEmailSuffixWhitelistDraftKeydown,
    handleRegistrationEmailSuffixWhitelistPaste,
    loadSubscriptionGroups,
    registrationEmailSuffixWhitelistDraft,
    registrationEmailSuffixWhitelistTags,
    removeAuthSourceDefaultSubscription,
    removeDefaultSubscription,
    removeRegistrationEmailSuffixWhitelistTag,
    subscriptionGroups,
  } = useSettingsRegistrationDefaults(form, t, localText);

  const {
    addCodexBlacklistRow,
    addCodexFingerprintRow,
    addCodexWhitelistRow,
    addEndpoint,
    addLoginAgreementDocument,
    addMenuItem,
    codexBlacklistRows,
    codexFingerprintNoRequired,
    codexFingerprintRows,
    codexWhitelistRows,
    defaultFingerprintSignalRows,
    findDuplicateLoginAgreementDocumentId,
    formatTablePageSizeOptions,
    moveMenuItem,
    normalizeLoginAgreementDocumentsForSave,
    parseCodexEntriesToRows,
    parseFingerprintSignalsToRows,
    parseTablePageSizeOptionsInput,
    removeCodexBlacklistRow,
    removeCodexFingerprintRow,
    removeCodexWhitelistRow,
    removeEndpoint,
    removeLoginAgreementDocument,
    removeMenuItem,
    serializeCodexRowsToJSON,
    serializeFingerprintRowsToJSON,
    tablePageSizeOptionsInput,
  } = useSettingsStructuredEditors(form);
  type OpenAIAdvancedSchedulerOverrideKey =
    | "openai_advanced_scheduler_lb_top_k"
    | "openai_advanced_scheduler_weight_priority"
    | "openai_advanced_scheduler_weight_load"
    | "openai_advanced_scheduler_weight_queue"
    | "openai_advanced_scheduler_weight_error_rate"
    | "openai_advanced_scheduler_weight_ttft"
    | "openai_advanced_scheduler_weight_reset"
    | "openai_advanced_scheduler_weight_quota_headroom"
    | "openai_advanced_scheduler_weight_upstream_cost"
    | "openai_advanced_scheduler_weight_previous_response"
    | "openai_advanced_scheduler_weight_session_sticky";

  type OpenAIAdvancedSchedulerEffectiveKey =
    | "openai_advanced_scheduler_effective_lb_top_k"
    | "openai_advanced_scheduler_effective_weight_priority"
    | "openai_advanced_scheduler_effective_weight_load"
    | "openai_advanced_scheduler_effective_weight_queue"
    | "openai_advanced_scheduler_effective_weight_error_rate"
    | "openai_advanced_scheduler_effective_weight_ttft"
    | "openai_advanced_scheduler_effective_weight_reset"
    | "openai_advanced_scheduler_effective_weight_quota_headroom"
    | "openai_advanced_scheduler_effective_weight_upstream_cost"
    | "openai_advanced_scheduler_effective_weight_previous_response"
    | "openai_advanced_scheduler_effective_weight_session_sticky";

  const openAIAdvancedSchedulerWeightFields = computed<
    Array<{
      key: OpenAIAdvancedSchedulerOverrideKey;
      label: string;
      placeholder: string;
    }>
  >(() => {
    const placeholder = (
      effectiveKey: OpenAIAdvancedSchedulerEffectiveKey,
      fallbackValue: string,
    ) => {
      const effectiveValue = String(
        (form as Record<string, unknown>)[effectiveKey] ?? "",
      ).trim();
      return t("admin.settings.openaiExperimentalScheduler.defaultPlaceholder", {
        value: effectiveValue || fallbackValue,
      });
    };

    return [
      {
        key: "openai_advanced_scheduler_lb_top_k",
        label: t("admin.settings.openaiExperimentalScheduler.topKLabel"),
        placeholder: placeholder("openai_advanced_scheduler_effective_lb_top_k", "7"),
      },
      {
        key: "openai_advanced_scheduler_weight_priority",
        label: t("admin.settings.openaiExperimentalScheduler.priorityWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_priority", "1"),
      },
      {
        key: "openai_advanced_scheduler_weight_load",
        label: t("admin.settings.openaiExperimentalScheduler.loadWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_load", "1"),
      },
      {
        key: "openai_advanced_scheduler_weight_queue",
        label: t("admin.settings.openaiExperimentalScheduler.queueWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_queue", "0.7"),
      },
      {
        key: "openai_advanced_scheduler_weight_error_rate",
        label: t("admin.settings.openaiExperimentalScheduler.errorRateWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_error_rate", "0.8"),
      },
      {
        key: "openai_advanced_scheduler_weight_ttft",
        label: t("admin.settings.openaiExperimentalScheduler.ttftWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_ttft", "0.5"),
      },
      {
        key: "openai_advanced_scheduler_weight_reset",
        label: t("admin.settings.openaiExperimentalScheduler.resetWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_reset", "0"),
      },
      {
        key: "openai_advanced_scheduler_weight_quota_headroom",
        label: t("admin.settings.openaiExperimentalScheduler.quotaHeadroomWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_quota_headroom", "0"),
      },
      {
        key: "openai_advanced_scheduler_weight_upstream_cost",
        label: t("admin.settings.openaiExperimentalScheduler.upstreamCostWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_upstream_cost", "0"),
      },
      {
        key: "openai_advanced_scheduler_weight_previous_response",
        label: t("admin.settings.openaiExperimentalScheduler.previousResponseWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_previous_response", "5"),
      },
      {
        key: "openai_advanced_scheduler_weight_session_sticky",
        label: t("admin.settings.openaiExperimentalScheduler.sessionStickyWeight"),
        placeholder: placeholder("openai_advanced_scheduler_effective_weight_session_sticky", "3"),
      },
    ];
  });

  const schedulerV2StatusLabel = computed(() => {
    if (!form.scheduler_v2_enabled) {
      return t("admin.settings.schedulerV2.statusDisabled");
    }
    switch (form.scheduler_v2_status) {
      case "active":
        return t("admin.settings.schedulerV2.statusActive");
      case "failed":
        return t("admin.settings.schedulerV2.statusFailed");
      default:
        return t("admin.settings.schedulerV2.statusBuilding");
    }
  });

  const schedulerV2StatusClass = computed(() => {
    if (!form.scheduler_v2_enabled) {
      return "text-gray-500 dark:text-gray-400";
    }
    if (form.scheduler_v2_status === "active") {
      return "text-green-600 dark:text-green-400";
    }
    if (form.scheduler_v2_status === "failed") {
      return "text-red-600 dark:text-red-400";
    }
    return "text-amber-600 dark:text-amber-400";
  });


  const { adminApiKeyExists, adminApiKeyForm, adminApiKeyLoading, adminApiKeyMasked, adminApiKeyMinExpiry, adminApiKeyOperating, adminApiKeyPanelLoading, adminApiKeyPanelOperating, adminApiKeyPanelSecret, adminApiKeyScopeOptions, cancelEditScopedAdminApiKey, copyNewKey, copyScopedAdminApiKey, createAdminApiKey, createScopedAdminApiKey, deleteAdminApiKey, editScopedAdminApiKey, editingAdminApiKeyId, formatAdminApiKeyDate, loadAdminApiKey, loadScopedAdminApiKeys, newAdminApiKey, regenerateAdminApiKey, revokeScopedAdminApiKey, rotateScopedAdminApiKey, scopedAdminApiKeys } = useSettingsAdminApiKeys(copyToClipboard)
  const {
    addOpenAIFastPolicyModelPattern,
    addOpenAIFastPolicyRule,
    addQuickPattern,
    applyBetaPreset,
    betaPolicyActionOptions,
    betaPolicyForm,
    betaPolicyLoading,
    betaPolicySaving,
    betaPolicyScopeOptions,
    betaPresets,
    codexSimulationForm,
    codexSimulationLoadFailed,
    codexSimulationLoading,
    codexSimulationSaving,
    commonModelPatterns,
    getBetaDisplayName,
    globalTempUnschedulableForm,
    globalTempUnschedulableLoading,
    globalTempUnschedulableSaving,
    loadBetaPolicySettings,
    loadCodexSimulationSettings,
    loadGlobalTempUnschedulableSettings,
    loadOllamaCloudUsageSettings,
    loadOverloadCooldownSettings,
    loadRateLimit429CooldownSettings,
    loadRectifierSettings,
    loadStreamTimeoutSettings,
    loadUpstreamBillingProbeSettings,
    ollamaCloudUsageForm,
    ollamaCloudUsageLoading,
    ollamaCloudUsageSaving,
    openaiFastPolicyActionOptions,
    openaiFastPolicyForm,
    openaiFastPolicyLoaded,
    openaiFastPolicyScopeOptions,
    openaiFastPolicyTierOptions,
    overloadCooldownForm,
    overloadCooldownLoading,
    overloadCooldownSaving,
    rateLimit429CooldownForm,
    rateLimit429CooldownLoading,
    rateLimit429CooldownSaving,
    rectifierForm,
    rectifierLoading,
    rectifierSaving,
    removeOpenAIFastPolicyModelPattern,
    removeOpenAIFastPolicyRule,
    restoreOriginalCodexBehavior,
    saveBetaPolicySettings,
    saveCodexSimulationSettings,
    saveGlobalTempUnschedulableSettings,
    saveOllamaCloudUsageSettings,
    saveOverloadCooldownSettings,
    saveRateLimit429CooldownSettings,
    saveRectifierSettings,
    saveStreamTimeoutSettings,
    saveUpstreamBillingProbeSettings,
    streamTimeoutForm,
    streamTimeoutLoading,
    streamTimeoutSaving,
    upstreamBillingProbeForm,
    upstreamBillingProbeLoading,
    upstreamBillingProbeSaving,
  } = useSettingsGatewayPolicies()
  const { addWebSearchProvider, apiKeyVisible, copyApiKey, expandedProviders, formatSubscribedAt, loadWebSearchConfig, openTestDialog, parseSubscribedAt, quotaPercentage, removeWebSearchProvider, resetWebSearchUsage, saveWebSearchConfig, testWebSearchProvider, toggleProviderExpand, webSearchConfig, webSearchProxies, wsTestDialogOpen, wsTestLoading, wsTestQuery, wsTestResult } = useSettingsWebSearch()
  const { allPaymentTypes, cancelRateLimitModeOptions, cancelRateLimitUnitOptions, confirmDeleteProvider, editingProvider, enabledProviderKeyOptions, handleDeleteProvider, handleReorderProviders, handleSaveProvider, handleToggleField, handleToggleType, hasAnyPaymentTypeEnabled, isPaymentTypeEnabled, loadBalanceOptions, loadProviders, openCreateProvider, openEditProvider, providerDialogRef, providerKeyOptions, providerSaving, providers, providersLoading, showDeleteProviderDialog, showProviderDialog, togglePaymentType } = useSettingsPaymentProviders(form, saveSettings)
  const { affiliateBatchModal, affiliateConfirmDialog, affiliateModal, affiliateModalCanSubmit, affiliateState, askResetAffiliateUser, cancelAffiliateConfirm, changeAffiliatePage, clearSelectedAffiliateUser, closeAffiliateModal, handleAffiliateConfirm, onAffiliateSearchInput, onAffiliateUserSearchInput, openAffiliateBatchModal, openAffiliateModal, selectAffiliateUser, submitAffiliateBatchModal, submitAffiliateModal, toggleAffiliateSelect, toggleAffiliateSelectAll } = useSettingsAffiliate(form)

  async function loadSettings() {
    loading.value = true;
    loadFailed.value = false;
    try {
      const settings = await getSettings();
      settings.payment_load_balance_strategy =
        settings.payment_load_balance_strategy || "round-robin";
      // Only assign non-null values from backend (null means unconfigured, keep defaults)
      for (const [key, value] of Object.entries(settings)) {
        if (value !== null && value !== undefined) {
          (form as Record<string, unknown>)[key] = value;
        }
      }
      normalizeHumanVerificationProvider();
      clientIPTrustedProxiesText.value = (
        settings.client_ip_trusted_proxies || []
      ).join("\n");
      loadClaudeOAuthSystemPromptBlocks();
      codexBlacklistRows.value = parseCodexEntriesToRows(
        form.codex_cli_only_blacklist,
      );
      codexWhitelistRows.value = parseCodexEntriesToRows(
        form.codex_cli_only_whitelist,
      );
      codexFingerprintRows.value = form.codex_cli_only_engine_fingerprint_signals
        ? parseFingerprintSignalsToRows(form.codex_cli_only_engine_fingerprint_signals)
        : defaultFingerprintSignalRows();
      form.login_agreement_mode =
        settings.login_agreement_mode === "checkbox" ? "checkbox" : "modal";
      form.login_agreement_updated_at =
        settings.login_agreement_updated_at || "2026-03-31";
      form.login_agreement_documents =
        Array.isArray(settings.login_agreement_documents) &&
        settings.login_agreement_documents.length > 0
          ? settings.login_agreement_documents.map((doc) => ({
              id: doc.id || "",
              title: doc.title || "",
              content_md: doc.content_md || "",
            }))
          : defaultLoginAgreementDocuments(localText);
      Object.assign(authSourceDefaults, buildAuthSourceDefaultsState(settings));
      form.default_platform_quotas = normalizePlatformQuotasMap(settings.default_platform_quotas);
      form.backend_mode_enabled = settings.backend_mode_enabled;
      form.default_subscriptions = normalizeDefaultSubscriptionSettings(
        settings.default_subscriptions,
      );
      registrationEmailSuffixWhitelistTags.value =
        normalizeRegistrationEmailSuffixDomains(
          settings.registration_email_suffix_whitelist,
        );
      tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
        Array.isArray(settings.table_page_size_options)
          ? settings.table_page_size_options
          : [10, 20, 50, 100],
      );
      registrationEmailSuffixWhitelistDraft.value = "";
      form.smtp_password = "";
      smtpPasswordManuallyEdited.value = false;
      form.turnstile_secret_key = "";
      form.recaptcha_secret_key = "";
      form.cap_secret_key = "";
      form.aliyun_captcha_access_key_secret = "";
      form.linuxdo_connect_client_secret = "";
      form.dingtalk_connect_client_secret = "";
      form.github_oauth_client_secret = "";
      form.google_oauth_client_secret = "";
      form.wechat_connect_app_secret = "";
      form.wechat_connect_open_app_secret = "";
      form.wechat_connect_mp_app_secret = "";
      form.wechat_connect_mobile_app_secret = "";
      const wechatCapabilities = resolveWeChatConnectModeCapabilities(
        settings.wechat_connect_open_enabled,
        settings.wechat_connect_mp_enabled,
        settings.wechat_connect_mobile_enabled,
        settings.wechat_connect_mode,
      );
      form.wechat_connect_open_enabled = wechatCapabilities.openEnabled;
      form.wechat_connect_mp_enabled = wechatCapabilities.mpEnabled;
      form.wechat_connect_mobile_enabled = wechatCapabilities.mobileEnabled;
      form.wechat_connect_mode = deriveWeChatConnectStoredMode(
        wechatCapabilities.openEnabled,
        wechatCapabilities.mpEnabled,
        wechatCapabilities.mobileEnabled,
        settings.wechat_connect_mode,
      );
      const legacyWeChatAppID = String(settings.wechat_connect_app_id || "").trim();
      const legacyWeChatSecretConfigured = Boolean(
        settings.wechat_connect_app_secret_configured,
      );
      if (!form.wechat_connect_open_app_id && wechatCapabilities.openEnabled) {
        form.wechat_connect_open_app_id = legacyWeChatAppID;
      }
      if (!form.wechat_connect_mp_app_id && wechatCapabilities.mpEnabled) {
        form.wechat_connect_mp_app_id = legacyWeChatAppID;
      }
      if (!form.wechat_connect_mobile_app_id && wechatCapabilities.mobileEnabled) {
        form.wechat_connect_mobile_app_id = legacyWeChatAppID;
      }
      if (
        !form.wechat_connect_open_app_secret_configured &&
        wechatCapabilities.openEnabled
      ) {
        form.wechat_connect_open_app_secret_configured =
          legacyWeChatSecretConfigured;
      }
      if (
        !form.wechat_connect_mp_app_secret_configured &&
        wechatCapabilities.mpEnabled
      ) {
        form.wechat_connect_mp_app_secret_configured = legacyWeChatSecretConfigured;
      }
      if (
        !form.wechat_connect_mobile_app_secret_configured &&
        wechatCapabilities.mobileEnabled
      ) {
        form.wechat_connect_mobile_app_secret_configured =
          legacyWeChatSecretConfigured;
      }
      form.wechat_connect_scopes = defaultWeChatConnectScopesForMode(
        form.wechat_connect_mode,
      );
      form.oidc_connect_client_secret = "";

      // Load OpenAI fast/flex policy rules from bulk settings.
      // 仅当 payload 真的包含该字段时填充并标记为已加载；否则保持表单空值，
      // 让 saveSettings 在未加载时跳过该字段，防止覆盖后端默认规则。
      if (
        settings.openai_fast_policy_settings &&
        Array.isArray(settings.openai_fast_policy_settings.rules)
      ) {
        openaiFastPolicyForm.rules =
          settings.openai_fast_policy_settings.rules.map((rule) => ({
            ...rule,
            user_ids: rule.user_ids ? [...rule.user_ids] : [],
            model_whitelist: rule.model_whitelist
              ? [...rule.model_whitelist]
              : [],
          }));
        openaiFastPolicyLoaded.value = true;
      }

      // Load web search emulation config separately
      await loadWebSearchConfig();
    } catch (error: unknown) {
      loadFailed.value = true;
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.failedToLoad")),
      );
    } finally {
      loading.value = false;
    }
  }


  function showSettingsSaveValidationError(
    error: SettingsSaveValidationError,
  ): void {
    switch (error.kind) {
      case "tableDefaultPageSize":
        appStore.showError(
          t("admin.settings.site.tableDefaultPageSizeRangeError", {
            min: tablePageSizeMin,
            max: tablePageSizeMax,
          }),
        );
        return;
      case "tablePageSizeOptions":
        appStore.showError(
          t("admin.settings.site.tablePageSizeOptionsFormatError", {
            min: tablePageSizeMin,
            max: tablePageSizeMax,
          }),
        );
        return;
      case "loginAgreementDocumentRequired":
        appStore.showError(
          localText(
            "启用登录条款确认时，至少需要保留一份文档。",
            "At least one document is required when login agreement is enabled.",
          ),
        );
        return;
      case "loginAgreementDocumentTitleRequired":
        appStore.showError(
          localText(
            "登录条款文档名称不能为空。",
            "Login agreement document title cannot be empty.",
          ),
        );
        return;
      case "duplicateLoginAgreementDocumentId":
        appStore.showError(
          localText(
            `登录条款文档路由不能重复：/legal/${error.documentId}`,
            `Login agreement document routes cannot be duplicated: /legal/${error.documentId}`,
          ),
        );
        return;
      case "duplicateDefaultSubscription":
        appStore.showError(
          t("admin.settings.defaults.defaultSubscriptionsDuplicate", {
            groupId: error.groupId,
          }),
        );
        return;
      case "duplicateAuthSourceDefaultSubscription":
        appStore.showError(
          `${error.sourceTitle}: ${t(
            "admin.settings.defaults.defaultSubscriptionsDuplicate",
            { groupId: error.groupId },
          )}`,
        );
        return;
      case "conflictingWeChatApplications":
        appStore.showError(
          localText(
            "公众号和移动应用不能同时启用。",
            "Official Account and Mobile App cannot be enabled at the same time.",
          ),
        );
    }
  }

  async function saveSettings() {
    saving.value = true;
    try {
      const preparation = prepareSettingsSave({
        form,
        tablePageSizeOptionsInput: tablePageSizeOptionsInput.value,
        authSourceDefaults,
        authSourceDefaultsMeta: authSourceDefaultsMeta.value,
        parseTablePageSizeOptionsInput,
        normalizeLoginAgreementDocumentsForSave,
        findDuplicateLoginAgreementDocumentId,
        findDuplicateDefaultSubscription,
        syncWeChatConnectMode,
        serializeClaudeOAuthSystemPromptBlocks,
      });
      if (!preparation.ok) {
        showSettingsSaveValidationError(preparation.error);
        return;
      }

      const payload = buildSettingsSavePayload({
        form,
        normalizedDefaultSubscriptions:
          preparation.normalizedDefaultSubscriptions,
        registrationEmailSuffixWhitelistTags:
          registrationEmailSuffixWhitelistTags.value,
        clientIPTrustedProxies: parseClientIPTrustedProxies(
          clientIPTrustedProxiesText.value,
        ),
        wechatStoredMode: preparation.wechatStoredMode,
        claudeOAuthSystemPromptBlocksJSON:
          preparation.claudeOAuthSystemPromptBlocksJSON,
        codexFingerprintSignalsJSON: serializeFingerprintRowsToJSON(
          codexFingerprintRows.value,
        ),
        codexBlacklistJSON: serializeCodexRowsToJSON(codexBlacklistRows.value),
        codexWhitelistJSON: serializeCodexRowsToJSON(codexWhitelistRows.value),
        currentOrigin,
        openaiFastPolicyLoaded: openaiFastPolicyLoaded.value,
        openaiFastPolicyRules: openaiFastPolicyForm.rules,
        authSourceDefaults,
      });

      const updated = await settingsStepUp.run(() =>
        updateSettings(payload),
      );
      applySettingsSaveResponse({
        form,
        updated,
        normalizeHumanVerificationProvider,
        clientIPTrustedProxiesText,
        authSourceDefaults,
        registrationEmailSuffixWhitelistTags,
        registrationEmailSuffixWhitelistDraft,
        tablePageSizeOptionsInput,
        formatTablePageSizeOptions,
        smtpPasswordManuallyEdited,
        openaiFastPolicyForm,
        openaiFastPolicyLoaded,
      });

      // Save web search emulation config separately (errors handled internally)
      const wsOk = await saveWebSearchConfig();
      // Refresh cached settings so sidebar/header update immediately
      await appStore.fetchPublicSettings(true);
      await adminSettingsStore.fetch(true);
      if (wsOk) {
        appStore.showSuccess(t("admin.settings.settingsSaved"));
      }
    } catch (error: unknown) {
      // 用户取消 step-up 验证：静默返回，不弹错误
      if (isStepUpCancelled(error)) {
        return;
      }
      if (isStepUpBlocked(error)) {
        appStore.showError(
          stepUpBlockReason(error) === "STEP_UP_ADMIN_API_KEY_FORBIDDEN"
            ? t("stepUp.adminApiKeyForbidden")
            : t("stepUp.notEnabled"),
        );
        return;
      }
      // 开启 step-up 开关但本人未启用 2FA：给出可操作的专用提示
      if (
        (error as { reason?: string })?.reason === "STEP_UP_ENABLE_REQUIRES_TOTP"
      ) {
        appStore.showError(t("admin.settings.security.stepUpEnableRequiresTotp"));
        return;
      }
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.failedToSave")),
      );
    } finally {
      saving.value = false;
    }
  }

  async function testSmtpConnection() {
    testingSmtp.value = true;
    try {
      const smtpPasswordForTest = smtpPasswordManuallyEdited.value
        ? form.smtp_password
        : "";
      const result = await testSmtpConnectionAction({
        smtp_host: form.smtp_host,
        smtp_port: form.smtp_port,
        smtp_username: form.smtp_username,
        smtp_password: smtpPasswordForTest,
        smtp_use_tls: form.smtp_use_tls,
      });
      // API returns { message: "..." } on success, errors are thrown as exceptions
      appStore.showSuccess(
        result.message || t("admin.settings.smtpConnectionSuccess"),
      );
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.failedToTestSmtp")),
      );
    } finally {
      testingSmtp.value = false;
    }
  }

  async function sendTestEmail() {
    if (!testEmailAddress.value) {
      appStore.showError(t("admin.settings.testEmail.enterRecipientHint"));
      return;
    }

    sendingTestEmail.value = true;
    try {
      const smtpPasswordForSend = smtpPasswordManuallyEdited.value
        ? form.smtp_password
        : "";
      const result = await sendTestEmailAction({
        email: testEmailAddress.value,
        smtp_host: form.smtp_host,
        smtp_port: form.smtp_port,
        smtp_username: form.smtp_username,
        smtp_password: smtpPasswordForSend,
        smtp_from_email: form.smtp_from_email,
        smtp_from_name: form.smtp_from_name,
        smtp_use_tls: form.smtp_use_tls,
      });
      // API returns { message: "..." } on success, errors are thrown as exceptions
      appStore.showSuccess(result.message || t("admin.settings.testEmailSent"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.failedToSendTestEmail")),
      );
    } finally {
      sendingTestEmail.value = false;
    }
  }

  onMounted(() => {
    loadSettings();
    loadSubscriptionGroups();
    loadAdminApiKey();
    loadScopedAdminApiKeys();
    loadUpstreamBillingProbeSettings();
    loadOllamaCloudUsageSettings();
    loadOverloadCooldownSettings();
    loadRateLimit429CooldownSettings();
    loadGlobalTempUnschedulableSettings();
    loadCodexSimulationSettings();
    loadStreamTimeoutSettings();
    loadRectifierSettings();
    loadBetaPolicySettings();
    loadProviders();
  });


  return {
    activeTab,
    aliyunCaptchaRegionOptions,
		tencentCaptchaRegionOptions,
    addAuthSourceDefaultSubscription,
    addClaudeOAuthSystemPromptBlock,
    addCodexBlacklistRow,
    addCodexFingerprintRow,
    addCodexWhitelistRow,
    addDefaultSubscription,
    addEndpoint,
    addLoginAgreementDocument,
    addMenuItem,
    addOpenAIFastPolicyModelPattern,
    addOpenAIFastPolicyRule,
    addQuickPattern,
    addQuotaNotifyEmail,
    addWebSearchProvider,
    adminApiKeyExists,
    adminApiKeyForm,
    adminApiKeyLoading,
    adminApiKeyMasked,
    adminApiKeyMinExpiry,
    adminApiKeyOperating,
    adminApiKeyPanelLoading,
    adminApiKeyPanelOperating,
    adminApiKeyPanelSecret,
    adminApiKeyScopeOptions,
    affiliateBatchModal,
    affiliateConfirmDialog,
    affiliateModal,
    affiliateModalCanSubmit,
    affiliateState,
    allPaymentTypes,
    apiKeyVisible,
    applyBetaPreset,
    applyClaudeOAuthSystemPromptPreset,
    askResetAffiliateUser,
    authSourceDefaults,
    authSourceDefaultsMeta,
    betaPolicyActionOptions,
    betaPolicyForm,
    betaPolicyLoading,
    betaPolicySaving,
    betaPolicyScopeOptions,
    betaPresets,
    cancelAffiliateConfirm,
    cancelEditScopedAdminApiKey,
    cancelRateLimitModeOptions,
    cancelRateLimitUnitOptions,
    changeAffiliatePage,
    claudeOAuthSystemPromptBlockTypeOptions,
    claudeOAuthSystemPromptBlocks,
    claudeOAuthSystemPromptCacheTTLOptions,
    claudeOAuthSystemPromptPresetOptions,
    clearSelectedAffiliateUser,
    clientIPLastRefreshText,
    clientIPResolutionModeOptions,
    clientIPTrustedProxiesText,
    closeAffiliateModal,
    codexBlacklistRows,
    codexFingerprintNoRequired,
    codexFingerprintRows,
    codexSimulationForm,
    codexSimulationLoadFailed,
    codexSimulationLoading,
    codexSimulationSaving,
    codexWhitelistRows,
    commitRegistrationEmailSuffixWhitelistDraft,
    commonModelPatterns,
    confirmDeleteProvider,
    copyApiKey,
    copyNewKey,
    copyScopedAdminApiKey,
    createAdminApiKey,
    createScopedAdminApiKey,
    currentOrigin,
    defaultSubscriptionGroupOptions,
    deleteAdminApiKey,
    editScopedAdminApiKey,
    editingAdminApiKeyId,
    editingProvider,
    enabledProviderKeyOptions,
    expandedProviders,
    form,
    formatAdminApiKeyDate,
    formatSubscribedAt,
    getBetaDisplayName,
    getClaudeOAuthPresetLabel,
    githubOAuthRedirectUrlSuggestion,
    globalTempUnschedulableForm,
    globalTempUnschedulableLoading,
    globalTempUnschedulableSaving,
    googleOAuthRedirectUrlSuggestion,
    handleAffiliateConfirm,
    handleDeleteProvider,
    handleRegistrationEmailSuffixWhitelistDraftInput,
    handleRegistrationEmailSuffixWhitelistDraftKeydown,
    handleRegistrationEmailSuffixWhitelistPaste,
    handleReorderProviders,
    handleSaveProvider,
    handleSettingsTabKeydown,
    handleToggleField,
    handleToggleType,
    handleWeChatMPEnabledChange,
    handleWeChatMobileEnabledChange,
    handleWeChatOpenEnabledChange,
    hasAnyPaymentTypeEnabled,
    humanVerificationProviders,
    isPaymentTypeEnabled,
    isZhLocale,
    linuxdoRedirectUrlSuggestion,
    loadBalanceOptions,
    loadFailed,
    loadProviders,
    loading,
    localText,
    loginAgreementRoutePath,
    markClaudeOAuthSystemPromptBlockCustom,
    moveClaudeOAuthSystemPromptBlock,
    moveMenuItem,
    newAdminApiKey,
    oidcRedirectUrlSuggestion,
    ollamaCloudUsageForm,
    ollamaCloudUsageLoading,
    ollamaCloudUsageSaving,
    onAffiliateSearchInput,
    onAffiliateUserSearchInput,
    openAIAdvancedSchedulerWeightFields,
    openAffiliateBatchModal,
    openAffiliateModal,
    openCreateProvider,
    openEditProvider,
    openTestDialog,
    openaiFastPolicyActionOptions,
    openaiFastPolicyForm,
    openaiFastPolicyScopeOptions,
    openaiFastPolicyTierOptions,
    overloadCooldownForm,
    overloadCooldownLoading,
    overloadCooldownSaving,
    panelRateLimitSettingsMounted,
    parseSubscribedAt,
    paymentGuideHref,
    paymentMethodsHref,
    providerDialogRef,
    providerKeyOptions,
    providerSaving,
    providers,
    providersLoading,
    quotaPercentage,
    rateLimit429CooldownForm,
    rateLimit429CooldownLoading,
    rateLimit429CooldownSaving,
    rectifierForm,
    rectifierLoading,
    rectifierSaving,
    regenerateAdminApiKey,
    registrationEmailSuffixWhitelistDraft,
    registrationEmailSuffixWhitelistTags,
    removeAuthSourceDefaultSubscription,
    removeClaudeOAuthSystemPromptBlock,
    removeCodexBlacklistRow,
    removeCodexFingerprintRow,
    removeCodexWhitelistRow,
    removeDefaultSubscription,
    removeEndpoint,
    removeLoginAgreementDocument,
    removeMenuItem,
    removeOpenAIFastPolicyModelPattern,
    removeOpenAIFastPolicyRule,
    removeRegistrationEmailSuffixWhitelistTag,
    removeWebSearchProvider,
    resetClaudeOAuthSystemPromptBlocks,
    resetWebSearchUsage,
    restoreOriginalCodexBehavior,
    revokeScopedAdminApiKey,
    rotateScopedAdminApiKey,
    saveBetaPolicySettings,
    saveCodexSimulationSettings,
    saveGlobalTempUnschedulableSettings,
    saveOllamaCloudUsageSettings,
    saveOverloadCooldownSettings,
    saveRateLimit429CooldownSettings,
    saveRectifierSettings,
    saveSettings,
    saveStreamTimeoutSettings,
    saveUpstreamBillingProbeSettings,
    saving,
    schedulerV2StatusClass,
    schedulerV2StatusLabel,
    scopedAdminApiKeys,
    selectAffiliateUser,
    selectSettingsTab,
    sendTestEmail,
    sendingTestEmail,
    setAndCopyEmailOAuthRedirectUrl,
    setAndCopyLinuxdoRedirectUrl,
    setAndCopyOIDCRedirectUrl,
    setAndCopyWeChatRedirectUrl,
    setHumanVerificationProvider,
    settingsStepUp,
    settingsTabs,
    showDeleteProviderDialog,
    showProviderDialog,
    smtpPasswordManuallyEdited,
    streamTimeoutForm,
    streamTimeoutLoading,
    streamTimeoutSaving,
    submitAffiliateBatchModal,
    submitAffiliateModal,
    subscriptionGroups,
    t,
    tablePageSizeOptionsInput,
    testEmailAddress,
    testSmtpConnection,
    testWebSearchProvider,
    testingSmtp,
    toggleAffiliateSelect,
    toggleAffiliateSelectAll,
    toggleClaudeOAuthSystemPromptBlock,
    togglePaymentType,
    toggleProviderExpand,
    upstreamBillingProbeForm,
    upstreamBillingProbeLoading,
    upstreamBillingProbeSaving,
    webSearchConfig,
    webSearchProxies,
    wechatRedirectUrlSuggestion,
    wsTestDialogOpen,
    wsTestLoading,
    wsTestQuery,
    wsTestResult,
  }
}

export type SettingsPageContext = ReturnType<typeof useSettingsPage>
