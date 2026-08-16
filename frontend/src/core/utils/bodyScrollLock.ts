let activeLocks = 0
let previousOverflow: string | null = null
let previousPaddingRight: string | null = null
let hadModalOpenClass = false

function getScrollbarWidth(): number {
  if (typeof window === 'undefined') return 0
  return Math.max(0, window.innerWidth - document.documentElement.clientWidth)
}

/**
 * Locks body scrolling until every owner has released its lock. The first
 * owner preserves any pre-existing inline overflow and right padding for the
 * last owner to restore. A classic layout scrollbar is compensated while the
 * lock is active so opening a modal does not shift the page horizontally.
 */
export function acquireBodyScrollLock(): () => void {
  if (typeof document === 'undefined' || !document.body) return () => {}

  if (activeLocks === 0) {
    previousOverflow = document.body.style.overflow
    previousPaddingRight = document.body.style.paddingRight
    hadModalOpenClass = document.body.classList.contains('modal-open')

    const scrollbarWidth = getScrollbarWidth()
    if (scrollbarWidth > 0) {
      const computedPaddingRight = Number.parseFloat(
        window.getComputedStyle(document.body).paddingRight,
      ) || 0
      document.body.style.paddingRight = `${computedPaddingRight + scrollbarWidth}px`
    }

    document.body.classList.add('modal-open')
    document.body.style.overflow = 'hidden'
  }
  activeLocks += 1

  let released = false
  return () => {
    if (released) return
    released = true
    activeLocks = Math.max(0, activeLocks - 1)
    if (activeLocks !== 0) return

    if (!hadModalOpenClass) {
      document.body.classList.remove('modal-open')
    }
    document.body.style.overflow = previousOverflow ?? ''
    document.body.style.paddingRight = previousPaddingRight ?? ''
    previousOverflow = null
    previousPaddingRight = null
    hadModalOpenClass = false
  }
}
