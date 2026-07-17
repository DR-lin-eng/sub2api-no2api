import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

import DashboardView from '../DashboardView.vue'

const { keysList, getDashboardApiKeysUsage } = vi.hoisted(() => ({
  keysList: vi.fn(),
  getDashboardApiKeysUsage: vi.fn()
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: ref({ balance: 0 }),
    isSimpleMode: false,
    refreshUser: vi.fn().mockResolvedValue(undefined)
  })
}))

vi.mock('@/api/keys', () => ({ keysAPI: { list: keysList } }))
vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats: vi.fn().mockResolvedValue({}),
    getDashboardTrend: vi.fn().mockResolvedValue({ trend: [] }),
    getDashboardModels: vi.fn().mockResolvedValue({ models: [] }),
    getByDateRange: vi.fn().mockResolvedValue({ items: [] }),
    getDashboardApiKeysUsage
  }
}))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: vi.fn().mockResolvedValue({ platform_quotas: [] }) }))

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
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      LoadingSpinner: true,
      UserDashboardStats: true,
      UserDashboardCharts: {
        props: ['startDate', 'endDate'],
        emits: ['update:startDate', 'update:endDate', 'dateRangeChange'],
        template: '<button data-test="range" @click="$emit(\'update:startDate\', \'2026-03-07\'); $emit(\'update:endDate\', \'2026-03-08\'); $emit(\'dateRangeChange\')">range</button>'
      },
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

describe('user DashboardView API key usage orchestration', () => {
  beforeEach(() => {
    keysList.mockReset()
    getDashboardApiKeysUsage.mockReset()
    keysList.mockResolvedValue(page([], 1, 1))
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
  })

  it('retrieves every key page and requests usage in chunks of at most 100 IDs', async () => {
    keysList
      .mockResolvedValueOnce(page(Array.from({ length: 100 }, (_, i) => key(i + 1)), 1, 3))
      .mockResolvedValueOnce(page(Array.from({ length: 100 }, (_, i) => key(i + 101)), 2, 3))
      .mockResolvedValueOnce(page([key(201)], 3, 3))

    mountDashboard()
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

    mountDashboard()
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
    await wrapper.get('[data-test="api-keys"]').trigger('click')
    await flushPromises()

    const panel = wrapper.findComponent({ name: 'UserDashboardApiKeyUsage' })
    expect(panel.props('error')).toBe(true)
    expect(panel.props('rows')).toEqual([{ id: 1, name: 'Key 1', totalTokens: 30, actualSpend: 3 }])
  })
})
