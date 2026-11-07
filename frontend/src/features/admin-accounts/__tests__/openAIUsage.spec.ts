import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { Account } from '@/types'
import { openOpenAIUsageSocket, queryOpenAIUsage } from '../data/datasources/openAIUsageDatasource'
import { useOpenAIUsagePush } from '../presentation/composables/useOpenAIUsagePush'
import { useAccountsOpenAIUsagePush } from '../presentation/composables/useAccountsOpenAIUsagePush'
import { useOpenAIUsageDetails } from '../presentation/composables/useOpenAIUsageDetails'
import { dailyModelUsage, sampleUsageBalance, threadUsageRows } from '../presentation/openAIUsageDetailsDisplay'

vi.mock('../data/datasources/openAIUsageDatasource', () => ({ openOpenAIUsageSocket: vi.fn(), queryOpenAIUsage: vi.fn() }))
enableAutoUnmount(afterEach)

class FakeSocket {
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  close = vi.fn()
  send(value: unknown) { this.onmessage?.({ data: JSON.stringify(value) }) }
}

const account = { id: 41, status: 'active', platform: 'openai', type: 'oauth', credentials: { chatgpt_account_id: 'acct-41' }, proxy_id: null } as unknown as Account
const quota = { fetched_at: 123, rate_limit: { allowed: true, limit_reached: false, primary_window: { used_percent: 25 } } }
let hidden = false

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  hidden = false
  vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
})
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

describe('OpenAI usage push lifecycle', () => {
  it('opens one independent socket for every active account and skips non-normal statuses', async () => {
    const sockets: FakeSocket[] = []
    vi.mocked(openOpenAIUsageSocket).mockImplementation(() => {
      const socket = new FakeSocket()
      sockets.push(socket)
      return socket as unknown as WebSocket
    })
    const accounts = ref([
      { ...account, id: 41, status: 'active' },
      { ...account, id: 42, status: 'active' },
      { ...account, id: 43, status: 'error' },
      { ...account, id: 44, status: 'inactive' }
    ] as Account[])
    const quotas = new Map<number, unknown>()
    const wrapper = mount(defineComponent({
      setup() {
        useAccountsOpenAIUsagePush({
          getAccounts: () => accounts.value,
          onQuota: (id, value) => quotas.set(id, value),
          onTransport: () => {},
          onStopped: id => quotas.delete(id)
        })
        return () => h('div')
      }
    }))
    await nextTick()
    expect(sockets).toHaveLength(2)
    sockets.forEach(socket => socket.send({ type: 'quota', transport: 'websocket', quota }))
    expect(quotas.size).toBe(2)
    accounts.value = accounts.value.map(item => item.id === 42 ? { ...item, status: 'error' } : item)
    await nextTick()
    expect(sockets[1].close).toHaveBeenCalledOnce()
    expect(quotas.has(42)).toBe(false)
    wrapper.unmount()
  })

  it('applies pushed quota and retains existing refresh behavior after failure', async () => {
    const sockets: FakeSocket[] = []
    vi.mocked(openOpenAIUsageSocket).mockImplementation(() => { const socket = new FakeSocket(); sockets.push(socket); return socket as unknown as WebSocket })
    const onQuota = vi.fn()
    const wrapper = mount(defineComponent({ setup() { const { transport } = useOpenAIUsagePush(() => account, () => true, onQuota); return () => h('div', transport.value) } }))
    await nextTick()
    expect(wrapper.text()).toBe('connecting')
    sockets[0].send({ type: 'quota', transport: 'websocket', quota })
    await nextTick()
    expect(wrapper.text()).toBe('websocket')
    expect(onQuota).toHaveBeenCalledWith(quota)
    sockets[0].send({ type: 'fallback', transport: 'polling' })
    await nextTick()
    expect(wrapper.text()).toBe('polling')
    expect(sockets[0].close).toHaveBeenCalledOnce()
    expect(queryOpenAIUsage).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(59_999)
    expect(sockets).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(sockets).toHaveLength(2)
    wrapper.unmount()
    expect(sockets[1].close).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(600_000)
    expect(sockets).toHaveLength(2)
  })

  it('releases hidden and changed accounts and ignores late frames', async () => {
    const sockets: FakeSocket[] = []
    vi.mocked(openOpenAIUsageSocket).mockImplementation(() => { const socket = new FakeSocket(); sockets.push(socket); return socket as unknown as WebSocket })
    const onQuota = vi.fn()
    const wrapper = mount(defineComponent({
      props: ['account'],
      setup(props) { useOpenAIUsagePush(() => props.account, () => true, onQuota); return () => h('div') }
    }), { props: { account } })
    const late = sockets[0].onmessage
    await wrapper.setProps({ account: { ...account, id: 42 } })
    expect(sockets[0].close).toHaveBeenCalledOnce()
    late?.({ data: JSON.stringify({ type: 'quota', transport: 'websocket', quota }) })
    expect(onQuota).not.toHaveBeenCalled()
    hidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    expect(sockets[1].close).toHaveBeenCalledOnce()
    hidden = false
    document.dispatchEvent(new Event('visibilitychange'))
    expect(sockets).toHaveLength(3)
    wrapper.unmount()
  })
})

