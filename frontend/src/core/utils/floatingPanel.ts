export interface FloatingPanelPosition {
  top: number | null
  bottom: number | null
  left: number
  width: number
  maxHeight: number
}

export interface FloatingPanelOptions {
  viewportPadding?: number
  gap?: number
  maxWidth?: number
  maxHeightRatio?: number
  mobileBreakpoint?: number
  minComfortableHeight?: number
}

/**
 * 计算挂载到 body 的浮层位置，避免触发按钮靠近视口边缘时浮层被挤到屏幕外。
 */
export const getFloatingPanelPosition = (
  triggerRect: Pick<DOMRect, 'top' | 'right' | 'bottom'>,
  viewportWidth: number,
  viewportHeight: number,
  options: FloatingPanelOptions = {}
): FloatingPanelPosition => {
  const viewportPadding = options.viewportPadding ?? 16
  const gap = options.gap ?? 8
  const maxWidth = options.maxWidth ?? 320
  const maxHeightRatio = options.maxHeightRatio ?? 0.7
  const mobileBreakpoint = options.mobileBreakpoint ?? 768
  const minComfortableHeight = options.minComfortableHeight ?? 240

  const availableWidth = Math.max(0, viewportWidth - viewportPadding * 2)
  const width = Math.min(maxWidth, availableWidth)
  const left = viewportWidth < mobileBreakpoint
    ? viewportPadding
    : Math.max(
        viewportPadding,
        Math.min(triggerRect.right - width, viewportWidth - width - viewportPadding)
      )

  const preferredMaxHeight = Math.max(0, Math.floor(viewportHeight * maxHeightRatio))
  const spaceBelow = Math.max(0, viewportHeight - triggerRect.bottom - gap - viewportPadding)
  const spaceAbove = Math.max(0, triggerRect.top - gap - viewportPadding)
  const openAbove = spaceBelow < Math.min(minComfortableHeight, preferredMaxHeight) && spaceAbove > spaceBelow
  const maxHeight = Math.min(preferredMaxHeight, openAbove ? spaceAbove : spaceBelow)

  return {
    top: openAbove ? null : triggerRect.bottom + gap,
    bottom: openAbove ? viewportHeight - triggerRect.top + gap : null,
    left,
    width,
    maxHeight
  }
}

export function getFloatingTooltipPosition(
  anchor: Pick<DOMRect, 'left' | 'right' | 'top' | 'bottom' | 'width' | 'height'>,
  width: number,
  height: number,
  viewportWidth: number,
  viewportHeight: number,
  placement: 'above' | 'right'
) {
  const gap = 8
  const fitsRight = anchor.right + gap + width <= viewportWidth - gap
  const fitsLeft = anchor.left - gap - width >= gap
  const centered = anchor.left + (anchor.width - width) / 2
  let left = centered
  let top = anchor.top - gap - height
  if (placement === 'right' && (fitsRight || fitsLeft)) {
    left = fitsRight ? anchor.right + gap : anchor.left - gap - width
    top = anchor.top + (anchor.height - height) / 2
  } else if (top < gap) {
    top = anchor.bottom + gap
  }

  return {
    left: Math.max(gap, Math.min(left, viewportWidth - width - gap)),
    top: Math.max(gap, Math.min(top, viewportHeight - height - gap))
  }
}
