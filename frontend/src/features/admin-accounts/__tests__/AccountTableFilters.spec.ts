import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountTableFilters from '@/features/admin-accounts/presentation/widgets/AccountTableFilters.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `
    <div v-bind="$attrs">
      <button
        v-for="option in options"
        :key="String(option.value)"
        :data-value="String(option.value ?? '')"
        @click="$emit('update:modelValue', option.value); $emit('change')"
      >{{ option.label }}</button>
    </div>
  `
})

const SearchInputStub = defineComponent({
  props: ['modelValue'],
  template: '<input :value="modelValue" />'
})

describe('AccountTableFilters OAuth quota options', () => {
  it('offers all server-side quota modes and scopes OpenAI modes to OpenAI', async () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: { platform: 'anthropic', type: '', status: '', oauth_quota: '', privacy_mode: '', group: '' },
        groups: []
      },
      global: {
        stubs: {
          Select: SelectStub,
          SearchInput: SearchInputStub
        }
      }
    })

    const quotaFilter = wrapper.find('[data-test="oauth-quota-filter"]')
    expect(quotaFilter.findAll('button').map(button => button.attributes('data-value'))).toEqual([
      '', 'has_quota', 'exhausted', 'with_reset', '5h_exhausted', '7d_exhausted'
    ])

    await quotaFilter.find('[data-value="has_quota"]').trigger('click')
    expect(wrapper.emitted('update:filters')?.at(-1)?.[0]).toMatchObject({
      platform: 'anthropic',
      type: 'oauth',
      oauth_quota: 'has_quota'
    })

    await quotaFilter.find('[data-value="with_reset"]').trigger('click')
    expect(wrapper.emitted('update:filters')?.at(-1)?.[0]).toMatchObject({
      platform: 'openai',
      type: 'oauth',
      oauth_quota: 'with_reset'
    })
  })
})
