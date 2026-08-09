import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { proxiesAPI } from '@/features/admin-proxies/data/datasources/adminProxiesDatasource'
import type {
  WebSearchEmulationConfig,
  WebSearchProviderConfig,
  WebSearchTestResult,
} from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import { getWebSearchEmulationConfig } from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import {
  resetWebSearchUsage as resetWebSearchUsageAction,
  testWebSearchEmulation,
  updateWebSearchEmulationConfig,
} from '@/features/admin-settings/data/datasources/adminSettingsActions'
import type { Proxy } from '@/types'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { useAppStore } from '@/core/stores/appStore'

export function useSettingsWebSearch() {
  const { t } = useI18n()
  const appStore = useAppStore()

  // Proxies for web search emulation ProxySelector
  const webSearchProxies = ref<Proxy[]>([]);

  // Web Search Emulation config (loaded/saved separately)
  const DEFAULT_WEB_SEARCH_QUOTA_LIMIT = 1000;

  const webSearchConfig = reactive<WebSearchEmulationConfig>({
    enabled: false,
    providers: [],
  });

  const expandedProviders = reactive<Record<number, boolean>>({});
  const apiKeyVisible = reactive<Record<number, boolean>>({});
  const wsTestQuery = ref("");
  const wsTestLoading = ref(false);
  const wsTestResult = ref<WebSearchTestResult | null>(null);
  const wsTestDialogOpen = ref(false);

  function openTestDialog() {
    wsTestResult.value = null;
    wsTestDialogOpen.value = true;
  }

  function toggleProviderExpand(idx: number) {
    expandedProviders[idx] = !expandedProviders[idx];
  }

  function removeWebSearchProvider(idx: number) {
    webSearchConfig.providers.splice(idx, 1);
    // Re-index expandedProviders and apiKeyVisible after removal
    const newExpanded: Record<number, boolean> = {};
    const newVisible: Record<number, boolean> = {};
    for (let i = 0; i < webSearchConfig.providers.length; i++) {
      const oldIdx = i >= idx ? i + 1 : i;
      newExpanded[i] = expandedProviders[oldIdx] ?? false;
      newVisible[i] = apiKeyVisible[oldIdx] ?? false;
    }
    Object.keys(expandedProviders).forEach(
      (k) => delete expandedProviders[Number(k)],
    );
    Object.keys(apiKeyVisible).forEach((k) => delete apiKeyVisible[Number(k)]);
    Object.assign(expandedProviders, newExpanded);
    Object.assign(apiKeyVisible, newVisible);
  }

  function addWebSearchProvider() {
    const idx = webSearchConfig.providers.length;
    webSearchConfig.providers.push({
      type: "brave",
      api_key: "",
      api_key_configured: false,
      quota_limit: DEFAULT_WEB_SEARCH_QUOTA_LIMIT,
      subscribed_at: null,
      proxy_id: null,
      expires_at: null,
    } as WebSearchProviderConfig);
    expandedProviders[idx] = true;
  }

  function formatSubscribedAt(ts: number | null): string {
    if (!ts) return "";
    // Use UTC to avoid timezone drift on repeated edits
    const d = new Date(ts * 1000);
    const y = d.getUTCFullYear();
    const m = String(d.getUTCMonth() + 1).padStart(2, "0");
    const day = String(d.getUTCDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }

  function parseSubscribedAt(dateStr: string): number | null {
    if (!dateStr) return null;
    // Parse as UTC to match formatSubscribedAt
    return Math.floor(new Date(dateStr + "T00:00:00Z").getTime() / 1000);
  }

  function quotaPercentage(provider: WebSearchProviderConfig): number {
    if (!provider.quota_limit || provider.quota_limit <= 0) return 0;
    return ((provider.quota_used ?? 0) / provider.quota_limit) * 100;
  }

  async function resetWebSearchUsage(idx: number) {
    const provider = webSearchConfig.providers[idx];
    if (!provider) return;
    if (!confirm(t("admin.settings.webSearchEmulation.resetUsageConfirm")))
      return;
    try {
      await resetWebSearchUsageAction({
        provider_type: provider.type,
      });
      provider.quota_used = 0;
      appStore.showSuccess(
        t("admin.settings.webSearchEmulation.resetUsageSuccess"),
      );
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t("common.error")));
    }
  }

  async function copyApiKey(idx: number) {
    const key = webSearchConfig.providers[idx]?.api_key;
    if (!key) {
      appStore.showError(
        t("admin.settings.webSearchEmulation.apiKeyPlaceholder"),
      );
      return;
    }
    try {
      await navigator.clipboard.writeText(key);
      appStore.showSuccess(t("admin.settings.webSearchEmulation.copied"));
    } catch {
      appStore.showError(t("common.error"));
    }
  }

  async function testWebSearchProvider() {
    wsTestLoading.value = true;
    wsTestResult.value = null;
    try {
      const query =
        wsTestQuery.value.trim() ||
        t("admin.settings.webSearchEmulation.testDefaultQuery");
      wsTestResult.value = await testWebSearchEmulation(query);
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t("common.error")));
    } finally {
      wsTestLoading.value = false;
    }
  }

  async function loadWebSearchConfig() {
    try {
      const [resp, proxiesResp] = await Promise.all([
        getWebSearchEmulationConfig(),
        proxiesAPI.list().catch(() => ({ items: [] as Proxy[] })),
      ]);
      if (resp) {
        webSearchConfig.enabled = resp.enabled || false;
        webSearchConfig.providers = resp.providers || [];
      }
      webSearchProxies.value = proxiesResp.items || [];
    } catch (err: unknown) {
      // 404 is expected when config hasn't been created yet; show error for other failures
      const status = (err as { status?: number })?.status;
      if (status !== 404 && status !== undefined) {
        appStore.showError(extractApiErrorMessage(err, t("common.error")));
      }
    }
  }

  async function saveWebSearchConfig(): Promise<boolean> {
    try {
      for (const p of webSearchConfig.providers) {
        const raw = p.quota_limit;
        if (raw != null && Number(raw) !== 0 && Number(raw) < 1) {
          appStore.showError(
            t("admin.settings.webSearchEmulation.quotaLimitMustBePositive"),
          );
          return false;
        }
      }
      const providers = webSearchConfig.providers.map(
        (p: WebSearchProviderConfig) => ({
          ...p,
          quota_limit: Number(p.quota_limit) > 0 ? Number(p.quota_limit) : null,
        }),
      );
      await updateWebSearchEmulationConfig({
        enabled: webSearchConfig.enabled,
        providers,
      });
      return true;
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t("common.error")));
      return false;
    }
  }

  return {
    addWebSearchProvider,
    apiKeyVisible,
    copyApiKey,
    expandedProviders,
    formatSubscribedAt,
    loadWebSearchConfig,
    openTestDialog,
    parseSubscribedAt,
    quotaPercentage,
    removeWebSearchProvider,
    resetWebSearchUsage,
    saveWebSearchConfig,
    testWebSearchProvider,
    toggleProviderExpand,
    webSearchConfig,
    webSearchProxies,
    wsTestDialogOpen,
    wsTestLoading,
    wsTestQuery,
    wsTestResult,
  }
}
