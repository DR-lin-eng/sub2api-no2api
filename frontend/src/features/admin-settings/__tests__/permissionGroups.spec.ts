import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Tab from '../presentation/widgets/settings-tabs/SettingsPermissionGroupsTab.vue'

const { getGroups, saveGroups } = vi.hoisted(() => ({ getGroups: vi.fn(), saveGroups: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/features/admin-settings/data/datasources/permissionGroupsDatasource', () => ({ getPermissionGroups: getGroups, updatePermissionGroups: saveGroups }))
const document = () => ({ groups: [{ id: 'support', name: '客服', built_in: true, permissions: ['support.read'] }], permissions: [{ key: 'support.read', name: 'Read', description: 'Read' }] })

describe('permission group editor', () => {
  beforeEach(() => { vi.resetAllMocks(); getGroups.mockResolvedValue(document()); saveGroups.mockResolvedValue(document()) })
  it('keeps drafts editable after save failure and permits retry', async () => {
    saveGroups.mockRejectedValueOnce(new Error('duplicate name'))
    const wrapper = mount(Tab)
    await flushPromises()
    await wrapper.get('input[type="text"]').setValue('Helpdesk')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('duplicate name')
    expect((wrapper.get('input[type="text"]').element as HTMLInputElement).value).toBe('Helpdesk')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(saveGroups).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('disables changes until load succeeds and can retry loading', async () => {
    getGroups.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(Tab)
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('input[type="text"]').exists()).toBe(true)
    expect(wrapper.find('button.btn-danger').exists()).toBe(false)
    wrapper.unmount()
  })
})
