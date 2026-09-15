import type { AccountPlatform } from '@/types'

export const CN_ACCOUNT_PLATFORMS = ['kimi', 'zhipu', 'deepseek', 'minimax'] as const
export type CNAccountPlatform = (typeof CN_ACCOUNT_PLATFORMS)[number]

export const CN_ACCOUNT_PLATFORM_LABELS: Record<CNAccountPlatform, string> = {
  kimi: 'Kimi',
  zhipu: 'Zhipu GLM',
  deepseek: 'DeepSeek',
  minimax: 'MiniMax',
}

export function isCNAccountPlatform(platform: string): platform is CNAccountPlatform {
  return (CN_ACCOUNT_PLATFORMS as readonly string[]).includes(platform)
}

export function defaultAPIKeyBaseURL(platform: AccountPlatform): string {
  switch (platform) {
    case 'openai': return 'https://api.openai.com'
    case 'gemini': return 'https://generativelanguage.googleapis.com'
    case 'antigravity': return 'https://cloudcode-pa.googleapis.com'
    case 'grok': return 'https://api.x.ai/v1'
    case 'kimi': return 'https://api.moonshot.cn/v1'
    case 'zhipu': return 'https://open.bigmodel.cn/api/paas/v4'
    case 'deepseek': return 'https://api.deepseek.com'
    case 'minimax': return 'https://api.minimaxi.com/v1'
    default: return 'https://api.anthropic.com'
  }
}

export function defaultAPIKeyPlaceholder(platform: AccountPlatform): string {
  switch (platform) {
    case 'anthropic': return 'sk-ant-...'
    case 'openai': return 'sk-proj-...'
    case 'gemini': return 'AIza...'
    case 'grok': return 'xai-...'
    case 'kimi':
    case 'deepseek':
    case 'antigravity': return 'sk-...'
    default: return 'API Key'
  }
}

/** WebSearch emulation mode values (must match backend WebSearchMode* constants in account.go) */
export const WEB_SEARCH_MODE_DEFAULT = 'default' as const
export const WEB_SEARCH_MODE_ENABLED = 'enabled' as const
export const WEB_SEARCH_MODE_DISABLED = 'disabled' as const
export type WebSearchMode = typeof WEB_SEARCH_MODE_DEFAULT | typeof WEB_SEARCH_MODE_ENABLED | typeof WEB_SEARCH_MODE_DISABLED

/** Quota notification threshold type values (must match thresholdType* constants in balance_notify_service.go) */
export const QUOTA_THRESHOLD_TYPE_FIXED = 'fixed' as const
export const QUOTA_THRESHOLD_TYPE_PERCENTAGE = 'percentage' as const
export type QuotaThresholdType = typeof QUOTA_THRESHOLD_TYPE_FIXED | typeof QUOTA_THRESHOLD_TYPE_PERCENTAGE

/** Quota reset mode values */
export const QUOTA_RESET_MODE_ROLLING = 'rolling' as const
export const QUOTA_RESET_MODE_FIXED = 'fixed' as const
export type QuotaResetMode = typeof QUOTA_RESET_MODE_ROLLING | typeof QUOTA_RESET_MODE_FIXED

/** Vertex AI location options for Service Account accounts */
export const VERTEX_LOCATION_OPTIONS = [
  {
    label: 'Common',
    options: [
      { value: 'us-central1', label: 'us-central1 (Iowa)' },
      { value: 'global', label: 'global' },
      { value: 'us', label: 'us' },
      { value: 'eu', label: 'eu' }
    ]
  },
  {
    label: 'United States',
    options: [
      { value: 'us-east1', label: 'us-east1 (South Carolina)' },
      { value: 'us-east4', label: 'us-east4 (Northern Virginia)' },
      { value: 'us-east5', label: 'us-east5 (Columbus)' },
      { value: 'us-south1', label: 'us-south1 (Dallas)' },
      { value: 'us-west1', label: 'us-west1 (Oregon)' },
      { value: 'us-west4', label: 'us-west4 (Las Vegas)' }
    ]
  },
  {
    label: 'Europe',
    options: [
      { value: 'europe-west1', label: 'europe-west1 (Belgium)' },
      { value: 'europe-west2', label: 'europe-west2 (London)' },
      { value: 'europe-west3', label: 'europe-west3 (Frankfurt)' },
      { value: 'europe-west4', label: 'europe-west4 (Netherlands)' },
      { value: 'europe-west6', label: 'europe-west6 (Zurich)' },
      { value: 'europe-west8', label: 'europe-west8 (Milan)' },
      { value: 'europe-west9', label: 'europe-west9 (Paris)' }
    ]
  },
  {
    label: 'Asia Pacific',
    options: [
      { value: 'asia-east1', label: 'asia-east1 (Taiwan)' },
      { value: 'asia-east2', label: 'asia-east2 (Hong Kong)' },
      { value: 'asia-northeast1', label: 'asia-northeast1 (Tokyo)' },
      { value: 'asia-northeast3', label: 'asia-northeast3 (Seoul)' },
      { value: 'asia-south1', label: 'asia-south1 (Mumbai)' },
      { value: 'asia-southeast1', label: 'asia-southeast1 (Singapore)' },
      { value: 'australia-southeast1', label: 'australia-southeast1 (Sydney)' }
    ]
  }
] as const
