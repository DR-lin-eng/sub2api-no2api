import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/core/networks/client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  },
  buildGatewayUrl: (path: string) => `https://gateway.example${path}`
}))

import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import {
  OPS_WS_CLOSE_CODES,
  subscribeQPS
} from '@/features/admin-ops/data/datasources/opsRealtimeSubscription'

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  readonly url: string
  readonly protocols: string[]
  readyState = FakeWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  close = vi.fn(() => {
    this.readyState = FakeWebSocket.CLOSING
  })

  constructor(url: string | URL, protocols?: string | string[]) {
    this.url = String(url)
    this.protocols = typeof protocols === 'string' ? [protocols] : [...(protocols ?? [])]
    instances.push(this)
  }

  emitOpen(): void {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  emitMessage(data: string): void {
    this.onmessage?.({ data } as MessageEvent)
  }

  emitClose(code: number): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.({ code } as CloseEvent)
  }
}

const instances: FakeWebSocket[] = []

describe('admin ops realtime subscription owner', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-17T00:00:00Z'))
    vi.spyOn(Math, 'random').mockReturnValue(0)
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
    instances.length = 0
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('keeps the facade identity and preserves URL, auth protocol, messages, and disposal', () => {
    const onMessage = vi.fn()
    const onOpen = vi.fn()
    const onStatusChange = vi.fn()
    localStorage.setItem('auth_token', ' local-token ')

    const dispose = subscribeQPS(onMessage, {
      onOpen,
      onStatusChange,
      token: ' explicit-token ',
      staleTimeoutMs: 0
    })

    expect(opsAPI.subscribeQPS).toBe(subscribeQPS)
    expect(instances).toHaveLength(1)
    expect(instances[0].url).toBe('wss://gateway.example/api/v1/admin/ops/ws/qps')
    expect(instances[0].protocols).toEqual(['sub2api-admin', 'jwt.explicit-token'])
    expect(onStatusChange).toHaveBeenLastCalledWith('connecting')

    instances[0].emitOpen()
    instances[0].emitMessage('{"qps":7}')

    expect(onOpen).toHaveBeenCalledOnce()
    expect(onStatusChange).toHaveBeenLastCalledWith('connected')
    expect(onMessage).toHaveBeenCalledWith({ qps: 7 })

    dispose()

    expect(instances[0].close).toHaveBeenCalledOnce()
    expect(onStatusChange).toHaveBeenLastCalledWith('closed')
  })

  it('does not read a legacy browser-storage token when no memory token is supplied', () => {
    localStorage.setItem('auth_token', 'legacy-token')
    const dispose = subscribeQPS(vi.fn(), { staleTimeoutMs: 0 })

    expect(instances[0].protocols).toEqual(['sub2api-admin'])
    dispose()
  })

  it('stops reconnecting after the server reports realtime monitoring is disabled', () => {
    const onFatalClose = vi.fn()
    const onClose = vi.fn()
    const onStatusChange = vi.fn()
    const dispose = subscribeQPS(vi.fn(), {
      token: 'explicit-token',
      onClose,
      onFatalClose,
      onStatusChange,
      staleTimeoutMs: 0
    })

    instances[0].emitOpen()
    instances[0].emitClose(OPS_WS_CLOSE_CODES.REALTIME_DISABLED)
    vi.advanceTimersByTime(60_000)

    expect(onClose).toHaveBeenCalledOnce()
    expect(onFatalClose).toHaveBeenCalledOnce()
    expect(onStatusChange).toHaveBeenLastCalledWith('closed')
    expect(instances).toHaveLength(1)

    dispose()
  })

  it('preserves reconnect scheduling and reconnect status', () => {
    const onReconnectScheduled = vi.fn()
    const onStatusChange = vi.fn()
    const dispose = subscribeQPS(vi.fn(), {
      maxReconnectAttempts: 2,
      reconnectBaseDelayMs: 25,
      reconnectMaxDelayMs: 100,
      staleTimeoutMs: 0,
      onReconnectScheduled,
      onStatusChange
    })

    instances[0].emitOpen()
    instances[0].emitClose(1006)

    expect(onReconnectScheduled).toHaveBeenCalledWith({ attempt: 1, delayMs: 25 })

    vi.advanceTimersByTime(25)

    expect(instances).toHaveLength(2)
    expect(onStatusChange).toHaveBeenLastCalledWith('reconnecting')

    instances[1].emitClose(1006)

    expect(onReconnectScheduled).toHaveBeenLastCalledWith({ attempt: 2, delayMs: 50 })

    vi.advanceTimersByTime(50)

    expect(instances).toHaveLength(3)

    dispose()
  })

  it('waits while offline and reconnects when the browser comes online', () => {
    let online = true
    vi.spyOn(window.navigator, 'onLine', 'get').mockImplementation(() => online)
    const onReconnectScheduled = vi.fn()
    const onStatusChange = vi.fn()
    const dispose = subscribeQPS(vi.fn(), {
      staleTimeoutMs: 0,
      onReconnectScheduled,
      onStatusChange
    })

    instances[0].emitOpen()
    online = false
    window.dispatchEvent(new Event('offline'))
    instances[0].emitClose(1006)

    expect(onStatusChange).toHaveBeenLastCalledWith('offline')
    expect(onReconnectScheduled).not.toHaveBeenCalled()
    expect(instances).toHaveLength(1)

    online = true
    window.dispatchEvent(new Event('online'))

    expect(instances).toHaveLength(2)
    expect(onStatusChange).toHaveBeenLastCalledWith('reconnecting')

    dispose()
  })

  it('closes a connected socket after the configured stale window', () => {
    const dispose = subscribeQPS(vi.fn(), {
      staleTimeoutMs: 100,
      staleCheckIntervalMs: 50
    })

    instances[0].emitOpen()
    vi.advanceTimersByTime(150)

    expect(instances[0].close).toHaveBeenCalledOnce()

    dispose()
  })
})
