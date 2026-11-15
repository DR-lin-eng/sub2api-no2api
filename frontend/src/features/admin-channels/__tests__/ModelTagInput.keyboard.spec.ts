import { afterEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import ModelTagInput from '@/features/admin-channels/presentation/widgets/ModelTagInput.vue'

enableAutoUnmount(afterEach)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

it('allows Tab to leave an empty model input', () => {
  const wrapper = mount(ModelTagInput, { props: { models: ['gpt-5'] }, global: { stubs: { Icon: true } } })
  const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
  wrapper.get('input').element.dispatchEvent(event)
  expect(event.defaultPrevented).toBe(false)
})

it('keeps model tags and composing text intact during IME key events', async () => {
  const wrapper = mount(ModelTagInput, { props: { models: ['existing'] }, global: { stubs: { Icon: true } } })
  const input = wrapper.get('input')
  await input.setValue('中文')
  for (const key of ['Enter', 'Tab', 'Backspace']) {
    input.element.dispatchEvent(new KeyboardEvent('keydown', { key, isComposing: true, bubbles: true, cancelable: true }))
  }
  expect(wrapper.emitted('update:models')).toBeUndefined()
  await input.trigger('keydown', { key: 'Enter' })
  expect(wrapper.emitted('update:models')).toEqual([[['existing', '中文']]])
})
