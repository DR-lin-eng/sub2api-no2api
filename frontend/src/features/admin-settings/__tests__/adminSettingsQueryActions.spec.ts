import { beforeEach, describe, expect, it, vi } from "vitest";

const { del, get, post, put } = vi.hoisted(() => ({
  del: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}));

vi.mock("@/core/networks/client", () => ({
  apiClient: { delete: del, get, post, put },
}));

import settingsAPI, {
  DEFAULT_PANEL_RATE_LIMIT_SETTINGS as legacyPanelDefaults,
  buildAuthSourceDefaultsState as legacyBuildAuthSourceDefaultsState,
  normalizePanelRateLimitSettings as legacyNormalizePanelSettings,
} from "@/features/admin-settings/data/datasources/adminSettingsDatasource";
import {
  getAdminApiKey,
  getBetaPolicySettings,
  getCodexSimulationSettings,
  getEmailTemplate,
  getEmailTemplates,
  getGlobalTempUnschedulableSettings,
  getOverloadCooldownSettings,
  getPanelRateLimitSettings,
  getRateLimit429CooldownSettings,
  getRectifierSettings,
  getSettings,
  getStreamTimeoutSettings,
  getWebSearchEmulationConfig,
  listAdminApiKeys,
} from "@/features/admin-settings/data/datasources/adminSettingsQueries";
import {
  createAdminApiKey,
  deleteAdminApiKey,
  forceDisableCodexSimulationSettings,
  previewEmailTemplate,
  regenerateAdminApiKey,
  resetWebSearchUsage,
  restoreOfficialEmailTemplate,
  revokeAdminApiKey,
  rotateAdminApiKey,
  sendTestEmail,
  testSmtpConnection,
  testWebSearchEmulation,
  updateAdminApiKey,
  updateBetaPolicySettings,
  updateCodexSimulationSettings,
  updateEmailTemplate,
  updateGlobalTempUnschedulableSettings,
  updateOverloadCooldownSettings,
  updatePanelRateLimitSettings,
  updateRateLimit429CooldownSettings,
  updateRectifierSettings,
  updateSettings,
  updateStreamTimeoutSettings,
  updateWebSearchEmulationConfig,
} from "@/features/admin-settings/data/datasources/adminSettingsActions";
import {
  DEFAULT_PANEL_RATE_LIMIT_SETTINGS,
  normalizePanelRateLimitSettings,
} from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import { buildAuthSourceDefaultsState } from "@/features/admin-settings/data/dtos/systemSettingsDtos";

