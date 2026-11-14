import { onMounted, onUnmounted, reactive, watch } from 'vue'
import type { Account } from '@/types'
import type { OpenAIQuotaUsage } from '../../data/dtos/openAIQuotaDtos'
import { openOpenAIUsageSocket, type OpenAIUsagePushEvent } from '../../data/datasources/openAIUsageDatasource'

export type OpenAIUsageLiveTransport = 'connecting' | 'websocket' | 'polling'

interface UsagePushEntry {
  account: Account
  identity: string
  socket: WebSocket | null
  retryTimer: ReturnType<typeof setTimeout> | undefined
  generation: number
  attempts: number
  fallbackScheduled: boolean
}

interface AccountsOpenAIUsagePushOptions {
  getAccounts: () => Account[]
  onQuota: (accountID: number, quota: OpenAIQuotaUsage) => void
  onTransport: (accountID: number, transport: OpenAIUsageLiveTransport) => void
  onStopped?: (accountID: number) => void
}

function isNormalOpenAIAccount(account: Account): boolean {
  // The persisted active status is the normal account state. A manually
  // unschedulable account can still be observed for quota changes.
  return account.status === 'active' && account.platform === 'openai' && account.type === 'oauth'
}

function accountIdentity(account: Account): string {
  return JSON.stringify([
    account.id,
    account.status,
    account.platform,
    account.type,
    account.parent_account_id,
    account.proxy_id,
    account.egress_mode,
    account.credentials,
    account.credentials_status
  ])
}

/** Maintains one WS for every normal OpenAI OAuth account in the current list. */
export function useAccountsOpenAIUsagePush(options: AccountsOpenAIUsagePushOptions) {
  const transports = reactive(new Map<number, OpenAIUsageLiveTransport>())
  const entries = new Map<number, UsagePushEntry>()
  let mounted = false
  let disposed = false

  const stopEntry = (accountID: number, entry: UsagePushEntry) => {
    entry.generation += 1
    clearTimeout(entry.retryTimer)
    entry.retryTimer = undefined
    const socket = entry.socket
    entry.socket = null
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      socket.close()
    }
    entries.delete(accountID)
    transports.delete(accountID)
    options.onStopped?.(accountID)
  }

  const connect = (entry: UsagePushEntry) => {
    if (disposed || document.hidden || entries.get(entry.account.id) !== entry || entry.socket) return
    const generation = ++entry.generation
    const fallback = () => {
      if (disposed || entry.generation !== generation || entry.fallbackScheduled) return
      entry.fallbackScheduled = true
      entry.socket?.close()
      entry.socket = null
      transports.set(entry.account.id, 'polling')
      options.onTransport(entry.account.id, 'polling')
      const delay = Math.min(300_000, 60_000 * 2 ** Math.min(entry.attempts++, 3))
      entry.retryTimer = setTimeout(() => {
        if (disposed || entries.get(entry.account.id) !== entry) return
        entry.retryTimer = undefined
        entry.fallbackScheduled = false
        connect(entry)
      }, delay)
    }
    try {
      const socket = openOpenAIUsageSocket(entry.account.id)
      if (!socket) { fallback(); return }
      entry.socket = socket
      transports.set(entry.account.id, 'connecting')
      options.onTransport(entry.account.id, 'connecting')
      socket.onerror = fallback
      socket.onclose = fallback
      socket.onmessage = (event) => {
        if (disposed || generation !== entry.generation || entries.get(entry.account.id) !== entry) return
        try {
          const message = JSON.parse(event.data) as OpenAIUsagePushEvent
          if (message.type === 'fallback') { fallback(); return }
          if (message.type !== 'quota' || !message.quota || typeof message.quota.fetched_at !== 'number') { fallback(); return }
          if (message.transport !== 'websocket' && message.transport !== 'polling') { fallback(); return }
          transports.set(entry.account.id, message.transport)
          options.onTransport(entry.account.id, message.transport)
          if (message.transport === 'websocket') entry.attempts = 0
          options.onQuota(entry.account.id, message.quota)
        } catch { fallback() }
      }
    } catch { fallback() }
  }

  const reconcile = () => {
    if (!mounted || disposed || document.hidden) return
    const desired = new Map<number, { account: Account; identity: string }>()
    for (const account of options.getAccounts()) {
      if (isNormalOpenAIAccount(account)) desired.set(account.id, { account, identity: accountIdentity(account) })
    }
    for (const [accountID, entry] of entries) {
      const next = desired.get(accountID)
      if (!next || next.identity !== entry.identity) stopEntry(accountID, entry)
      else entry.account = next.account
    }
    for (const [accountID, next] of desired) {
      if (entries.has(accountID)) continue
      const entry: UsagePushEntry = {
        account: next.account, identity: next.identity, socket: null, retryTimer: undefined,
        generation: 0, attempts: 0, fallbackScheduled: false
      }
      entries.set(accountID, entry)
      transports.set(accountID, 'connecting')
      connect(entry)
    }
  }

  const stopAll = () => { for (const [accountID, entry] of [...entries]) stopEntry(accountID, entry) }
  const handleVisibility = () => { if (document.hidden) stopAll(); else reconcile() }

  watch(() => options.getAccounts().map(account => [account.id, accountIdentity(account)]), reconcile, { deep: true })
  onMounted(() => {
    mounted = true
    document.addEventListener('visibilitychange', handleVisibility)
    reconcile()
  })
  onUnmounted(() => {
    disposed = true
    document.removeEventListener('visibilitychange', handleVisibility)
    stopAll()
  })

  return { transports, stopAll, reconcile }
}
