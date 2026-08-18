import { describe, expect, it } from 'vitest'

import enAdmin from '@/core/i18n/locales/en/admin'
import enCommon from '@/core/i18n/locales/en/common'
import zhAdmin from '@/core/i18n/locales/zh/admin'
import zhCommon from '@/core/i18n/locales/zh/common'
import { backupRecordStatusLabel } from '@/features/admin-backup/backupStatus'
import type {
  BackupRecordProgress,
  BackupRecordStatus,
} from '@/features/admin-backup/data/datasources/adminBackupDatasource'

type Messages = Record<string, unknown>

const messages = {
  en: { ...enCommon, admin: enAdmin },
  zh: { ...zhCommon, admin: zhAdmin },
} satisfies Record<'en' | 'zh', Messages>

function resolveMessage(messages: Messages, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Messages)[segment]
  }, messages)
}

function translator(locale: 'en' | 'zh') {
  return (key: string): string => {
    const value = resolveMessage(messages[locale], key)
    return typeof value === 'string' ? value : key
  }
}

describe('backup status locale contract', () => {
  const statuses: BackupRecordStatus[] = ['pending', 'running', 'completed', 'failed']
  const progress: BackupRecordProgress[] = ['pending', 'dumping', 'uploading']

  const expected = {
    en: {
      statuses: ['Pending', 'Running', 'Completed', 'Failed'],
      progress: ['Preparing', 'Dumping database', 'Uploading'],
      unknown: 'Unknown',
    },
    zh: {
      statuses: ['等待中', '执行中', '已完成', '失败'],
      progress: ['准备中', '导出数据库', '上传中'],
      unknown: '未知',
    },
  } as const

  it.each(['en', 'zh'] as const)('maps every status and progress value in %s', (locale) => {
    const t = translator(locale)

    expect(statuses.map((status) => backupRecordStatusLabel(t, status))).toEqual(
      expected[locale].statuses,
    )
    expect(progress.map((value) => backupRecordStatusLabel(t, 'running', value))).toEqual(
      expected[locale].progress,
    )
  })

  it.each(['en', 'zh'] as const)('uses safe localized fallbacks in %s', (locale) => {
    const t = translator(locale)

    expect(backupRecordStatusLabel(t, 'running', 'future_progress')).toBe(
      expected[locale].statuses[1],
    )
    expect(backupRecordStatusLabel(t, 'future_status')).toBe(expected[locale].unknown)
  })
})
