import { apiClient } from "@/core/networks/client";
import {
  normalizePanelRateLimitSettings,
  type AdminApiKey,
  type AdminApiKeyStatus,
  type BetaPolicySettings,
  type EmailTemplateDetail,
  type EmailTemplateListResponse,
  type GlobalTempUnschedulableSettings,
  type OverloadCooldownSettings,
  type PanelRateLimitSettings,
  type RateLimit429CooldownSettings,
  type RectifierSettings,
  type StreamTimeoutSettings,
  type WebSearchEmulationConfig,
} from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import type { SystemSettings } from "@/features/admin-settings/data/dtos/systemSettingsDtos";

export async function getSettings(): Promise<SystemSettings> {
  const { data } = await apiClient.get<SystemSettings>("/admin/settings");
  return data;
}

export async function getEmailTemplates(): Promise<EmailTemplateListResponse> {
  const { data } = await apiClient.get<EmailTemplateListResponse>(
    "/admin/settings/email-templates",
  );
  return data;
}

export async function getEmailTemplate(
  event: string,
  locale: string,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.get<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}`,
  );
  return data;
}

export async function getPanelRateLimitSettings(): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.get<unknown>(
    "/admin/settings/panel-rate-limit",
  );
  return normalizePanelRateLimitSettings(data);
}

export async function listAdminApiKeys(): Promise<{ items: AdminApiKey[] }> {
  const { data } = await apiClient.get<{ items: AdminApiKey[] }>(
    "/admin/settings/admin-api-keys",
  );
  return data;
}

export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data } = await apiClient.get<AdminApiKeyStatus>(
    "/admin/settings/admin-api-key",
  );
  return data;
}

export async function getOverloadCooldownSettings(): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.get<OverloadCooldownSettings>(
    "/admin/settings/overload-cooldown",
  );
  return data;
}

export async function getRateLimit429CooldownSettings(): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.get<RateLimit429CooldownSettings>(
    "/admin/settings/rate-limit-429-cooldown",
  );
  return data;
}

export async function getGlobalTempUnschedulableSettings(): Promise<GlobalTempUnschedulableSettings> {
  const { data } = await apiClient.get<GlobalTempUnschedulableSettings>(
    "/admin/settings/temp-unschedulable",
  );
  return data;
}

export async function getStreamTimeoutSettings(): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.get<StreamTimeoutSettings>(
    "/admin/settings/stream-timeout",
  );
  return data;
}

export async function getRectifierSettings(): Promise<RectifierSettings> {
  const { data } = await apiClient.get<RectifierSettings>(
    "/admin/settings/rectifier",
  );
  return data;
}

export async function getBetaPolicySettings(): Promise<BetaPolicySettings> {
  const { data } = await apiClient.get<BetaPolicySettings>(
    "/admin/settings/beta-policy",
  );
  return data;
}

export async function getWebSearchEmulationConfig(): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.get<WebSearchEmulationConfig>(
    "/admin/settings/web-search-emulation",
  );
  return data;
}
