import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminPaymentDashboardView from '../presentation/pages/AdminPaymentDashboardPage.vue'
import type { DashboardStats } from '@/features/billing/paymentContracts'

const { getDashboard, showError } = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/features/admin-orders/data/datasources/adminPaymentQueries', () => ({
  getDashboard,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('@/core/utils/apiError', () => ({
  extractI18nErrorMessage: () => 'payment error',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })
  return { promise, resolve }
}

function dashboardStats(totalCount: number): DashboardStats {
  return {
    today_amount: totalCount,
    total_amount: totalCount,
    today_count: totalCount,
    total_count: totalCount,
    avg_amount: totalCount,
    pending_orders: 0,
    daily_series: [],
    payment_methods: [],
    top_users: [],
  }
}

function mountDashboard() {
  return mount(AdminPaymentDashboardView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        LoadingSpinner: true,
        DailyRevenueChart: true,
        OrderStatsCards: {
          props: ['stats'],
          template: '<div data-testid="dashboard-stats">{{ stats.total_count }}</div>',
        },
      },
    },
  })
}

describe('AdminPaymentDashboardView request ordering', () => {
  beforeEach(() => {
    getDashboard.mockReset()
    showError.mockReset()
  })

  it('keeps the latest day selection when an older request resolves last', async () => {
    const initial = deferred<{ data: DashboardStats }>()
    const selected = deferred<{ data: DashboardStats }>()
    getDashboard.mockImplementation((days: number) =>
      days === 30 ? initial.promise : selected.promise,
    )
    const wrapper = mountDashboard()
    await wrapper.vm.$nextTick()

    await wrapper.findAll('button')[0]!.trigger('click')
    expect(getDashboard.mock.calls.map(([days]) => days)).toEqual([30, 7])

    selected.resolve({ data: dashboardStats(7) })
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-stats"]').text()).toBe('7')

    initial.resolve({ data: dashboardStats(30) })
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-stats"]').text()).toBe('7')
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps loading active when only an obsolete request has completed', async () => {
    const initial = deferred<{ data: DashboardStats }>()
    const selected = deferred<{ data: DashboardStats }>()
    getDashboard.mockImplementation((days: number) =>
      days === 30 ? initial.promise : selected.promise,
    )
    const wrapper = mountDashboard()
    await wrapper.vm.$nextTick()

    await wrapper.findAll('button')[0]!.trigger('click')
    initial.resolve({ data: dashboardStats(30) })
    await flushPromises()

    expect(wrapper.find('[data-testid="dashboard-stats"]').exists()).toBe(false)
    expect(wrapper.findAll('button').at(-1)?.attributes('disabled')).toBeDefined()

    selected.resolve({ data: dashboardStats(7) })
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-stats"]').text()).toBe('7')
    expect(wrapper.findAll('button').at(-1)?.attributes('disabled')).toBeUndefined()
  })
})