describe('OpenAI usage detail requests', () => {
  it('keeps old account responses out and aborts requests on close', async () => {
    let resolveFirst: (value: unknown) => void = () => {}
    vi.mocked(queryOpenAIUsage).mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve as (value: unknown) => void }))
      .mockResolvedValue({ kind: 'plan_history', data: { periods: [{ id: 'new-account' }] }, unavailable: false, fetched_at: Date.now() / 1000, refresh_after_seconds: 300 })
    const wrapper = mount(defineComponent({ props: ['id'], setup(props) {
      const { result } = useOpenAIUsageDetails(() => props.id, () => ({ kind: 'plan_history' }))
      return () => h('div', JSON.stringify(result.value?.data))
    } }), { props: { id: 41 } })
    const firstSignal = vi.mocked(queryOpenAIUsage).mock.calls[0][2]
    await wrapper.setProps({ id: 42 })
    await flushPromises()
    resolveFirst({ kind: 'plan_history', data: { periods: [{ id: 'stale-account' }] }, unavailable: false, fetched_at: 1, refresh_after_seconds: 300 })
    await flushPromises()
    expect(firstSignal?.aborted).toBe(true)
    expect(wrapper.text()).toContain('new-account')
    expect(wrapper.text()).not.toContain('stale-account')
    await vi.advanceTimersByTimeAsync(299_999)
    expect(queryOpenAIUsage).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(600_000)
    expect(queryOpenAIUsage).toHaveBeenCalledTimes(2)
  })

  it('queries monthly and balance on foreground return without polling in the background', async () => {
    vi.mocked(queryOpenAIUsage).mockResolvedValue({ kind: 'monthly', data: {}, unavailable: false, fetched_at: Date.now() / 1000, refresh_after_seconds: 60 })
    const wrapper = mount(defineComponent({ setup() { useOpenAIUsageDetails(() => 41, () => ({ kind: 'monthly' })); return () => h('div') } }))
    await flushPromises()
    hidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(180_000)
    expect(queryOpenAIUsage).toHaveBeenCalledOnce()
    hidden = false
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(queryOpenAIUsage).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('invalidates a pending detail request when the same account is reauthorized', async () => {
    let resolveOld: (value: unknown) => void = () => {}
    vi.mocked(queryOpenAIUsage).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve as (value: unknown) => void }))
      .mockResolvedValue({ kind: 'daily', data: { generation: 'new' }, unavailable: false, fetched_at: 1, refresh_after_seconds: 60 })
    const wrapper = mount(defineComponent({ props: ['identity'], setup(props) {
      const { result } = useOpenAIUsageDetails(() => 41, () => ({ kind: 'daily' }), () => props.identity)
      return () => h('div', JSON.stringify(result.value?.data))
    } }), { props: { identity: 'old-credentials' } })
    await wrapper.setProps({ identity: 'new-credentials' })
    await flushPromises()
    resolveOld({ kind: 'daily', data: { generation: 'old' }, unavailable: false, fetched_at: 1, refresh_after_seconds: 60 })
    await flushPromises()
    expect(wrapper.text()).toContain('new')
    expect(wrapper.text()).not.toContain('old')
    wrapper.unmount()
  })
})

describe('OpenAI usage display units', () => {
  it('keeps credits distinct from token counters and preserves missing counters', () => {
    const rows = dailyModelUsage({ data: [{ date: '2026-11-07', models: [{ model: 'model-a', credits: 1.25 }, { model: 'model-b', uncached_text_input_tokens: 10, cached_text_input_tokens: 20, text_output_tokens: 30 }] }] })
    expect(rows[0]).toMatchObject({ credits: 1.25, total: null, uncached: null })
    expect(rows[1]).toMatchObject({ credits: null, total: 60, uncached: 10, cached: 20, output: 30 })
    expect(dailyModelUsage({ units: 'tokens', data: [{ date: '2026-11-07', models: [{ model: 'model-c', credits: 1200 }] }] })[0]).toMatchObject({ credits: null, total: 1200 })
  })

  it('samples at most 64 balance points and estimates detected resets', () => {
    const points = [
      { observed_at: '2026-11-07T00:00:00Z', estimated_reset_at: '2026-11-07T01:00:00Z', remaining_basis_points: 2000, plan_type: 'plus' },
      { observed_at: '2026-11-07T00:30:00Z', estimated_reset_at: '2026-11-07T01:00:00Z', remaining_basis_points: 1500, plan_type: 'plus' },
      { observed_at: '2026-11-07T03:00:00Z', estimated_reset_at: '2026-11-07T06:00:00Z', remaining_basis_points: 7500, plan_type: 'plus' }
    ]
    const samples = sampleUsageBalance(points)
    expect(samples.some(point => point.remaining === 100)).toBe(true)
    expect(samples[samples.length - 1].remaining).toBe(75)
    expect(sampleUsageBalance(Array.from({ length: 200 }, (_, index) => ({ ...points[0], observed_at: new Date(index * 1000).toISOString() })))).toHaveLength(64)
    expect(sampleUsageBalance([{ observed_at: '2026-11-07T00:00:00Z' }])[0].remaining).toBeNull()
  })

  it('aggregates reported thread token groups without inventing missing usage', () => {
    const rows = threadUsageRows({ threads: [
      { thread_id: 'a', groups: [{ net_new_input_tokens: 10, cached_input_tokens: 20 }, { net_new_input_tokens: 30, cached_input_tokens: 40 }] },
      { thread_id: 'b', output_tokens: 0, groups: [{ output_tokens: 15 }] },
      { thread_id: 'c' }
    ] })
    expect(rows[0]).toMatchObject({ net_new_input_tokens: 40, cached_input_tokens: 60, output_tokens: null })
    expect(rows[1].output_tokens).toBe(0)
    expect(rows[2].net_new_input_tokens).toBeNull()
  })
})
