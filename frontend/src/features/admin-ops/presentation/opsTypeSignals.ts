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

export type {
  AlertRule,
  AlertEvent,
  AlertSeverity,
  ThresholdMode,
  MetricType,
  Operator
} from '@/features/admin-ops/data/dtos/opsAlertDtos'

export type {
  EmailNotificationConfig,
  OpsDistributedLockSettings,
  OpsAlertRuntimeSettings,
  OpsMetricThresholds,
  OpsAdvancedSettings,
  OpsDataRetentionSettings,
  OpsAggregationSettings
} from '@/features/admin-ops/data/dtos/opsSettingsDtos'

export type {
  OpsRuntimeLogConfig,
  OpsSystemLog,
  OpsSystemLogSinkHealth
} from '@/features/admin-ops/data/dtos/opsLogDtos'
