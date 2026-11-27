import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('uses viewport coordinates without double-counting window scroll offsets', async () => {
    const origScrollX = window.scrollX
    const origScrollY = window.scrollY
    Object.defineProperty(window, 'scrollX', { configurable: true, value: 140 })
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 320 })

    try {
      const wrapper = mount(HelpTooltip, {
        attachTo: document.body,
        props: { content: 'scroll check' },
      })
      const trigger = wrapper.get('.group')
      trigger.element.getBoundingClientRect = () => ({
        top: 50,
        left: 80,
        width: 20,
        height: 20,
        right: 100,
        bottom: 70,
        x: 80,
        y: 50,
        toJSON: () => ({}),
      })

      await trigger.trigger('mouseenter')
      await nextTick()
      const tooltip = getTooltipElement()
      expect(tooltip.style.top).toBe('calc(42px)')
      expect(tooltip.style.left).toBe('90px')
      wrapper.unmount()
    } finally {
      Object.defineProperty(window, 'scrollX', { configurable: true, value: origScrollX })
      Object.defineProperty(window, 'scrollY', { configurable: true, value: origScrollY })
    }
  })
})
