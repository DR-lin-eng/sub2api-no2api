import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePaymentStore } from '@/features/billing/presentation/stores/paymentStore'

const getConfig = vi.hoisted(() => vi.fn())
vi.mock('@/features/billing/data/datasources/paymentDatasource', () => ({
  paymentAPI: { getConfig }
}))

beforeEach(() => {
  setActivePinia(createPinia())
  getConfig.mockReset()
})

describe('payment config request deduplication', () => {
  it('shares the in-flight response for concurrent callers', async () => {
    let resolve!: (value: { data: any }) => void
    getConfig.mockReturnValue(new Promise((yes) => { resolve = yes }))
    const store = usePaymentStore()
    const first = store.fetchConfig()
    const second = store.fetchConfig()
    expect(getConfig).toHaveBeenCalledTimes(1)
    resolve({ data: { payment_enabled: true, stripe_publishable_key: 'pk_test' } })
    expect(await Promise.all([first, second])).toEqual([
      { payment_enabled: true, stripe_publishable_key: 'pk_test' },
      { payment_enabled: true, stripe_publishable_key: 'pk_test' }
    ])
    expect(store.configLoading).toBe(false)
  })
})
