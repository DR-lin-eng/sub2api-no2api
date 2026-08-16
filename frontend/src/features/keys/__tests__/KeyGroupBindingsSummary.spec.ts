import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ApiKey } from '@/types'
import type { GroupOption } from '../presentation/keysPageContext'
import KeyGroupBindingsSummary from '../presentation/widgets/KeyGroupBindingsSummary.vue'

const messages: Record<string, string> = {
  'keys.groupBindings.primary': 'Primary',
  'keys.groupBindings.fallback': 'Fallback',
  'keys.groupBindings.fallbackPosition': 'Fallback {position}',
  'keys.groupBindings.singleGroup': 'No fallback groups configured',
  'keys.groupBindings.notConfigured': 'No routing groups',
  'keys.groupBindings.configure': 'Configure',
  'keys.groupBindings.manage': 'Manage',
  'keys.groupBindings.manageFor': 'Manage routing groups for {name}',
  'keys.groupBindings.manageEmpty': 'No routing groups configured',
  'keys.groupBindings.moreGroups': '+{count}',
  'keys.groupBindings.rateCeilingShort': '≤{rate}x',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params: Record<string, string | number> = {}) => {
        let message = messages[key] ?? key
        for (const [name, value] of Object.entries(params)) {
          message = message.replace(`{${name}}`, String(value))
        }
        return message
      }
    })
  }
})

const options: GroupOption[] = [
  { value: 1, label: 'Primary One', description: null, rate: 1, userRate: null, peakRateEnabled: false, peakStart: '', peakEnd: '', peakRateMultiplier: 1, subscriptionType: 'standard', platform: 'openai' },
  { value: 2, label: 'Fallback Two', description: null, rate: 1.2, userRate: null, peakRateEnabled: false, peakStart: '', peakEnd: '', peakRateMultiplier: 1, subscriptionType: 'standard', platform: 'openai' },
  { value: 3, label: 'Fallback Three', description: null, rate: 1.4, userRate: null, peakRateEnabled: false, peakStart: '', peakEnd: '', peakRateMultiplier: 1, subscriptionType: 'standard', platform: 'openai' },
  { value: 4, label: 'Fallback Four', description: null, rate: 1.6, userRate: null, peakRateEnabled: false, peakStart: '', peakEnd: '', peakRateMultiplier: 1, subscriptionType: 'standard', platform: 'openai' },
]

const createKey = (overrides: Partial<ApiKey> = {}): ApiKey => ({
  id: 9,
  user_id: 1,
  key: 'sk-test',
  name: 'route-key',
  group_id: null,
  group_bindings: [],
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-08-16T00:00:00Z',
  updated_at: '2026-08-16T00:00:00Z',
  concurrency_limit: 0,
  current_concurrency: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
  ...overrides,
})

const mountSummary = (apiKey: ApiKey) => mount(KeyGroupBindingsSummary, {
  props: { apiKey, groupOptions: options },
  global: {
    stubs: {
      GroupBadge: {
        props: ['name', 'rateMultiplier'],
        template: '<span>{{ name }} {{ rateMultiplier }}x</span>',
      },
      Icon: true,
    },
  },
})

describe('KeyGroupBindingsSummary', () => {
  it('shows the primary group, ordered fallbacks, ceilings, and hidden count', async () => {
    const wrapper = mountSummary(createKey({
      group_id: 1,
      group_bindings: [
        { group_id: 1, max_rate_multiplier: null },
        { group_id: 2, max_rate_multiplier: 1.5 },
        { group_id: 3, max_rate_multiplier: null },
        { group_id: 4, max_rate_multiplier: 2 },
      ],
    }))

    expect(wrapper.text()).toContain('Primary')
    expect(wrapper.text()).toContain('Primary One 1x')
    expect(wrapper.text()).toContain('Fallback')
    expect(wrapper.text()).toContain('Fallback Two')
    expect(wrapper.text()).toContain('≤1.5x')
    expect(wrapper.text()).toContain('Fallback Three')
    expect(wrapper.text()).toContain('+1')
    expect(wrapper.text()).not.toContain('Fallback Four')
    expect(wrapper.get('button').attributes('title')).toContain('Fallback Four')

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('manage')).toHaveLength(1)
  })

  it('keeps legacy group_id-only keys readable as a single primary group', () => {
    const wrapper = mountSummary(createKey({ group_id: 1 }))

    expect(wrapper.text()).toContain('Primary One 1x')
    expect(wrapper.text()).toContain('No fallback groups configured')
  })

  it('shows a clear configuration entry when the key has no groups', () => {
    const wrapper = mountSummary(createKey())

    expect(wrapper.text()).toContain('No routing groups')
    expect(wrapper.text()).toContain('Configure')
  })
})
