import { apiClient } from '@/core/networks/client'
import type {
  EmailNotificationConfig,
  OpsAdvancedSettings,
  OpsAlertRuntimeSettings,
  OpsMetricThresholds
} from '@/features/admin-ops/data/dtos/opsSettingsDtos'

export async function updateEmailNotificationConfig(
  config: EmailNotificationConfig
): Promise<EmailNotificationConfig> {
  const { data } = await apiClient.put<EmailNotificationConfig>(
    '/admin/ops/email-notification/config',
    config
  )
  return data
}

export async function updateAlertRuntimeSettings(
  config: OpsAlertRuntimeSettings
): Promise<OpsAlertRuntimeSettings> {
  const { data } = await apiClient.put<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert', config)
  return data
}

export async function updateAdvancedSettings(
  config: OpsAdvancedSettings
): Promise<OpsAdvancedSettings> {
  const { data } = await apiClient.put<OpsAdvancedSettings>('/admin/ops/advanced-settings', config)
  return data
}

export async function updateMetricThresholds(thresholds: OpsMetricThresholds): Promise<void> {
  await apiClient.put('/admin/ops/settings/metric-thresholds', thresholds)
}
