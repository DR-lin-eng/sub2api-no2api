import type { CPAOAuthOptions, CPASyncPlatform } from '../data/dtos/cpaSyncDtos'

export function defaultCPAOAuthOptions(platform: CPASyncPlatform): CPAOAuthOptions {
  const common = { tls_fingerprint: false }
  if (platform === 'openai') {
    return {
      ...common,
      passthrough: false,
      flatten_namespaces: false,
      prewarm_continuation: false,
      codex_cli_only: false,
      allow_app_server: false,
      long_context_billing: false,
      ws_mode: 'off',
      fingerprint_mode: 'off',
      compact_mode: 'auto'
    }
  }
  if (platform === 'anthropic') {
    return { ...common, session_id_masking: false, intercept_warmup: false }
  }
  return common
}
