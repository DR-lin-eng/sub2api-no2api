import { onMounted, onUnmounted, ref, watch } from 'vue'
import type { Account } from '@/types'
import type { OpenAIQuotaUsage } from '../../data/dtos/openAIQuotaDtos'
import { openOpenAIUsageSocket, type OpenAIUsagePushEvent } from '../../data/datasources/openAIUsageDatasource'

export function useOpenAIUsagePush(account: () => Account, enabled: () => boolean, onQuota: (quota: OpenAIQuotaUsage) => void) {
  const transport = ref<'connecting' | 'websocket' | 'polling'>('polling')
  let socket: WebSocket | null = null
  let retry: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  let mounted = false
  let generation = 0
  let attempts = 0
  const identity = () => JSON.stringify([
    account().id, account().platform, account().type, account().parent_account_id,
    account().proxy_id, account().credentials, enabled()
  ])

  const stop = () => {
    generation += 1
    clearTimeout(retry)
    const previous = socket
    socket = null
    if (previous) {
      previous.onopen = previous.onmessage = previous.onerror = previous.onclose = null
      previous.close()
    }
    transport.value = 'polling'
  }

  const connect = () => {
    if (socket || disposed || !mounted || !enabled() || document.hidden || account().status !== 'active' || account().platform !== 'openai' || account().type !== 'oauth') return
    clearTimeout(retry)
    const current = ++generation
    const fallback = () => {
      if (current !== generation || disposed) return
      stop()
      // Keep the existing quota query, passive snapshots and list refreshes.
      // Retry only the optional push channel, with bounded backoff.
      retry = setTimeout(connect, Math.min(300_000, 60_000 * 2 ** Math.min(attempts++, 3)))
    }
    try {
      const ws = openOpenAIUsageSocket(account().id)
      if (!ws) return
      socket = ws
      transport.value = 'connecting'
      ws.onerror = fallback
      ws.onclose = fallback
      ws.onmessage = event => {
        if (current !== generation || disposed) return
        try {
          const message = JSON.parse(event.data) as OpenAIUsagePushEvent
          if (message.type === 'fallback') { fallback(); return }
          if (message.type !== 'quota' || !message.quota || typeof message.quota.fetched_at !== 'number' || !['websocket', 'polling'].includes(message.transport)) {
            fallback()
            return
          }
          transport.value = message.transport
          if (message.transport === 'websocket') attempts = 0
          onQuota(message.quota)
        } catch { fallback() }
      }
    } catch { fallback() }
  }

  const restart = () => { stop(); attempts = 0; connect() }
  const visibility = () => { if (document.hidden) stop(); else connect() }
  watch(identity, restart)
  onMounted(() => {
    mounted = true
    document.addEventListener('visibilitychange', visibility)
    connect()
  })
  onUnmounted(() => {
    disposed = true
    document.removeEventListener('visibilitychange', visibility)
    stop()
  })
  return { transport }
}
