import { createPinia } from 'pinia'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'

import ActivityCenterDetailPage from '../presentation/pages/ActivityCenterDetailPage.vue'

const { getByIdMock, participateMock, listMyRecordsMock } = vi.hoisted(() => ({
  getByIdMock: vi.fn(),
  participateMock: vi.fn(),
  listMyRecordsMock: vi.fn(),
}))

const showErrorMock = vi.hoisted(() => vi.fn())

vi.mock('@/features/activity-center/data/datasources/activityCenterDatasource', () => ({
  default: { getById: getByIdMock, participate: participateMock, listMyRecords: listMyRecordsMock },
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
  }),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ params: { id: '7' } }),
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        key === 'activityCenter.errors.ACTIVITY_CAMPAIGN_DAILY_LIMIT'
          ? '今天已经抽过了，明天再来吧'
          : key === 'activityCenter.lottery.drawFailed'
            ? '抽奖失败'
            : params && Object.keys(params).length > 0
              ? `${key}${JSON.stringify(params)}`
              : key,
    }),
  }
})

describe('ActivityCenterDetailPage', () => {
  beforeEach(() => {
    getByIdMock.mockResolvedValue({
      id: 7,
      title: 'Lucky Draw',
      subtitle: 'Win rewards',
      banner_url: '',
      banner_html: '<div>Safe banner</div>',
      type: 'lottery',
      ref_id: 'lottery-7',
      config_json: JSON.stringify({
        lottery: {
          pools: [
            {
              id: 'pool-1',
              tier: 'basic',
              name: 'Basic pool',
              description: 'Open to everyone',
              required_group_ids: [],
              enabled: true,
              daily_limit: 1,
              sort_order: 0,
              prizes: [
                {
                  id: 'prize-1',
                  label: 'Balance reward',
                  prize_type: 'balance',
                  value_amount: '5',
                  reward_group_id: null,
                  value: '',
                  discount_rate: '',
                  weight: 25,
                  is_fallback: false,
                  color: '#22c55e',
                  sort_order: 0,
                  available_count: 10,
                  codes: [],
                },
                {
                  id: 'prize-2',
                  label: 'Thanks',
                  prize_type: 'none',
                  value_amount: '',
                  reward_group_id: null,
                  value: '',
                  discount_rate: '',
                  weight: 75,
                  is_fallback: true,
                  color: '#8b5cf6',
                  sort_order: 1,
                  available_count: null,
                  codes: [],
                },
              ],
            },
          ],
        },
      }),
      starts_at: '2026-08-01T00:00:00Z',
      ends_at: '2026-09-01T00:00:00Z',
      content: 'Full activity details',
    })
    listMyRecordsMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    participateMock.mockResolvedValue({
      id: 99,
      campaign_id: 7,
      campaign_title: 'Lucky Draw',
      campaign_type: 'lottery',
      user_id: 11,
      user_email: '',
      user_name: '',
      pool_id: 'pool-1',
      pool_name: 'Basic pool',
      prize_id: 'prize-1',
      prize_label: 'Balance reward',
      prize_type: 'balance',
      prize_color: '#22c55e',
      result_status: 'won',
      reward_status: 'pending',
      created_at: '2026-08-01T00:00:00Z',
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
    showErrorMock.mockReset()
  })

  const mountPage = () => mount(ActivityCenterDetailPage, {
    global: {
      plugins: [createPinia()],
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<i />' },
        RouterLink: RouterLinkStub,
      },
    },
  })

  it('loads the visible campaign and renders its full details', async () => {
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const wrapper = mountPage()
    await flushPromises()

    expect(getByIdMock).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('Lucky Draw')
    expect(wrapper.text()).toContain('Win rewards')
    expect(wrapper.text()).not.toContain('Safe banner')
    expect(wrapper.text()).toContain('Full activity details')
    expect(wrapper.text()).toContain('activityCenter.fields.content')
    expect(wrapper.text()).toContain('Basic pool')
    expect(wrapper.text()).toContain('Balance reward')
    expect(wrapper.text()).not.toContain('25%')

    const drawButton = wrapper.findAll('button').find((button) => button.text().includes('activityCenter.lottery.drawNow'))
    expect(drawButton).toBeTruthy()
    await drawButton!.trigger('click')
    await flushPromises()
    expect(participateMock).toHaveBeenCalledWith(7, 'pool-1')
    await vi.advanceTimersByTimeAsync(4300)
    await flushPromises()

    expect(wrapper.text()).toContain('activityCenter.lottery.drawResult')
    expect(wrapper.text()).toContain('Balance reward')
    expect(wrapper.findComponent(RouterLinkStub).props('to')).toBe('/activity-center')

    const firstRotation = Number(wrapper.find('svg').attributes('style').match(/rotate\(([-\d.]+)deg\)/)?.[1])
    await drawButton!.trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(4300)
    await flushPromises()

    const secondRotation = Number(wrapper.find('svg').attributes('style').match(/rotate\(([-\d.]+)deg\)/)?.[1])
    expect(participateMock).toHaveBeenCalledTimes(2)
    expect(firstRotation % 360).toBeCloseTo(secondRotation % 360, 5)
  })

  it('shows a friendly message when the daily limit is reached', async () => {
    participateMock.mockRejectedValueOnce({
      code: 'ACTIVITY_CAMPAIGN_DAILY_LIMIT',
      message: 'daily activity participation limit reached',
    })

    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const wrapper = mountPage()
    await flushPromises()

    const drawButton = wrapper.findAll('button').find((button) => button.text().includes('activityCenter.lottery.drawNow'))
    expect(drawButton).toBeTruthy()
    await drawButton!.trigger('click')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith('今天已经抽过了，明天再来吧')
  })
})
