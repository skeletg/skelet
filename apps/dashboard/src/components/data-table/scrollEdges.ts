import { useEffect, useState, type RefObject } from 'react'

const DEFAULT_SLACK = 24 // how close to an edge counts as "arrived" and drops a bar's floating treatment

// No scrollHeight check - an ancestor owning the overflow is still the right answer on a short list,
// and testing the amount resolves to the wrong element until the rows arrive
export function findScrollParent(el: HTMLElement): HTMLElement | null {
  let node: HTMLElement | null = el.parentElement
  while (node) {
    const overflowY = getComputedStyle(node).overflowY
    if (overflowY === 'auto' || overflowY === 'scroll') return node
    node = node.parentElement
  }
  return document.scrollingElement as HTMLElement | null
}

// enabled:false skips the listeners and reports both edges, which is the inert answer
export function useScrollEdges(
  ref: RefObject<HTMLElement | null>,
  { slack = DEFAULT_SLACK, enabled = true }: { slack?: number; enabled?: boolean } = {},
) {
  const [edges, setEdges] = useState({ atStart: true, atEnd: true }) // a list that never scrolls renders plain, not floating

  useEffect(() => {
    if (!enabled) return
    const el = ref.current
    if (!el) return
    const scroller = findScrollParent(el)
    if (!scroller) return

    const check = () => {
      setEdges({
        atStart: scroller.scrollTop <= slack,
        atEnd: scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= slack,
      })
    }
    check()

    scroller.addEventListener('scroll', check, { passive: true })
    window.addEventListener('resize', check)
    const ro = new ResizeObserver(check)
    ro.observe(scroller)
    if (scroller.firstElementChild) ro.observe(scroller.firstElementChild) // the scroller keeps its own size when rows change

    return () => {
      scroller.removeEventListener('scroll', check)
      window.removeEventListener('resize', check)
      ro.disconnect()
    }
  }, [ref, slack, enabled])

  return edges
}
