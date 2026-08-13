import { onBeforeUnmount, ref } from 'vue'
import {
  buildChatWebSocket,
  parseChatSocketEvent,
  type ChatMessage,
} from '@/features/support-chat/data/datasources/supportChatDatasource'

export interface SupportChatSocketOptions {
  scope: 'user' | 'admin'
  onMessage: (message: ChatMessage) => void
  onStatusChange?: (connected: boolean) => void
  onReadState?: (conversationID: number, reader: 'user' | 'admin') => void
}

const RECONNECT_DELAYS_MS = [800, 1600, 3000, 5000, 15_000, 30_000]

export function useSupportChatSocket(options: SupportChatSocketOptions) {
  const connected = ref(false)
  const connecting = ref(false)
  const supported = typeof WebSocket !== 'undefined'
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempt = 0
  let closedByOwner = false

  function setConnected(value: boolean) {
    connected.value = value
    options.onStatusChange?.(value)
  }

  function clearReconnectTimer() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function scheduleReconnect() {
    if (closedByOwner || reconnectTimer || !supported) return
    const delay = RECONNECT_DELAYS_MS[Math.min(reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)]
    reconnectAttempt += 1
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  function connect() {
    if (!supported || socket || connecting.value) return
    closedByOwner = false
    connecting.value = true

    const ws = buildChatWebSocket(options.scope)
    if (!ws) {
      connecting.value = false
      setConnected(false)
      return
    }

    socket = ws
    ws.onopen = () => {
      reconnectAttempt = 0
      connecting.value = false
      setConnected(true)
    }
    ws.onmessage = (event) => {
      if (typeof event.data !== 'string') return
      const parsed = parseChatSocketEvent(event.data)
      if (parsed?.type === 'message' && parsed.message) {
        options.onMessage(parsed.message)
      } else if (parsed?.type === 'read_state' && parsed.read_state) {
        options.onReadState?.(parsed.read_state.conversation_id, parsed.read_state.reader)
      }
    }
    ws.onerror = () => {
      connecting.value = false
      setConnected(false)
    }
    ws.onclose = () => {
      socket = null
      connecting.value = false
      setConnected(false)
      scheduleReconnect()
    }
  }

  function disconnect() {
    closedByOwner = true
    clearReconnectTimer()
    if (socket) {
      socket.onclose = null
      socket.close()
      socket = null
    }
    connecting.value = false
    setConnected(false)
  }

  onBeforeUnmount(disconnect)

  return {
    connected,
    connecting,
    supported,
    connect,
    disconnect,
  }
}
