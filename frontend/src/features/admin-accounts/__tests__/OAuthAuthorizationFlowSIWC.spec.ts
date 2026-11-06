import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OAuthAuthorizationFlow from '../presentation/widgets/OAuthAuthorizationFlow.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))

describe('SIWC authorization input', () => {
  it('keeps the complete callback and clears it when changing protocols', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: { addMethod: 'oauth', platform: 'openai', showCookieOption: false, showSiwcOption: true },
      global: { stubs: { Icon: true } }
    })
    await wrapper.get('input[value="siwc"]').setValue(true)
    expect(wrapper.find('[data-testid="siwc-description"]').exists()).toBe(true)
    const callback = 'http://127.0.0.1:1455/auth/callback?code=issued-code&state=state&client_id=oaiapp_issued'
    await wrapper.get('textarea').setValue(callback)
    expect(wrapper.get('textarea').element.value).toBe(callback)
    expect((wrapper.vm as unknown as { authCode: string }).authCode).toBe(callback)
    await wrapper.get('input[value="manual"]').setValue(true)
    expect(wrapper.get('textarea').element.value).toBe('')
    expect(wrapper.emitted('reset-authorization')).toHaveLength(2)
    wrapper.unmount()
  })

  it('does not expose SIWC for another platform', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: { addMethod: 'oauth', platform: 'anthropic', showSiwcOption: true },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.find('input[value="siwc"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
