import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UserDashboardApiKeyUsage from '../UserDashboardApiKeyUsage.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('UserDashboardApiKeyUsage', () => {
  it('renders per-key token totals and actual spend', () => {
    const wrapper = mount(UserDashboardApiKeyUsage, {
      props: {
        loading: false,
        error: false,
        rows: [
          { id: 1, name: 'Production', totalTokens: 12345, actualSpend: 1.23456 },
          { id: 2, name: 'Unused', totalTokens: 0, actualSpend: 0 }
        ]
      },
      global: { stubs: { LoadingSpinner: true, EmptyState: true } }
    })

    expect(wrapper.text()).toContain('Production')
    expect(wrapper.text()).toContain('12,345')
    expect(wrapper.text()).toContain('$1.2346')
    expect(wrapper.text()).toContain('Unused')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
  })

  it('renders the empty state when the user owns no keys', () => {
    const wrapper = mount(UserDashboardApiKeyUsage, {
      props: { loading: false, error: false, rows: [] },
      global: {
        stubs: {
          LoadingSpinner: true,
          EmptyState: { props: ['title', 'description'], template: '<div>{{ title }} {{ description }}</div>' }
        }
      }
    })

    expect(wrapper.text()).toContain('dashboard.noApiKeys')
    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('distinguishes failure from an empty key list and emits retry', async () => {
    const wrapper = mount(UserDashboardApiKeyUsage, {
      props: { loading: false, error: true, rows: [] },
      global: { stubs: { LoadingSpinner: true, EmptyState: true, Icon: true } }
    })

    expect(wrapper.text()).toContain('dashboard.apiKeyUsageLoadFailed')
    expect(wrapper.text()).not.toContain('dashboard.noApiKeys')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})
