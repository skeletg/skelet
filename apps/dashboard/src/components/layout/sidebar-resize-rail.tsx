import { useRef, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react'
import { cn } from '@/lib/utils'
import { useTheme } from '@/lib/theme'
import { useSidebar } from '@/components/ui/sidebar'
import {
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  clampSidebarWidth,
  useSidebarWidthPref,
} from '@/lib/sidebar-pref'

// Replaces shadcn's SidebarRail: same strip on the sidebar's edge, but it now resizes on drag and
// keeps the old click-to-toggle for anyone who only ever tapped it
//
// Three inputs, because a drag handle alone is mouse-only: drag, arrow keys (it reports itself as a
// separator, which is what a splitter is), and the presets in Preferences

// Below this a press counts as a click, not a drag - without it every toggle nudged the width
const DRAG_SLOP = 3
const STEP = 16
const COARSE_STEP = 48

export function SidebarResizeRail({ className }: { className?: string }) {
  const { appearance } = useTheme()
  // a right-hand rail grows leftwards
  const dir = appearance.sidebarSide === 'right' ? -1 : 1
  const { state, toggleSidebar } = useSidebar()
  const { width, setWidth, resetWidth } = useSidebarWidthPref()
  const drag = useRef<{ x: number; w: number; moved: boolean } | null>(null)
  const railRef = useRef<HTMLDivElement | null>(null)

  const collapsed = state === 'collapsed'

  // Painting the variable straight onto the wrapper keeps the drag at pointer speed - going through
  // the preference store would debounce it into a stutter. The commit on release is what persists
  function paint(px: number) {
    const wrapper = railRef.current?.closest<HTMLElement>('[data-slot="sidebar-wrapper"]')
    wrapper?.style.setProperty('--sidebar-width', `${px}px`)
  }

  function onPointerDown(e: ReactPointerEvent<HTMLDivElement>) {
    if (collapsed || e.button !== 0) return
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    drag.current = { x: e.clientX, w: width, moved: false }
  }

  function onPointerMove(e: ReactPointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.x
    if (!d.moved) {
      if (Math.abs(dx) < DRAG_SLOP) return
      d.moved = true
      // Transitions are sized for the collapse animation and fight a live drag, so they're off for
      // its duration - see the [data-sidebar-resizing] rule in index.css
      document.body.dataset.sidebarResizing = 'true'
    }
    paint(clampSidebarWidth(d.w + dir * dx))
  }

  function endDrag(e: ReactPointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    drag.current = null
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
    delete document.body.dataset.sidebarResizing
    if (!d.moved) {
      toggleSidebar() // a press that never moved is the old rail's click
      return
    }
    setWidth(clampSidebarWidth(d.w + dir * (e.clientX - d.x)))
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (collapsed) return
    const step = e.shiftKey ? COARSE_STEP : STEP
    if (e.key === 'ArrowLeft') setWidth(width - dir * step)
    else if (e.key === 'ArrowRight') setWidth(width + dir * step)
    else if (e.key === 'Home') setWidth(SIDEBAR_WIDTH_MIN)
    else if (e.key === 'End') setWidth(SIDEBAR_WIDTH_MAX)
    else return
    e.preventDefault()
  }

  return (
    <div
      ref={railRef}
      data-slot="sidebar-resize-rail"
      role={collapsed ? undefined : 'separator'}
      aria-orientation={collapsed ? undefined : 'vertical'}
      aria-label={collapsed ? 'Expand sidebar' : 'Sidebar width'}
      aria-valuenow={collapsed ? undefined : width}
      aria-valuemin={collapsed ? undefined : SIDEBAR_WIDTH_MIN}
      aria-valuemax={collapsed ? undefined : SIDEBAR_WIDTH_MAX}
      tabIndex={collapsed ? -1 : 0}
      title={collapsed ? 'Expand sidebar' : 'Drag to resize, double-click to reset'}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => !collapsed && resetWidth()}
      onClick={() => collapsed && toggleSidebar()}
      className={cn(
        'absolute inset-y-0 z-20 hidden w-4 group-data-[side=left]:-right-4 group-data-[side=right]:-left-4 sm:flex',
        'after:absolute after:inset-y-0 after:start-1/2 after:w-[2px] after:transition-colors hover:after:bg-sidebar-border',
        'focus-visible:outline-none focus-visible:after:bg-primary',
        collapsed ? (dir < 0 ? 'cursor-w-resize' : 'cursor-e-resize') : 'cursor-col-resize',
        className,
      )}
    />
  )
}
