/**
 * Transitional Admin Ops compatibility facade.
 * Runtime consumers should import the explicit query, action, or subscription owner.
 */

import {
  createAlertRule,
  createAlertSilence,
  deleteAlertRule,
  updateAlertEventStatus,
  updateAlertRule
} from '@/features/admin-ops/data/datasources/opsAlertActions'
import {
  getAlertEvent,
  listAlertEvents,
  listAlertRules
} from '@/features/admin-ops/data/datasources/opsAlertQueries'
import {
  getDashboardOverview,
  getDashboardSnapshotV2,
  getErrorDistribution,
  getErrorTrend,
  getLatencyHistogram,
  getSwitchTrend,
  getThroughputTrend
} from '@/features/admin-ops/data/datasources/opsDashboardQueries'
import {
  updateErrorResolved,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved
} from '@/features/admin-ops/data/datasources/opsErrorActions'
import {
  getErrorLogDetail,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  listErrorLogs,
  listRequestErrors,
  listRequestErrorUpstreamErrors,
  listUpstreamErrors
} from '@/features/admin-ops/data/datasources/opsErrorQueries'
import {
  cleanupSystemLogs,
  resetRuntimeLogConfig,
  updateRuntimeLogConfig
} from '@/features/admin-ops/data/datasources/opsLogActions'
import {
  getRuntimeLogConfig,
  getSystemLogSinkHealth,
  listRequestDetails,
  listSystemLogs
} from '@/features/admin-ops/data/datasources/opsLogQueries'
import {
  getAccountAvailabilityStats,
  getConcurrencySnapshot,
  getConcurrencyStats,
  getImageGenerationStats,
  getOpenAITokenStats,
  getRealtimeTrafficSummary,
  getUserConcurrencyStats,
  getUserUsageStats
} from '@/features/admin-ops/data/datasources/opsMetricsQueries'
import { subscribeQPS } from '@/features/admin-ops/data/datasources/opsRealtimeSubscription'
import {
  updateAdvancedSettings,
  updateAlertRuntimeSettings,
  updateEmailNotificationConfig,
  updateMetricThresholds
} from '@/features/admin-ops/data/datasources/opsSettingsActions'
import {
  getAdvancedSettings,
  getAlertRuntimeSettings,
  getEmailNotificationConfig,
  getMetricThresholds,
  getSettingsSnapshot
} from '@/features/admin-ops/data/datasources/opsSettingsQueries'

export * from '@/features/admin-ops/data/datasources/opsAlertActions'
export * from '@/features/admin-ops/data/datasources/opsAlertQueries'
export * from '@/features/admin-ops/data/datasources/opsDashboardQueries'
export * from '@/features/admin-ops/data/datasources/opsErrorActions'
export * from '@/features/admin-ops/data/datasources/opsErrorQueries'
export * from '@/features/admin-ops/data/datasources/opsLogActions'
export * from '@/features/admin-ops/data/datasources/opsLogQueries'
export * from '@/features/admin-ops/data/datasources/opsMetricsQueries'
export * from '@/features/admin-ops/data/datasources/opsRealtimeSubscription'
export * from '@/features/admin-ops/data/datasources/opsSettingsActions'
export * from '@/features/admin-ops/data/datasources/opsSettingsQueries'
export * from '@/features/admin-ops/data/dtos/opsAlertDtos'
export * from '@/features/admin-ops/data/dtos/opsDashboardDtos'
export * from '@/features/admin-ops/data/dtos/opsErrorDtos'
export * from '@/features/admin-ops/data/dtos/opsLogDtos'
export * from '@/features/admin-ops/data/dtos/opsMetricsDtos'
export * from '@/features/admin-ops/data/dtos/opsSettingsDtos'

export const opsAPI = {
  getDashboardSnapshotV2,
  getDashboardOverview,
  getThroughputTrend,
  getSwitchTrend,
  getLatencyHistogram,
  getErrorTrend,
  getErrorDistribution,
  getImageGenerationStats,
  getOpenAITokenStats,
  getUserUsageStats,
  getConcurrencyStats,
  getConcurrencySnapshot,
  getUserConcurrencyStats,
  getAccountAvailabilityStats,
  getRealtimeTrafficSummary,
  subscribeQPS,

  listErrorLogs,
  getErrorLogDetail,
  updateErrorResolved,

  listRequestErrors,
  listUpstreamErrors,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved,
  listRequestErrorUpstreamErrors,

  listRequestDetails,
  listAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  listAlertEvents,
  getAlertEvent,
  updateAlertEventStatus,
  createAlertSilence,
  getSettingsSnapshot,
  getEmailNotificationConfig,
  updateEmailNotificationConfig,
  getAlertRuntimeSettings,
  updateAlertRuntimeSettings,
  getRuntimeLogConfig,
  updateRuntimeLogConfig,
  resetRuntimeLogConfig,
  getAdvancedSettings,
  updateAdvancedSettings,
  getMetricThresholds,
  updateMetricThresholds,
  listSystemLogs,
  cleanupSystemLogs,
  getSystemLogSinkHealth
}

export default opsAPI
