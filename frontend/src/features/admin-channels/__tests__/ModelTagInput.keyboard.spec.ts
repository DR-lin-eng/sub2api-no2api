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
