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
const dashboardQuerySource = readFeatureSource('data/datasources/opsDashboardQueries.ts')
const logActionSource = readFeatureSource('data/datasources/opsLogActions.ts')
const logQuerySource = readFeatureSource('data/datasources/opsLogQueries.ts')
const metricsQuerySource = readFeatureSource('data/datasources/opsMetricsQueries.ts')
const dashboardDtoSource = readFeatureSource('data/dtos/opsDashboardDtos.ts')
const logDtoSource = readFeatureSource('data/dtos/opsLogDtos.ts')
const metricsDtoSource = readFeatureSource('data/dtos/opsMetricsDtos.ts')

describe('admin ops modularization', () => {
  it('owns dashboard and metrics contracts outside the compatibility datasource', () => {
    expect(dashboardDtoSource).toContain('export interface OpsDashboardOverview')
    expect(dashboardDtoSource).toContain('export interface OpsDashboardSnapshotParams')
    expect(metricsDtoSource).toContain('export interface OpsConcurrencySnapshotResponse')
    expect(metricsDtoSource).toContain('export interface OpsUserUsageStatsResponse')
    expect(logDtoSource).toContain('export interface OpsRequestDetailsParams')
    expect(logDtoSource).toContain('export interface OpsRuntimeLogConfig')
    expect(logDtoSource).toContain('export interface OpsSystemLogCleanupRequest')
    expect(dashboardDtoSource).not.toContain('apiClient')
    expect(logDtoSource).not.toContain('apiClient')
    expect(metricsDtoSource).not.toContain('apiClient')
    expect(facadeSource).not.toContain('export interface OpsDashboardOverview')
    expect(facadeSource).not.toContain('export interface OpsConcurrencyStatsResponse')
    expect(facadeSource).not.toContain('export interface OpsRuntimeLogConfig')
    expect(facadeSource).not.toContain('export interface OpsSystemLog')
  })

  it('keeps dashboard and metrics network calls in explicit query owners', () => {
    expect(dashboardQuerySource).toContain("'/admin/ops/dashboard/snapshot-v2'")
    expect(dashboardQuerySource).toContain('signal: options.signal')
    expect(metricsQuerySource).toContain("'/admin/ops/concurrency-snapshot'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/image-generation-stats'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/openai-token-stats'")
    expect(metricsQuerySource).toContain("'/admin/ops/dashboard/user-usage-stats'")
    expect(logQuerySource).toContain("'/admin/ops/requests'")
    expect(logQuerySource).toContain("'/admin/ops/system-logs/health'")
    expect(logActionSource).toContain("'/admin/ops/runtime/logging/reset'")
    expect(logActionSource).toContain("'/admin/ops/system-logs/cleanup'")
  })

  it('keeps the transitional facade as a re-export without duplicate query implementations', () => {
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsDashboardQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsLogActions'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsLogQueries'")
    expect(facadeSource).toContain("export * from '@/features/admin-ops/data/datasources/opsMetricsQueries'")
    expect(facadeSource).not.toContain('export async function getDashboardSnapshotV2')
    expect(facadeSource).not.toContain('export async function getImageGenerationStats')
    expect(facadeSource).not.toContain('export async function listRequestDetails')
    expect(facadeSource).not.toContain('export async function listSystemLogs')
    expect(facadeSource.split('\n').length).toBeLessThan(800)
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
      'listRequestDetails',
      'getRuntimeLogConfig',
      'updateRuntimeLogConfig',
      'resetRuntimeLogConfig',
      'listSystemLogs',
      'cleanupSystemLogs',
      'getSystemLogSinkHealth'
    ]
    const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))
    for (const runtime of presentationSources) {
      for (const method of migratedMethods) {
        expect(runtime.source, `${runtime.path}: opsAPI.${method}`).not.toContain(`opsAPI.${method}`)
      }
    }
  })
})
