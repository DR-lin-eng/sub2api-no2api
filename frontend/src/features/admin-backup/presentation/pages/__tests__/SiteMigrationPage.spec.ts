import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SiteMigrationPage from '../SiteMigrationPage.vue'
import type { MigrationJob } from '../../../data/datasources/siteMigrationDatasource'

const { list, start, resume, secure } = vi.hoisted(() => ({
  list: vi.fn(), start: vi.fn(), resume: vi.fn(), secure: vi.fn(),
}))
vi.mock('../../../data/datasources/siteMigrationDatasource', () => ({
  listMigrationJobs: list, startMigration: start, resumeMigration: resume,
  downloadMigrationReport: vi.fn(), secureMigrationOrigin: secure,
}))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/common/widgets/layout/AppLayout.vue', () => ({ default: {template: '<main><slot /></main>'} }))
vi.mock('@/features/auth/totpStepUpDialog', () => ({ default: { template: '<span />' } }))
const baseJob: MigrationJob = {
  id: 'f339ae03-2a34-44c5-8eb4-b727afcc527d', direction: 'pull', peer: 'https://peer.example',
  profile: 'login', phase: 'failed', error_code: 'peer_auth', inserted: 0, issues: 0,
  total: 0, acknowledged: 0, report_bytes: 0, created_at: '2026-11-20T00:00:00Z', expires_at: '2026-11-21T00:00:00Z',
}
const render = () => mount(SiteMigrationPage, { global: { stubs: {
  AppLayout: {template: '<main><slot /></main>'}, Icon: true,
  BaseDialog: {props: ['show'],template: '<div v-if="show" data-test="resume"><slot /></div>'},
  ConfirmDialog: {props: ['show'],template: '<button v-if="show" data-test="confirm" @click="$emit(\'confirm\')">OK</button>'},
}} })
beforeEach(() => { vi.clearAllMocks(); list.mockResolvedValue([]); secure.mockReturnValue(true) })
afterEach(() => { vi.restoreAllMocks() })

describe('site migration console', () => {
  it('requires confirmation and clears the peer key after submission', async () => {
    start.mockResolvedValue({...baseJob, phase:'completed'})
    const w=render()
    try {
      await w.get('input[placeholder="gateway.example.com"]').setValue('peer.example')
      await w.get('input[type="password"]').setValue('fixture-peer-key-with-scope')
      await w.get('select').setValue('login-quota')
      await w.get('form').trigger('submit')
      expect(start).not.toHaveBeenCalled()
      // Keep a snapshot of the request before the component clears sensitive values.
      let sent: Record<string,unknown> = {}
      start.mockImplementationOnce(async form => { sent={...form}; return {...baseJob, phase:'completed'} })
      await w.get('[data-test="confirm"]').trigger('click')
      await flushPromises()
      expect(sent).toMatchObject({peer:'peer.example',direction:'push',profile:'login-quota',key:'fixture-peer-key-with-scope'})
      expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    } finally { w.unmount() }
  })
  it('resumes the listed job without creating a new one after refresh', async () => {
    list.mockResolvedValue([baseJob]); resume.mockResolvedValue({...baseJob,phase:'completed'})
    const w=render()
    try {
      await flushPromises()
      const action=w.findAll('button').find(b=>b.text()==='admin.migration.resume')
      expect(action).toBeDefined(); await action!.trigger('click')
      await w.get('[data-test="resume"] input').setValue('fixture-peer-key-with-scope')
      await w.get('[data-test="resume"] form').trigger('submit')
      await flushPromises()
      expect(resume).toHaveBeenCalledWith(baseJob.id,'fixture-peer-key-with-scope')
      expect(start).not.toHaveBeenCalled()
    } finally { w.unmount() }
  })
  it('disables key entry on insecure origins', async () => {
    secure.mockReturnValue(false)
    const w=render()
    try {
      await flushPromises()
      expect(w.get('input[type="password"]').attributes('disabled')).toBeDefined()
      expect(list).not.toHaveBeenCalled()
      expect(w.get('[role="alert"]').text()).toBe('admin.migration.https')
    } finally { w.unmount() }
  })
})
