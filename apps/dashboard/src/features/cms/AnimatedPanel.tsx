import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { CLOSE_EASE, OPEN_EASE, canAnimate, panelDuration, visibleTravel } from './motion'

// Height reveal for a freshly mounted form. Mounting one is a long frame, after which the frame clock reports a
// stale time and then catches up in a burst - an animation started on it had already run a third of its course
// by the first frame anyone saw. Pinning the start to real time, holding the first keyframe until then, keeps
// the whole movement on screen
export function AnimatedPanel({ open, children }: { open: boolean; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [mounted, setMounted] = useState(open)
  if (open && !mounted) setMounted(true)
  if (!open && mounted && !canAnimate()) setMounted(false)
  const initial = useRef(true)

  useLayoutEffect(() => {
    const el = ref.current
    const first = initial.current
    initial.current = false
    if (!el || !canAnimate()) return
    if (first && open) {
      el.dataset.shown = '1'
      return
    }
    // A panel caught mid-move continues from where it is, not from either end
    const from = el.dataset.shown ? visibleTravel(el, el.getBoundingClientRect().height) : 0
    el.getAnimations().forEach((a) => a.cancel())
    el.style.overflow = 'hidden'

    if (open) {
      const to = Math.max(from, visibleTravel(el, el.scrollHeight))
      el.dataset.shown = '1'
      const anim = el.animate(
        [{ height: `${from}px`, opacity: from ? 1 : 0 }, { opacity: 1, offset: 0.6 }, { height: `${to}px`, opacity: 1 }],
        { duration: panelDuration(to - from), easing: OPEN_EASE, fill: 'backwards' },
      )
      anim.startTime = performance.now()
      anim.onfinish = () => {
        el.style.overflow = ''
      }
      return () => {
        anim.onfinish = null
      }
    }

    const anim = el.animate([{ height: `${from}px`, opacity: 1 }, { height: '0px', opacity: 0 }], {
      duration: Math.round(panelDuration(from) * 0.7),
      easing: CLOSE_EASE,
      fill: 'both',
    })
    anim.startTime = performance.now()
    anim.onfinish = () => setMounted(false)
    return () => {
      anim.onfinish = null
    }
  }, [open])

  if (!mounted) return null
  return <div ref={ref}>{children}</div>
}
