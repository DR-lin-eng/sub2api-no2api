import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

import DashboardView from '../presentation/pages/DashboardPage.vue'

const {
  keysList,
  getDashboardApiKeysUsage,
  getDashboardStats,
  getDashboardTrend,
  getDashboardModels,
  getByDateRange,
} = vi.hoisted(() => ({
  keysList: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
}))

const intersectionObservers: TestDashboardIntersectionObserver[] = []

class TestDashboardIntersectionObserver {
  private readonly observedElements: Element[] = []

  constructor(private readonly callback: IntersectionObserverCallback) {
    intersectionObservers.push(this)
  }

  observe(element: Element): void {
    this.observedElements.push(element)
  }

  unobserve(element: Element): void {
    const index = this.observedElements.indexOf(element)
    if (index >= 0) this.observedElements.splice(index, 1)
  }

  disconnect(): void {
    this.observedElements.length = 0
  }

  trigger(element: Element): void {
    this.callback(
      [{ target: element, isIntersecting: true } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
}

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => ({
    user: ref({ balance: 0 }),
    isSimpleMode: false,
    refreshUser: vi.fn().mockResolvedValue(undefined)
  })
}))

vi.mock('@/features/keys/data/datasources/keysDatasource', () => ({ keysAPI: { list: keysList } }))
vi.mock('@/features/usage/data/datasources/usageDatasource', () => ({
  usageAPI: {
    getDashboardStats,
    getDashboardTrend,
    getDashboardModels,
    getByDateRange,
    getDashboardApiKeysUsage
  }
}))
vi.mock('@/features/profile/data/datasources/profileDatasource', () => ({ getMyPlatformQuotas: vi.fn().mockResolvedValue({ platform_quotas: [] }) }))
vi.mock('../presentation/widgets/UserDashboardCharts.vue', () => ({
  default: {
    name: 'UserDashboardCharts',
    props: ['startDate', 'endDate'],
    emits: ['update:startDate', 'update:endDate', 'dateRangeChange', 'refresh'],
    template: `
      <div>
        <button
          data-test="range"
          @click="$emit('update:startDate', '2026-03-07'); $emit('update:endDate', '2026-03-08'); $emit('dateRangeChange')"
        >range</button>
        <button data-test="refresh-all" @click="$emit('refresh')">refresh</button>
      </div>
    `
  }
}))

const deferred = <T>() => {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const key = (id: number) => ({ id, name: `Key ${id}` })
const page = (items: ReturnType<typeof key>[], current: number, pages: number) => ({
  items, total: items.length, page: current, page_size: 100, pages
})

const mountDashboard = () => mount(DashboardView, {
  global: {
    mocks: {
      $t: (key: string) => key
    },
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      LoadingSpinner: true,
      UserDashboardStats: true,
      UserDashboardApiKeyUsage: {
        name: 'UserDashboardApiKeyUsage',
        props: ['rows', 'loading', 'error'],
        emits: ['retry'],
        template: '<button data-test="api-keys" @click="$emit(\'retry\')" />'
      },
      UserDashboardRecentUsage: true,
      UserDashboardQuickActions: true
    }
  }
})

function triggerDashboardSection(wrapper: ReturnType<typeof mountDashboard>, testId: string): void {
  const section = wrapper.get(`[data-testid="${testId}"]`).element
  const observer = intersectionObservers[0]
  expect(observer).toBeDefined()
  observer.trigger(section)
}

