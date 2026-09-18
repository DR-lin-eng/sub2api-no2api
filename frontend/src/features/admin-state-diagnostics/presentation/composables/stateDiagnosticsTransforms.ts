import type { Account } from '@/types'
import type { CodexTurnStateObservation } from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import type {
  StateDiagnosticsAccountRow,
  StateDiagnosticsFilter,
  StateDiagnosticsHealth,
} from '../../data/dtos/stateDiagnosticsDtos'

export type StateDiagnosticsProbePhase = 'running' | 'queued' | 'scheduled' | 'unscheduled'

export function probePhase(item: CodexTurnStateObservation, now = Date.now()): StateDiagnosticsProbePhase {
  if (item.probe.in_flight) return 'running'
  if (!item.probe.next_probe_at) return 'unscheduled'
  const nextProbeAt = Date.parse(item.probe.next_probe_at)
  if (Number.isNaN(nextProbeAt)) return 'unscheduled'
  return nextProbeAt <= now ? 'queued' : 'scheduled'
}

export function isHealthyObservation(item: CodexTurnStateObservation): boolean {
  return (
    item.state.valid &&
    !item.state.expired &&
    item.length_match &&
    item.rotation.count === 0 &&
    item.last_response_had_state
  )
}

export function observationHealth(item: CodexTurnStateObservation): 'ready' | 'attention' {
  return isHealthyObservation(item) ? 'ready' : 'attention'
}

function compareRows(left: StateDiagnosticsAccountRow, right: StateDiagnosticsAccountRow): number {
  const priority: Record<StateDiagnosticsHealth, number> = {
    attention: 0,
    blocked: 1,
    unobserved: 2,
    ready: 3,
  }
  const healthOrder = priority[left.health] - priority[right.health]
  if (healthOrder !== 0) return healthOrder
  return left.account.name.localeCompare(right.account.name, undefined, { sensitivity: 'base' })
}

function accountHealth(account: Account, observations: CodexTurnStateObservation[]): StateDiagnosticsHealth {
  if (account.status !== 'active' || !account.schedulable) return 'blocked'
  if (observations.length === 0) return 'unobserved'
  return observations.some((item) => observationHealth(item) === 'attention') ? 'attention' : 'ready'
}

function syntheticAccount(accountId: number): Account {
  return {
    id: accountId,
    name: `#${accountId}`,
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 0,
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
  }
}

export function buildAccountRows(
  accounts: Account[],
  observations: CodexTurnStateObservation[],
): StateDiagnosticsAccountRow[] {
  const byAccount = new Map<number, CodexTurnStateObservation[]>()
  for (const item of observations) {
    const current = byAccount.get(item.account_id) ?? []
    current.push(item)
    byAccount.set(item.account_id, current)
  }

  const accountByID = new Map(accounts.map((account) => [account.id, account]))
  for (const item of observations) {
    if (!accountByID.has(item.account_id)) accountByID.set(item.account_id, syntheticAccount(item.account_id))
  }

  return [...accountByID.values()]
    .map((account) => {
      const accountObservations = (byAccount.get(account.id) ?? []).sort((left, right) => left.model.localeCompare(right.model))
      const healthyObservations = accountObservations.filter(isHealthyObservation).length
      const seenAtValues = accountObservations
        .map((item) => item.last_seen_at)
        .filter((value): value is string => Boolean(value))
        .sort()
      const lastSeenAt = seenAtValues.length ? seenAtValues[seenAtValues.length - 1] : undefined
      return {
        account,
        observations: accountObservations,
        health: accountHealth(account, accountObservations),
        healthyObservations,
        attentionObservations: accountObservations.length - healthyObservations,
        lastSeenAt,
      }
    })
    .sort(compareRows)
}

export function filterAccountRows(
  rows: StateDiagnosticsAccountRow[],
  search: string,
  filter: StateDiagnosticsFilter,
  model: string,
): StateDiagnosticsAccountRow[] {
  const normalizedSearch = search.trim().toLowerCase()
  return rows.filter((row) => {
    if (filter !== 'all' && row.health !== filter) return false
    if (model && !row.observations.some((item) => item.model === model)) return false
    if (!normalizedSearch) return true
    return [
      row.account.name,
      String(row.account.id),
      ...row.observations.map((item) => item.model),
    ].some((value) => value.toLowerCase().includes(normalizedSearch))
  })
}
