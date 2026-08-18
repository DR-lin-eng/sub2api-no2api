import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const featureDir = resolve(currentDir, '..')
const readFeatureSource = (relativePath: string) => readFileSync(resolve(featureDir, relativePath), 'utf8')

function collectRuntimeSources(directory: string): Array<{ path: string, source: string }> {
  const sources: Array<{ path: string, source: string }> = []
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.name === '__tests__') continue
    const absolutePath = join(directory, entry.name)
    if (entry.isDirectory()) {
      sources.push(...collectRuntimeSources(absolutePath))
      continue
    }
    if (!new Set(['.ts', '.vue']).has(extname(entry.name))) continue
    sources.push({
      path: relative(featureDir, absolutePath),
      source: readFileSync(absolutePath, 'utf8')
    })
  }
  return sources
}

const facadeSource = readFeatureSource('data/datasources/adminOpsDatasource.ts')
const alertActionSource = readFeatureSource('data/datasources/opsAlertActions.ts')
const alertQuerySource = readFeatureSource('data/datasources/opsAlertQueries.ts')
const dashboardQuerySource = readFeatureSource('data/datasources/opsDashboardQueries.ts')
const errorActionSource = readFeatureSource('data/datasources/opsErrorActions.ts')
const errorQuerySource = readFeatureSource('data/datasources/opsErrorQueries.ts')
const logActionSource = readFeatureSource('data/datasources/opsLogActions.ts')
const logQuerySource = readFeatureSource('data/datasources/opsLogQueries.ts')
const metricsQuerySource = readFeatureSource('data/datasources/opsMetricsQueries.ts')
const realtimeSubscriptionSource = readFeatureSource('data/datasources/opsRealtimeSubscription.ts')
const settingsActionSource = readFeatureSource('data/datasources/opsSettingsActions.ts')
const settingsQuerySource = readFeatureSource('data/datasources/opsSettingsQueries.ts')
const alertDtoSource = readFeatureSource('data/dtos/opsAlertDtos.ts')
const dashboardDtoSource = readFeatureSource('data/dtos/opsDashboardDtos.ts')
const errorDtoSource = readFeatureSource('data/dtos/opsErrorDtos.ts')
const logDtoSource = readFeatureSource('data/dtos/opsLogDtos.ts')
const metricsDtoSource = readFeatureSource('data/dtos/opsMetricsDtos.ts')
const settingsDtoSource = readFeatureSource('data/dtos/opsSettingsDtos.ts')

