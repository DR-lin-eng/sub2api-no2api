import { buildGatewayUrl } from '@/core/networks/client'

export type OpsUpstreamErrorEvent = {
  at_unix_ms?: number
  platform?: string
  account_id?: number
  account_name?: string
  upstream_status_code?: number
  upstream_request_id?: string
  kind?: string
  message?: string
  detail?: string
}

export interface SubscribeQPSOptions {
  token?: string | null
  onOpen?: () => void
  onClose?: (event: CloseEvent) => void
  onError?: (event: Event) => void
  onFatalClose?: (event: CloseEvent) => void
  onStatusChange?: (status: OpsWSStatus) => void
  onReconnectScheduled?: (info: { attempt: number, delayMs: number }) => void
  wsBaseUrl?: string
  /** Maximum scheduled reconnect attempts. Set to 0 to disable reconnects. */
  maxReconnectAttempts?: number
  reconnectBaseDelayMs?: number
  reconnectMaxDelayMs?: number
  staleTimeoutMs?: number
  staleCheckIntervalMs?: number
}

export type OpsWSStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline' | 'closed'

export const OPS_WS_CLOSE_CODES = {
  REALTIME_DISABLED: 4001
} as const

const OPS_WS_BASE_PROTOCOL = 'sub2api-admin'

export function subscribeQPS(
  onMessage: (data: any) => void,
  options: SubscribeQPSOptions = {}
): () => void {
  let ws: WebSocket | null = null
  let reconnectAttempts = 0
  const maxReconnectAttempts = Number.isFinite(options.maxReconnectAttempts as number)
    ? (options.maxReconnectAttempts as number)
    : Infinity
  const baseDelayMs = options.reconnectBaseDelayMs ?? 1000
  const maxDelayMs = options.reconnectMaxDelayMs ?? 30000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let shouldReconnect = true
  let isConnecting = false
  let hasConnectedOnce = false
  let lastMessageAt = 0
  const staleTimeoutMs = options.staleTimeoutMs ?? 120_000
  const staleCheckIntervalMs = options.staleCheckIntervalMs ?? 30_000
  let staleTimer: ReturnType<typeof setInterval> | null = null

  const setStatus = (status: OpsWSStatus) => {
    options.onStatusChange?.(status)
  }

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const clearStaleTimer = () => {
    if (staleTimer) {
      clearInterval(staleTimer)
      staleTimer = null
    }
  }

  const startStaleTimer = () => {
    clearStaleTimer()
    if (!staleTimeoutMs || staleTimeoutMs <= 0) return
    staleTimer = setInterval(() => {
      if (!shouldReconnect) return
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      if (!lastMessageAt) return
      const ageMs = Date.now() - lastMessageAt
      if (ageMs > staleTimeoutMs) {
        ws.close()
      }
    }, staleCheckIntervalMs)
  }

  const scheduleReconnect = () => {
    if (!shouldReconnect) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    if (typeof navigator !== 'undefined' && 'onLine' in navigator && !navigator.onLine) {
      setStatus('offline')
      return
    }

    const expDelay = baseDelayMs * Math.pow(2, reconnectAttempts)
    const delay = Math.min(expDelay, maxDelayMs)
    const jitter = Math.floor(Math.random() * 250)
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      reconnectAttempts++
      connect(true)
    }, delay + jitter)
    options.onReconnectScheduled?.({ attempt: reconnectAttempts + 1, delayMs: delay + jitter })
  }

  const handleOnline = () => {
    if (!shouldReconnect) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    connect()
  }

  const handleOffline = () => {
    setStatus('offline')
  }

  const connect = (scheduledReconnect = false) => {
    if (!shouldReconnect) return
    if (isConnecting) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts && !scheduledReconnect) return

    isConnecting = true
    setStatus(hasConnectedOnce ? 'reconnecting' : 'connecting')
    const wsBaseUrl = options.wsBaseUrl || import.meta.env.VITE_WS_BASE_URL
    const wsURL = wsBaseUrl
      ? new URL(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${wsBaseUrl}/api/v1/admin/ops/ws/qps`)
      : new URL(buildGatewayUrl('/api/v1/admin/ops/ws/qps').replace(/^http/, 'ws'))

    // Access tokens are memory-only; never resurrect a bearer token from browser storage.
    const rawToken = String(options.token ?? '').trim()
    const protocols: string[] = [OPS_WS_BASE_PROTOCOL]
    if (rawToken) protocols.push(`jwt.${rawToken}`)

    ws = new WebSocket(wsURL.toString(), protocols)

    ws.onopen = () => {
      reconnectAttempts = 0
      isConnecting = false
      hasConnectedOnce = true
      clearReconnectTimer()
      lastMessageAt = Date.now()
      startStaleTimer()
      setStatus('connected')
      options.onOpen?.()
    }

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        lastMessageAt = Date.now()
        onMessage(data)
      } catch (err) {
        console.warn('[OpsWS] Failed to parse message:', err)
      }
    }

    ws.onerror = (error) => {
      console.error('[OpsWS] Connection error:', error)
      options.onError?.(error)
    }

    ws.onclose = (event) => {
      isConnecting = false
      options.onClose?.(event)
      clearStaleTimer()
      ws = null

      if (event && typeof event.code === 'number' && event.code === OPS_WS_CLOSE_CODES.REALTIME_DISABLED) {
        shouldReconnect = false
        clearReconnectTimer()
        setStatus('closed')
        options.onFatalClose?.(event)
        return
      }

      scheduleReconnect()
    }
  }

  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
  connect()

  return () => {
    shouldReconnect = false
    window.removeEventListener('online', handleOnline)
    window.removeEventListener('offline', handleOffline)
    clearReconnectTimer()
    clearStaleTimer()
    if (ws) ws.close()
    ws = null
    setStatus('closed')
  }
}
