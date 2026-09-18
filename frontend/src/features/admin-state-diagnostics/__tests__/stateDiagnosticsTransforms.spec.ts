import { describe, expect, it } from 'vitest'
import type { Account } from '@/types'
import type { CodexTurnStateObservation } from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import { buildAccountRows, filterAccountRows, isHealthyObservation, probePhase } from '../presentation/composables/stateDiagnosticsTransforms'

function account(id: number, overrides: Partial<Account> = {}): Account {
  return {
    id,
    name: `account-${id}`,
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 4,
    priority: 0,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '',
    updated_at: '',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

function observation(accountId: number, overrides: Partial<CodexTurnStateObservation> = {}): CodexTurnStateObservation {
  return {
    account_id: accountId,
    model: 'gpt-5.6-codex',
    source: 'response',
    proxy_enabled: false,
    last_response_had_state: true,
    length_match: true,
    state_digest: 'sha256:test',
    state: {
      valid: true,
      expired: false,
      version: 128,
      token_characters: 292,
      token_bytes: 219,
      token_bytes_known: true,
    },
    encrypted_content: {
      last_bytes: 20,
      last_bytes_known: true,
      baseline_bytes: 20,
      delta_bytes: 0,
      classification: 'baseline',
    },
    rotation: { count: 0 },
    probe: { in_flight: false },
    ...overrides,
  }
}

describe('state diagnostics transforms', () => {
  it('keeps unobserved and blocked accounts visible beside healthy observations', () => {
    const rows = buildAccountRows(
      [account(1), account(2, { schedulable: false }), account(3)],
      [observation(1)],
    )

    expect(rows.map((row) => [row.account.id, row.health])).toEqual([
      [2, 'blocked'],
      [3, 'unobserved'],
      [1, 'ready'],
    ])
  })

  it('marks malformed, expired, missing, or rotated observations as attention', () => {
    const cases = [
      { state: { ...observation(1).state, expired: true } },
      { state: { ...observation(1).state, valid: false } },
      { length_match: false },
      { last_response_had_state: false },
      { rotation: { count: 1 } },
    ]
    for (const overrides of cases) {
      const item = observation(1, overrides)
      expect(isHealthyObservation(item)).toBe(false)
      expect(buildAccountRows([account(1)], [item])[0]?.health).toBe('attention')
    }
  })

  it('filters rows by account text, model, and health', () => {
    const rows = buildAccountRows([account(1), account(2)], [observation(1)])
    expect(filterAccountRows(rows, 'account-1', 'all', '')).toHaveLength(1)
    expect(filterAccountRows(rows, '', 'unobserved', '')[0]?.account.id).toBe(2)
    expect(filterAccountRows(rows, '', 'all', 'gpt-5.6-codex')[0]?.account.id).toBe(1)
  })

  it('distinguishes running, queued, scheduled, and unscheduled probes', () => {
    const now = Date.parse('2026-09-18T12:00:00Z')
    expect(probePhase(observation(1, { probe: { in_flight: true } }), now)).toBe('running')
    expect(probePhase(observation(1, { probe: { in_flight: false, next_probe_at: '2026-09-18T11:59:59Z' } }), now)).toBe('queued')
    expect(probePhase(observation(1, { probe: { in_flight: false, next_probe_at: '2026-09-18T12:00:01Z' } }), now)).toBe('scheduled')
    expect(probePhase(observation(1), now)).toBe('unscheduled')
  })
})