describe('admin ops modularization', () => {
  it('owns all ops contracts outside the compatibility datasource', () => {
    expect(alertDtoSource).toContain('export interface AlertRule')
    expect(alertDtoSource).toContain('export interface AlertEvent')
    expect(alertDtoSource).toContain('export interface AlertSilenceRequest')
    expect(dashboardDtoSource).toContain('export interface OpsDashboardOverview')
    expect(dashboardDtoSource).toContain('export interface OpsDashboardSnapshotParams')
    expect(metricsDtoSource).toContain('export interface OpsConcurrencySnapshotResponse')
    expect(metricsDtoSource).toContain('export interface OpsUserUsageStatsResponse')
    expect(logDtoSource).toContain('export interface OpsRequestDetailsParams')
    expect(logDtoSource).toContain('export interface OpsRuntimeLogConfig')
    expect(logDtoSource).toContain('export interface OpsSystemLogCleanupRequest')
    expect(errorDtoSource).toContain('export interface OpsErrorLog')
    expect(errorDtoSource).toContain('export interface OpsErrorDetail')
    expect(errorDtoSource).toContain('export interface OpsErrorListQueryParams')
    expect(settingsDtoSource).toContain('export interface OpsSettingsSnapshot')
    expect(settingsDtoSource).toContain('export interface OpsAlertRuntimeSettings')
    expect(settingsDtoSource).toContain('export interface OpsAdvancedSettings')
    expect(alertDtoSource).not.toContain('apiClient')
    expect(dashboardDtoSource).not.toContain('apiClient')
    expect(errorDtoSource).not.toContain('apiClient')
    expect(logDtoSource).not.toContain('apiClient')
    expect(metricsDtoSource).not.toContain('apiClient')
    expect(settingsDtoSource).not.toContain('apiClient')
    expect(facadeSource).not.toContain('export interface AlertRule')
    expect(facadeSource).not.toContain('export interface OpsDashboardOverview')
    expect(facadeSource).not.toContain('export interface OpsConcurrencyStatsResponse')
    expect(facadeSource).not.toContain('export interface OpsRuntimeLogConfig')
    expect(facadeSource).not.toContain('export interface OpsSystemLog')
    expect(facadeSource).not.toContain('export interface OpsErrorLog')
    expect(facadeSource).not.toContain('export interface OpsErrorDetail')
    expect(facadeSource).not.toContain('export interface OpsSettingsSnapshot')
  })

  it('keeps dashboard and metrics network calls in explicit query owners', () => {
    expect(dashboardQuerySource).toContain("'/admin/ops/dashboard/snapshot-v2'")
    expect(dashboardQuerySource).toContain("'/admin/ops/dashboard/network-bandwidth-trend'")
    expect(dashboardQuerySource).toContain('signal: options.signal')
    expect(metricsQuerySource).toContain("'/admin/ops/concurrency-snapshot'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/image-generation-stats'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/openai-token-stats'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/user-usage-stats'")
    expect(errorQuerySource).toContain("'/admin/ops/errors'")
    expect(errorQuerySource).toContain("'/admin/ops/request-errors'")
    expect(errorQuerySource).toContain("'/admin/ops/upstream-errors'")
    expect(errorQuerySource).toContain("include_detail = '1'")
    expect(errorActionSource).toContain('/admin/ops/errors/${errorId}/resolve')
    expect(errorActionSource).toContain('/admin/ops/request-errors/${errorId}/resolve')
    expect(errorActionSource).toContain('/admin/ops/upstream-errors/${errorId}/resolve')
    expect(logQuerySource).toContain("'/admin/ops/requests'")
    expect(logQuerySource).toContain("'/admin/ops/system-logs/health'")
    expect(logActionSource).toContain("'/admin/ops/runtime/logging/reset'")
    expect(logActionSource).toContain("'/admin/ops/system-logs/cleanup'")
    expect(alertQuerySource).toContain("'/admin/ops/alert-rules'")
    expect(alertQuerySource).toContain("'/admin/ops/alert-events'")
    expect(alertActionSource).toContain("'/admin/ops/alert-silences'")
    expect(settingsQuerySource).toContain("'/admin/ops/settings/snapshot'")
    expect(settingsQuerySource).toContain("'/admin/ops/settings/metric-thresholds'")
    expect(settingsActionSource).toContain("'/admin/ops/advanced-settings'")
    expect(realtimeSubscriptionSource).toContain("buildGatewayUrl('/api/v1/admin/ops/ws/qps')")
    expect(realtimeSubscriptionSource).toContain("protocols.push(`jwt.${rawToken}`)")
  })

  it('keeps the transitional facade as a re-export without duplicate query implementations', () => {
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsAlertActions'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsAlertQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsDashboardQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsErrorActions'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsErrorQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsLogActions'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsLogQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsMetricsQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsRealtimeSubscription'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsSettingsActions'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsSettingsQueries'")
    expect(facadeSource).not.toContain('export async function getDashboardSnapshotV2')
    expect(facadeSource).not.toContain('export async function getImageGenerationStats')
    expect(facadeSource).not.toContain('export async function listErrorLogs')
    expect(facadeSource).not.toContain('export async function getRequestErrorDetail')
    expect(facadeSource).not.toContain('export async function updateRequestErrorResolved')
    expect(facadeSource).not.toContain('export async function listRequestDetails')
    expect(facadeSource).not.toContain('export async function listSystemLogs')
    expect(facadeSource).not.toContain('export async function listAlertRules')
    expect(facadeSource).not.toContain('export async function getSettingsSnapshot')
    expect(facadeSource).not.toContain('export function subscribeQPS')
    expect(facadeSource).not.toContain('apiClient')
    expect(facadeSource).not.toContain('buildGatewayUrl')
    expect(facadeSource.split('\n').length).toBeLessThan(200)
  })

  it('keeps migrated presentation consumers off the compatibility methods', () => {
    const migratedMethods = [
      'getDashboardOverview',
      'getDashboardSnapshotV2',
      'getThroughputTrend',
      'getSwitchTrend',
      'getLatencyHistogram',
      'getErrorTrend',
      'getErrorDistribution',
      'getImageGenerationStats',
      'getOpenAITokenStats',
      'getUserUsageStats',
      'getConcurrencyStats',
      'getConcurrencySnapshot',
      'getUserConcurrencyStats',
      'getAccountAvailabilityStats',
      'getRealtimeTrafficSummary',
      'listErrorLogs',
      'getErrorLogDetail',
      'updateErrorResolved',
      'listRequestErrors',
      'listUpstreamErrors',
      'getRequestErrorDetail',
      'getUpstreamErrorDetail',
      'updateRequestErrorResolved',
      'updateUpstreamErrorResolved',
      'listRequestErrorUpstreamErrors',
      'listRequestDetails',
      'getRuntimeLogConfig',
      'updateRuntimeLogConfig',
      'resetRuntimeLogConfig',
      'listSystemLogs',
      'cleanupSystemLogs',
      'getSystemLogSinkHealth',
      'subscribeQPS',
      'listAlertRules',
      'createAlertRule',
      'updateAlertRule',
      'deleteAlertRule',
      'listAlertEvents',
      'getAlertEvent',
      'updateAlertEventStatus',
      'createAlertSilence',
      'getSettingsSnapshot',
      'getEmailNotificationConfig',
      'updateEmailNotificationConfig',
      'getAlertRuntimeSettings',
      'updateAlertRuntimeSettings',
      'getAdvancedSettings',
      'updateAdvancedSettings',
      'getMetricThresholds',
      'updateMetricThresholds'
    ]
    const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))
    for (const runtime of presentationSources) {
      for (const method of migratedMethods) {
        expect(runtime.source, `${runtime.path}: opsAPI.${method}`).not.toContain(`opsAPI.${method}`)
      }
    }
  })

  it('keeps admin ops presentation off compatibility barrels and the compatibility facade', () => {
    const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))
    for (const runtime of presentationSources) {
      expect(runtime.source, `${runtime.path}: adminOpsDatasource`).not.toContain(
        "@/features/admin-ops/data/datasources/adminOpsDatasource"
      )
      expect(runtime.source, `${runtime.path}: @/api`).not.toMatch(/from ['"]@\/api(?:\/admin)?['"]/)
      expect(runtime.source, `${runtime.path}: @/stores`).not.toContain("from '@/stores'")
    }
  })
})
