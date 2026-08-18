import { describe, expect, it } from 'vitest'
import { normalizePaymentDashboardStats } from '../presentation/widgets/dashboardStats'
import type { DashboardStats } from '@/features/billing/paymentContracts'

function legacyStats(): DashboardStats {
  return {
    today_amount: 12,
    total_amount: 30,
    today_count: 1,
    total_count: 2,
    avg_amount: 15,
    daily_series: [{ date: '2026-07-27', amount: 12, count: 1 }],
    payment_methods: [{ type: 'stripe', amount: 12, count: 1 }],
    top_users: [{ user_id: 1, email: 'legacy@example.com', amount: 12 }],
  }
}

describe('normalizePaymentDashboardStats', () => {
  it('prefers currency-aware additions while retaining scalar wire fields', () => {
    const normalized = normalizePaymentDashboardStats({
      ...legacyStats(),
      today_amount_by_currency: { USD: 5, NZD: 7 },
      total_amount_by_currency: { USD: 10, NZD: 20 },
      avg_amount_by_currency: { USD: 10, NZD: 20 },
      daily_series: [
        {
          date: '2026-07-27',
          amount: 12,
          amount_by_currency: { USD: 5, NZD: 7 },
          count: 1,
        },
      ],
      payment_methods: [
        { type: 'stripe', amount: 12, amount_by_currency: { USD: 12 }, count: 1 },
      ],
      top_users_by_currency: {
        USD: [{ user_id: 2, email: 'new@example.com', amount: 12 }],
      },
    })

    expect(normalized.today_amount).toEqual({ USD: 5, NZD: 7 })
    expect(normalized.daily_series[0]?.amount).toEqual({ USD: 5, NZD: 7 })
    expect(normalized.payment_methods[0]?.amount).toEqual({ USD: 12 })
    expect(normalized.top_users.USD?.[0]?.email).toBe('new@example.com')
  })

  it('falls back to the legacy CNY aggregates during a rolling upgrade', () => {
    const normalized = normalizePaymentDashboardStats(legacyStats())

    expect(normalized.today_amount).toEqual({ CNY: 12 })
    expect(normalized.total_amount).toEqual({ CNY: 30 })
    expect(normalized.daily_series[0]?.amount).toEqual({ CNY: 12 })
    expect(normalized.payment_methods[0]?.amount).toEqual({ CNY: 12 })
    expect(normalized.top_users.CNY?.[0]?.email).toBe('legacy@example.com')
  })

  it('preserves a visible zero amount for legacy responses', () => {
    const normalized = normalizePaymentDashboardStats({
      ...legacyStats(),
      today_amount: 0,
    })

    expect(normalized.today_amount).toEqual({ CNY: 0 })
  })

  it('fills missing top-level currencies with zero while keeping empty daily maps empty', () => {
    const normalized = normalizePaymentDashboardStats({
      ...legacyStats(),
      today_amount: 0,
      today_amount_by_currency: {},
      total_amount_by_currency: { USD: 30 },
      avg_amount_by_currency: { EUR: 15 },
      daily_series: [
        {
          date: '2026-07-27',
          amount: 0,
          amount_by_currency: {},
          count: 0,
        },
      ],
    })

    expect(normalized.today_amount).toEqual({ EUR: 0, USD: 0 })
    expect(normalized.total_amount).toEqual({ EUR: 0, USD: 30 })
    expect(normalized.avg_amount).toEqual({ EUR: 15, USD: 0 })
    expect(normalized.daily_series[0]?.amount).toEqual({})
  })

  it('shows a CNY zero when a new response has no summary currency yet', () => {
    const normalized = normalizePaymentDashboardStats({
      ...legacyStats(),
      today_amount: 0,
      total_amount: 0,
      avg_amount: 0,
      today_amount_by_currency: {},
      total_amount_by_currency: {},
      avg_amount_by_currency: {},
    })

    expect(normalized.today_amount).toEqual({ CNY: 0 })
    expect(normalized.total_amount).toEqual({ CNY: 0 })
    expect(normalized.avg_amount).toEqual({ CNY: 0 })
  })
})
