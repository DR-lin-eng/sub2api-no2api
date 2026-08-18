import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const jobStatusKeys = {
  queued: 'batchImage.status.queued',
  running: 'batchImage.status.running',
  indexing: 'batchImage.status.processingResults',
  processing_results: 'batchImage.status.processingResults',
  settling: 'batchImage.status.settling',
  completed: 'batchImage.status.completed',
  failed: 'batchImage.status.failed',
  cancelled: 'batchImage.status.cancelled',
  output_deleted: 'batchImage.status.outputDeleted',
} as const

const itemStatusKeys = {
  pending: 'batchImage.itemStatus.pending',
  succeeded: 'batchImage.itemStatus.succeeded',
  success: 'batchImage.itemStatus.succeeded',
  failed: 'batchImage.itemStatus.failed',
  cancelled: 'batchImage.itemStatus.cancelled',
} as const

export function batchImageJobStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, jobStatusKeys, value)
}

export function batchImageItemStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, itemStatusKeys, value)
}
