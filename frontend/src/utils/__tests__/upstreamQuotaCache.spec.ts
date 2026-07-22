import { beforeEach, describe, expect, it } from 'vitest'
import type { Account, UpstreamQuotaQueryResult } from '@/types'
import {
  persistUpstreamQuotaCache,
  readUpstreamQuotaCache,
  removeUpstreamQuotaCache,
  upstreamQuotaCacheIdentity
} from '../upstreamQuotaCache'

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 7,
  name: 'upstream-key',
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  proxy_id: 3,
  credentials: { base_url: 'https://upstream.example' },
  credentials_status: { has_api_key: true },
  proxy: { id: 3, updated_at: '2026-07-20T00:00:00Z' },
  extra: { enable_tls_fingerprint: true, tls_fingerprint_profile_id: 5 },
  created_at: '2026-07-20T00:00:00Z',
  updated_at: '2026-07-20T00:00:00Z',
  ...overrides
} as Account)

const result: UpstreamQuotaQueryResult = {
  account_id: 7,
  observed_at: '2026-07-20T01:00:00Z',
  quota: {
    provider: 'sub2api',
    mode: 'subscription',
    unit: 'USD',
    remaining: 8,
    subscription: {
      plan_name: 'Pro',
      expires_at: '2026-08-20T00:00:00Z',
      windows: [{ name: 'daily', used: 2, limit: 10, remaining: 8 }]
    }
  }
}

const key = (adminID: number, accountID = 7) =>
  `sub2api:admin:upstream-quota:v2:${adminID}:${accountID}`

describe('upstreamQuotaCache', () => {
  beforeEach(() => localStorage.clear())

  it('isolates successful entries by administrator and account', () => {
    const account = makeAccount()
    persistUpstreamQuotaCache(localStorage, 99, account, result)

    expect(readUpstreamQuotaCache(localStorage, 99, account)).toEqual(result)
    expect(readUpstreamQuotaCache(localStorage, 100, account)).toBeNull()
    expect(localStorage.getItem(key(99))).not.toBeNull()
  })

  it('invalidates an entry when the upstream connection identity changes', () => {
    const account = makeAccount()
    persistUpstreamQuotaCache(localStorage, 99, account, result)

    const changed = makeAccount({ credentials: { base_url: 'https://other.example' } })
    expect(upstreamQuotaCacheIdentity(changed)).not.toBe(upstreamQuotaCacheIdentity(account))
    expect(readUpstreamQuotaCache(localStorage, 99, changed)).toBeNull()
    expect(localStorage.getItem(key(99))).toBeNull()
  })

  it('rejects and removes structurally invalid cached responses', () => {
    localStorage.setItem(key(99), JSON.stringify({
      identity: upstreamQuotaCacheIdentity(makeAccount()),
      result: {
        ...result,
        quota: {
          provider: 'sub2api',
          mode: 'quota',
          windows: [{ name: 'daily', used: '2', limit: 10 }]
        }
      }
    }))

    expect(readUpstreamQuotaCache(localStorage, 99, makeAccount())).toBeNull()
    expect(localStorage.getItem(key(99))).toBeNull()
  })

  it('does not persist responses without quota data and clears only the active admin scope', () => {
    persistUpstreamQuotaCache(localStorage, 99, makeAccount(), { ...result, quota: null })
    persistUpstreamQuotaCache(localStorage, 100, makeAccount(), result)
    expect(localStorage.getItem(key(99))).toBeNull()

    persistUpstreamQuotaCache(localStorage, 99, makeAccount(), result)
    removeUpstreamQuotaCache(localStorage, 99)
    expect(localStorage.getItem(key(99))).toBeNull()
    expect(localStorage.getItem(key(100))).not.toBeNull()
  })

  it('keeps in-memory behavior available when browser storage is disabled', () => {
    const unavailableStorage = {
      getItem: () => { throw new DOMException('disabled', 'SecurityError') },
      setItem: () => { throw new DOMException('disabled', 'SecurityError') },
      removeItem: () => { throw new DOMException('disabled', 'SecurityError') },
      clear: () => undefined,
      key: () => null,
      length: 0
    } as Storage

    expect(() => persistUpstreamQuotaCache(unavailableStorage, 99, makeAccount(), result)).not.toThrow()
    expect(readUpstreamQuotaCache(unavailableStorage, 99, makeAccount())).toBeNull()
    expect(() => removeUpstreamQuotaCache(unavailableStorage, 99)).not.toThrow()
  })
})
