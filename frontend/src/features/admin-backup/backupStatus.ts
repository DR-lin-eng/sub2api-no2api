import type {
  BackupRecordProgress,
  BackupRecordStatus,
} from '@/features/admin-backup/data/datasources/adminBackupDatasource'

type Translate = (key: string) => string

const statusMetadata = {
  pending: { key: 'admin.backup.status.pending' },
  running: { key: 'admin.backup.status.running' },
  completed: { key: 'admin.backup.status.completed' },
  failed: { key: 'admin.backup.status.failed' },
} as const satisfies Record<BackupRecordStatus, { key: string }>

const progressMetadata = {
  pending: { key: 'admin.backup.progress.pending' },
  dumping: { key: 'admin.backup.progress.dumping' },
  uploading: { key: 'admin.backup.progress.uploading' },
} as const satisfies Record<BackupRecordProgress, { key: string }>

function normalizeRecordValue<Value extends string>(
  value: unknown,
  metadata: Record<Value, { key: string }>,
): Value | null {
  const normalized = String(value ?? '').trim().toLowerCase()
  return Object.prototype.hasOwnProperty.call(metadata, normalized)
    ? normalized as Value
    : null
}

export function backupRecordStatusLabel(
  t: Translate,
  status: unknown,
  progress?: unknown,
): string {
  const normalizedStatus = normalizeRecordValue(status, statusMetadata)
  if (!normalizedStatus) return t('common.unknown')

  if (normalizedStatus === 'running') {
    const normalizedProgress = normalizeRecordValue(progress, progressMetadata)
    if (normalizedProgress) return t(progressMetadata[normalizedProgress].key)
  }

  return t(statusMetadata[normalizedStatus].key)
}
