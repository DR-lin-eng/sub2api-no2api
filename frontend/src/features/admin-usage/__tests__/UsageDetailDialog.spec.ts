import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { AdminUsageLog } from '@/types'

const messages: Record<string, string> = {
  'usage.sync': 'Synchronous',
  'usage.detail.balanceBilling': 'Balance',
  'usage.detail.subscriptionBilling': 'Subscription',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('@/common/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }),
}))

import UsageDetailDialog from '@/features/admin-usage/presentation/widgets/UsageDetailDialog.vue'

const usage = {
  id: 41,
  user_id: 7,
  api_key_id: 8,
  account_id: 9,
  request_id: 'visible-request-id',
  session_id: 'visible-session-id',
  model: 'visible-request-model',
  service_tier: 'priority',
  reasoning_effort: 'high',
  inbound_endpoint: '/v1/visible-inbound',
  upstream_endpoint: '/private/upstream-endpoint',
  group_id: 10,
  subscription_id: null,
  input_tokens: 100,
  output_tokens: 20,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: 0.01,
  output_cost: 0.02,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  total_cost: 0.03,
  actual_cost: 0.03,
  rate_multiplier: 1,
  long_context_billing_applied: false,
  billing_type: 0,
  request_type: 'sync',
  stream: false,
  openai_ws_mode: false,
  duration_ms: 500,
  first_token_ms: 100,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  user_agent: 'visible-user-agent',
  ip_address: '203.0.113.88',
  cache_ttl_overridden: false,
  billing_mode: 'token',
  created_at: '2026-08-17T00:00:00Z',
  api_key: { name: 'visible-api-key' },
  group: { id: 10, name: 'visible-group-name' },
  upstream_model: 'private-upstream-model',
  upstream_response_model: 'private-response-model',
  upstream_model_mismatch: true,
  model_mapping_chain: 'visible-request-model→private-mapped-model',
  channel_id: 987654321,
  billing_tier: 'private-billing-tier',
  account_rate_multiplier: 1.75,
  account_stats_cost: 0.7,
  account: { id: 9981, name: 'private-account-name' },
} as AdminUsageLog

const mountDialog = (audience: 'user' | 'admin') => mount(UsageDetailDialog, {
  props: {
    show: true,
    usage,
    audience,
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show'],
        template: '<div v-if="show"><slot /></div>',
      },
      Icon: true,
    },
  },
})

describe('UsageDetailDialog audience boundary', () => {
  it('shows user-owned request and billing fields without administrator internals', () => {
    const text = mountDialog('user').text()

    expect(text).toContain('visible-request-id')
    expect(text).toContain('visible-api-key')
    expect(text).toContain('visible-request-model')
    expect(text).toContain('/v1/visible-inbound')
    expect(text).toContain('visible-group-name')
    expect(text).not.toContain('/private/upstream-endpoint')
    expect(text).not.toContain('private-upstream-model')
    expect(text).not.toContain('private-mapped-model')
    expect(text).not.toContain('private-account-name')
    expect(text).not.toContain('#987654321')
    expect(text).not.toContain('private-billing-tier')
  })

  it('shows administrator diagnostics for admin audiences', () => {
    const text = mountDialog('admin').text()

    expect(text).toContain('/private/upstream-endpoint')
    expect(text).toContain('private-upstream-model')
    expect(text).toContain('private-mapped-model')
    expect(text).toContain('private-account-name')
    expect(text).toContain('#987654321')
    expect(text).toContain('private-billing-tier')
  })
})
