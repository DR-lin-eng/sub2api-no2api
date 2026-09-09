import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '@/features/admin-accounts/presentation/widgets/AccountActionMenu.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function account(cpaMode: boolean) {
  return {
    id: 7,
    type: 'apikey',
    credentials: { cpa_mode: cpaMode },
    status: 'active'
  } as any
}

describe('AccountActionMenu CPA sync', () => {
  it('emits sync for an enabled CPA account', async () => {
    const value = account(true)
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: value, anchorRect: new DOMRect(10, 10, 40, 24) },
      global: { stubs: { Icon: true, Teleport: true } }
    })

    const button = wrapper.findAll('button').find(item => item.text().includes('admin.accounts.syncCPA'))
    expect(button).toBeTruthy()
    await button!.trigger('click')
    expect(wrapper.emitted('sync-cpa')?.[0]).toEqual([value])
  })

  it('hides sync outside enabled CPA mode', () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: account(false), anchorRect: new DOMRect(10, 10, 40, 24) },
      global: { stubs: { Icon: true, Teleport: true } }
    })
    expect(wrapper.text()).not.toContain('admin.accounts.syncCPA')
  })
})
