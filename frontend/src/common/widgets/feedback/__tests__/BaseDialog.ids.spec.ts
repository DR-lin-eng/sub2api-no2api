import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'

enableAutoUnmount(afterEach)

describe('BaseDialog accessible title IDs', () => {
  it('assigns distinct title IDs to simultaneous instances', async () => {
    mount(BaseDialog, { props: { show: true, title: 'First' }, global: { stubs: { Icon: true } } })
    mount(BaseDialog, { props: { show: true, title: 'Second' }, global: { stubs: { Icon: true } } })
    await nextTick()
    const dialogs = Array.from(document.body.querySelectorAll('[role="dialog"]'))
    expect(dialogs).toHaveLength(2)
    const ids = dialogs.map((dialog) => dialog.getAttribute('aria-labelledby'))
    expect(new Set(ids).size).toBe(2)
  })
})
