import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const instanceStatusKeys = {
  online: 'admin.cluster.status.online',
  stale: 'admin.cluster.status.stale',
  stopped: 'admin.cluster.status.stopped',
} as const

const taskStatusKeys = {
  running: 'admin.cluster.status.running',
  succeeded: 'admin.cluster.status.succeeded',
  failed: 'admin.cluster.status.failed',
  lost: 'admin.cluster.status.lost',
} as const

const rolloutStatusKeys = {
  running: 'admin.cluster.release.rolloutStatus.running',
  paused: 'admin.cluster.release.rolloutStatus.paused',
  completed: 'admin.cluster.release.rolloutStatus.completed',
  cancelled: 'admin.cluster.release.rolloutStatus.cancelled',
} as const

const rolloutTargetStatusKeys = {
  pending: 'admin.cluster.release.targetStatus.pending',
  draining: 'admin.cluster.release.targetStatus.draining',
  installing: 'admin.cluster.release.targetStatus.installing',
  restarting: 'admin.cluster.release.targetStatus.restarting',
  verifying: 'admin.cluster.release.targetStatus.verifying',
  succeeded: 'admin.cluster.release.targetStatus.succeeded',
  failed: 'admin.cluster.release.targetStatus.failed',
  cancelled: 'admin.cluster.release.targetStatus.cancelled',
} as const

export function clusterInstanceStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, instanceStatusKeys, value)
}

export function clusterTaskStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, taskStatusKeys, value)
}

export function clusterRolloutStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, rolloutStatusKeys, value)
}

export function clusterRolloutTargetStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, rolloutTargetStatusKeys, value)
}
