import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import KeyGroupBindingsEditor from '../presentation/widgets/KeyGroupBindingsEditor.vue'
import type { ApiKeyGroupBinding } from '@/types'
import type { GroupOption } from '../presentation/keysPageContext'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params?.group ? `${key}:${params.group}` : key,
    }),
  }
})

const bindings: ApiKeyGroupBinding[] = [
  { group_id: 1, max_rate_multiplier: null },
  { group_id: 2, max_rate_multiplier: 1.5 },
]

const option = (value: number, overrides: Partial<GroupOption> = {}): GroupOption => ({
  value,
  label: `Group ${value}`,
  description: `Description ${value}`,
  rate: value,
  userRate: null,
  peakRateEnabled: false,
  peakStart: '',
  peakEnd: '',
  peakRateMultiplier: 1,
  subscriptionType: 'standard',
  platform: 'openai',
  ...overrides,
})

const groupOptions = [
  option(1),
  option(2),
  option(3),
  option(4, { platform: 'gemini' }),
  option(5, { subscriptionType: 'subscription' }),
]

const mountEditor = (modelValue: ApiKeyGroupBinding[] = bindings) => mount(KeyGroupBindingsEditor, {
  props: { modelValue, groupOptions },
  global: {
    stubs: {
      GroupBadge: {
        props: ['name'],
        template: '<span>{{ name }}</span>',
      },
      Icon: true,
      VueDraggable: {
        name: 'VueDraggable',
        props: ['modelValue'],
        emits: ['update:modelValue'],
        template: '<div><slot /></div>',
      },
    },
  },
})

const latestModel = (wrapper: ReturnType<typeof mountEditor>): ApiKeyGroupBinding[] => {
  const events = wrapper.emitted('update:modelValue')
  expect(events).toBeTruthy()
  return events!.at(-1)![0] as ApiKeyGroupBinding[]
}

describe('KeyGroupBindingsEditor', () => {
  it('reorders bindings with explicit controls and drag updates', async () => {
    const wrapper = mountEditor()

    expect(wrapper.get('[data-test="key-group-binding-1"]').text()).toContain('keys.groupBindings.primary')
    expect(wrapper.get('[data-test="key-group-binding-2"]').text()).toContain('keys.groupBindings.fallbackPosition')

    await wrapper.get('[data-test="key-group-move-down-1"]').trigger('click')
    expect(latestModel(wrapper).map(binding => binding.group_id)).toEqual([2, 1])

    wrapper.findComponent({ name: 'VueDraggable' }).vm.$emit('update:modelValue', [bindings[1], bindings[0]])
    expect(latestModel(wrapper).map(binding => binding.group_id)).toEqual([2, 1])
  })

  it('updates rate protection and removes a binding without mutating props', async () => {
    const wrapper = mountEditor()

    await wrapper.get('[data-test="key-group-rate-ceiling-1"]').setValue('0.75')
    expect(latestModel(wrapper)).toEqual([
      { group_id: 1, max_rate_multiplier: 0.75 },
      { group_id: 2, max_rate_multiplier: 1.5 },
    ])
    expect(bindings[0].max_rate_multiplier).toBeNull()

    await wrapper.get('[data-test="key-group-remove-2"]').trigger('click')
    expect(latestModel(wrapper).map(binding => binding.group_id)).toEqual([1])
  })

  it('adds only compatible unselected standard groups', async () => {
    const wrapper = mountEditor()

    await wrapper.get('[data-test="key-group-add"]').trigger('click')
    expect(wrapper.find('[data-test="key-group-option-3"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="key-group-option-4"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="key-group-option-5"]').exists()).toBe(false)

    await wrapper.get('[data-test="key-group-option-3"]').trigger('click')
    expect(latestModel(wrapper)).toEqual([
      ...bindings,
      { group_id: 3, max_rate_multiplier: null },
    ])
  })
})
