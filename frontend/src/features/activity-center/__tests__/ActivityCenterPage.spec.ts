import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'

import ActivityCenterPage from '../presentation/pages/ActivityCenterPage.vue'

const { listMock } = vi.hoisted(() => ({
  listMock: vi.fn()
}))

vi.mock('@/features/activity-center/data/datasources/activityCenterDatasource', () => ({
  default: { list: listMock },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params && Object.keys(params).length > 0
          ? `${key}${JSON.stringify(params)}`
          : key,
    }),
  }
})

describe('ActivityCenterPage', () => {
  beforeEach(() => {
    listMock.mockResolvedValue([
      {
        id: 1,
        title: 'Spring Bonus',
        subtitle: 'Weekly active rewards',
        banner_url: '',
        type: 'lottery',
        ref_id: 'announce-1',
        status: 'active',
        starts_at: '2026-08-01T00:00:00Z',
        ends_at: '2026-09-01T00:00:00Z',
        sort_order: 1,
        content: 'Campaign content one',
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-02T00:00:00Z',
      },
      {
        id: 2,
        title: 'External Link',
        subtitle: 'Visit partner site',
        banner_url: '',
        type: 'external_link',
        ref_id: 'https://example.com',
        status: 'draft',
        starts_at: '',
        ends_at: '',
        sort_order: 2,
        content: 'Campaign content two',
        created_at: '2026-08-03T00:00:00Z',
        updated_at: '2026-08-04T00:00:00Z',
      },
    ])
  })

  const mountPage = () => mount(ActivityCenterPage, {
    global: {
      plugins: [createPinia()],
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<i />' },
      },
    },
  })

  it('renders real campaigns from the datasource', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(listMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Spring Bonus')
    expect(wrapper.text()).toContain('External Link')
    expect(wrapper.text()).toContain('activityCenter.rules.title')
  })

  it('filters campaigns and updates the detail panel', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('select').setValue('external_link')
    await flushPromises()

    expect(wrapper.text()).toContain('External Link')
    expect(wrapper.text()).not.toContain('Spring Bonus')

    const campaignButton = wrapper.findAll('button').find((button) => button.text().includes('External Link'))
    expect(campaignButton).toBeTruthy()
    await campaignButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('activityCenter.selected.title')
    expect(wrapper.text()).toContain('Campaign content two')
  })
})
