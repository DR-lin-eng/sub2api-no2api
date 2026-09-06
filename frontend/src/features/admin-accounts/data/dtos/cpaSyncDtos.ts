import type { OpenAICompactMode } from '@/types'
import type { OpenAIWSMode } from '@/core/utils/openaiWsMode'

export type CPASyncPlatform = 'openai' | 'anthropic' | 'gemini' | 'antigravity'
export interface CPAConnectionParams {
  base_url: string
  management_password: string
  platform: CPASyncPlatform
}
export interface CPAOAuthOptions {
  tls_fingerprint?: boolean
  session_id_masking?: boolean
  intercept_warmup?: boolean
  passthrough?: boolean
  flatten_namespaces?: boolean
  prewarm_continuation?: boolean
  codex_cli_only?: boolean
  allow_app_server?: boolean
  long_context_billing?: boolean
  ws_mode?: OpenAIWSMode
  fingerprint_mode?: 'off' | 'device' | 'session' | 'full'
  compact_mode?: OpenAICompactMode
}
export interface CPAPreviewAccount {
  file_name: string
  name: string
  email?: string
  platform: CPASyncPlatform
  existing: boolean
  account_id?: number
}
export interface CPAPreviewResult {
  accounts: CPAPreviewAccount[]
  total: number
  skipped: number
  skip_reasons: Record<string, number>
  batch_size: number
}
export interface CPASyncParams extends CPAConnectionParams {
  selected_files: string[]
  group_ids: number[]
  oauth_options: CPAOAuthOptions
  apply_settings_to_existing: boolean
}
export interface CPASyncItem {
  file_name: string
  action: 'created' | 'updated' | 'skipped' | 'failed'
  reason?: string
  message?: string
  account_id?: number
}
export interface CPASyncResult {
  created: number
  updated: number
  skipped: number
  failed: number
  items: CPASyncItem[]
}
