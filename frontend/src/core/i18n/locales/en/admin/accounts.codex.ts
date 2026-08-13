export default {
  flattenNamespaces: 'Flatten Codex namespace tools',
  flattenNamespacesDesc:
    'OAuth compatibility switch for relays that reject namespace tools. Disabled by default so native Codex requests preserve namespace declarations; compact requests always use flattened tools.',
  codexFingerprintMode: 'Codex fingerprint convergence',
  codexFingerprintModeDesc:
    'Opt-in for OpenAI OAuth accounts. Existing and newly created accounts remain off unless an administrator selects a convergence mode.',
  codexFingerprintModeOff: 'Off (preserve client identifiers)',
  codexFingerprintModeDevice: 'Device only',
  codexFingerprintModeSession: 'Device and session',
  codexFingerprintModeFull: 'Device, session, and thread',
  codexPrewarmContinuation: 'Codex account prewarm',
  codexThinkingTagNormalization: 'Normalize Codex thinking tags',
  codexThinkingTagNormalizationDesc:
    'For OpenAI API-key Responses fallback, convert a leading <thinking>...</thinking> block into a Codex reasoning item. Disabled by default.',
}
