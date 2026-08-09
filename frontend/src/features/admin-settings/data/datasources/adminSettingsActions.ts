import { apiClient } from "@/core/networks/client";
import {
  normalizePanelRateLimitSettings,
  type AdminApiKey,
  type BetaPolicySettings,
  type CreateAdminApiKeyRequest,
  type EmailTemplateDetail,
  type EmailTemplatePreviewResponse,
  type GlobalTempUnschedulableSettings,
  type OverloadCooldownSettings,
  type PanelRateLimitSettings,
  type PreviewEmailTemplateRequest,
  type RateLimit429CooldownSettings,
  type RectifierSettings,
  type SendTestEmailRequest,
  type StreamTimeoutSettings,
  type TestSmtpRequest,
  type UpdateEmailTemplateRequest,
  type UpdateAdminApiKeyRequest,
  type WebSearchEmulationConfig,
  type WebSearchTestResult,
} from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import type {
  SystemSettings,
  UpdateSettingsRequest,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";

export async function updateSettings(
  settings: UpdateSettingsRequest,
): Promise<SystemSettings> {
  const { data } = await apiClient.put<SystemSettings>(
    "/admin/settings",
    settings,
  );
  return data;
}

export async function updateEmailTemplate(
  event: string,
  locale: string,
  request: UpdateEmailTemplateRequest,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.put<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}`,
    request,
  );
  return data;
}

export async function restoreOfficialEmailTemplate(
  event: string,
  locale: string,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.post<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}/restore-official`,
  );
  return data;
}

export async function previewEmailTemplate(
  request: PreviewEmailTemplateRequest,
): Promise<EmailTemplatePreviewResponse> {
  const { data } = await apiClient.post<EmailTemplatePreviewResponse>(
    "/admin/settings/email-template-preview",
    request,
  );
  return data;
}

export async function updatePanelRateLimitSettings(
  settings: PanelRateLimitSettings,
): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.put<unknown>(
    "/admin/settings/panel-rate-limit",
    settings,
  );
  return normalizePanelRateLimitSettings(data);
}

export async function testSmtpConnection(
  config: TestSmtpRequest,
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    "/admin/settings/test-smtp",
    config,
  );
  return data;
}

export async function sendTestEmail(
  request: SendTestEmailRequest,
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    "/admin/settings/send-test-email",
    request,
  );
  return data;
}

export async function createAdminApiKey(
  request: CreateAdminApiKeyRequest,
): Promise<{ key: string; metadata: AdminApiKey }> {
  const { data } = await apiClient.post<{ key: string; metadata: AdminApiKey }>(
    "/admin/settings/admin-api-keys",
    request,
  );
  return data;
}

export async function updateAdminApiKey(
  id: string,
  request: UpdateAdminApiKeyRequest,
): Promise<AdminApiKey> {
  const { data } = await apiClient.put<AdminApiKey>(
    `/admin/settings/admin-api-keys/${encodeURIComponent(id)}`,
    request,
  );
  return data;
}

export async function rotateAdminApiKey(
  id: string,
): Promise<{ key: string; metadata: AdminApiKey }> {
  const { data } = await apiClient.post<{ key: string; metadata: AdminApiKey }>(
    `/admin/settings/admin-api-keys/${encodeURIComponent(id)}/rotate`,
  );
  return data;
}

export async function revokeAdminApiKey(
  id: string,
): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/settings/admin-api-keys/${encodeURIComponent(id)}`,
  );
  return data;
}

export async function regenerateAdminApiKey(): Promise<{ key: string }> {
  const { data } = await apiClient.post<{ key: string }>(
    "/admin/settings/admin-api-key/regenerate",
  );
  return data;
}

export async function deleteAdminApiKey(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    "/admin/settings/admin-api-key",
  );
  return data;
}

export async function updateOverloadCooldownSettings(
  settings: OverloadCooldownSettings,
): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.put<OverloadCooldownSettings>(
    "/admin/settings/overload-cooldown",
    settings,
  );
  return data;
}

export async function updateRateLimit429CooldownSettings(
  settings: RateLimit429CooldownSettings,
): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.put<RateLimit429CooldownSettings>(
    "/admin/settings/rate-limit-429-cooldown",
    settings,
  );
  return data;
}

export async function updateGlobalTempUnschedulableSettings(
  settings: GlobalTempUnschedulableSettings,
): Promise<GlobalTempUnschedulableSettings> {
  const { data } = await apiClient.put<GlobalTempUnschedulableSettings>(
    "/admin/settings/temp-unschedulable",
    settings,
  );
  return data;
}

export async function updateStreamTimeoutSettings(
  settings: StreamTimeoutSettings,
): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.put<StreamTimeoutSettings>(
    "/admin/settings/stream-timeout",
    settings,
  );
  return data;
}

export async function updateRectifierSettings(
  settings: RectifierSettings,
): Promise<RectifierSettings> {
  const { data } = await apiClient.put<RectifierSettings>(
    "/admin/settings/rectifier",
    settings,
  );
  return data;
}

export async function updateBetaPolicySettings(
  settings: BetaPolicySettings,
): Promise<BetaPolicySettings> {
  const { data } = await apiClient.put<BetaPolicySettings>(
    "/admin/settings/beta-policy",
    settings,
  );
  return data;
}

export async function updateWebSearchEmulationConfig(
  config: WebSearchEmulationConfig,
): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.put<WebSearchEmulationConfig>(
    "/admin/settings/web-search-emulation",
    config,
  );
  return data;
}

export async function testWebSearchEmulation(
  query: string,
): Promise<WebSearchTestResult> {
  const { data } = await apiClient.post<WebSearchTestResult>(
    "/admin/settings/web-search-emulation/test",
    { query },
  );
  return data;
}

export async function resetWebSearchUsage(payload: {
  provider_type: string;
}): Promise<void> {
  await apiClient.post(
    "/admin/settings/web-search-emulation/reset-usage",
    payload,
  );
}
