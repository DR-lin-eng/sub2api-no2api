import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))
const routerPush = vi.hoisted(() => vi.fn())
const getOrder = vi.hoisted(() => vi.fn())
const paymentStore = vi.hoisted(() => ({
  config: { stripe_publishable_key: 'pk_test' } as { stripe_publishable_key?: string },
  fetchConfig: vi.fn(),
  pollOrderStatus: vi.fn(),
}))
const loadStripe = vi.hoisted(() => vi.fn())
const stripeElements = vi.hoisted(() => ({
  create: vi.fn(),
}))
const stripePaymentElement = vi.hoisted(() => ({
  mount: vi.fn(),
  on: vi.fn(),
}))
const stripeInstance = vi.hoisted(() => ({
  elements: vi.fn(),
  confirmPayment: vi.fn(),
  confirmAlipayPayment: vi.fn(),
  confirmWechatPayPayment: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({ push: routerPush }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/features/billing/presentation/stores/paymentStore', () => ({
  usePaymentStore: () => paymentStore,
}))

vi.mock('@/features/billing/data/datasources/paymentDatasource', () => ({
  paymentAPI: {
    getOrder,
  },
}))

vi.mock('@stripe/stripe-js/pure', () => ({
  loadStripe,
}))

import StripePaymentView from '../presentation/pages/StripePaymentPage.vue'
import { formatPaymentAmount } from '@/features/billing/paymentDisplay'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  type PaymentRecoverySnapshot,
} from '@/features/billing/presentation/paymentFlowResolver'
import type { PaymentOrder } from '@/features/billing/paymentContracts'

function orderFactory(overrides: Partial<PaymentOrder> = {}): PaymentOrder {
  return {
    id: 42,
    user_id: 7,
    amount: 100,
    pay_amount: 103,
    currency: 'CNY',
    fee_rate: 0.03,
    payment_type: 'stripe',
    out_trade_no: 'sub2_stripe_42',
    status: 'PENDING',
    order_type: 'balance',
    created_at: '2026-04-20T12:00:00Z',
    expires_at: '2026-04-20T12:30:00Z',
    refund_amount: 0,
    ...overrides,
  }
}

function writeSnapshot(overrides: Partial<PaymentRecoverySnapshot> = {}) {
  const snapshot: PaymentRecoverySnapshot = {
    orderId: 42,
    amount: 100,
    qrCode: '',
    expiresAt: '2099-01-01T00:30:00.000Z',
    paymentType: 'stripe',
    payUrl: '/payment/stripe?order_id=42',
    outTradeNo: 'sub2_stripe_42',
    clientSecret: 'pi_secret_42',
    intentId: '',
    currency: 'CNY',
    countryCode: '',
    paymentEnv: '',
    payAmount: 103,
    orderType: 'balance',
    paymentMode: '',
    resumeToken: '',
    createdAt: Date.UTC(2099, 0, 1),
    ...overrides,
  }
  window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify(snapshot))
}

function mountView() {
  return shallowMount(StripePaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
      },
    },
  })
}

describe('StripePaymentView', () => {
  beforeEach(() => {
    routeState.query = {
      order_id: '42',
    }
    routerPush.mockReset()
    getOrder.mockReset().mockResolvedValue({ data: orderFactory() })
    paymentStore.config = { stripe_publishable_key: 'pk_test' }
    paymentStore.fetchConfig.mockReset().mockResolvedValue(undefined)
    paymentStore.pollOrderStatus.mockReset()
    loadStripe.mockReset().mockResolvedValue(stripeInstance)
    stripeElements.create.mockReset().mockReturnValue(stripePaymentElement)
    stripePaymentElement.mount.mockReset()
    stripePaymentElement.on.mockReset().mockImplementation((event: string, callback: () => void) => {
      if (event === 'ready') callback()
    })
    stripeInstance.elements.mockReset().mockReturnValue(stripeElements)
    stripeInstance.confirmPayment.mockReset()
    stripeInstance.confirmAlipayPayment.mockReset()
    stripeInstance.confirmWechatPayPayment.mockReset()
    window.localStorage.clear()
    writeSnapshot()
  })

  it('uses the snapshot secret for the same order and ignores a URL secret', async () => {
    routeState.query.method = 'alipay'
    routeState.query.client_secret = 'pi_other_secret'
    mountView()
    await flushPromises()
    await flushPromises()

    expect(getOrder).toHaveBeenCalledWith(42)
    expect(stripeInstance.confirmAlipayPayment).toHaveBeenCalledWith('pi_secret_42', expect.any(Object))
  })

  it.each([
    { label: 'missing snapshot', snapshot: null },
    { label: 'different order', snapshot: { orderId: 7, clientSecret: 'pi_secret_7' } },
    { label: 'different resume token', snapshot: { resumeToken: 'other-token' } },
    { label: 'expired snapshot', snapshot: { expiresAt: '2000-01-01T00:00:00Z' } },
  ])('rejects $label before loading Stripe', async ({ snapshot }) => {
    window.localStorage.clear()
    if (snapshot) writeSnapshot({ resumeToken: 'bound-token', ...snapshot })
    routeState.query = { order_id: '42', client_secret: 'pi_other_secret', method: 'alipay', resume_token: 'bound-token' }
    const wrapper = mountView()
    await flushPromises()

    expect(getOrder).not.toHaveBeenCalled()
    expect(loadStripe).not.toHaveBeenCalled()
    expect(stripeInstance.confirmAlipayPayment).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.stripeMissingParams')
  })

  it('shows the missing snapshot state when browser storage is unavailable', async () => {
    const storageRead = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('Storage unavailable', 'SecurityError')
    })
    try {
      const wrapper = mountView()
      await flushPromises()
      expect(loadStripe).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('payment.stripeMissingParams')
    } finally {
      storageRead.mockRestore()
    }
  })

  it('本地恢复快照未记录币种时使用订单接口返回的 Stripe 币种展示金额', async () => {
    writeSnapshot({ currency: '' })
    getOrder.mockResolvedValue({
      data: orderFactory({ currency: 'HKD', pay_amount: 103 }),
    })

    const wrapper = mountView()
    await flushPromises()
    await flushPromises()

    expect(getOrder).toHaveBeenCalledWith(42)
    expect(loadStripe).toHaveBeenCalledWith('pk_test')
    expect(wrapper.text()).toContain(formatPaymentAmount(103, 'HKD', 'zh-CN'))
  })
})
