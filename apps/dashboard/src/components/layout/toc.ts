import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from 'react'
import { UNSAFE_LocationContext, UNSAFE_NavigationContext } from 'react-router-dom'
import { findScrollParent } from '@/components/data-table/scrollEdges'

export type TocEntry = { id: string; title: string; el: HTMLElement; expand?: () => void }

// No provider means a no-op - that is both how a section outside a layout keeps working and how sidebar sections stay out of the index
export const TocContext = createContext<((entry: TocEntry) => () => void) | null>(null)

function slugify(title: string): string {
  return title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

export function useTocSection(title: ReactNode, opts: { id?: string; expand?: () => void } = {}) { // core's DetailPageSection and the product page's own Section both register through this
  const { id, expand } = opts
  const ref = useRef<HTMLElement>(null)
  const register = useContext(TocContext)
  const tocId = id ?? (typeof title === 'string' ? slugify(title) : '') // a richer title needs an explicit id to name itself
  const tocTitle = typeof title === 'string' ? title : id

  const expandRef = useRef(expand) // a ref, or an inline closure would re-register the section every render
  useEffect(() => {
    expandRef.current = expand // in an effect, not during render - only ever read from a click long after mount
  })

  useEffect(() => {
    if (!register || !ref.current || !tocId || !tocTitle) return
    return register({ id: tocId, title: tocTitle, el: ref.current, expand: () => expandRef.current?.() })
  }, [register, tocId, tocTitle])

  return { ref, anchorId: tocId || undefined }
}

const HEADER_GAP = 8 // breathing room under the sticky page header when a section is scrolled to

// The page header is sticky inside the same scroller, so a raw scrollIntoView parks the heading
// underneath it - measure whatever is there and land below it
export function headerOffset(scroller: HTMLElement): number {
  const header = scroller.querySelector<HTMLElement>('[data-page-header]')
  return (header?.offsetHeight ?? 0) + HEADER_GAP
}

const SETTLE_MS = 1200 // how long a deep link keeps its landing pinned while the page finishes loading
const HANDOVER = ['wheel', 'touchstart', 'pointerdown'] as const // the reader taking over

// The one way anything scrolls a section into view - the contents rail, the heading anchor and a
// #hash arriving from another page all land in the same place
export function scrollToSection(
  el: HTMLElement,
  { expand, behavior = 'smooth' }: { expand?: () => void; behavior?: ScrollBehavior } = {},
) {
  expand?.()
  const scroller = findScrollParent(el)
  if (!scroller) return
  const align = (mode: ScrollBehavior) => {
    const delta = el.getBoundingClientRect().top - scroller.getBoundingClientRect().top
    scroller.scrollTo({ top: scroller.scrollTop + delta - headerOffset(scroller), behavior: mode })
  }
  // A frame later, so an expand has laid out before the target offset is read
  requestAnimationFrame(() => {
    align(behavior)
    // A deep link fires the moment its section exists, which is usually BEFORE the sections above it
    // have their data - they grow, the landing drifts, and the reader arrives above the heading they
    // asked for. Hold the alignment while the page settles, and let go the moment they take over
    // (only for the instant landing - re-aligning mid-smooth-scroll would snap the animation)
    if (behavior !== 'auto' || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => align('auto'))
    observer.observe(scroller)
    for (const child of scroller.children) observer.observe(child)
    const release = () => {
      observer.disconnect()
      clearTimeout(timer)
      for (const event of HANDOVER) scroller.removeEventListener(event, release)
    }
    const timer = setTimeout(release, SETTLE_MS)
    for (const event of HANDOVER) scroller.addEventListener(event, release, { passive: true })
  })
}

// Read through the contexts rather than useLocation/useNavigate, which THROW outside a Router - the
// same reason useParentPage does, plus one specific to split view: a background pane is handed a
// frozen location, so reading the context is what stops a hash meant for the focused pane from
// scrolling an unrelated page sitting behind it
function useRouterLocation() {
  return useContext(UNSAFE_LocationContext)?.location ?? null
}

// Stamps a section into the URL, preserving the path and query around it - a no-op outside a Router
export function useSectionLink(): (id: string) => void {
  const location = useRouterLocation()
  const navigator = useContext(UNSAFE_NavigationContext)?.navigator
  return useCallback(
    (id: string) => {
      if (!navigator || !location) return
      // replace, so walking a page's sections doesn't bury the page you arrived from under Back
      navigator.replace({ pathname: location.pathname, search: location.search, hash: `#${id}` })
    },
    [navigator, location],
  )
}

// Deep links to a section: /orders/123#payment scrolls there once the section exists
//
// Keyed on the navigation, not the hash string, so re-picking the section you are already on
// scrolls again - and a hash whose section hasn't mounted yet stays UNHANDLED, so the effect
// re-runs as entries register and catches it the moment the data lands
export function useSectionHash(entries: TocEntry[]) {
  const location = useRouterLocation()
  const hash = location?.hash ?? ''
  const key = location?.key ?? ''
  const handledKey = useRef<string | null>(null)
  const deepLinked = useRef(hash !== '') // entering with a hash means jumping, not travelling - that one is instant

  useEffect(() => {
    if (handledKey.current === key) return
    const id = decodeURIComponent(hash.replace(/^#/, ''))
    if (!id) {
      handledKey.current = key
      return
    }
    const entry = entries.find((e) => e.id === id)
    const el = entry?.el ?? document.getElementById(id) // sections outside the index still answer to their id
    if (!el) return
    const first = handledKey.current === null
    handledKey.current = key
    scrollToSection(el, {
      expand: entry?.expand,
      behavior: first && deepLinked.current ? 'auto' : 'smooth',
    })
  }, [hash, key, entries])
}