describe('user DashboardView API key usage orchestration', () => {
  beforeEach(() => {
    intersectionObservers.length = 0
    vi.stubGlobal('IntersectionObserver', TestDashboardIntersectionObserver)
    keysList.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getDashboardStats.mockReset()
    getDashboardTrend.mockReset()
    getDashboardModels.mockReset()
    getByDateRange.mockReset()
    keysList.mockResolvedValue(page([], 1, 1))
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getDashboardStats.mockResolvedValue({})
    getDashboardTrend.mockResolvedValue({ trend: [] })
    getDashboardModels.mockResolvedValue({ models: [] })
    getByDateRange.mockResolvedValue({ items: [] })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('defers chart and lower-page requests until their sections approach the viewport', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(getDashboardStats).toHaveBeenCalledTimes(1)
    expect(getDashboardTrend).not.toHaveBeenCalled()
    expect(getDashboardModels).not.toHaveBeenCalled()
    expect(keysList).not.toHaveBeenCalled()
    expect(getByDateRange).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="range"]').exists()).toBe(false)

    triggerDashboardSection(wrapper, 'dashboard-charts-section')
    await flushPromises()
    expect(wrapper.find('[data-test="range"]').exists()).toBe(true)
    expect(getDashboardTrend).toHaveBeenCalledTimes(1)
    expect(getDashboardModels).toHaveBeenCalledTimes(1)
    expect(keysList).not.toHaveBeenCalled()

    triggerDashboardSection(wrapper, 'dashboard-api-key-usage-section')
    await flushPromises()
    expect(keysList).toHaveBeenCalledTimes(1)

    triggerDashboardSection(wrapper, 'dashboard-recent-usage-section')
    await flushPromises()
    expect(getByDateRange).toHaveBeenCalledTimes(1)
  })

  it('loads deferred sections when the dashboard refresh action is used', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    triggerDashboardSection(wrapper, 'dashboard-charts-section')
    await flushPromises()

    await wrapper.get('[data-test="refresh-all"]').trigger('click')
    await flushPromises()

    expect(wrapper.findComponent({ name: 'UserDashboardApiKeyUsage' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'UserDashboardRecentUsage' }).exists()).toBe(true)
    expect(keysList).toHaveBeenCalledTimes(1)
    expect(getByDateRange).toHaveBeenCalledTimes(1)
  })

  it('keeps the stats layout visible and exposes a retry after an initial failure', async () => {
    getDashboardStats.mockRejectedValueOnce(new Error('stats failed')).mockResolvedValueOnce({ total_requests: 12 })
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('user-dashboard-stats-stub').exists()).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('dashboard.statsLoadFailed')

    await wrapper.get('[data-test="dashboard-stats-retry"]').trigger('click')
    await flushPromises()

    expect(getDashboardStats).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('retrieves every key page and requests usage in chunks of at most 100 IDs', async () => {
    keysList
      .mockResolvedValueOnce(page(Array.from({ length: 100 }, (_, i) => key(i + 1)), 1, 3))
      .mockResolvedValueOnce(page(Array.from({ length: 100 }, (_, i) => key(i + 101)), 2, 3))
      .mockResolvedValueOnce(page([key(201)], 3, 3))

    const wrapper = mountDashboard()
    await flushPromises()
    triggerDashboardSection(wrapper, 'dashboard-api-key-usage-section')
    await flushPromises()

    expect(keysList.mock.calls).toEqual([[1, 100], [2, 100], [3, 100]])
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(3)
    expect(getDashboardApiKeysUsage.mock.calls.map(call => call[0].length)).toEqual([100, 100, 1])
    expect(getDashboardApiKeysUsage.mock.calls.flatMap(call => call[0])).toEqual(Array.from({ length: 201 }, (_, i) => i + 1))
  })

  it('limits parallel API key page requests to four', async () => {
    const gate = deferred<void>()
    let inFlight = 0
    let maxInFlight = 0
    keysList.mockImplementation(async (pageNumber: number) => {
      if (pageNumber === 1) return page([], 1, 7)
      inFlight += 1
      maxInFlight = Math.max(maxInFlight, inFlight)
      await gate.promise
      inFlight -= 1
      return page([], pageNumber, 7)
    })

    const wrapper = mountDashboard()
    await flushPromises()
    triggerDashboardSection(wrapper, 'dashboard-api-key-usage-section')
    await flushPromises()

    expect(maxInFlight).toBe(4)
    expect(keysList).toHaveBeenCalledTimes(5)
    gate.resolve()
    await flushPromises()
    expect(keysList).toHaveBeenCalledTimes(7)
    expect(maxInFlight).toBe(4)
  })

  it('uses request date snapshots and ignores stale completion and loading cleanup', async () => {
    keysList.mockResolvedValue(page([key(1)], 1, 1))
    const first = deferred<{ stats: Record<string, any> }>()
    const second = deferred<{ stats: Record<string, any> }>()
    getDashboardApiKeysUsage.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const wrapper = mountDashboard()
    await flushPromises()
    triggerDashboardSection(wrapper, 'dashboard-charts-section')
    triggerDashboardSection(wrapper, 'dashboard-api-key-usage-section')
    await flushPromises()

    const initialDates = getDashboardApiKeysUsage.mock.calls[0][1]
    await wrapper.get('[data-test="range"]').trigger('click')
    await flushPromises()
    expect(getDashboardApiKeysUsage.mock.calls[1][1]).toMatchObject({ startDate: '2026-03-07', endDate: '2026-03-08' })
    expect(initialDates).not.toMatchObject({ startDate: '2026-03-07', endDate: '2026-03-08' })

    first.resolve({ stats: { 1: { api_key_id: 1, total_tokens: 10, total_actual_cost: 1 } } })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'UserDashboardApiKeyUsage' }).props('loading')).toBe(true)

    second.resolve({ stats: { 1: { api_key_id: 1, total_tokens: 20, total_actual_cost: 2 } } })
    await flushPromises()
    const panel = wrapper.findComponent({ name: 'UserDashboardApiKeyUsage' })
    expect(panel.props('loading')).toBe(false)
    expect(panel.props('rows')).toEqual([{ id: 1, name: 'Key 1', totalTokens: 20, actualSpend: 2 }])
  })

  it('retains prior valid rows and exposes an error when refresh fails', async () => {
    keysList.mockResolvedValue(page([key(1)], 1, 1))
    getDashboardApiKeysUsage
      .mockResolvedValueOnce({ stats: { 1: { api_key_id: 1, total_tokens: 30, total_actual_cost: 3 } } })
      .mockRejectedValueOnce(new Error('refresh failed'))
    const wrapper = mountDashboard()
    await flushPromises()
    triggerDashboardSection(wrapper, 'dashboard-api-key-usage-section')
    await flushPromises()
    await wrapper.get('[data-test="api-keys"]').trigger('click')
    await flushPromises()

    const panel = wrapper.findComponent({ name: 'UserDashboardApiKeyUsage' })
    expect(panel.props('error')).toBe(true)
    expect(panel.props('rows')).toEqual([{ id: 1, name: 'Key 1', totalTokens: 30, actualSpend: 3 }])
  })
})
