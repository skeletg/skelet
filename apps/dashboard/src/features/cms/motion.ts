import { findScrollParent } from '@/components/data-table/scrollEdges'

export const OPEN_EASE = 'cubic-bezier(0.33, 1, 0.68, 1)'
export const CLOSE_EASE = 'cubic-bezier(0.4, 0, 1, 1)'

export function canAnimate(): boolean {
  if (typeof Element === 'undefined' || typeof Element.prototype.animate !== 'function') return false
  return !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
}

// Longer for a taller panel, so a one-line block and a full hero form move at a similar pace
export function panelDuration(px: number): number {
  return Math.round(Math.min(340, Math.max(200, 150 + px * 0.2)))
}

// No more than a screenful of a panel can be seen at once, so only that much is worth moving - the rest of a
// tall form unfolds below the fold, where the jump to full height can't be seen
export function visibleTravel(el: HTMLElement, px: number): number {
  const scroller = findScrollParent(el)
  return scroller ? Math.min(px, scroller.clientHeight) : px
}

const HOLD_MS = 420
const easeOut = (p: number) => 1 - (1 - p) ** 3

// Opening a block closes the open one, and when that one sits above, everything under it slid up and the
// clicked block left the screen. This holds the clicked header where it was through the collapse - or, when it
// is low enough that its form would open out of sight, eases it up to a quarter of the way down
// `shrinkAbove` is how much is about to fold away over it: past the top of the page there's nothing left to
// scroll back, so the header has to give up that much - easing it there from the start beats a drop at the end
export function holdInPlace(card: HTMLElement, opts: { reveal?: boolean; shrinkAbove?: number } = {}) {
  const scroller = findScrollParent(card)
  if (!scroller) return
  const box = scroller.getBoundingClientRect()
  const start = card.getBoundingClientRect().top
  const raise = opts.reveal || start > box.top + box.height * 0.55
  const deficit = Math.max(0, (opts.shrinkAbove ?? 0) - scroller.scrollTop)
  const target = Math.min(raise ? box.top + box.height * 0.25 : start, start - deficit)
  const smooth = canAnimate()
  // A hand on the wheel wins - stop steering the moment the admin scrolls themselves
  let stopped = false
  const stop = () => {
    stopped = true
  }
  scroller.addEventListener('wheel', stop, { once: true, passive: true })
  scroller.addEventListener('touchmove', stop, { once: true, passive: true })
  // Timed from the first frame that runs, in real time - the frame timestamp is stale after the long mount frame
  let t0: number | undefined
  const step = () => {
    if (stopped) return
    const now = performance.now()
    t0 ??= now
    const p = Math.min(1, (now - t0) / HOLD_MS)
    const want = smooth ? start + (target - start) * easeOut(p) : target
    scroller.scrollTop += card.getBoundingClientRect().top - want
    if (p < 1) requestAnimationFrame(step)
    else {
      scroller.removeEventListener('wheel', stop)
      scroller.removeEventListener('touchmove', stop)
    }
  }
  requestAnimationFrame(step)
}