describe("admin settings query and action owners", () => {
  beforeEach(() => {
    del.mockReset();
    get.mockReset();
    post.mockReset();
    put.mockReset();
  });

  it("keeps the transitional settings facade on the owner function identities", () => {
    expect(settingsAPI.getSettings).toBe(getSettings);
    expect(settingsAPI.updateSettings).toBe(updateSettings);
    expect(settingsAPI.getEmailTemplates).toBe(getEmailTemplates);
    expect(settingsAPI.getEmailTemplate).toBe(getEmailTemplate);
    expect(settingsAPI.updateEmailTemplate).toBe(updateEmailTemplate);
    expect(settingsAPI.restoreOfficialEmailTemplate).toBe(
      restoreOfficialEmailTemplate,
    );
    expect(settingsAPI.previewEmailTemplate).toBe(previewEmailTemplate);
    expect(settingsAPI.getPanelRateLimitSettings).toBe(
      getPanelRateLimitSettings,
    );
    expect(settingsAPI.updatePanelRateLimitSettings).toBe(
      updatePanelRateLimitSettings,
    );
    expect(legacyPanelDefaults).toBe(DEFAULT_PANEL_RATE_LIMIT_SETTINGS);
    expect(legacyNormalizePanelSettings).toBe(normalizePanelRateLimitSettings);
    expect(legacyBuildAuthSourceDefaultsState).toBe(
      buildAuthSourceDefaultsState,
    );
  });

  it("preserves the unified settings query and update contract", async () => {
    const response = { site_name: "Sub2API", registration_enabled: true };
    const update = { site_name: "Sub2API" };
    get.mockResolvedValueOnce({ data: response });
    put.mockResolvedValueOnce({ data: response });

    await expect(getSettings()).resolves.toBe(response);
    await expect(updateSettings(update)).resolves.toBe(response);

    expect(get).toHaveBeenCalledWith("/admin/settings");
    expect(put).toHaveBeenCalledWith("/admin/settings", update);
  });

  it("keeps the remaining migrated facade methods on owner identities", () => {
    expect(settingsAPI.listAdminApiKeys).toBe(listAdminApiKeys);
    expect(settingsAPI.getAdminApiKey).toBe(getAdminApiKey);
    expect(settingsAPI.createAdminApiKey).toBe(createAdminApiKey);
    expect(settingsAPI.updateAdminApiKey).toBe(updateAdminApiKey);
    expect(settingsAPI.rotateAdminApiKey).toBe(rotateAdminApiKey);
    expect(settingsAPI.revokeAdminApiKey).toBe(revokeAdminApiKey);
    expect(settingsAPI.regenerateAdminApiKey).toBe(regenerateAdminApiKey);
    expect(settingsAPI.deleteAdminApiKey).toBe(deleteAdminApiKey);
    expect(settingsAPI.getOverloadCooldownSettings).toBe(
      getOverloadCooldownSettings,
    );
    expect(settingsAPI.updateOverloadCooldownSettings).toBe(
      updateOverloadCooldownSettings,
    );
    expect(settingsAPI.getRateLimit429CooldownSettings).toBe(
      getRateLimit429CooldownSettings,
    );
    expect(settingsAPI.updateRateLimit429CooldownSettings).toBe(
      updateRateLimit429CooldownSettings,
    );
    expect(settingsAPI.getGlobalTempUnschedulableSettings).toBe(
      getGlobalTempUnschedulableSettings,
    );
    expect(settingsAPI.updateGlobalTempUnschedulableSettings).toBe(
      updateGlobalTempUnschedulableSettings,
    );
    expect(settingsAPI.getStreamTimeoutSettings).toBe(getStreamTimeoutSettings);
    expect(settingsAPI.updateStreamTimeoutSettings).toBe(
      updateStreamTimeoutSettings,
    );
    expect(settingsAPI.getRectifierSettings).toBe(getRectifierSettings);
    expect(settingsAPI.updateRectifierSettings).toBe(updateRectifierSettings);
    expect(settingsAPI.getBetaPolicySettings).toBe(getBetaPolicySettings);
    expect(settingsAPI.updateBetaPolicySettings).toBe(
      updateBetaPolicySettings,
    );
    expect(settingsAPI.getWebSearchEmulationConfig).toBe(
      getWebSearchEmulationConfig,
    );
    expect(settingsAPI.updateWebSearchEmulationConfig).toBe(
      updateWebSearchEmulationConfig,
    );
    expect(settingsAPI.testWebSearchEmulation).toBe(testWebSearchEmulation);
    expect(settingsAPI.resetWebSearchUsage).toBe(resetWebSearchUsage);
    expect(settingsAPI.testSmtpConnection).toBe(testSmtpConnection);
    expect(settingsAPI.sendTestEmail).toBe(sendTestEmail);
  });

  it("preserves email template query paths and segment encoding", async () => {
    const listResponse = {
      events: ["password/reset"],
      locales: ["zh-CN"],
    };
    const detailResponse = {
      event: "password/reset",
      locale: "zh-CN",
      subject: "subject",
      html: "<p>body</p>",
    };
    get
      .mockResolvedValueOnce({ data: listResponse })
      .mockResolvedValueOnce({ data: detailResponse });

    await expect(getEmailTemplates()).resolves.toEqual(listResponse);
    await expect(
      getEmailTemplate("password/reset", "zh-CN"),
    ).resolves.toEqual(detailResponse);

    expect(get).toHaveBeenNthCalledWith(
      1,
      "/admin/settings/email-templates",
    );
    expect(get).toHaveBeenNthCalledWith(
      2,
      "/admin/settings/email-templates/password%2Freset/zh-CN",
    );
  });

  it("preserves email template write and preview payloads", async () => {
    const template = {
      event: "welcome",
      locale: "en",
      subject: "Welcome",
      html: "<p>Welcome</p>",
    };
    const update = { subject: template.subject, html: template.html };
    put.mockResolvedValueOnce({ data: template });
    post
      .mockResolvedValueOnce({ data: template })
      .mockResolvedValueOnce({ data: update });

    await expect(updateEmailTemplate("welcome", "en", update)).resolves.toEqual(
      template,
    );
    await expect(
      restoreOfficialEmailTemplate("welcome", "en"),
    ).resolves.toEqual(template);
    await expect(
      previewEmailTemplate({ event: "welcome", locale: "en", ...update }),
    ).resolves.toEqual(update);

    expect(put).toHaveBeenCalledWith(
      "/admin/settings/email-templates/welcome/en",
      update,
    );
    expect(post).toHaveBeenNthCalledWith(
      1,
      "/admin/settings/email-templates/welcome/en/restore-official",
    );
    expect(post).toHaveBeenNthCalledWith(
      2,
      "/admin/settings/email-template-preview",
      { event: "welcome", locale: "en", ...update },
    );
  });

  it("keeps panel rate-limit compatibility defaults on reads and writes", async () => {
    const partialResponse = {
      enabled: true,
      user_rpm: 120,
      heavy_rpm: -1,
      exempt_admin: false,
    };
    get.mockResolvedValueOnce({ data: partialResponse });
    put.mockResolvedValueOnce({ data: null });

    await expect(getPanelRateLimitSettings()).resolves.toEqual({
      enabled: true,
      user_rpm: 120,
      heavy_rpm: 60,
      exempt_admin: false,
      public_ip_rpm: 300,
    });
    await expect(
      updatePanelRateLimitSettings({
        enabled: true,
        user_rpm: 120,
        heavy_rpm: 60,
        exempt_admin: false,
        public_ip_rpm: 300,
      }),
    ).resolves.toEqual(DEFAULT_PANEL_RATE_LIMIT_SETTINGS);

    expect(get).toHaveBeenCalledWith("/admin/settings/panel-rate-limit");
    expect(put).toHaveBeenCalledWith("/admin/settings/panel-rate-limit", {
      enabled: true,
      user_rpm: 120,
      heavy_rpm: 60,
      exempt_admin: false,
      public_ip_rpm: 300,
    });
  });

  it("preserves independent settings query paths", async () => {
    const responses = [
      { items: [] },
      { exists: true, masked_key: "sk-admin...1234" },
      { enabled: true, cooldown_minutes: 10 },
      { enabled: true, cooldown_seconds: 5 },
      { enabled: true },
      {
        response_header_timeout_degradation_enabled: true,
        response_header_timeout_seconds: 20,
        enabled: true,
        action: "temp_unsched",
        temp_unsched_minutes: 5,
        threshold_count: 3,
        threshold_window_minutes: 10,
        openai_first_output_timeout_seconds: 90,
        openai_high_effort_first_output_timeout_seconds: 180,
        stream_keepalive_interval_seconds: 10,
      },
      {
        enabled: true,
        thinking_signature_enabled: true,
        thinking_budget_enabled: true,
        thinking_display_mode: "display_only",
        apikey_signature_enabled: false,
        apikey_signature_patterns: [],
      },
      { rules: [] },
      { enabled: false, providers: [] },
    ];
    for (const response of responses) {
      get.mockResolvedValueOnce({ data: response });
    }

    await listAdminApiKeys();
    await getAdminApiKey();
    await getOverloadCooldownSettings();
    await getRateLimit429CooldownSettings();
    await getGlobalTempUnschedulableSettings();
    await getStreamTimeoutSettings();
    await getRectifierSettings();
    await getBetaPolicySettings();
    await getWebSearchEmulationConfig();

    expect(get.mock.calls.map(([path]) => path)).toEqual([
      "/admin/settings/admin-api-keys",
      "/admin/settings/admin-api-key",
      "/admin/settings/overload-cooldown",
      "/admin/settings/rate-limit-429-cooldown",
      "/admin/settings/temp-unschedulable",
      "/admin/settings/stream-timeout",
      "/admin/settings/rectifier",
      "/admin/settings/beta-policy",
      "/admin/settings/web-search-emulation",
    ]);
  });

  it("uses the dedicated Codex simulation runtime settings contract", async () => {
    const response = {
      full_simulation_enabled: true,
      continuation_mode: "shadow" as const,
      state_ttl_seconds: 604800,
      identity_secret_configured: true,
    };
    const payload = {
      full_simulation_enabled: false,
      continuation_mode: "off" as const,
      state_ttl_seconds: 604800,
    };
    get.mockResolvedValueOnce({ data: response });
    put.mockResolvedValueOnce({ data: { ...response, ...payload } });
    post.mockResolvedValueOnce({
      data: {
        ...response,
        full_simulation_enabled: false,
        continuation_mode: "off",
      },
    });

    await expect(getCodexSimulationSettings()).resolves.toEqual(response);
    await expect(updateCodexSimulationSettings(payload)).resolves.toEqual({
      ...response,
      ...payload,
    });
    await expect(forceDisableCodexSimulationSettings()).resolves.toEqual({
      ...response,
      full_simulation_enabled: false,
      continuation_mode: "off",
    });

    expect(get).toHaveBeenCalledWith("/admin/settings/codex-simulation");
    expect(put).toHaveBeenCalledWith(
      "/admin/settings/codex-simulation",
      payload,
    );
    expect(post).toHaveBeenCalledWith(
      "/admin/settings/codex-simulation/restore-original",
    );
  });

  it("preserves admin API key payloads and identifier encoding", async () => {
    const key = {
      id: "key/one",
      name: "ops",
      key_prefix: "sk-admin",
      last_four: "1234",
      scopes: ["admin.read"],
      status: "active",
      created_by: 1,
      created_at: "2026-08-09T00:00:00Z",
      updated_at: "2026-08-09T00:00:00Z",
    };
    const createRequest = {
      name: "ops",
      scopes: ["admin.read" as const],
      expires_at: null,
    };
    const updateRequest = { name: "ops-readonly" };
    post
      .mockResolvedValueOnce({ data: { key: "created", metadata: key } })
      .mockResolvedValueOnce({ data: { key: "rotated", metadata: key } })
      .mockResolvedValueOnce({ data: { key: "legacy" } });
    put.mockResolvedValueOnce({ data: key });
    del
      .mockResolvedValueOnce({ data: { message: "revoked" } })
      .mockResolvedValueOnce({ data: { message: "deleted" } });

    await createAdminApiKey(createRequest);
    await updateAdminApiKey("key/one", updateRequest);
    await rotateAdminApiKey("key/one");
    await revokeAdminApiKey("key/one");
    await regenerateAdminApiKey();
    await deleteAdminApiKey();

    expect(post).toHaveBeenNthCalledWith(
      1,
      "/admin/settings/admin-api-keys",
      createRequest,
    );
    expect(put).toHaveBeenCalledWith(
      "/admin/settings/admin-api-keys/key%2Fone",
      updateRequest,
    );
    expect(post).toHaveBeenNthCalledWith(
      2,
      "/admin/settings/admin-api-keys/key%2Fone/rotate",
    );
    expect(del).toHaveBeenNthCalledWith(
      1,
      "/admin/settings/admin-api-keys/key%2Fone",
    );
    expect(post).toHaveBeenNthCalledWith(
      3,
      "/admin/settings/admin-api-key/regenerate",
    );
    expect(del).toHaveBeenNthCalledWith(
      2,
      "/admin/settings/admin-api-key",
    );
  });

  it("preserves SMTP and Web Search action payloads", async () => {
    const smtp = {
      smtp_host: "smtp.example.com",
      smtp_port: 587,
      smtp_username: "mailer",
      smtp_password: "secret",
      smtp_use_tls: true,
    };
    const email = {
      ...smtp,
      email: "ops@example.com",
      smtp_from_email: "noreply@example.com",
      smtp_from_name: "Sub2API",
    };
    const webSearch = { enabled: false, providers: [] };
    post
      .mockResolvedValueOnce({ data: { message: "connected" } })
      .mockResolvedValueOnce({ data: { message: "sent" } })
      .mockResolvedValueOnce({
        data: { provider: "brave", results: [], query: "codex" },
      })
      .mockResolvedValueOnce({ data: undefined });
    put.mockResolvedValueOnce({ data: webSearch });

    await testSmtpConnection(smtp);
    await sendTestEmail(email);
    await updateWebSearchEmulationConfig(webSearch);
    await testWebSearchEmulation("codex");
    await expect(
      resetWebSearchUsage({ provider_type: "brave" }),
    ).resolves.toBeUndefined();

    expect(post).toHaveBeenNthCalledWith(
      1,
      "/admin/settings/test-smtp",
      smtp,
    );
    expect(post).toHaveBeenNthCalledWith(
      2,
      "/admin/settings/send-test-email",
      email,
    );
    expect(put).toHaveBeenCalledWith(
      "/admin/settings/web-search-emulation",
      webSearch,
    );
    expect(post).toHaveBeenNthCalledWith(
      3,
      "/admin/settings/web-search-emulation/test",
      { query: "codex" },
    );
    expect(post).toHaveBeenNthCalledWith(
      4,
      "/admin/settings/web-search-emulation/reset-usage",
      { provider_type: "brave" },
    );
  });

  it("preserves gateway policy update paths and payload identities", async () => {
    const cases = [
      [
        updateOverloadCooldownSettings,
        "/admin/settings/overload-cooldown",
        { enabled: true, cooldown_minutes: 10 },
      ],
      [
        updateRateLimit429CooldownSettings,
        "/admin/settings/rate-limit-429-cooldown",
        { enabled: true, cooldown_seconds: 5 },
      ],
      [
        updateGlobalTempUnschedulableSettings,
        "/admin/settings/temp-unschedulable",
        { enabled: true },
      ],
      [
        updateStreamTimeoutSettings,
        "/admin/settings/stream-timeout",
        {
          response_header_timeout_degradation_enabled: true,
          response_header_timeout_seconds: 20,
          enabled: true,
          action: "temp_unsched",
          temp_unsched_minutes: 5,
          threshold_count: 3,
          threshold_window_minutes: 10,
          openai_first_output_timeout_seconds: 90,
          openai_high_effort_first_output_timeout_seconds: 180,
          stream_keepalive_interval_seconds: 10,
        },
      ],
      [
        updateRectifierSettings,
        "/admin/settings/rectifier",
        {
          enabled: true,
          thinking_signature_enabled: true,
          thinking_budget_enabled: true,
          thinking_display_mode: "display_only",
          apikey_signature_enabled: false,
          apikey_signature_patterns: [],
        },
      ],
      [
        updateBetaPolicySettings,
        "/admin/settings/beta-policy",
        { rules: [] },
      ],
    ] as const;

    for (const [action, , payload] of cases) {
      put.mockResolvedValueOnce({ data: payload });
      await action(payload as never);
    }

    expect(put.mock.calls).toEqual(
      cases.map(([, path, payload]) => [path, payload]),
    );
  });
});
