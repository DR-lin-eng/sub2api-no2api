import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { accountsAPI } from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'
import {
  type CodexSimulationSettings,
  type OpenAIFastPolicyRule,
  type ThinkingDisplayMode,
} from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import {
  getBetaPolicySettings,
  getCodexSimulationSettings,
  getGlobalTempUnschedulableSettings,
  getOverloadCooldownSettings,
  getRateLimit429CooldownSettings,
  getRectifierSettings,
  getStreamTimeoutSettings,
} from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import {
  forceDisableCodexSimulationSettings,
  updateBetaPolicySettings,
  updateCodexSimulationSettings,
  updateGlobalTempUnschedulableSettings,
  updateOverloadCooldownSettings,
  updateRateLimit429CooldownSettings,
  updateRectifierSettings,
  updateStreamTimeoutSettings,
} from '@/features/admin-settings/data/datasources/adminSettingsActions'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { useAppStore } from '@/core/stores/appStore'

export function useSettingsGatewayPolicies() {
  const { t } = useI18n()
  const appStore = useAppStore()

  // Upstream billing probe state
  const upstreamBillingProbeLoading = ref(true);
  const upstreamBillingProbeSaving = ref(false);
  const upstreamBillingProbeForm = reactive({
    enabled: true,
    interval_minutes: 30,
  });

  const ollamaCloudUsageLoading = ref(true);
  const ollamaCloudUsageSaving = ref(false);
  const ollamaCloudUsageForm = reactive({
    enabled: false,
    interval_minutes: 60,
    debounce_minutes: 1,
  });

  // Overload Cooldown (529) 状态
  const overloadCooldownLoading = ref(true);
  const overloadCooldownSaving = ref(false);
  const overloadCooldownForm = reactive({
    enabled: true,
    cooldown_minutes: 10,
  });

  // Rate Limit Cooldown (429) 状态
  const rateLimit429CooldownLoading = ref(true);
  const rateLimit429CooldownSaving = ref(false);
  const rateLimit429CooldownForm = reactive({
    enabled: true,
    cooldown_seconds: 5,
    auto_disable_enabled: false,
    auto_disable_threshold: 3,
    auto_disable_quota_check_enabled: false,
  });

  // Global Temporary Unschedulable 状态
  const globalTempUnschedulableLoading = ref(true);
  const globalTempUnschedulableSaving = ref(false);
  const globalTempUnschedulableForm = reactive({
    enabled: true,
  });

  const codexSimulationLoading = ref(true);
  const codexSimulationLoadFailed = ref(false);
  const codexSimulationSaving = ref(false);
  const codexSimulationForm = reactive<CodexSimulationSettings>({
    full_simulation_enabled: false,
    c_level_simulation_enabled: false,
    continuation_mode: "off",
    state_ttl_seconds: 604800,
    identity_secret_configured: false,
  });
  const lastCodexSimulationSettings = ref<CodexSimulationSettings>({
    ...codexSimulationForm,
  });

  // Stream Timeout 状态
  const streamTimeoutLoading = ref(true);
  const streamTimeoutSaving = ref(false);
  const streamTimeoutForm = reactive({
    response_header_timeout_degradation_enabled: true,
    response_header_timeout_seconds: 20,
    enabled: true,
    action: "temp_unsched" as "temp_unsched" | "error" | "none",
    temp_unsched_minutes: 5,
    threshold_count: 3,
    threshold_window_minutes: 10,
    openai_first_output_timeout_seconds: 90,
    openai_high_effort_first_output_timeout_seconds: 180,
    stream_keepalive_interval_seconds: 10,
  });

  // Rectifier 状态
  const rectifierLoading = ref(true);
  const rectifierSaving = ref(false);
  const rectifierForm = reactive({
    enabled: true,
    thinking_signature_enabled: true,
    thinking_budget_enabled: true,
    // 与后端 DefaultRectifierSettings 保持一致：display_only 只取消隐藏已经产生并
    // 计费的思考摘要，零成本；force 会改变成本与缓存行为，须由运维显式选择。
    thinking_display_mode: "display_only" as ThinkingDisplayMode,
    apikey_signature_enabled: false,
    apikey_signature_patterns: [] as string[],
  });

  // Beta Policy 状态
  const betaPolicyLoading = ref(true);
  const betaPolicySaving = ref(false);
  const betaPolicyForm = reactive({
    rules: [] as Array<{
      beta_token: string;
      action: "pass" | "filter" | "block";
      scope: "all" | "oauth" | "apikey" | "bedrock";
      error_message?: string;
      model_whitelist?: string[];
      fallback_action?: "pass" | "filter" | "block";
      fallback_error_message?: string;
    }>,
  });

  // OpenAI Fast/Flex Policy 状态
  const openaiFastPolicyForm = reactive({
    rules: [] as OpenAIFastPolicyRule[],
  });
  // 标记 openai_fast_policy_settings 是否已成功从后端加载，
  // 避免后端 GET 出错或字段缺失时，保存把默认规则覆盖成空数组。
  const openaiFastPolicyLoaded = ref(false);


  async function loadUpstreamBillingProbeSettings() {
    upstreamBillingProbeLoading.value = true;
    try {
      Object.assign(
        upstreamBillingProbeForm,
        await accountsAPI.getUpstreamBillingProbeSettings(),
      );
    } catch {
      // Keep defaults when this optional setting cannot be loaded.
    } finally {
      upstreamBillingProbeLoading.value = false;
    }
  }

  async function saveUpstreamBillingProbeSettings() {
    upstreamBillingProbeSaving.value = true;
    try {
      const updated = await accountsAPI.updateUpstreamBillingProbeSettings({
        ...upstreamBillingProbeForm,
      });
      Object.assign(upstreamBillingProbeForm, updated);
      appStore.showSuccess(t("admin.settings.upstreamBillingProbe.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.upstreamBillingProbe.saveFailed"),
        ),
      );
    } finally {
      upstreamBillingProbeSaving.value = false;
    }
  }

  async function loadOllamaCloudUsageSettings() {
    ollamaCloudUsageLoading.value = true;
    try {
      Object.assign(
        ollamaCloudUsageForm,
        await accountsAPI.getOllamaCloudUsageSettings(),
      );
    } catch {
      // Keep the fail-safe disabled defaults when this optional setting cannot be loaded.
    } finally {
      ollamaCloudUsageLoading.value = false;
    }
  }

  async function saveOllamaCloudUsageSettings() {
    ollamaCloudUsageSaving.value = true;
    try {
      const updated = await accountsAPI.updateOllamaCloudUsageSettings({
        ...ollamaCloudUsageForm,
      });
      Object.assign(ollamaCloudUsageForm, updated);
      appStore.showSuccess(t("admin.settings.ollamaCloudUsage.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.ollamaCloudUsage.saveFailed")),
      );
    } finally {
      ollamaCloudUsageSaving.value = false;
    }
  }

  // Overload Cooldown 方法
  async function loadOverloadCooldownSettings() {
    overloadCooldownLoading.value = true;
    try {
      const settings = await getOverloadCooldownSettings();
      Object.assign(overloadCooldownForm, settings);
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      overloadCooldownLoading.value = false;
    }
  }

  async function saveOverloadCooldownSettings() {
    overloadCooldownSaving.value = true;
    try {
      const updated = await updateOverloadCooldownSettings({
        enabled: overloadCooldownForm.enabled,
        cooldown_minutes: overloadCooldownForm.cooldown_minutes,
      });
      Object.assign(overloadCooldownForm, updated);
      appStore.showSuccess(t("admin.settings.overloadCooldown.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.overloadCooldown.saveFailed"),
        ),
      );
    } finally {
      overloadCooldownSaving.value = false;
    }
  }

  // Rate Limit Cooldown (429) 方法
  async function loadRateLimit429CooldownSettings() {
    rateLimit429CooldownLoading.value = true;
    try {
      const settings = await getRateLimit429CooldownSettings();
      Object.assign(rateLimit429CooldownForm, settings);
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      rateLimit429CooldownLoading.value = false;
    }
  }

  async function saveRateLimit429CooldownSettings() {
    rateLimit429CooldownSaving.value = true;
    try {
      const updated = await updateRateLimit429CooldownSettings({
        enabled: rateLimit429CooldownForm.enabled,
        cooldown_seconds: rateLimit429CooldownForm.cooldown_seconds,
        auto_disable_enabled: rateLimit429CooldownForm.auto_disable_enabled,
        auto_disable_threshold: rateLimit429CooldownForm.auto_disable_threshold,
        auto_disable_quota_check_enabled:
          rateLimit429CooldownForm.auto_disable_quota_check_enabled,
      });
      Object.assign(rateLimit429CooldownForm, updated);
      appStore.showSuccess(t("admin.settings.rateLimit429Cooldown.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.rateLimit429Cooldown.saveFailed"),
        ),
      );
    } finally {
      rateLimit429CooldownSaving.value = false;
    }
  }

  // Global Temporary Unschedulable 方法
  async function loadGlobalTempUnschedulableSettings() {
    globalTempUnschedulableLoading.value = true;
    try {
      const settings =
        await getGlobalTempUnschedulableSettings();
      Object.assign(globalTempUnschedulableForm, settings);
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      globalTempUnschedulableLoading.value = false;
    }
  }

  async function saveGlobalTempUnschedulableSettings() {
    globalTempUnschedulableSaving.value = true;
    try {
      const updated =
        await updateGlobalTempUnschedulableSettings({
          enabled: globalTempUnschedulableForm.enabled,
        });
      Object.assign(globalTempUnschedulableForm, updated);
      appStore.showSuccess(
        t("admin.settings.globalTempUnschedulable.saved"),
      );
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.globalTempUnschedulable.saveFailed"),
        ),
      );
    } finally {
      globalTempUnschedulableSaving.value = false;
    }
  }

  function applyCodexSimulationSettings(settings: CodexSimulationSettings) {
    Object.assign(codexSimulationForm, settings);
    lastCodexSimulationSettings.value = { ...settings };
    codexSimulationLoadFailed.value = false;
  }

  async function loadCodexSimulationSettings() {
    codexSimulationLoading.value = true;
    codexSimulationLoadFailed.value = false;
    try {
      applyCodexSimulationSettings(await getCodexSimulationSettings());
    } catch (error: unknown) {
      codexSimulationLoadFailed.value = true;
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.codexSimulation.loadFailed"),
        ),
      );
    } finally {
      codexSimulationLoading.value = false;
    }
  }

  async function persistCodexSimulationSettings(
    payload: Pick<
      CodexSimulationSettings,
      | "full_simulation_enabled"
      | "c_level_simulation_enabled"
      | "continuation_mode"
      | "state_ttl_seconds"
    >,
    successKey: string,
    failureKey: string,
  ) {
    const rollback = { ...lastCodexSimulationSettings.value };
    codexSimulationSaving.value = true;
    try {
      const updated = await updateCodexSimulationSettings(payload);
      applyCodexSimulationSettings(updated);
      appStore.showSuccess(t(successKey));
    } catch (error: unknown) {
      Object.assign(codexSimulationForm, rollback);
      appStore.showError(extractApiErrorMessage(error, t(failureKey)));
    } finally {
      codexSimulationSaving.value = false;
    }
  }

  async function saveCodexSimulationSettings() {
    await persistCodexSimulationSettings(
      {
        full_simulation_enabled: codexSimulationForm.full_simulation_enabled,
        c_level_simulation_enabled: Boolean(codexSimulationForm.c_level_simulation_enabled),
        continuation_mode: codexSimulationForm.continuation_mode,
        state_ttl_seconds: codexSimulationForm.state_ttl_seconds,
      },
      "admin.settings.codexSimulation.saved",
      "admin.settings.codexSimulation.saveFailed",
    );
  }

  async function restoreOriginalCodexBehavior() {
    const rollback = { ...lastCodexSimulationSettings.value };
    codexSimulationSaving.value = true;
    try {
      applyCodexSimulationSettings(await forceDisableCodexSimulationSettings());
      appStore.showSuccess(t("admin.settings.codexSimulation.restored"));
    } catch (error: unknown) {
      Object.assign(codexSimulationForm, rollback);
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.codexSimulation.restoreFailed"),
        ),
      );
    } finally {
      codexSimulationSaving.value = false;
    }
  }

  // Stream Timeout 方法
  async function loadStreamTimeoutSettings() {
    streamTimeoutLoading.value = true;
    try {
      const settings = await getStreamTimeoutSettings();
      Object.assign(streamTimeoutForm, settings);
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      streamTimeoutLoading.value = false;
    }
  }

  async function saveStreamTimeoutSettings() {
    streamTimeoutSaving.value = true;
    try {
      const updated = await updateStreamTimeoutSettings({
        response_header_timeout_degradation_enabled:
          streamTimeoutForm.response_header_timeout_degradation_enabled,
        response_header_timeout_seconds:
          streamTimeoutForm.response_header_timeout_seconds,
        enabled: streamTimeoutForm.enabled,
        action: streamTimeoutForm.action,
        temp_unsched_minutes: streamTimeoutForm.temp_unsched_minutes,
        threshold_count: streamTimeoutForm.threshold_count,
        threshold_window_minutes: streamTimeoutForm.threshold_window_minutes,
        openai_first_output_timeout_seconds:
          streamTimeoutForm.openai_first_output_timeout_seconds,
        openai_high_effort_first_output_timeout_seconds:
          streamTimeoutForm.openai_high_effort_first_output_timeout_seconds,
        stream_keepalive_interval_seconds:
          streamTimeoutForm.stream_keepalive_interval_seconds,
      });
      Object.assign(streamTimeoutForm, updated);
      appStore.showSuccess(t("admin.settings.streamTimeout.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.streamTimeout.saveFailed"),
        ),
      );
    } finally {
      streamTimeoutSaving.value = false;
    }
  }

  // Rectifier 方法
  async function loadRectifierSettings() {
    rectifierLoading.value = true;
    try {
      const settings = await getRectifierSettings();
      Object.assign(rectifierForm, settings);
      // 确保 patterns 是数组（旧数据可能为 null）
      if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
        rectifierForm.apikey_signature_patterns = [];
      }
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      rectifierLoading.value = false;
    }
  }

  async function saveRectifierSettings() {
    rectifierSaving.value = true;
    try {
      const updated = await updateRectifierSettings({
        enabled: rectifierForm.enabled,
        thinking_signature_enabled: rectifierForm.thinking_signature_enabled,
        thinking_budget_enabled: rectifierForm.thinking_budget_enabled,
        thinking_display_mode: rectifierForm.thinking_display_mode,
        apikey_signature_enabled: rectifierForm.apikey_signature_enabled,
        apikey_signature_patterns: rectifierForm.apikey_signature_patterns.filter(
          (p) => p.trim() !== "",
        ),
      });
      Object.assign(rectifierForm, updated);
      if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
        rectifierForm.apikey_signature_patterns = [];
      }
      appStore.showSuccess(t("admin.settings.rectifier.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.rectifier.saveFailed")),
      );
    } finally {
      rectifierSaving.value = false;
    }
  }

  const betaPolicyActionOptions = computed(() => [
    { value: "pass", label: t("admin.settings.betaPolicy.actionPass") },
    { value: "filter", label: t("admin.settings.betaPolicy.actionFilter") },
    { value: "block", label: t("admin.settings.betaPolicy.actionBlock") },
  ]);

  const betaPolicyScopeOptions = computed(() => [
    { value: "all", label: t("admin.settings.betaPolicy.scopeAll") },
    { value: "oauth", label: t("admin.settings.betaPolicy.scopeOAuth") },
    { value: "apikey", label: t("admin.settings.betaPolicy.scopeAPIKey") },
    { value: "bedrock", label: t("admin.settings.betaPolicy.scopeBedrock") },
  ]);

  // Beta Policy 方法
  const betaDisplayNames: Record<string, string> = {
    "fast-mode-2026-02-01": "Fast Mode",
    "context-1m-2025-08-07": "Context 1M",
  };

  // 快捷预设：按 beta_token 定义预设方案
  const betaPresets: Record<
    string,
    Array<{
      label: string;
      description: string;
      action: "pass" | "filter" | "block";
      model_whitelist: string[];
      fallback_action: "pass" | "filter" | "block";
    }>
  > = {
    "context-1m-2025-08-07": [
      {
        label: t("admin.settings.betaPolicy.presetOpusOnly"),
        description: t("admin.settings.betaPolicy.presetOpusOnlyDesc"),
        action: "pass",
        model_whitelist: ["claude-opus-4-6"],
        fallback_action: "filter",
      },
    ],
  };

  // 常用模型模式（具体 ID + 通配符示例）
  const commonModelPatterns = [
    "claude-opus-4-6",
    "claude-sonnet-4-6",
    "claude-opus-*",
    "claude-sonnet-*",
  ];

  function getBetaDisplayName(token: string): string {
    return betaDisplayNames[token] || token;
  }

  function applyBetaPreset(
    rule: (typeof betaPolicyForm.rules)[number],
    preset: {
      action: "pass" | "filter" | "block";
      model_whitelist: string[];
      fallback_action: "pass" | "filter" | "block";
    },
  ) {
    rule.action = preset.action;
    rule.model_whitelist = [...preset.model_whitelist];
    rule.fallback_action = preset.fallback_action;
  }

  function addQuickPattern(
    rule: (typeof betaPolicyForm.rules)[number],
    pattern: string,
  ) {
    if (!rule.model_whitelist) rule.model_whitelist = [];
    if (!rule.model_whitelist.includes(pattern)) {
      rule.model_whitelist.push(pattern);
    }
  }

  async function loadBetaPolicySettings() {
    betaPolicyLoading.value = true;
    try {
      const settings = await getBetaPolicySettings();
      betaPolicyForm.rules = settings.rules;
    } catch {
      // Silent fail - settings will use defaults
    } finally {
      betaPolicyLoading.value = false;
    }
  }

  // ==================== OpenAI Fast/Flex Policy ====================

  const openaiFastPolicyTierOptions = computed(() => [
    { value: "all", label: t("admin.settings.openaiFastPolicy.tierAll") },
    {
      value: "priority",
      label: t("admin.settings.openaiFastPolicy.tierPriority"),
    },
    { value: "flex", label: t("admin.settings.openaiFastPolicy.tierFlex") },
  ]);

  const openaiFastPolicyActionOptions = computed(() => [
    { value: "pass", label: t("admin.settings.openaiFastPolicy.actionPass") },
    { value: "filter", label: t("admin.settings.openaiFastPolicy.actionFilter") },
    {
      value: "force_priority",
      label: t("admin.settings.openaiFastPolicy.actionForcePriority"),
    },
    { value: "block", label: t("admin.settings.openaiFastPolicy.actionBlock") },
  ]);

  const openaiFastPolicyScopeOptions = computed(() => [
    { value: "all", label: t("admin.settings.openaiFastPolicy.scopeAll") },
    { value: "oauth", label: t("admin.settings.openaiFastPolicy.scopeOAuth") },
    { value: "apikey", label: t("admin.settings.openaiFastPolicy.scopeAPIKey") },
    {
      value: "bedrock",
      label: t("admin.settings.openaiFastPolicy.scopeBedrock"),
    },
  ]);

  function addOpenAIFastPolicyRule() {
    openaiFastPolicyForm.rules.push({
      service_tier: "priority",
      action: "filter",
      scope: "all",
      user_ids: [],
      error_message: "",
      model_whitelist: [],
      fallback_action: "pass",
      fallback_error_message: "",
    });
  }

  function removeOpenAIFastPolicyRule(index: number) {
    openaiFastPolicyForm.rules.splice(index, 1);
  }

  function addOpenAIFastPolicyModelPattern(rule: OpenAIFastPolicyRule) {
    if (!rule.model_whitelist) rule.model_whitelist = [];
    rule.model_whitelist.push("");
  }

  function removeOpenAIFastPolicyModelPattern(
    rule: OpenAIFastPolicyRule,
    idx: number,
  ) {
    rule.model_whitelist?.splice(idx, 1);
  }

  async function saveBetaPolicySettings() {
    betaPolicySaving.value = true;
    try {
      // Clean up empty patterns before saving
      const cleanedRules = betaPolicyForm.rules.map((rule) => {
        const whitelist = rule.model_whitelist?.filter((p) => p.trim() !== "");
        const hasWhitelist = whitelist && whitelist.length > 0;
        return {
          beta_token: rule.beta_token,
          action: rule.action,
          scope: rule.scope,
          error_message: rule.error_message,
          model_whitelist: hasWhitelist ? whitelist : undefined,
          fallback_action: hasWhitelist
            ? rule.fallback_action || "pass"
            : undefined,
          fallback_error_message:
            hasWhitelist && rule.fallback_action === "block"
              ? rule.fallback_error_message
              : undefined,
        };
      });
      const updated = await updateBetaPolicySettings({
        rules: cleanedRules,
      });
      betaPolicyForm.rules = updated.rules;
      appStore.showSuccess(t("admin.settings.betaPolicy.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.betaPolicy.saveFailed")),
      );
    } finally {
      betaPolicySaving.value = false;
    }
  }


  return {
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
    commonModelPatterns,
    codexSimulationForm,
    codexSimulationLoadFailed,
    codexSimulationLoading,
    codexSimulationSaving,
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
  }
}
