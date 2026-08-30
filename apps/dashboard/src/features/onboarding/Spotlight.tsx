import { useEffect, useLayoutEffect, useRef, type ReactNode } from 'react'

// Dims the page and cuts a hole around one real element, with a card beside it
//
// Pointing at the thing beats describing it, and pointing at the LIVE element means the tour can't
// drift from the UI the way a screenshot would. A step whose element isn't on screen (a
// permission-gated target, the collapsed icon rail, a narrow viewport) dims the page and centres the
// card instead of highlighting the wrong thing
//
// Geometry is written straight to the DOM rather than held in state: a scroll or a resize then costs
// two style writes instead of a React render, and there's no setState-in-effect to work around

const PAD = 6 // breathing room around the target, so the hole doesn't clip a border
const GAP = 14 // between the hole and the card
const EDGE = 16 // keep-away from the viewport edges
const CARD_W = 380

export function Spotlight({
  selector,
  children,
  onSkip,
}: {
  selector?: string
  children: ReactNode
  onSkip: () => void
}) {
  const holeRef = useRef<HTMLDivElement>(null)
  const dimRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const position = () => {
      const hole = holeRef.current
      const dim = dimRef.current
      const card = cardRef.current
      if (!hole || !dim || !card) return

      const el = selector ? document.querySelector<HTMLElement>(selector) : null
      const r = el?.getBoundingClientRect()
      // zero-sized means hidden (icon rail, mobile sheet) - treat it as absent
      const target = r && r.width > 0 && r.height > 0 ? r : null

      if (!target) {
        hole.style.display = 'none'
        dim.style.display = 'block'
        card.style.top = `${Math.max(EDGE, (window.innerHeight - card.offsetHeight) / 2)}px`
        card.style.left = `${Math.max(EDGE, (window.innerWidth - card.offsetWidth) / 2)}px`
        return
      }

      hole.style.display = 'block'
      dim.style.display = 'none'
      hole.style.top = `${target.top - PAD}px`
      hole.style.left = `${target.left - PAD}px`
      hole.style.width = `${target.width + PAD * 2}px`
      hole.style.height = `${target.height + PAD * 2}px`

      const cw = card.offsetWidth
      const ch = card.offsetHeight
      const clampX = (x: number) => Math.min(Math.max(EDGE, x), window.innerWidth - cw - EDGE)
      const clampY = (y: number) => Math.min(Math.max(EDGE, y), window.innerHeight - ch - EDGE)

      // Beside a target on the left (the sidebar), otherwise below it, and above when there's no
      // room below
      if (target.right < window.innerWidth / 3) {
        card.style.top = `${clampY(target.top - PAD)}px`
        card.style.left = `${clampX(target.right + GAP)}px`
        return
      }
      const below = target.bottom + GAP
      card.style.left = `${clampX(target.left + target.width / 2 - cw / 2)}px`
      card.style.top =
        below + ch < window.innerHeight - EDGE ? `${below}px` : `${clampY(target.top - ch - GAP)}px`
    }

    position()
    const onMove = () => position()
    window.addEventListener('scroll', onMove, true) // capture, so inner scrollers count
    window.addEventListener('resize', onMove)
    // Cheap catch-all for layout that settles late: the sidebar's expand transition, a font swap,
    // an image reflowing the header. Style writes only, so it never re-renders
    const timer = window.setInterval(position, 250)
    return () => {
      window.removeEventListener('scroll', onMove, true)
      window.removeEventListener('resize', onMove)
      window.clearInterval(timer)
    }
  }, [selector, children])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onSkip()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onSkip])

  return (
    <div className="fixed inset-0 z-50" role="dialog" aria-modal="true" aria-label="Getting started">
      {/* One element does the dimming: a transparent box over the target with an enormous shadow
          spread. No four-rectangle arithmetic, and the rounded corners come for free */}
      <div
        ref={holeRef}
        aria-hidden="true"
        style={{ boxShadow: '0 0 0 9999px rgb(0 0 0 / 0.55)' }}
        className="pointer-events-none absolute rounded-lg ring-2 ring-primary/70 transition-[top,left,width,height] duration-200"
      />
      <div ref={dimRef} aria-hidden="true" className="absolute inset-0 hidden bg-black/55" />

      {/* Clicking the dimmed area gets out, same as Escape */}
      <button
        type="button"
        aria-label="Close the tour"
        onClick={onSkip}
        className="absolute inset-0 cursor-default"
      />

      <div
        ref={cardRef}
        style={{ width: CARD_W }}
        className="absolute max-w-[calc(100vw-2rem)] rounded-lg border bg-popover p-4 text-popover-foreground shadow-xl"
      >
        {children}
      </div>
    </div>
  )
}
