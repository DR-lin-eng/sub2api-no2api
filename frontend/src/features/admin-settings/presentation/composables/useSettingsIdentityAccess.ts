import { computed, ref, watch } from "vue";
import {
  defaultWeChatConnectScopesForMode,
  deriveWeChatConnectStoredMode,
  resolveWeChatConnectModeCapabilities,
  type WeChatConnectMode,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import type { SettingsForm } from "./settingsForm";

type Translate = (key: string) => string;
type LocalText = (zh: string, en: string) => string;
type CopyToClipboard = (
  text: string,
  successMessage?: string,
) => Promise<boolean>;
type EmailOAuthProvider = "github" | "google";
type HumanVerificationEnabledKey =
  | "turnstile_enabled"
  | "recaptcha_enabled"
  | "cap_enabled"
  | "tencent_captcha_enabled"
  | "aliyun_captcha_enabled"
  | "local_captcha_enabled";

export function useSettingsIdentityAccess(
  form: SettingsForm,
  t: Translate,
  localText: LocalText,
  copyToClipboard: CopyToClipboard,
) {
  const clientIPTrustedProxiesText = ref("");
  const humanVerificationProviders: Array<{
    key: HumanVerificationEnabledKey;
    label: string;
    hint: string;
  }> = [
    {
      key: "turnstile_enabled",
      label: "admin.settings.turnstile.enableTurnstile",
      hint: "admin.settings.turnstile.enableTurnstileHint",
    },
    {
      key: "recaptcha_enabled",
      label: "admin.settings.turnstile.enableRecaptcha",
      hint: "admin.settings.turnstile.enableRecaptchaHint",
    },
    {
      key: "cap_enabled",
      label: "admin.settings.turnstile.enableCap",
      hint: "admin.settings.turnstile.enableCapHint",
    },
    {
      key: "tencent_captcha_enabled",
      label: "admin.settings.turnstile.enableTencentCaptcha",
      hint: "admin.settings.turnstile.enableTencentCaptchaHint",
    },
    {
      key: "aliyun_captcha_enabled",
      label: "admin.settings.turnstile.enableAliyunCaptcha",
      hint: "admin.settings.turnstile.enableAliyunCaptchaHint",
    },
    {
      key: "local_captcha_enabled",
      label: "admin.settings.turnstile.enableLocalCaptcha",
      hint: "admin.settings.turnstile.enableLocalCaptchaHint",
    },
  ];

  const clientIPResolutionModeOptions = computed(() => [
    {
      value: "auto_compat",
      label: t("admin.settings.apiKeyAcl.modes.auto_compat"),
    },
    {
      value: "trusted_proxy",
      label: t("admin.settings.apiKeyAcl.modes.trusted_proxy"),
    },
    {
      value: "direct",
      label: t("admin.settings.apiKeyAcl.modes.direct"),
    },
  ]);

  const aliyunCaptchaRegionOptions = computed(() => [
    {
      value: "cn",
      label: t("admin.settings.aliyunCaptcha.regionCn"),
    },
    {
      value: "sgp",
      label: t("admin.settings.aliyunCaptcha.regionSgp"),
    },
  ]);

  const tencentCaptchaRegionOptions = computed(() => [
    {
      value: "cn",
      label: t("admin.settings.turnstile.tencentRegionCn"),
    },
    {
      value: "intl",
      label: t("admin.settings.turnstile.tencentRegionIntl"),
    },
  ]);

  const clientIPLastRefreshText = computed(() => {
    const raw = form.client_ip_resolution_status.cloudflare_last_success_at;
    if (!raw) return "";
    const value = new Date(raw);
    return Number.isNaN(value.getTime()) ? raw : value.toLocaleString();
  });

  function parseClientIPTrustedProxies(value: string): string[] {
    return Array.from(
      new Set(
        value
          .split(/[\n,]/)
          .map((item) => item.trim())
          .filter(Boolean),
      ),
    );
  }

  function setHumanVerificationProvider(
    provider: HumanVerificationEnabledKey,
    enabled: boolean,
  ): void {
    for (const option of humanVerificationProviders) {
      form[option.key] = enabled && option.key === provider;
    }
  }

  function normalizeHumanVerificationProvider(): void {
    const selected = humanVerificationProviders.find(
      (option) => form[option.key],
    );
    if (selected) {
      setHumanVerificationProvider(selected.key, true);
    }
  }

  const currentOrigin =
    typeof window !== "undefined" ? window.location.origin : "";

  function buildApiCallbackUrl(path: string): string {
    const base = (form.api_base_url || currentOrigin).replace(/\/+$/, "");
    const apiRoot = base.endsWith("/api/v1") ? base : `${base}/api/v1`;
    return `${apiRoot}${path.startsWith("/") ? path : `/${path}`}`;
  }

  const linuxdoRedirectUrlSuggestion = computed(() =>
    buildApiCallbackUrl("/auth/oauth/linuxdo/callback"),
  );

  async function setAndCopyLinuxdoRedirectUrl() {
    const url = linuxdoRedirectUrlSuggestion.value;
    if (!url) return;
    form.linuxdo_connect_redirect_url = url;
    await copyToClipboard(
      url,
      t("admin.settings.linuxdo.redirectUrlSetAndCopied"),
    );
  }

  const githubOAuthRedirectUrlSuggestion = computed(() =>
    buildApiCallbackUrl("/auth/oauth/github/callback"),
  );
  const googleOAuthRedirectUrlSuggestion = computed(() =>
    buildApiCallbackUrl("/auth/oauth/google/callback"),
  );

  async function setAndCopyEmailOAuthRedirectUrl(
    provider: EmailOAuthProvider,
  ) {
    const url =
      provider === "github"
        ? githubOAuthRedirectUrlSuggestion.value
        : googleOAuthRedirectUrlSuggestion.value;
    if (!url) return;

    if (provider === "github") {
      form.github_oauth_redirect_url = url;
    } else {
      form.google_oauth_redirect_url = url;
    }
    await copyToClipboard(
      url,
      localText("回调地址已写入并复制。", "Callback URL set and copied."),
    );
  }

  const wechatRedirectUrlSuggestion = computed(() =>
    buildApiCallbackUrl("/auth/oauth/wechat/callback"),
  );

  function syncWeChatConnectMode(preferredMode?: WeChatConnectMode) {
    if (form.wechat_connect_mp_enabled && form.wechat_connect_mobile_enabled) {
      if (preferredMode === "mobile") {
        form.wechat_connect_mp_enabled = false;
      } else {
        form.wechat_connect_mobile_enabled = false;
      }
    }

    const capabilities = resolveWeChatConnectModeCapabilities(
      form.wechat_connect_open_enabled,
      form.wechat_connect_mp_enabled,
      form.wechat_connect_mobile_enabled,
      form.wechat_connect_mode,
    );
    form.wechat_connect_open_enabled = capabilities.openEnabled;
    form.wechat_connect_mp_enabled = capabilities.mpEnabled;
    form.wechat_connect_mobile_enabled = capabilities.mobileEnabled;
    form.wechat_connect_mode = deriveWeChatConnectStoredMode(
      capabilities.openEnabled,
      capabilities.mpEnabled,
      capabilities.mobileEnabled,
      form.wechat_connect_mode,
    );
    form.wechat_connect_scopes = defaultWeChatConnectScopesForMode(
      form.wechat_connect_mode,
    );
  }

  function handleWeChatOpenEnabledChange(value: boolean) {
    form.wechat_connect_open_enabled = value;
    syncWeChatConnectMode(value ? "open" : undefined);
  }

  function handleWeChatMPEnabledChange(value: boolean) {
    form.wechat_connect_mp_enabled = value;
    if (value) {
      form.wechat_connect_mobile_enabled = false;
    }
    syncWeChatConnectMode(value ? "mp" : undefined);
  }

  function handleWeChatMobileEnabledChange(value: boolean) {
    form.wechat_connect_mobile_enabled = value;
    if (value) {
      form.wechat_connect_mp_enabled = false;
    }
    syncWeChatConnectMode(value ? "mobile" : undefined);
  }

  async function setAndCopyWeChatRedirectUrl() {
    const url = wechatRedirectUrlSuggestion.value;
    if (!url) return;
    form.wechat_connect_redirect_url = url;
    await copyToClipboard(
      url,
      t("admin.settings.wechatConnect.redirectUrlSetAndCopied"),
    );
  }

  const oidcRedirectUrlSuggestion = computed(() =>
    buildApiCallbackUrl("/auth/oauth/oidc/callback"),
  );

  async function setAndCopyOIDCRedirectUrl() {
    const url = oidcRedirectUrlSuggestion.value;
    if (!url) return;
    form.oidc_connect_redirect_url = url;
    await copyToClipboard(
      url,
      t("admin.settings.oidc.redirectUrlSetAndCopied"),
    );
  }

  watch(
    () => form.dingtalk_connect_corp_restriction_policy,
    (policy) => {
      if (policy === "internal_only") return;
      form.dingtalk_connect_bypass_registration = false;
      form.dingtalk_connect_sync_corp_email = false;
      form.dingtalk_connect_sync_display_name = false;
      form.dingtalk_connect_sync_dept = false;
    },
  );

  return {
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
  };
}
