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

  it('imports images from the clipboard and drag-and-drop without blocking text paste', async () => {
    const wrapper = mount(SupportMessageComposer)
    const textarea = wrapper.get('textarea')
    const file = new File(['jpeg'], 'pasted.jpg', { type: 'image/jpeg' })

    const textPaste = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
    Object.defineProperty(textPaste, 'clipboardData', {
      value: { files: [], items: [] },
    })
    textarea.element.dispatchEvent(textPaste)
    expect(textPaste.defaultPrevented).toBe(false)

    const imagePaste = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
    Object.defineProperty(imagePaste, 'clipboardData', {
      value: {
        files: [file],
        items: [{ kind: 'file', type: 'image/jpeg', getAsFile: () => file }],
      },
    })
    textarea.element.dispatchEvent(imagePaste)
    await nextTick()

    expect(imagePaste.defaultPrevented).toBe(true)
    expect(wrapper.emitted('upload')).toEqual([[
      { file, content: '', reply_to_id: null },
    ]])

    const drop = new Event('drop', { bubbles: true, cancelable: true }) as DragEvent
    Object.defineProperty(drop, 'dataTransfer', {
      value: { files: [file], types: ['Files'] },
    })
    wrapper.get('form').element.dispatchEvent(drop)
    await nextTick()

    expect(drop.defaultPrevented).toBe(true)
    expect(wrapper.emitted('upload')).toHaveLength(2)
  })
})
