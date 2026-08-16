import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LocalCaptchaWidget from '@/features/auth/presentation/widgets/LocalCaptchaWidget.vue'

const { getLocalCaptchaMock } = vi.hoisted(() => ({
  getLocalCaptchaMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/features/auth/data/datasources/authDatasource', () => ({
  getLocalCaptcha: (...args: unknown[]) => getLocalCaptchaMock(...args),
}))

describe('LocalCaptchaWidget', () => {
  beforeEach(() => {
    getLocalCaptchaMock.mockReset()
    getLocalCaptchaMock.mockResolvedValue({
      captcha_id: 'challenge-1',
      image_data: 'data:image/png;base64,abc',
      expires_in: 300,
    })
  })

  it('loads a challenge and normalizes user input', async () => {
    const wrapper = mount(LocalCaptchaWidget, {
      props: {
        captchaId: '',
        captchaCode: '',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.emitted('update:captchaId')?.at(-1)).toEqual(['challenge-1'])
    expect(wrapper.get('img').attributes('src')).toBe('data:image/png;base64,abc')

    await wrapper.get('input').setValue(' a7 k9p ')
    expect(wrapper.emitted('update:captchaCode')?.at(-1)).toEqual(['A7K9P'])
  })

  it('does not write intermediate mobile IME letters back into the controlled value', async () => {
    const wrapper = mount(LocalCaptchaWidget, {
      props: {
        captchaId: '',
        captchaCode: '',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    const input = wrapper.get('input')
    const emittedBeforeComposition = wrapper.emitted('update:captchaCode')?.length ?? 0

    await input.trigger('compositionstart')
    ;(input.element as HTMLInputElement).value = 'n'
    await input.trigger('input')
    ;(input.element as HTMLInputElement).value = 'ni'
    await input.trigger('input')

    expect(wrapper.emitted('update:captchaCode')?.length ?? 0).toBe(emittedBeforeComposition)
    expect((input.element as HTMLInputElement).value).toBe('ni')

    // Committing a raw captcha letter emits it exactly once, after composition.
    ;(input.element as HTMLInputElement).value = 'n'
    await input.trigger('compositionend')
    const updates = wrapper.emitted('update:captchaCode') ?? []
    expect(updates).toHaveLength(emittedBeforeComposition + 1)
    expect(updates.at(-1)).toEqual(['N'])
  })

  it('clears the old challenge before refreshing', async () => {
    const wrapper = mount(LocalCaptchaWidget, {
      props: {
        captchaId: 'old-id',
        captchaCode: 'OLD',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    getLocalCaptchaMock.mockResolvedValueOnce({
      captcha_id: 'challenge-2',
      image_data: 'data:image/png;base64,def',
      expires_in: 300,
    })
    await wrapper.get('button').trigger('click')
    await flushPromises()

    const idEvents = wrapper.emitted('update:captchaId') ?? []
    expect(idEvents.at(-2)).toEqual([''])
    expect(idEvents.at(-1)).toEqual(['challenge-2'])
  })
})
