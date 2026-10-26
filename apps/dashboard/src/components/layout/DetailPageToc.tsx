import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { findScrollParent } from '@/components/data-table/scrollEdges'
import { headerOffset, useSectionLink, type TocEntry } from './toc'

const IDLE_MS = 450 // how long the rail lingers after the last scroll event

export function DetailPageToc({ entries }: { entries: TocEntry[] }) {
  const linkToSection = useSectionLink()
  const [visible, setVisible] = useState<Set<string>>(new Set())
  const [scrolling, setScrolling] = useState(false)
  const [hovered, setHovered] = useState(false) // or the rail would fade out from under the pointer before it could be clicked
  const [focused, setFocused] = useState(false)
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => {
    if (entries.length === 0) return
    const scroller = findScrollParent(entries[0].el)
    if (!scroller) return

    const measure = () => {
      const box = scroller.getBoundingClientRect()
      const top = box.top + headerOffset(scroller)
      const next = new Set<string>()
      for (const e of entries) {
        const r = e.el.getBoundingClientRect()
        if (r.bottom > top && r.top < box.bottom) next.add(e.id)
      }
      // Same-set check keeps a scroll inside one section from re-rendering the rail on every frame
      setVisible((prev) =>
        prev.size === next.size && [...next].every((id) => prev.has(id)) ? prev : next,
      )
    }

    const onScroll = () => {
      measure()
      setScrolling(true)
      clearTimeout(idle.current)
      idle.current = setTimeout(() => setScrolling(false), IDLE_MS)
    }

    measure()
    scroller.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', measure)
    return () => {
      clearTimeout(idle.current)
      scroller.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', measure)
    }
  }, [entries])

  // Stamps the hash and lets useSectionHash do the scrolling, so the rail, the heading anchor and an
  // incoming deep link all take one path - and the URL in the bar is the one to share
  function jump(entry: TocEntry) {
    linkToSection(entry.id)
  }

  if (entries.length < 2) return null // a contents list of one is noise

  // Scrolling shows only the ticks, which fit in the sections' right padding - the labels (and the blur behind them)
  // spread over the page only once the pointer or focus is on the rail, so they never sit over a button uninvited
  const open = hovered || focused
  const show = scrolling || open
  return (
    <nav
      aria-label="Sections"
      data-open={open || undefined}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false)
      }}
      className={cn(
        // right-3 clears the 10px scrollbar - at right-0 the blur layer sat on top of it
        'absolute right-3 top-1/2 z-20 hidden -translate-y-1/2 py-2 transition-opacity duration-200 xl:block',
        show ? 'pointer-events-auto opacity-100' : 'pointer-events-none opacity-0',
      )}
    >
      {/* Two masks intersected so the blur fades out on the left AND both ends, instead of stopping
          on a hard edge - the bleed gives each gradient room to resolve past the labels */}
      {open ? (
        <div
          aria-hidden
          data-toc-backdrop
          className="pointer-events-none absolute -inset-y-8 -left-10 right-0 bg-gradient-to-l from-background/70 to-transparent backdrop-blur-[3px] [mask-composite:intersect] [mask-image:linear-gradient(to_left,black_45%,transparent),linear-gradient(to_bottom,transparent,black_25%,black_75%,transparent)]"
        />
      ) : null}
      <ul className="relative grid gap-0.5 border-r border-border/60">
        {entries.map((e) => {
          const on = visible.has(e.id)
          return (
            <li key={e.id} className="flex justify-end">
              <button
                type="button"
                onClick={() => jump(e)}
                className="group/toc flex h-4 items-center justify-end gap-2 text-right"
              >
                <span
                  className={cn(
                    'max-w-44 truncate text-[11px] leading-none transition-colors duration-200 group-hover/toc:text-foreground',
                    on ? 'text-foreground' : 'text-muted-foreground/60',
                    !open && 'sr-only',
                  )}
                >
                  {e.title}
                </span>
                {/* Closed, the widest tick and the rule stay inside 12px - right-3 plus that fits the sections'
                    24px padding even where the scrollbar takes no room */}
                <span
                  className={cn(
                    'h-px transition-all duration-200 group-hover/toc:bg-foreground',
                    open ? 'group-hover/toc:w-4' : '',
                    open ? (on ? 'w-4' : 'w-2') : on ? 'w-2.5' : 'w-1.5',
                    on ? 'bg-foreground' : 'bg-border',
                  )}
                />
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
