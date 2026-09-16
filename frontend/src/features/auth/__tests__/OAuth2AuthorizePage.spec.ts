import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getPreview, submitAuthorization } = vi.hoisted(() => ({
  getPreview: vi.fn(),
  submitAuthorization: vi.fn(),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {
      client_id: 's2c_test',
      redirect_uri: 'https://client.example.test/callback',
      response_type: 'code',
      scope: 'profile email',
      state: 'state-123',
      code_challenge: 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~',
      code_challenge_method: 'S256',
    },
  }),
}))
vi.mock('@/features/auth/data/datasources/oauth2ProviderDatasource', () => ({
  getOAuth2AuthorizationPreview: getPreview,
  submitOAuth2Authorization: submitAuthorization,
}))

import OAuth2AuthorizePage from '@/features/auth/presentation/pages/OAuth2AuthorizePage.vue'

describe('OAuth2AuthorizePage', () => {
  beforeEach(() => {
    getPreview.mockReset().mockResolvedValue({
      client_id: 's2c_test',
      client_name: 'Analytics console',
      redirect_uri: 'https://client.example.test/callback',
      scopes: [
        { name: 'profile', description: 'profile' },
        { name: 'email', description: 'email' },
      ],
    })
    submitAuthorization.mockReset().mockRejectedValue(new Error('redirect stopped in test'))
  })

  it('loads a server-validated client and scope preview', async () => {
    const wrapper = mount(OAuth2AuthorizePage, {
      global: { stubs: { AuthLayout: { template: '<main><slot /></main>' }, Icon: true } },
    })
    await flushPromises()

    expect(getPreview).toHaveBeenCalledWith(expect.objectContaining({
      client_id: 's2c_test',
      redirect_uri: 'https://client.example.test/callback',
      state: 'state-123',
      code_challenge_method: 'S256',
    }))
    expect(wrapper.text()).toContain('Analytics console')
    expect(wrapper.text()).toContain('profile')
    expect(wrapper.text()).toContain('email')
  })

  it('submits an explicit resource-owner decision', async () => {
    const wrapper = mount(OAuth2AuthorizePage, {
      global: { stubs: { AuthLayout: { template: '<main><slot /></main>' }, Icon: true } },
    })
    await flushPromises()
    const allow = wrapper.findAll('button').find((button) => button.text().includes('oauth2Consent.allow'))
    expect(allow).toBeDefined()
    await allow?.trigger('click')
    await flushPromises()

    expect(submitAuthorization).toHaveBeenCalledWith(expect.objectContaining({
      client_id: 's2c_test',
      state: 'state-123',
    }), true)
  })
})
