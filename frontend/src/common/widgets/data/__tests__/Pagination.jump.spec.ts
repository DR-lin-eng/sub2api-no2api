import { afterEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import Pagination from '@/common/widgets/data/Pagination.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

it('accepts a numeric value from the number input model', async () => {
  const wrapper = mount(Pagination, {
    props: { total: 200, page: 1, pageSize: 20, showJump: true, showPageSizeSelector: false },
    global: { stubs: { Icon: true, Select: true } }
  })
  const input = wrapper.get('input[type="number"]')
  await input.setValue('3')
  await wrapper.get('.btn').trigger('click')
  expect(wrapper.emitted('update:page')).toEqual([[3]])
})
