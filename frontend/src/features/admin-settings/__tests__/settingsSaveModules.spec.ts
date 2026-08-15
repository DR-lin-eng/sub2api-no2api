import { describe, expect, it, vi } from "vitest";
import {
  buildAuthSourceDefaultsState,
  type DefaultSubscriptionSetting,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import { createSettingsForm } from "@/features/admin-settings/presentation/composables/settingsForm";
import { buildSettingsSavePayload } from "@/features/admin-settings/presentation/composables/settingsSavePayload";
import { prepareSettingsSave } from "@/features/admin-settings/presentation/composables/settingsSavePreparation";
import { useSettingsIdentityAccess } from "@/features/admin-settings/presentation/composables/useSettingsIdentityAccess";

function createForm() {
  return createSettingsForm((zh) => zh);
}

function findDuplicateSubscription(
  subscriptions: DefaultSubscriptionSetting[],
): DefaultSubscriptionSetting | undefined {
  const seen = new Set<number>();
  return subscriptions.find((subscription) => {
    if (seen.has(subscription.group_id)) return true;
    seen.add(subscription.group_id);
    return false;
  });
}

describe("settings save preparation", () => {
  it("stops at the first validation error before later normalization", () => {
    const form = createForm();
    form.table_default_page_size = 4;
    const parseTablePageSizeOptionsInput = vi.fn(() => [10, 20]);
    const normalizeLoginAgreementDocumentsForSave = vi.fn(() => []);
    const syncWeChatConnectMode = vi.fn();

    const result = prepareSettingsSave({
      form,
      tablePageSizeOptionsInput: "10, 20",
      authSourceDefaults: buildAuthSourceDefaultsState({}),
      authSourceDefaultsMeta: [],
      parseTablePageSizeOptionsInput,
      normalizeLoginAgreementDocumentsForSave,
      findDuplicateLoginAgreementDocumentId: vi.fn(() => null),
      findDuplicateDefaultSubscription: findDuplicateSubscription,
      syncWeChatConnectMode,
      serializeClaudeOAuthSystemPromptBlocks: vi.fn(() => "[]"),
    });

    expect(result).toEqual({
      ok: false,
      error: { kind: "tableDefaultPageSize" },
    });
    expect(parseTablePageSizeOptionsInput).not.toHaveBeenCalled();
    expect(normalizeLoginAgreementDocumentsForSave).not.toHaveBeenCalled();
    expect(syncWeChatConnectMode).not.toHaveBeenCalled();
  });

  it("normalizes form state in the original validation order", () => {
    const form = createForm();
    form.table_default_page_size = 25.9;
    form.default_subscriptions = [{ group_id: 2.9, validity_days: 30.8 }];
    form.frontend_url = "javascript:invalid";
    form.doc_url = "not-a-url";
    form.wechat_connect_open_enabled = true;
    const authSourceDefaults = buildAuthSourceDefaultsState({});
    authSourceDefaults.email.subscriptions = [
      { group_id: 3.9, validity_days: 60.4 },
    ];
    const syncWeChatConnectMode = vi.fn();
    const serializeClaudeOAuthSystemPromptBlocks = vi.fn(() =>
      JSON.stringify([{ type: "text", text: "system" }]),
    );

    const result = prepareSettingsSave({
      form,
      tablePageSizeOptionsInput: "10, 25",
      authSourceDefaults,
      authSourceDefaultsMeta: [{ source: "email", title: "Email" }],
      parseTablePageSizeOptionsInput: vi.fn(() => [10, 25]),
      normalizeLoginAgreementDocumentsForSave: vi.fn(() => [
        { id: "terms", title: "Terms", content_md: "Body" },
      ]),
      findDuplicateLoginAgreementDocumentId: vi.fn(() => null),
      findDuplicateDefaultSubscription: findDuplicateSubscription,
      syncWeChatConnectMode,
      serializeClaudeOAuthSystemPromptBlocks,
    });

    expect(result).toEqual({
      ok: true,
      normalizedDefaultSubscriptions: [
        { group_id: 2, validity_days: 30 },
      ],
      wechatStoredMode: "open",
      claudeOAuthSystemPromptBlocksJSON:
        '[{"type":"text","text":"system"}]',
    });
    expect(form.table_default_page_size).toBe(25);
    expect(form.table_page_size_options).toEqual([10, 25]);
    expect(form.login_agreement_documents).toEqual([
      { id: "terms", title: "Terms", content_md: "Body" },
    ]);
    expect(authSourceDefaults.email.subscriptions).toEqual([
      { group_id: 3, validity_days: 60 },
    ]);
    expect(form.frontend_url).toBe("");
    expect(form.doc_url).toBe("");
    expect(syncWeChatConnectMode).toHaveBeenCalledTimes(1);
    expect(syncWeChatConnectMode.mock.invocationCallOrder[0]).toBeLessThan(
      serializeClaudeOAuthSystemPromptBlocks.mock.invocationCallOrder[0]!,
    );
  });

  it("returns the first duplicate auth source without touching later sources", () => {
    const form = createForm();
    const authSourceDefaults = buildAuthSourceDefaultsState({});
    authSourceDefaults.email.subscriptions = [
      { group_id: 1, validity_days: 30 },
      { group_id: 1, validity_days: 60 },
    ];
    authSourceDefaults.google.subscriptions = [
      { group_id: 4.9, validity_days: 90.8 },
    ];

    const result = prepareSettingsSave({
      form,
      tablePageSizeOptionsInput: "10, 20",
      authSourceDefaults,
      authSourceDefaultsMeta: [
        { source: "email", title: "Email" },
        { source: "google", title: "Google" },
      ],
      parseTablePageSizeOptionsInput: vi.fn(() => [10, 20]),
      normalizeLoginAgreementDocumentsForSave: vi.fn(
        () => form.login_agreement_documents,
      ),
      findDuplicateLoginAgreementDocumentId: vi.fn(() => null),
      findDuplicateDefaultSubscription: findDuplicateSubscription,
      syncWeChatConnectMode: vi.fn(),
      serializeClaudeOAuthSystemPromptBlocks: vi.fn(() => "[]"),
    });

    expect(result).toEqual({
      ok: false,
      error: {
        kind: "duplicateAuthSourceDefaultSubscription",
        groupId: 1,
        sourceTitle: "Email",
      },
    });
    expect(authSourceDefaults.google.subscriptions).toEqual([
      { group_id: 4.9, validity_days: 90.8 },
    ]);
  });
});

describe("settings save payload", () => {
  it("preserves compatibility fields and omits unloaded fast policy", () => {
    const form = createForm();
    form.balance_low_notify_recharge_url = "";
    form.model_plaza_auto_public_models = true;
    form.media_studio_enabled = true;

    const payload = buildSettingsSavePayload({
      form,
      normalizedDefaultSubscriptions: [],
      registrationEmailSuffixWhitelistTags: [
        "example.com",
        "*.sub.example.com",
      ],
      clientIPTrustedProxies: ["10.0.0.0/8"],
      wechatStoredMode: "open",
      claudeOAuthSystemPromptBlocksJSON: "[]",
      codexFingerprintSignalsJSON: '[{"type":"header_exact"}]',
      codexBlacklistJSON: '[{"originator":"legacy"}]',
      codexWhitelistJSON: '[{"originator":"allowed"}]',
      currentOrigin: "https://admin.example.com",
      openaiFastPolicyLoaded: false,
      openaiFastPolicyRules: [],
      authSourceDefaults: buildAuthSourceDefaultsState({}),
    });

    expect(payload.registration_email_suffix_whitelist).toEqual([
      "@example.com",
      "*.sub.example.com",
    ]);
    expect(payload.client_ip_trusted_proxies).toEqual(["10.0.0.0/8"]);
    expect(payload.codex_cli_only_engine_fingerprint_signals).toBe(
      '[{"type":"header_exact"}]',
    );
    expect(payload.codex_cli_only_blacklist).toBe(
      '[{"originator":"legacy"}]',
    );
    expect(payload.codex_cli_only_whitelist).toBe(
      '[{"originator":"allowed"}]',
    );
    expect(payload).not.toHaveProperty("openai_codex_client_version_synced");
    expect(payload.balance_low_notify_recharge_url).toBe(
      "https://admin.example.com",
    );
    expect(payload.model_plaza_auto_public_models).toBe(true);
    expect(payload.media_studio_enabled).toBe(true);
    expect(form.balance_low_notify_recharge_url).toBe(
      "https://admin.example.com",
    );
    expect(payload).not.toHaveProperty("openai_fast_policy_settings");
    expect(payload).not.toHaveProperty("api_key_acl_trust_forwarded_ip");
    expect(payload).not.toHaveProperty("payment_visible_method_alipay_source");
    expect(payload.default_platform_quotas).toHaveProperty("grok");
    expect(payload.auth_source_default_email_platform_quotas).toHaveProperty(
      "grok",
    );
  });

  it("normalizes fast policy only after it has been loaded", () => {
    const payload = buildSettingsSavePayload({
      form: createForm(),
      normalizedDefaultSubscriptions: [],
      registrationEmailSuffixWhitelistTags: [],
      clientIPTrustedProxies: [],
      wechatStoredMode: "open",
      claudeOAuthSystemPromptBlocksJSON: "[]",
      codexFingerprintSignalsJSON: "",
      codexBlacklistJSON: "",
      codexWhitelistJSON: "",
      currentOrigin: "https://admin.example.com",
      openaiFastPolicyLoaded: true,
      openaiFastPolicyRules: [
        {
          service_tier: "priority",
          action: "block",
          scope: "oauth",
          user_ids: [3],
          error_message: "blocked",
          model_whitelist: [" gpt-5* ", ""],
          fallback_action: "block",
          fallback_error_message: "fallback blocked",
        },
      ],
      authSourceDefaults: buildAuthSourceDefaultsState({}),
    });

    expect(payload.openai_fast_policy_settings).toEqual({
      rules: [
        {
          service_tier: "priority",
          action: "block",
          scope: "oauth",
          user_ids: [3],
          error_message: "blocked",
          model_whitelist: ["gpt-5*"],
          fallback_action: "block",
          fallback_error_message: "fallback blocked",
        },
      ],
    });
  });

  it("includes Tencent credentials only when the administrator entered them", () => {
    const form = createForm();
    form.tencent_captcha_enabled = true;
    form.tencent_captcha_app_id = "123456789";
    form.tencent_captcha_app_secret_key = "app-secret";
    form.tencent_captcha_cloud_secret_id = "cloud-id";
    form.tencent_captcha_cloud_secret_key = "";
    form.tencent_captcha_region = "intl";

    const payload = buildSettingsSavePayload({
      form,
      normalizedDefaultSubscriptions: [],
      registrationEmailSuffixWhitelistTags: [],
      clientIPTrustedProxies: [],
      wechatStoredMode: "open",
      claudeOAuthSystemPromptBlocksJSON: "[]",
      codexFingerprintSignalsJSON: "",
      codexBlacklistJSON: "",
      codexWhitelistJSON: "",
      currentOrigin: "https://admin.example.com",
      openaiFastPolicyLoaded: false,
      openaiFastPolicyRules: [],
      authSourceDefaults: buildAuthSourceDefaultsState({}),
    });

    expect(payload).toMatchObject({
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: "123456789",
      tencent_captcha_app_secret_key: "app-secret",
      tencent_captcha_cloud_secret_id: "cloud-id",
      tencent_captcha_cloud_secret_key: undefined,
      tencent_captcha_region: "intl",
    });
  });

  it("preserves the Alibaba Cloud secret when the administrator leaves it empty", () => {
    const form = createForm();
    form.aliyun_captcha_enabled = true;
    form.aliyun_captcha_access_key_id = "LTAI-test";
    form.aliyun_captcha_access_key_secret = "";
    form.aliyun_captcha_scene_id = "scene-1";
    form.aliyun_captcha_prefix = "tenant-1";
    form.aliyun_captcha_region = "sgp";

    const payload = buildSettingsSavePayload({
      form,
      normalizedDefaultSubscriptions: [],
      registrationEmailSuffixWhitelistTags: [],
      clientIPTrustedProxies: [],
      wechatStoredMode: "open",
      claudeOAuthSystemPromptBlocksJSON: "[]",
      codexFingerprintSignalsJSON: "",
      codexBlacklistJSON: "",
      codexWhitelistJSON: "",
      currentOrigin: "https://admin.example.com",
      openaiFastPolicyLoaded: false,
      openaiFastPolicyRules: [],
      authSourceDefaults: buildAuthSourceDefaultsState({}),
    });

    expect(payload).toMatchObject({
      aliyun_captcha_enabled: true,
      aliyun_captcha_access_key_id: "LTAI-test",
      aliyun_captcha_access_key_secret: undefined,
      aliyun_captcha_scene_id: "scene-1",
      aliyun_captcha_prefix: "tenant-1",
      aliyun_captcha_region: "sgp",
    });
  });
});

describe("human verification settings", () => {
  it("keeps Tencent and Alibaba Cloud mutually exclusive with every provider", () => {
    const form = createForm();
    form.turnstile_enabled = true;
    form.recaptcha_enabled = true;
    form.cap_enabled = true;
    form.aliyun_captcha_enabled = true;
    form.local_captcha_enabled = true;
    const access = useSettingsIdentityAccess(
      form,
      (key) => key,
      (_zh, en) => en,
      vi.fn(async () => true),
    );

    access.setHumanVerificationProvider("tencent_captcha_enabled", true);

    expect(form.tencent_captcha_enabled).toBe(true);
    expect(form.turnstile_enabled).toBe(false);
    expect(form.recaptcha_enabled).toBe(false);
    expect(form.cap_enabled).toBe(false);
    expect(form.aliyun_captcha_enabled).toBe(false);
    expect(form.local_captcha_enabled).toBe(false);

    access.setHumanVerificationProvider("aliyun_captcha_enabled", true);

    expect(form.aliyun_captcha_enabled).toBe(true);
    expect(form.tencent_captcha_enabled).toBe(false);
    expect(form.turnstile_enabled).toBe(false);
    expect(form.recaptcha_enabled).toBe(false);
    expect(form.cap_enabled).toBe(false);
    expect(form.local_captcha_enabled).toBe(false);
  });
});
