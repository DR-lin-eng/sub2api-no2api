import type {
  CurrencyAmounts,
  CurrencyAwareDashboardStats,
  DashboardStats,
  TopUserPaymentStats,
} from '@/features/billing/paymentContracts'

const LEGACY_PAYMENT_CURRENCY = 'CNY'

function copyCurrencyAmounts(
  amounts: CurrencyAmounts | null | undefined,
  legacyAmount: number,
): CurrencyAmounts {
  if (amounts != null) {
    return Object.fromEntries(
      Object.entries(amounts)
        .filter(([currency, amount]) => currency.trim() && Number.isFinite(amount))
        .map(([currency, amount]) => [currency.trim().toUpperCase(), amount]),
    )
  }
  return { [LEGACY_PAYMENT_CURRENCY]: legacyAmount }
}

function copyTopUsers(
  usersByCurrency: Record<string, TopUserPaymentStats[]> | null | undefined,
  legacyUsers: TopUserPaymentStats[],
): Record<string, TopUserPaymentStats[]> {
  if (usersByCurrency != null) {
    return Object.fromEntries(
      Object.entries(usersByCurrency).map(([currency, users]) => [
        currency.trim().toUpperCase(),
        [...users],
      ]),
    )
  }
  return legacyUsers.length === 0 ? {} : { [LEGACY_PAYMENT_CURRENCY]: [...legacyUsers] }
}

function fillSummaryCurrencyZeros(...amountGroups: CurrencyAmounts[]): void {
  const currencies = new Set(amountGroups.flatMap((amounts) => Object.keys(amounts)))
  if (currencies.size === 0) currencies.add(LEGACY_PAYMENT_CURRENCY)

  for (const currency of currencies) {
    for (const amounts of amountGroups) {
      if (amounts[currency] == null) amounts[currency] = 0
    }
  }
}

export function normalizePaymentDashboardStats(stats: DashboardStats): CurrencyAwareDashboardStats {
  const todayAmount = copyCurrencyAmounts(stats.today_amount_by_currency, stats.today_amount)
  const totalAmount = copyCurrencyAmounts(stats.total_amount_by_currency, stats.total_amount)
  const avgAmount = copyCurrencyAmounts(stats.avg_amount_by_currency, stats.avg_amount)
  fillSummaryCurrencyZeros(todayAmount, totalAmount, avgAmount)

  return {
    today_amount: todayAmount,
    total_amount: totalAmount,
    today_count: stats.today_count,
    total_count: stats.total_count,
    avg_amount: avgAmount,
    pending_orders: stats.pending_orders ?? 0,
    daily_series: (stats.daily_series || []).map((day) => ({
      date: day.date,
      amount: copyCurrencyAmounts(day.amount_by_currency, day.amount),
      count: day.count,
    })),
    payment_methods: (stats.payment_methods || []).map((method) => ({
      type: method.type,
      amount: copyCurrencyAmounts(method.amount_by_currency, method.amount),
      count: method.count,
    })),
    top_users: copyTopUsers(stats.top_users_by_currency, stats.top_users || []),
  }
}
