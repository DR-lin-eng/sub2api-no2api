import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserAttributeForm from '@/features/admin-users/presentation/widgets/UserAttributeForm.vue'

const { getUserAttributeValues, listEnabledDefinitions } = vi.hoisted(() => ({
  getUserAttributeValues: vi.fn(),
  listEnabledDefinitions: vi.fn()
}))
vi.mock('@/features/admin-users/data/datasources/userAttributesDatasource', () => ({
  getUserAttributeValues,
  listEnabledDefinitions
}))

beforeEach(() => {
  vi.clearAllMocks()
  listEnabledDefinitions.mockResolvedValue([{ id: 1, name: 'Team', type: 'text' }])
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())

function deferred() {
  let resolve!: (value: { attribute_id: number; value: string }[]) => void
  const promise = new Promise<{ attribute_id: number; value: string }[]>(res => { resolve = res })
  return { promise, resolve }
}
function open() {
  return mount(UserAttributeForm, {
    props: { userId: 1, modelValue: {} },
    global: { stubs: { Select: true } }
  })
}

describe('user attribute request ownership', () => {
  it('does not emit previous user values after the next user loads', async () => {
    const old = deferred()
    getUserAttributeValues.mockReturnValueOnce(old.promise).mockResolvedValueOnce([{ attribute_id: 1, value: 'current' }])
    const wrapper = open()
    await wrapper.setProps({ userId: 2 }); await flushPromises()
    old.resolve([{ attribute_id: 1, value: 'old' }]); await flushPromises()
    expect(wrapper.get('input').element.value).toBe('current')
    expect(wrapper.emitted('update:modelValue')).toEqual([[{ 1: 'current' }]])
    wrapper.unmount()
  })

  it('does not restore stale values after the user is cleared', async () => {
    const old = deferred()
    getUserAttributeValues.mockReturnValueOnce(old.promise)
    const wrapper = open()
    await flushPromises(); await wrapper.setProps({ userId: undefined })
    old.resolve([{ attribute_id: 1, value: 'old' }]); await flushPromises()
    expect(wrapper.get('input').element.value).toBe('')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('clears old values while the next user request fails', async () => {
    getUserAttributeValues.mockResolvedValueOnce([{ attribute_id: 1, value: 'old' }]).mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = open()
    await flushPromises(); await wrapper.setProps({ userId: 2 }); await flushPromises()
    expect(wrapper.get('input').element.value).toBe('')
    wrapper.unmount()
  })
})
