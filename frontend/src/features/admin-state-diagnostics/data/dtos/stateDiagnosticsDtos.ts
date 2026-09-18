import type { Account } from '@/types'
import type {
  CodexTurnStateObservation,
  CodexTurnStateObservability,
} from '@/features/admin-settings/data/dtos/adminSettingsDtos'

export type StateDiagnosticsFilter = 'all' | 'ready' | 'attention' | 'unobserved' | 'blocked'

export type StateDiagnosticsHealth = Exclude<StateDiagnosticsFilter, 'all'>

export interface StateDiagnosticsAccountRow {
  account: Account
  observations: CodexTurnStateObservation[]
  health: StateDiagnosticsHealth
  healthyObservations: number
  attentionObservations: number
  lastSeenAt?: string
}
export interface StateDiagnosticsViewModel {
  snapshot: CodexTurnStateObservability | null
  accounts: StateDiagnosticsAccountRow[]
}
