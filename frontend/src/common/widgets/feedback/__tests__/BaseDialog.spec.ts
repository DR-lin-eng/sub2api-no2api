import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
  document.body.className = ''
  document.body.removeAttribute('style')
})

describe('BaseDialog initial focus', () => {
  it('can focus the dialog without scrolling to a later action', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: {
        show: true,
        title: 'Compliance',
        initialFocus: 'dialog'
      },
      slots: {
        default: '<a href="#document">Open document</a>'
      }
    })

    await nextTick()
    await nextTick()

    const dialog = document.body.querySelector<HTMLElement>('.modal-content')
    expect(dialog).not.toBeNull()
    expect(document.activeElement).toBe(dialog)

    wrapper.unmount()
  })

  it('does not release another dialog scroll lock when a hidden dialog mounts', () => {
    const openDialog = mount(BaseDialog, {
      props: { show: true, title: 'Open dialog' },
    })

    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')

    const hiddenDialog = mount(BaseDialog, {
      props: { show: false, title: 'Hidden dialog' },
    })

    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')

    hiddenDialog.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')

    openDialog.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')
  })

  it('keeps the page locked until every open dialog releases its lock', async () => {
    const firstDialog = mount(BaseDialog, {
      props: { show: true, title: 'First dialog' },
    })
    const secondDialog = mount(BaseDialog, {
      props: { show: true, title: 'Second dialog' },
    })

    await firstDialog.setProps({ show: false })
    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')

    await secondDialog.setProps({ show: false })
    expect(document.body.classList.contains('modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')

    firstDialog.unmount()
    secondDialog.unmount()
  })
})
