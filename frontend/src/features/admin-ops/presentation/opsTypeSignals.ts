import type { OpsRequestDetailsParams } from '@/features/admin-ops/data/dtos/opsLogDtos'

// Ops 前端视图层的共享类型（与后端 DTO 解耦）。

export type ChartState = 'loading' | 'empty' | 'ready'

export interface OpsRequestDetailsPreset {
  title: string
  kind?: OpsRequestDetailsParams['kind']
  sort?: OpsRequestDetailsParams['sort']
  min_duration_ms?: number
  max_duration_ms?: number
  ttft_only?: boolean
}

// Re-export ops alert/settings types so view components can import from a single place.
export type {
  AlertRule,
  AlertEvent,
  AlertSeverity,
  ThresholdMode,
  MetricType,
  Operator,
  EmailNotificationConfig,
  OpsDistributedLockSettings,
  OpsAlertRuntimeSettings,
  OpsMetricThresholds,
  OpsAdvancedSettings,
  OpsDataRetentionSettings,
  OpsAggregationSettings
} from '@/features/admin-ops/data/datasources/adminOpsDatasource'

export type {
  OpsRuntimeLogConfig,
  OpsSystemLog,
  OpsSystemLogSinkHealth
} from '@/features/admin-ops/data/dtos/opsLogDtos'
