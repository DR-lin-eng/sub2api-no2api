import { describe, expect, it, vi } from 'vitest'

import { accountMatchesFilters, fetchAllAccountIds } from '@/features/admin-accounts/presentation/composables/accountSelection'
import type { Account } from '@/types'

describe('fetchAllAccountIds', () => {
  it('loads all pages using the same lightweight filter snapshot', async () => {
    const fetchPage = vi.fn(async (page: number, pageSize: number) => {
      const start = (page - 1) * pageSize + 1
      const end = Math.min(page * pageSize, 2505)
      return {
        items: Array.from({ length: end - start + 1 }, (_, index) => ({ id: start + index })),
        total: 2505,
        pages: 3
      }
    })

    const filters = { platform: 'grok', status: 'active', search: 'example' }
    const ids = await fetchAllAccountIds(fetchPage, filters)

    expect(ids).toHaveLength(2505)
    expect(fetchPage).toHaveBeenCalledTimes(3)
    expect(fetchPage).toHaveBeenNthCalledWith(1, 1, 1000, {
      ...filters,
      lite: '1',
      include_scheduler_score: '0'
    })
  })

  it('rejects incomplete or duplicated results', async () => {
    const fetchPage = vi.fn().mockResolvedValue({
      items: [{ id: 1 }, { id: 1 }],
      total: 2,
      pages: 1
    })

    await expect(fetchAllAccountIds(fetchPage, {})).rejects.toThrow('incomplete')
  })

  it('propagates a later page failure', async () => {
    const fetchPage = vi.fn()
      .mockResolvedValueOnce({
        items: Array.from({ length: 1000 }, (_, index) => ({ id: index + 1 })),
        total: 1001,
        pages: 2
      })
      .mockRejectedValueOnce(new Error('page 2 failed'))

    await expect(fetchAllAccountIds(fetchPage, { group: '7' })).rejects.toThrow('page 2 failed')
  })
})

describe('accountMatchesFilters', () => {
  const now = Date.parse('2026-08-27T00:00:00Z')
  const account = {
    id: 7,
    name: 'OpenAI primary',
    platform: 'openai',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    group_ids: [3],
    extra: {
      codex_7d_used_percent: 100,
      codex_7d_reset_at: '2026-08-28T00:00:00Z',
      privacy_mode: 'training_off'
    }
  } as Account

  it('preserves quota, group, privacy, status, and search filtering', () => {
    expect(accountMatchesFilters(account, {
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      oauth_quota: 'exhausted',
      group: '3',
      privacy_mode: 'training_off',
      search: 'primary'
    }, now)).toBe(true)
    expect(accountMatchesFilters(account, { group: 'ungrouped' }, now)).toBe(false)
    expect(accountMatchesFilters(account, { search: 'missing' }, now)).toBe(false)
  })

  it('stops treating an expired quota snapshot as exhausted', () => {
    expect(accountMatchesFilters(account, { oauth_quota: 'exhausted' }, Date.parse('2026-08-29T00:00:00Z'))).toBe(false)
  })
})
