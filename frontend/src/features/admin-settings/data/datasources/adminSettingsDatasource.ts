/**
 * Transitional admin settings compatibility facade.
 * New callers should import the DTO, Query, or Action owner directly.
 */

import {
  getAdminApiKey,
  getBetaPolicySettings,
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

export * from "@/features/admin-settings/data/datasources/adminSettingsActions";
export * from "@/features/admin-settings/data/datasources/adminSettingsQueries";
export * from "@/features/admin-settings/data/dtos/adminSettingsDtos";
export * from "@/features/admin-settings/data/dtos/systemSettingsDtos";

export const settingsAPI = {
  getSettings,
  updateSettings,
  testSmtpConnection,
  sendTestEmail,
  getEmailTemplates,
  getEmailTemplate,
  updateEmailTemplate,
  restoreOfficialEmailTemplate,
  previewEmailTemplate,
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
  listAdminApiKeys,
  createAdminApiKey,
  updateAdminApiKey,
  rotateAdminApiKey,
  revokeAdminApiKey,
  getOverloadCooldownSettings,
  updateOverloadCooldownSettings,
  getRateLimit429CooldownSettings,
  updateRateLimit429CooldownSettings,
  getGlobalTempUnschedulableSettings,
  updateGlobalTempUnschedulableSettings,
  getPanelRateLimitSettings,
  updatePanelRateLimitSettings,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getRectifierSettings,
  updateRectifierSettings,
  getBetaPolicySettings,
  updateBetaPolicySettings,
  getWebSearchEmulationConfig,
  updateWebSearchEmulationConfig,
  testWebSearchEmulation,
  resetWebSearchUsage,
};

export default settingsAPI;
