import { createPinia } from 'pinia'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
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
        banner_html: '',
        type: 'lottery',
        ref_id: 'announce-1',
        config_json: JSON.stringify({
          lottery: {
            pools: [
              {
                id: 'pool-1',
                tier: 'basic',
                name: 'Basic pool',
                description: '',
                required_group_ids: [],
                enabled: true,
                daily_limit: 1,
                sort_order: 0,
                prizes: [
                  {
                    id: 'prize-1',
                    label: 'Thanks',
                    prize_type: 'none',
                    value_amount: '',
                    reward_group_id: null,
                    value: '',
                    discount_rate: '',
                    weight: 100,
                    is_fallback: true,
                    color: '#8b5cf6',
                    sort_order: 0,
                    available_count: null,
                    codes: [],
                  },
                ],
              },
            ],
          },
        }),
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
        title: 'Redeem Bonus',
        subtitle: 'Use project reward config',
        banner_url: '',
        banner_html: '',
        type: 'redeem',
        ref_id: 'redeem-1',
        config_json: '{}',
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
        RouterLink: RouterLinkStub,
      },
    },
  })

  it('renders real campaigns from the datasource', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(listMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Spring Bonus')
    expect(wrapper.text()).toContain('Redeem Bonus')
    expect(wrapper.text()).not.toContain('Campaign content one')
    expect(wrapper.findComponent(RouterLinkStub).props('to')).toBe('/activity-center/1')
  })

  it('filters campaigns without exposing activity details in the list', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('select').setValue('redeem')
    await flushPromises()

    expect(wrapper.text()).toContain('Redeem Bonus')
    expect(wrapper.text()).not.toContain('Spring Bonus')

    expect(wrapper.text()).not.toContain('Campaign content two')
  })
})
