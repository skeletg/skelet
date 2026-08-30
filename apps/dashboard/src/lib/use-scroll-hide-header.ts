import { useEffect, useRef, useState } from 'react'

// Tracks scroll direction to give pages a `hidden` flag for sliding a sticky header offscreen
// Listens to EVERY scrollable ancestor, not just the nearest - detail-page layouts nest a
// route-level scroller inside a per-route one, and which moves depends on where the wheel lands
export function useScrollHideHeader<T extends HTMLElement>({
  threshold = 4,
  topGuard = 60,
}: {
  threshold?: number
  topGuard?: number
} = {}) {
  const ref = useRef<T | null>(null)
  const [hidden, setHidden] = useState(false)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    const scrollers: HTMLElement[] = []
    let cursor: HTMLElement | null = el.parentElement
    while (cursor && cursor !== document.body) {
      const overflowY = window.getComputedStyle(cursor).overflowY
      if (overflowY === 'auto' || overflowY === 'scroll') {
        scrollers.push(cursor)
      }
      cursor = cursor.parentElement
    }
    if (scrollers.length === 0) return

    const lastY = new Map<HTMLElement, number>()
    scrollers.forEach((s) => lastY.set(s, s.scrollTop))

    const onScroll = (e: Event) => {
      const s = e.currentTarget as HTMLElement
      const y = s.scrollTop
      const prev = lastY.get(s) ?? 0
      const dy = y - prev
      lastY.set(s, y)
      if (Math.abs(dy) < threshold) return
      if (y < topGuard) {
        setHidden(false)
      } else if (dy > 0) {
        setHidden(true)
      } else {
        setHidden(false)
      }
    }

    scrollers.forEach((s) => s.addEventListener('scroll', onScroll, { passive: true }))
    return () => {
      scrollers.forEach((s) => s.removeEventListener('scroll', onScroll))
    }
  }, [threshold, topGuard])

  return { ref, hidden }
}
