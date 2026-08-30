import { useCallback, useState, type ReactNode, type Ref } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeftIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BackLink } from '@/components/ui/back-link'
import { cn } from '@/lib/utils'
import { useScrollHideHeader } from '@/lib/use-scroll-hide-header'
import { useParentPage } from '@/nav/active-path'
import { useOptionalPreference } from '@/lib/admin-prefs'
import { DetailPageToc } from './DetailPageToc'
import { TocContext, useSectionHash, useSectionLink, useTocSection, type TocEntry } from './toc'

// Floating section index on long detail pages - an admin can switch it off for themselves in
// Preferences, and a page whose sections aren't a meaningful index passes `toc={false}`
export const DETAIL_TOC_PREF_KEY = 'ui:detailToc'

// Breaks out of the page padding so the sticky header and sections sit edge-to-edge, and owns
// its own scroll container so the header stays pinned while the body scrolls under it
//
// The optional sidebar slots render as siblings of the main column and own their width and
// scroll - with one present the main column gets `flex-1 min-w-0` so it can shrink below content
export function DetailPageLayout({
  children,
  leftSidebar,
  rightSidebar,
  scrollRef,
  className,
  toc = true,
}: {
  children: ReactNode
  leftSidebar?: ReactNode
  rightSidebar?: ReactNode
  // Lets a page drive auto-hide UI off its own scroll position rather than having
  // useScrollHideHeader walk the DOM
  scrollRef?: Ref<HTMLDivElement>
  className?: string
  toc?: boolean // a page whose sections aren't a meaningful index can opt out
}) {
  const hasSidebar = !!leftSidebar || !!rightSidebar
  const [entries, setEntries] = useState<TocEntry[]>([])

  // Mount order isn't document order once sections render conditionally, so sort by DOM position
  const register = useCallback((entry: TocEntry) => {
    setEntries((prev) =>
      [...prev.filter((e) => e.id !== entry.id), entry].sort((a, b) =>
        a.el.compareDocumentPosition(b.el) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
      ),
    )
    return () => setEntries((prev) => prev.filter((e) => e.id !== entry.id))
  }, [])

  const tocEnabled = useOptionalPreference<boolean>(DETAIL_TOC_PREF_KEY, true)
  const showToc = tocEnabled && toc
  // Registration is unconditional - #hash deep links have to resolve on a page whose rail is
  // switched off too, so the pref only decides whether the rail RENDERS
  useSectionHash(entries)
  return (
    <div
      className={cn(
        '-m-4 h-[calc(100%+2rem)] overflow-hidden md:-m-6 md:h-[calc(100%+3rem)]',
        className,
      )}
    >
      <div className="flex h-full">
        {leftSidebar}
        {/* The rail anchors to the content column, not the layout, so it floats inside the text
            rather than over a sidebar - and being outside the scroller it doesn't scroll away */}
        <div className={cn('relative flex h-full', hasSidebar ? 'min-w-0 flex-1' : 'w-full')}>
          <div
            ref={scrollRef}
            className="scrollbar-accent flex h-full w-full flex-col overflow-y-auto"
          >
            <TocContext.Provider value={register}>{children}</TocContext.Provider>
          </div>
          {showToc && <DetailPageToc entries={entries} />}
        </div>
        {rightSidebar}
      </div>
    </div>
  )
}

// Sits inside the scroll container, so content slides under the backdrop blur
//
// The back link is derived, not declared: any URL below a registered nav page gets one pointing at
// that page, so a new subpage can't ship without it and a route that moves in the nav registry
// takes its back link along. `backTo` overrides the target, `backTo={null}` suppresses it for the
// rare subpage that is genuinely a dead end
export function DetailPageHeader({
  backTo,
  backLabel,
  title,
  badges,
  meta,
  actions,
}: {
  backTo?: string | null
  backLabel?: string
  title: ReactNode
  badges?: ReactNode
  meta?: ReactNode
  actions?: ReactNode
}) {
  const { ref, hidden } = useScrollHideHeader<HTMLDivElement>()
  const parent = useParentPage()
  // undefined means "work it out"; an explicit null means "no back link here"
  const target = backTo === undefined ? (parent?.path ?? null) : backTo
  const label = backLabel ?? (parent ? `Back to ${parent.title}` : 'Back')
  return (
    <div
      ref={ref}
      // Marked so a sticky table toolbar can measure it and park itself below instead of underneath
      data-page-header
      className={cn(
        'sticky top-0 z-10 border-b bg-background/80 backdrop-blur transition-transform duration-200 supports-[backdrop-filter]:bg-background/60',
        hidden && '-translate-y-full',
      )}
    >
      <div className="flex flex-wrap items-center gap-3 px-6 py-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            {target ? (
              <Button
                asChild
                variant="ghost"
                size="icon"
                className="-ml-2 size-8 shrink-0 text-muted-foreground hover:text-foreground"
                aria-label={label}
                title={label}
              >
                <Link to={target}>
                  <ArrowLeftIcon className="size-4" />
                </Link>
              </Button>
            ) : null}
            <h1 className="min-w-0 truncate text-xl font-semibold tracking-tight">{title}</h1>
            {badges}
          </div>
          {meta ? (
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
              {meta}
            </div>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
    </div>
  )
}

// The derived back link on its own, for the handful of subpages that predate DetailPageHeader and
// render a plain form instead - same target and label, without restructuring the page around it
export function PageBackLink({ className }: { className?: string }) {
  const parent = useParentPage()
  if (!parent) return null
  return <BackLink to={parent.path} label={`Back to ${parent.title}`} className={className} />
}

// `divide-y` rules between sibling sections, `pb-16` keeps the last one clear of the scroll edge
export function DetailPageBody({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return <div className={cn('divide-y pb-16', className)}>{children}</div>
}

// The # was decorative - as a link it's the affordance for copying a section's address, and clicking
// it deep-links through the URL so the scroll behaves identically to arriving from another page
export function SectionAnchor({ id, title }: { id?: string; title: ReactNode }) {
  const linkToSection = useSectionLink()
  if (!id) return <span className="mr-1.5 text-primary">#</span>
  const label = typeof title === 'string' ? `Link to ${title}` : 'Link to this section'
  return (
    <a
      href={`#${id}`}
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.preventDefault() // a native hash jump skips the sticky-header offset and react-router never sees it
        linkToSection(id)
      }}
      className="mr-1.5 text-primary/70 transition-colors hover:text-primary"
    >
      #
    </a>
  )
}

export function DetailPageSection({
  title,
  description,
  trailing,
  children,
  className,
  id,
}: {
  title?: ReactNode
  description?: ReactNode
  trailing?: ReactNode
  children: ReactNode
  className?: string
  id?: string // anchor + contents key, derived from a string title when omitted
}) {
  const { ref, anchorId } = useTocSection(title, { id })
  return (
    <section ref={ref} id={anchorId} className={cn('grid gap-4 px-6 py-6', className)}>
      {(title || description || trailing) && (
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="grid gap-1">
            {title ? (
              <h3 className="text-lg font-semibold tracking-tight">
                <SectionAnchor id={anchorId} title={title} />
                {title}
              </h3>
            ) : null}
            {description ? (
              <p className="text-sm text-muted-foreground">{description}</p>
            ) : null}
          </div>
          {trailing}
        </div>
      )}
      <div className="grid gap-4">{children}</div>
    </section>
  )
}
