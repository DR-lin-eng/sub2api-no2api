import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import OpsErrorDetailDialog from '@/features/admin-ops/presentation/widgets/OpsErrorDetailDialog.vue'

const providerErrorDetail = {
  id: 1,
  created_at: '2026-11-27T08:00:00Z',
  phase: 'upstream',
  error_owner: 'provider',
  status_code: 400,
  user_id: 42,
  user_email: 'alice@example.com',
  account_id: 9,
  account_name: 'openai-prod-9',
  platform: 'openai',
  message: 'invalid parameter',
} as any

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/features/admin-ops/data/datasources/opsErrorQueries', () => ({
  getRequestErrorDetail: vi.fn(async () => providerErrorDetail),
  getUpstreamErrorDetail: vi.fn(async () => providerErrorDetail),
  listRequestErrorUpstreamErrors: vi.fn(async () => ({ items: [] })),
}))

describe('OpsErrorDetailDialog', () => {
  it('displays both requester user and upstream account in request error detail view', async () => {
    const wrapper = mount(OpsErrorDetailDialog, {
      props: {
        show: true,
        errorId: 1,
        errorType: 'request',
      },
      global: {
        plugins: [createPinia()],
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('openai-prod-9')
  })

  it('omits requester card in dedicated upstream error view', async () => {
    const wrapper = mount(OpsErrorDetailDialog, {
      props: {
        show: true,
        errorId: 1,
        errorType: 'upstream',
      },
      global: {
        plugins: [createPinia()],
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).not.toContain('alice@example.com')
    expect(wrapper.text()).toContain('openai-prod-9')
  })
})
