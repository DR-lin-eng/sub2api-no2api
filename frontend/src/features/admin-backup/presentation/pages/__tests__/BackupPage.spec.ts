import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import BackupPage from '../BackupPage.vue'

const {
  getS3Config,
  getImageStorageConfig,
  getSchedule,
  listBackups,
  getDownloadURL,
  testS3Connection,
  testImageStorageConnection,
  stepUpRun,
  showError,
} = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  getImageStorageConfig: vi.fn(),
  getSchedule: vi.fn(),
  listBackups: vi.fn(),
  getDownloadURL: vi.fn(),
  testS3Connection: vi.fn(),
  testImageStorageConnection: vi.fn(),
  stepUpRun: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config,
      updateS3Config: vi.fn(),
      testS3Connection,
      getImageStorageConfig,
      updateImageStorageConfig: vi.fn(),
      testImageStorageConnection,
      getSchedule,
      updateSchedule: vi.fn(),
      createBackup: vi.fn(),
      listBackups,
      getBackup: vi.fn(),
      deleteBackup: vi.fn(),
      getDownloadURL,
      restoreBackup: vi.fn(),
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/features/auth/totpStepUpDialog', () => ({
  default: { name: 'TotpStepUpDialog', template: '<div />' },
}))

vi.mock('@/common/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: (error: unknown) => error instanceof Error && error.message === 'cancelled',
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.index === undefined ? key : `${key}:${params.index}`,
  }),
}))

const baseRecord = (id: string, parts?: unknown[]) => ({
  id,
  status: 'completed',
  backup_type: 'postgres',
  file_name: `${id}.sql.gz`,
  s3_key: `backups/${id}.sql.gz`,
  parts,
  size_bytes: 10,
  triggered_by: 'manual',
  started_at: '2026-08-09T00:00:00Z',
})

function mountBackupPage() {
  return mount(BackupPage, {
    global: { stubs: { TotpStepUpDialog: true } },
  })
}

describe('BackupPage split archives', () => {
  beforeEach(() => {
    getS3Config.mockResolvedValue({})
    getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 })
    listBackups.mockReset()
    getDownloadURL.mockReset()
    showError.mockReset()
    stepUpRun.mockReset().mockImplementation((fn: () => unknown) => fn())
    testS3Connection.mockReset().mockResolvedValue({ ok: true })
    testImageStorageConnection.mockReset().mockResolvedValue({ ok: true })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('shows the part count and ordered download links', async () => {
    listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 }, { index: 2 }, { index: 3 }])],
    })
    getDownloadURL.mockResolvedValue({
      parts: [
        { index: 1, size_bytes: 5, url: 'https://example.test/part-1' },
        { index: 2, size_bytes: 6, url: 'https://example.test/part-2' },
        { index: 3, size_bytes: 7, url: 'https://example.test/part-3' },
      ],
    })

    const wrapper = mountBackupPage()
    await flushPromises()
    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('3')

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.backup.actions.download'),
    )
    await button!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:1')
    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:3')
    expect(document.body.querySelector('a[href="https://example.test/part-2"]')).not.toBeNull()
  })

  it('keeps the legacy single URL download flow', async () => {
    listBackups.mockResolvedValue({ items: [baseRecord('legacy')] })
    getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' })

    const wrapper = mountBackupPage()
    await flushPromises()
    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.backup.actions.download'),
    )
    await button!.trigger('click')
    await flushPromises()

    expect(getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  })

  it('does not expose delete while a backup is running', async () => {
    listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' }],
    })

    const wrapper = mountBackupPage()
    await flushPromises()
    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })

  it.each([0, 1])('runs storage connection test %s through step-up', async (index) => {
    listBackups.mockResolvedValue({ items: [] })
    const wrapper = mountBackupPage()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(button =>
      button.text() === 'admin.backup.s3.testConnection',
    )
    await buttons[index].trigger('click')
    await flushPromises()
    expect(stepUpRun).toHaveBeenCalledOnce()
    expect(index === 0 ? testS3Connection : testImageStorageConnection).toHaveBeenCalledOnce()
  })

  it.each([0, 1])('cancels storage connection test %s without sending it or showing an error', async (index) => {
    listBackups.mockResolvedValue({ items: [] })
    stepUpRun.mockRejectedValueOnce(new Error('cancelled'))
    const wrapper = mountBackupPage()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(button =>
      button.text() === 'admin.backup.s3.testConnection',
    )
    await buttons[index].trigger('click')
    await flushPromises()
    expect(testS3Connection).not.toHaveBeenCalled()
    expect(testImageStorageConnection).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    expect(buttons[index].attributes('disabled')).toBeUndefined()
  })

})
