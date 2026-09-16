import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getConfig, updateConfig, showSuccess, showError, stepUpRun } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  stepUpRun: vi.fn(async (action: () => Promise<unknown>) => action()),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key }),
}))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('@/features/admin-settings/presentation/composables/settingsPageContext', () => ({
  useSettingsPageContext: () => ({ settingsStepUp: { run: stepUpRun } }),
}))
vi.mock('@/features/admin-settings/data/datasources/oauth2ProviderQueries', () => ({ getOAuth2ProviderConfig: getConfig }))
vi.mock('@/features/admin-settings/data/datasources/oauth2ProviderActions', () => ({
  updateOAuth2ProviderConfig: updateConfig,
  createOAuth2ProviderClient: vi.fn(),
  updateOAuth2ProviderClient: vi.fn(),
  rotateOAuth2ProviderClientSecret: vi.fn(),
  deleteOAuth2ProviderClient: vi.fn(),
}))

import SettingsOAuth2ProviderPanel from '@/features/admin-settings/presentation/widgets/settings-tabs/SettingsOAuth2ProviderPanel.vue'

const providerConfig = {
  enabled: false,
  issuer: 'https://issuer.example.test',
  access_token_ttl_seconds: 3600,
  scopes: [
    { name: 'profile', description: 'profile' },
    { name: 'email', description: 'email' },
  ],
  clients: [{
    client_id: 's2c_test',
    name: 'Analytics console',
    client_type: 'confidential',
    redirect_uris: ['https://client.example.test/callback'],
    allowed_scopes: ['profile'],
    enabled: true,
    secret_configured: true,
    created_at: '2026-09-15T00:00:00Z',
    updated_at: '2026-09-15T00:00:00Z',
  }],
}

describe('SettingsOAuth2ProviderPanel', () => {
  beforeEach(() => {
    getConfig.mockReset().mockResolvedValue(structuredClone(providerConfig))
    updateConfig.mockReset().mockImplementation(async (value) => ({ ...structuredClone(providerConfig), ...value }))
    showSuccess.mockReset()
    showError.mockReset()
    stepUpRun.mockClear()
  })

  it('loads provider state and registered clients', async () => {
    const wrapper = mount(SettingsOAuth2ProviderPanel, {
      global: { stubs: { Icon: true, Toggle: true, BaseDialog: true } },
    })
    await flushPromises()

    expect(getConfig).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Analytics console')
    expect(wrapper.text()).toContain('s2c_test')
    expect(wrapper.text()).not.toContain('client_secret')
  })

  it('saves the global switch through the settings step-up controller', async () => {
    const wrapper = mount(SettingsOAuth2ProviderPanel, {
      global: { stubs: { Icon: true, Toggle: true, BaseDialog: true } },
    })
    await flushPromises()
    const save = wrapper.findAll('button').find((button) => button.text().includes('common.save'))
    expect(save).toBeDefined()
    await save?.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(updateConfig).toHaveBeenCalledWith({
      enabled: false,
      issuer: 'https://issuer.example.test',
      access_token_ttl_seconds: 3600,
    })
    expect(showSuccess).toHaveBeenCalled()
  })
})
