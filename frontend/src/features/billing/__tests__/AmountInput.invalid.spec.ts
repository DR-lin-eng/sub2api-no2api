import { afterEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AmountInput from '@/features/billing/presentation/widgets/AmountInput.vue'

enableAutoUnmount(afterEach)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

it('restores the last accepted recharge amount after invalid text', async () => {
  const wrapper = mount(AmountInput, { props: { modelValue: 10 } })
  const input = wrapper.get('input')
  await input.setValue('10abc')
  expect((input.element as HTMLInputElement).value).toBe('10')
})
