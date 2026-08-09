import type { Ref } from "vue";
import { normalizeRegistrationEmailSuffixDomains } from "@/core/utils/registrationEmailPolicy";
import {
  buildAuthSourceDefaultsState,
  defaultWeChatConnectScopesForMode,
  deriveWeChatConnectStoredMode,
  normalizePlatformQuotasMap,
  resolveWeChatConnectModeCapabilities,
  type AuthSourceDefaultsState,
  type SystemSettings,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import type { OpenAIFastPolicyRule } from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import type { SettingsForm } from "./settingsForm";

interface SettingsSaveResponseContext {
  form: SettingsForm;
  updated: SystemSettings;
  normalizeHumanVerificationProvider: () => void;
  clientIPTrustedProxiesText: Ref<string>;
  authSourceDefaults: AuthSourceDefaultsState;
  registrationEmailSuffixWhitelistTags: Ref<string[]>;
  registrationEmailSuffixWhitelistDraft: Ref<string>;
  tablePageSizeOptionsInput: Ref<string>;
  formatTablePageSizeOptions: (options: number[]) => string;
  smtpPasswordManuallyEdited: Ref<boolean>;
  openaiFastPolicyForm: { rules: OpenAIFastPolicyRule[] };
  openaiFastPolicyLoaded: Ref<boolean>;
}

export function applySettingsSaveResponse({
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
}: SettingsSaveResponseContext): void {
  for (const [key, value] of Object.entries(updated)) {
    if (key === "openai_fast_policy_settings") continue;
    if (value !== null && value !== undefined) {
      (form as Record<string, unknown>)[key] = value;
    }
  }

  normalizeHumanVerificationProvider();
  clientIPTrustedProxiesText.value = (
    updated.client_ip_trusted_proxies || []
  ).join("\n");
  Object.assign(authSourceDefaults, buildAuthSourceDefaultsState(updated));
  form.default_platform_quotas = normalizePlatformQuotasMap(
    updated.default_platform_quotas,
  );
  registrationEmailSuffixWhitelistTags.value =
    normalizeRegistrationEmailSuffixDomains(
      updated.registration_email_suffix_whitelist,
    );
  tablePageSizeOptionsInput.value = formatTablePageSizeOptions(
    Array.isArray(updated.table_page_size_options)
      ? updated.table_page_size_options
      : [10, 20, 50, 100],
  );
  registrationEmailSuffixWhitelistDraft.value = "";

  form.smtp_password = "";
  smtpPasswordManuallyEdited.value = false;
  form.turnstile_secret_key = "";
  form.recaptcha_secret_key = "";
  form.cap_secret_key = "";
  form.tencent_captcha_app_secret_key = "";
  form.tencent_captcha_cloud_secret_id = "";
  form.tencent_captcha_cloud_secret_key = "";
  form.aliyun_captcha_access_key_secret = "";
  form.linuxdo_connect_client_secret = "";
  form.dingtalk_connect_client_secret = "";
  form.github_oauth_client_secret = "";
  form.google_oauth_client_secret = "";
  form.wechat_connect_app_secret = "";
  form.wechat_connect_open_app_secret = "";
  form.wechat_connect_mp_app_secret = "";
  form.wechat_connect_mobile_app_secret = "";

  const updatedWechatCapabilities = resolveWeChatConnectModeCapabilities(
    updated.wechat_connect_open_enabled,
    updated.wechat_connect_mp_enabled,
    updated.wechat_connect_mobile_enabled,
    updated.wechat_connect_mode,
  );
  form.wechat_connect_open_enabled = updatedWechatCapabilities.openEnabled;
  form.wechat_connect_mp_enabled = updatedWechatCapabilities.mpEnabled;
  form.wechat_connect_mobile_enabled =
    updatedWechatCapabilities.mobileEnabled;
  form.wechat_connect_mode = deriveWeChatConnectStoredMode(
    updatedWechatCapabilities.openEnabled,
    updatedWechatCapabilities.mpEnabled,
    updatedWechatCapabilities.mobileEnabled,
    updated.wechat_connect_mode,
  );
  form.wechat_connect_scopes = defaultWeChatConnectScopesForMode(
    form.wechat_connect_mode,
  );
  form.oidc_connect_client_secret = "";

  if (
    updated.openai_fast_policy_settings &&
    Array.isArray(updated.openai_fast_policy_settings.rules)
  ) {
    openaiFastPolicyForm.rules =
      updated.openai_fast_policy_settings.rules.map((rule) => ({
        ...rule,
        user_ids: rule.user_ids ? [...rule.user_ids] : [],
        model_whitelist: rule.model_whitelist
          ? [...rule.model_whitelist]
          : [],
      }));
    openaiFastPolicyLoaded.value = true;
  }
}
