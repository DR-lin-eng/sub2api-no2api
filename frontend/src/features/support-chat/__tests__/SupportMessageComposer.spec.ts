import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SupportMessageComposer from '@/features/support-chat/presentation/widgets/SupportMessageComposer.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('SupportMessageComposer', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

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

  it('queues clipboard and drag-and-drop images for an explicit multi-image send', async () => {
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
    expect(wrapper.emitted('upload')).toBeUndefined()
    await textarea.setValue('two screenshots')

    const droppedFile = new File(['png'], 'dropped.png', { type: 'image/png' })
    const drop = new Event('drop', { bubbles: true, cancelable: true }) as DragEvent
    Object.defineProperty(drop, 'dataTransfer', {
      value: { files: [droppedFile], types: ['Files'] },
    })
    wrapper.get('form').element.dispatchEvent(drop)
    await nextTick()

    expect(drop.defaultPrevented).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('upload')).toEqual([[
      { files: [file, droppedFile], content: 'two screenshots', reply_to_id: null },
    ]])
  })

  it('restores per-conversation text drafts without persisting image files', async () => {
    localStorage.setItem('support_chat_draft_v2:admin:7', 'saved answer')
    const wrapper = mount(SupportMessageComposer, { props: { draftKey: 'admin:7' } })
    await nextTick()

    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('saved answer')
    await wrapper.get('textarea').setValue('updated answer')
    await nextTick()
    expect(localStorage.getItem('support_chat_draft_v2:admin:7')).toBe('updated answer')

    await wrapper.setProps({ draftKey: 'admin:8' })
    await nextTick()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    expect(localStorage.getItem('support_chat_draft_v2:admin:7')).toBe('updated answer')
  })

  it('supports opt-in one-click quick reply sending', async () => {
    const wrapper = mount(SupportMessageComposer, {
      props: {
        adminMode: true,
        quickReplies: [{
          id: 1,
          admin_id: 9,
          title: 'Greeting',
          content: 'Hello from support',
          sort_order: 0,
          created_at: '2026-08-30T00:00:00Z',
          updated_at: '2026-08-30T00:00:00Z',
        }],
      },
    })

    const openReplies = () => wrapper.findAll('button').find(button => button.text() === 'supportChat.quickReplies.short')!
    await openReplies().trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'Greeting')!.trigger('click')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Hello from support')
    expect(wrapper.emitted('submit')).toBeUndefined()

    await openReplies().trigger('click')
    await wrapper.get('[data-testid="support-quick-reply-one-click"]').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'Greeting')!.trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[
      { content: 'Hello from support', kind: 'text', reply_to_id: null },
    ]])
  })
})
