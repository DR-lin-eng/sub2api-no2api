export interface GrokModelDiagnostics {
  state: 'unknown' | 'fresh' | 'stale' | 'source_changed'
  fetched_at?: string
  expires_at?: string
  models: string[]
  mappings: Array<{ requested: string; upstream?: string; status: 'listed' | 'not_listed' | 'unverifiable' | 'out_of_scope' }>
}
