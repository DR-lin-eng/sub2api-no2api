import { beforeEach, describe, expect, it, vi } from 'vitest'

const { del, get, post, put } = vi.hoisted(() => ({
  del: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { delete: del, get, post, put },
}))

import {
  cancelOrder,
  createChannel,
  createPlan,
  createProvider,
  deleteChannel,
  deletePlan,
  deleteProvider,
  queryRefund,
  refundOrder,
  retryRecharge,
  updateChannel,
  updateConfig,
  updatePlan,
  updateProvider,
} from '@/features/admin-orders/data/datasources/adminPaymentActions'
import adminPaymentAPI from '@/features/admin-orders/data/datasources/adminPaymentDatasource'
import {
  getChannels,
  getConfig,
  getDashboard,
  getOrder,
  getOrders,
  getPlans,
  getProviders,
} from '@/features/admin-orders/data/datasources/adminPaymentQueries'

describe('admin payment protocol owners', () => {
  beforeEach(() => {
    del.mockReset().mockResolvedValue({ data: {} })
    get.mockReset().mockResolvedValue({ data: {} })
    post.mockReset().mockResolvedValue({ data: {} })
    put.mockReset().mockResolvedValue({ data: {} })
  })

  it('keeps every read endpoint and query mapping unchanged', async () => {
    const filters = {
      page: 2,
      page_size: 50,
      status: 'COMPLETED',
      payment_type: 'stripe',
      user_id: 7,
      keyword: 'sub2_42',
      start_date: '2026-08-01',
      end_date: '2026-08-17',
      order_type: 'subscription',
    }

    await getConfig()
    await getDashboard(30)
    await getDashboard()
    await getOrders(filters)
    await getOrder(42)
    await getChannels()
    await getPlans()
    await getProviders()

    expect(get).toHaveBeenNthCalledWith(1, '/admin/payment/config')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/payment/dashboard', {
      params: { days: 30 },
    })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/payment/dashboard', {
      params: undefined,
    })
    expect(get).toHaveBeenNthCalledWith(4, '/admin/payment/orders', { params: filters })
    expect(get).toHaveBeenNthCalledWith(5, '/admin/payment/orders/42')
    expect(get).toHaveBeenNthCalledWith(6, '/admin/payment/channels')
    expect(get).toHaveBeenNthCalledWith(7, '/admin/payment/plans')
    expect(get).toHaveBeenNthCalledWith(8, '/admin/payment/providers')
  })

  it('keeps config, order lifecycle, and refund payloads unchanged', async () => {
    const config = { enabled: true, min_amount: 10 }
    const refund = {
      amount: 25,
      reason: 'duplicate',
      deduct_balance: true,
      force: false,
    }

    await updateConfig(config)
    await cancelOrder(42)
    await retryRecharge(42)
    await refundOrder(42, refund)
    await queryRefund(42)

    expect(put).toHaveBeenCalledWith('/admin/payment/config', config)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/payment/orders/42/cancel')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/payment/orders/42/retry')
    expect(post).toHaveBeenNthCalledWith(3, '/admin/payment/orders/42/refund', refund)
    expect(post).toHaveBeenNthCalledWith(4, '/admin/payment/orders/42/refund/query')
  })

  it('keeps channel, plan, and provider CRUD paths distinct', async () => {
    const channel = { name: 'channel' }
    const plan = { name: 'plan', price: 9.9 }
    const provider = { name: 'provider', provider_key: 'stripe' }

    await createChannel(channel)
    await updateChannel(1, channel)
    await deleteChannel(1)
    await createPlan(plan)
    await updatePlan(2, plan)
    await deletePlan(2)
    await createProvider(provider)
    await updateProvider(3, provider)
    await deleteProvider(3)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/payment/channels', channel)
    expect(put).toHaveBeenNthCalledWith(1, '/admin/payment/channels/1', channel)
    expect(del).toHaveBeenNthCalledWith(1, '/admin/payment/channels/1')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/payment/plans', plan)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/payment/plans/2', plan)
    expect(del).toHaveBeenNthCalledWith(2, '/admin/payment/plans/2')
    expect(post).toHaveBeenNthCalledWith(3, '/admin/payment/providers', provider)
    expect(put).toHaveBeenNthCalledWith(3, '/admin/payment/providers/3', provider)
    expect(del).toHaveBeenNthCalledWith(3, '/admin/payment/providers/3')
  })

  it('keeps the compatibility facade wired to the exact owner functions', () => {
    expect(adminPaymentAPI).toEqual({
      getConfig,
      updateConfig,
      getDashboard,
      getOrders,
      getOrder,
      cancelOrder,
      retryRecharge,
      refundOrder,
      queryRefund,
      getChannels,
      createChannel,
      updateChannel,
      deleteChannel,
      getPlans,
      createPlan,
      updatePlan,
      deletePlan,
      getProviders,
      createProvider,
      updateProvider,
      deleteProvider,
    })
  })
})
