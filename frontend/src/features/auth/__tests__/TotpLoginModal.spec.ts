import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TotpLoginModal from '@/features/auth/presentation/widgets/TotpLoginDialog.vue'

const { showErrorMock } = vi.hoisted(() => ({
  showErrorMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

describe('TotpLoginModal', () => {
  beforeEach(() => {
    showErrorMock.mockReset()
  })

  it('sends verification errors to toast and does not render inline red text', async () => {
    const wrapper = mount(TotpLoginModal, {
      props: {
        tempToken: 'temp-token',
        userEmailMasked: 'u***@example.com',
      },
    })

    ;(wrapper.vm as unknown as { setError: (message: string) => void }).setError('Invalid code')
    await wrapper.vm.$nextTick()

    expect(showErrorMock).toHaveBeenCalledWith('Invalid code')
    expect(wrapper.text()).not.toContain('Invalid code')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })

  it('waits for mobile IME composition to finish before sanitizing or advancing OTP cells', async () => {
    const wrapper = mount(TotpLoginModal, {
      attachTo: document.body,
      props: {
        tempToken: 'temp-token',
      },
    })
    const cells = wrapper.findAll('input[pattern="[0-9]"]')
    const first = cells[0]

    await first.trigger('focus')
    await first.trigger('compositionstart')
    ;(first.element as HTMLInputElement).value = 'n'
    await first.trigger('input')

    expect((first.element as HTMLInputElement).value).toBe('n')
    expect(document.activeElement).toBe(first.element)
    expect(wrapper.emitted('verify')).toBeUndefined()

    ;(first.element as HTMLInputElement).value = '1'
    await first.trigger('compositionend')
    await wrapper.vm.$nextTick()

    expect((first.element as HTMLInputElement).value).toBe('1')
    expect(document.activeElement).toBe(cells[1].element)
    expect(wrapper.emitted('verify')).toBeUndefined()

    wrapper.unmount()
  })
})
