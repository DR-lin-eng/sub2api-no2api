import type { AccountPlatform, AccountType } from '@/types/gateway'

// Returned by the account models API and the compatible models endpoint.
export interface ClaudeModel {
  id: string
  type: string
  display_name: string
  created_at: string
}

export interface TempUnschedulableRule {
  scope?: 'account' | 'model'
  error_code: number
  keywords: string[]
  duration_minutes: number
  description: string
}

export interface TempUnschedulableState {
  scope?: 'account' | 'model'
  model?: string
  until_unix: number
  triggered_at_unix: number
  status_code: number
  matched_keyword: string
  rule_index: number
  error_message: string
}

export interface TempUnschedulableStatus {
  active: boolean
  state?: TempUnschedulableState
}

export interface CreateAccountRequest {
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id?: number | null
  concurrency?: number
  load_factor?: number | null
  priority?: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  group_ids?: number[]
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  upstream_billing_probe_enabled?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface UpdateAccountRequest {
  name?: string
  notes?: string | null
  type?: AccountType
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id?: number | null
  concurrency?: number
  load_factor?: number | null
  priority?: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  schedulable?: boolean
  status?: 'active' | 'inactive' | 'error'
  group_ids?: number[]
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  upstream_billing_probe_enabled?: boolean
  upstream_billing_rate_sync_enabled?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface CheckMixedChannelRequest {
  platform: AccountPlatform
  group_ids: number[]
  account_id?: number
}

export interface MixedChannelWarningDetails {
  group_id: number
  group_name: string
  current_platform: string
  other_platform: string
}

export interface CheckMixedChannelResponse {
  has_risk: boolean
  error?: string
  message?: string
  details?: MixedChannelWarningDetails
}

export interface CodexSessionImportRequest {
  content?: string
  contents?: string[]
  name?: string
  notes?: string | null
  group_ids?: number[]
  proxy_id?: number | null
  concurrency?: number
  priority?: number
  rate_multiplier?: number
  load_factor?: number | null
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  credential_extras?: Record<string, unknown>
  extra?: Record<string, unknown>
  update_existing?: boolean
  skip_default_group_bind?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface OpenAICodexPATCreateRequest {
  access_token: string
  name?: string
  notes?: string | null
  group_ids?: number[]
  proxy_id?: number | null
  concurrency?: number
  priority?: number
  rate_multiplier?: number
  load_factor?: number | null
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  credential_extras?: Record<string, unknown>
  extra?: Record<string, unknown>
  skip_default_group_bind?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface CodexSessionImportMessage {
  index: number
  name?: string
  message: string
}

export interface CodexSessionImportItem {
  index: number
  name?: string
  action: 'created' | 'updated' | 'skipped' | 'failed'
  account_id?: number
  message?: string
}

export interface CodexSessionImportResult {
  total: number
  created: number
  updated: number
  skipped: number
  failed: number
  items?: CodexSessionImportItem[]
  warnings?: CodexSessionImportMessage[]
  errors?: CodexSessionImportMessage[]
}
