import type { UsageBillingCostLine } from '@/core/utils/usageBillingCalculation'

type Translate = (key: string) => string
type UsageBillingCostLineKey = UsageBillingCostLine['key']

const costLineMetadata = {
  input: { key: 'usage.detail.costLines.input', order: 0 },
  image_input: { key: 'usage.detail.costLines.image_input', order: 1 },
  output: { key: 'usage.detail.costLines.output', order: 2 },
  image_output: { key: 'usage.detail.costLines.image_output', order: 3 },
  cache_creation: { key: 'usage.detail.costLines.cache_creation', order: 4 },
  cache_read: { key: 'usage.detail.costLines.cache_read', order: 5 },
  request: { key: 'usage.detail.costLines.request', order: 6 },
  image: { key: 'usage.detail.costLines.image', order: 7 },
  video: { key: 'usage.detail.costLines.video', order: 8 },
} as const satisfies Record<UsageBillingCostLineKey, { key: string; order: number }>

const errorCategoryKeys = {
  auth: 'usage.errors.categories.auth',
  rate_limit: 'usage.errors.categories.rate_limit',
  quota: 'usage.errors.categories.quota',
  invalid_request: 'usage.errors.categories.invalid_request',
  service_unavailable: 'usage.errors.categories.service_unavailable',
  upstream: 'usage.errors.categories.upstream',
  internal: 'usage.errors.categories.internal',
  other: 'usage.errors.categories.other',
  cyber: 'usage.errors.categories.cyber',
} as const

export const usageBillingCostLineValues = (
  Object.keys(costLineMetadata) as UsageBillingCostLineKey[]
).sort((left, right) => (
  costLineMetadata[left].order - costLineMetadata[right].order
))

export function usageBillingCostLineLabel(t: Translate, value: unknown): string {
  const normalized = String(value ?? '').trim() as UsageBillingCostLineKey
  return Object.prototype.hasOwnProperty.call(costLineMetadata, normalized)
    ? t(costLineMetadata[normalized].key)
    : t('common.unknown')
}

export function usageErrorCategoryLabel(t: Translate, value: unknown): string {
  const normalized = String(value ?? '').trim() as keyof typeof errorCategoryKeys
  return Object.prototype.hasOwnProperty.call(errorCategoryKeys, normalized)
    ? t(errorCategoryKeys[normalized])
    : t('common.unknown')
}
