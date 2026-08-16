import { afterEach, describe, expect, it, vi } from 'vitest'

import { acquireBodyScrollLock } from '../bodyScrollLock'

const originalInnerWidth = window.innerWidth
const originalClientWidthDescriptor = Object.getOwnPropertyDescriptor(
  document.documentElement,
  'clientWidth',
)

function setViewportWidths(innerWidth: number, clientWidth: number) {
  vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(innerWidth)
  Object.defineProperty(document.documentElement, 'clientWidth', {
    configurable: true,
    value: clientWidth,
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: originalInnerWidth,
    writable: true,
  })
  if (originalClientWidthDescriptor) {
    Object.defineProperty(
      document.documentElement,
      'clientWidth',
      originalClientWidthDescriptor,
    )
  } else {
    delete (document.documentElement as HTMLElement & { clientWidth?: number }).clientWidth
  }
  document.body.className = ''
  document.body.removeAttribute('style')
})

describe('bodyScrollLock', () => {
  it('compensates for the removed scrollbar and restores the previous body styles', () => {
    setViewportWidths(1200, 1184)
    document.body.style.overflow = 'clip'
    document.body.style.paddingRight = '4px'

    const release = acquireBodyScrollLock()

    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')
    expect(document.body.style.paddingRight).toBe('20px')

    release()

    expect(document.body.classList.contains('modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('clip')
    expect(document.body.style.paddingRight).toBe('4px')
  })

  it('keeps shared styles until the final owner releases and ignores duplicate releases', () => {
    setViewportWidths(1200, 1184)

    const releaseFirst = acquireBodyScrollLock()
    const releaseSecond = acquireBodyScrollLock()

    releaseFirst()
    releaseFirst()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(document.body.style.overflow).toBe('hidden')
    expect(document.body.style.paddingRight).toBe('16px')

    releaseSecond()
    expect(document.body.classList.contains('modal-open')).toBe(false)
    expect(document.body.style.overflow).toBe('')
    expect(document.body.style.paddingRight).toBe('')
  })

  it('does not add padding when the browser has no layout scrollbar gap', () => {
    setViewportWidths(1200, 1200)
    document.body.style.paddingRight = '0.5rem'

    const release = acquireBodyScrollLock()

    expect(document.body.style.paddingRight).toBe('0.5rem')

    release()
    expect(document.body.style.paddingRight).toBe('0.5rem')
  })
})
