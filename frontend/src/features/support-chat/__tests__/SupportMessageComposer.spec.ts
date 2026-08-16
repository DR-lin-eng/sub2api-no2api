import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import SupportMessageComposer from '@/features/support-chat/presentation/widgets/SupportMessageComposer.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('SupportMessageComposer', () => {
  it('does not intercept Enter during IME composition and clears only after a successful-send acknowledgement', async () => {
    const wrapper = mount(SupportMessageComposer)
    const textarea = wrapper.get('textarea')
    await textarea.setValue('中文候选')
    await textarea.trigger('compositionstart')

    const composingEnter = new KeyboardEvent('keydown', {
      key: 'Enter',
      bubbles: true,
      cancelable: true,
      isComposing: true,
    })
    textarea.element.dispatchEvent(composingEnter)
    await nextTick()

    expect(composingEnter.defaultPrevented).toBe(false)
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect((textarea.element as HTMLTextAreaElement).value).toBe('中文候选')

    await textarea.trigger('compositionend')
    const sendEnter = new KeyboardEvent('keydown', {
      key: 'Enter',
      bubbles: true,
      cancelable: true,
    })
    textarea.element.dispatchEvent(sendEnter)
    await nextTick()

    expect(sendEnter.defaultPrevented).toBe(true)
    expect(wrapper.emitted('submit')).toEqual([[
      { content: '中文候选', kind: 'text', reply_to_id: null },
    ]])
    expect((textarea.element as HTMLTextAreaElement).value).toBe('中文候选')

    const exposed = wrapper.vm as unknown as { clearDraft: () => void }
    exposed.clearDraft()
    await nextTick()
    expect((textarea.element as HTMLTextAreaElement).value).toBe('')
  })

  it('does not submit the form while composition is still active', async () => {
    const wrapper = mount(SupportMessageComposer)
    await wrapper.get('textarea').setValue('未完成拼音')
    await wrapper.get('textarea').trigger('compositionstart')
    await wrapper.get('form').trigger('submit')

    expect(wrapper.emitted('submit')).toBeUndefined()
  })
})
