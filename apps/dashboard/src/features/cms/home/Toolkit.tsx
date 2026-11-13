import { Link } from 'react-router-dom'
import { ArrowUpRightIcon, FileTextIcon, NewspaperIcon, StoreIcon, TriangleAlertIcon, type LucideIcon } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Mark } from './Mark'
import { STATES, STATE_LABEL, lanes, since, stateOf, tally, type SitePage } from './site'
import { STATE_COLOR } from './state-colors'

// Up to here the lead draws a mark for every page, a folder apart from the next; past it, one stretch for each state
const EACH_MARKED = 150

type GlyphKind = 'types' | 'attributes' | 'sections' | 'blocks' | 'posts' | 'taxonomy' | 'menus' | 'forms'
type Serves = 'pages' | 'blog' | 'storefront'
// `failed` is why the count couldn't be had - the box still opens, it just says so instead of showing nothing
export type Tool = { to: string; title: string; about: string; perm: string; glyph: GlyphKind; serves: Serves; count?: number; failed?: string }

const SERVES: Record<Serves, { note: string; icon: LucideIcon }> = {
  pages: { note: 'For pages', icon: FileTextIcon },
  blog: { note: 'For the blog', icon: NewspaperIcon },
  storefront: { note: 'For the storefront', icon: StoreIcon },
}

function Figure({ value, className }: { value: number; className?: string }) {
  const text = formatNumber(value)
  return (
    <span className={className}>
      <span className="sr-only">{text}</span>
      <span aria-hidden>
        {[...text].map((digit, i) => (
          <span key={`${i}${digit}`} className="roll-in" style={{ animationDelay: `${i * 45}ms` }}>
            {digit}
          </span>
        ))}
      </span>
    </span>
  )
}

const LINE = 'block rounded-[2px] ring-1 ring-inset ring-foreground/30'
const FILL = 'block rounded-[2px] bg-foreground/20'
const INK = 'block rounded-[2px] bg-foreground/70'

function Glyph({ kind }: { kind: GlyphKind }) {
  const frame = 'h-9 w-12 shrink-0 transition-transform duration-300 group-hover:-translate-y-0.5'
  switch (kind) {
    case 'types':
      return (
        <span aria-hidden className={cn(frame, 'grid content-start gap-1 rounded-[4px] p-1.5 ring-1 ring-inset ring-foreground/30')}>
          <i className={cn(INK, 'h-1 w-5')} />
          <i className={cn(LINE, 'h-1.5')} />
          <i className={cn(LINE, 'h-1.5')} />
        </span>
      )
    case 'attributes':
      return (
        <span aria-hidden className={cn(frame, 'grid content-center gap-1.5')}>
          {[INK, FILL, FILL].map((label, i) => (
            <span key={i} className="flex items-center gap-1">
              <i className={cn(label, 'h-1 w-3')} />
              <i className={cn(LINE, 'h-2 flex-1')} />
            </span>
          ))}
        </span>
      )
    case 'sections':
      return (
        <span aria-hidden className={cn(frame, 'grid content-center gap-1 rounded-[4px] border border-dashed border-foreground/40 p-1.5')}>
          <i className={cn(INK, 'h-1.5')} />
          <i className={cn(FILL, 'h-1.5')} />
          <i className={cn(FILL, 'h-1.5 w-2/3')} />
        </span>
      )
    case 'blocks':
      return (
        <span aria-hidden className={cn(frame, 'grid content-center gap-1')}>
          <span className="flex h-3 items-center gap-1 overflow-hidden rounded-[3px] bg-foreground/15">
            <i className="block h-full w-1 bg-foreground/70" />
            <i className="block h-1 w-5 rounded-full bg-foreground/40" />
          </span>
          <span className="flex h-3 items-center gap-1 overflow-hidden rounded-[3px] bg-foreground/15">
            <i className="block h-full w-1 bg-foreground/35" />
            <i className="block h-1 w-3 rounded-full bg-foreground/40" />
          </span>
        </span>
      )
    case 'posts':
      return (
        <span aria-hidden className={cn(frame, 'flex items-center gap-1.5')}>
          <i className={cn(FILL, 'size-5 shrink-0 rounded-[3px]')} />
          <span className="grid flex-1 gap-1">
            <i className={cn(INK, 'h-1')} />
            <i className={cn(FILL, 'h-1')} />
            <i className={cn(FILL, 'h-1 w-2/3')} />
          </span>
        </span>
      )
    case 'taxonomy':
      return (
        <span aria-hidden className={cn(frame, 'flex flex-wrap content-center gap-1')}>
          <i className="block h-2.5 w-6 rounded-full bg-foreground/70" />
          <i className="block h-2.5 w-4 rounded-full ring-1 ring-inset ring-foreground/30" />
          <i className="block h-2.5 w-4 rounded-full ring-1 ring-inset ring-foreground/30" />
          <i className="block h-2.5 w-6 rounded-full bg-foreground/20" />
        </span>
      )
    case 'menus':
      return (
        <span aria-hidden className={cn(frame, 'grid content-center gap-0.5')}>
          <span className="flex h-3.5 items-center gap-1 rounded-[3px] px-1 ring-1 ring-inset ring-foreground/30">
            <i className={cn(INK, 'h-1 w-2')} />
            <i className={cn(FILL, 'h-1 w-2')} />
            <i className={cn(FILL, 'h-1 w-2')} />
          </span>
          <span className="ml-0.5 grid w-6 gap-0.5 rounded-[3px] p-1 ring-1 ring-inset ring-foreground/30">
            <i className={cn(FILL, 'h-0.5')} />
            <i className={cn(FILL, 'h-0.5')} />
          </span>
        </span>
      )
    case 'forms':
      return (
        <span aria-hidden className={cn(frame, 'grid content-center gap-1')}>
          <i className={cn(LINE, 'h-2')} />
          <i className={cn(LINE, 'h-2')} />
          <i className="block h-2 w-5 rounded-full bg-foreground/70" />
        </span>
      )
  }
}

function Opens() {
  return (
    <span
      aria-hidden
      className="grid size-7 shrink-0 place-items-center rounded-full border text-muted-foreground transition-colors group-hover:border-foreground group-hover:bg-foreground group-hover:text-background group-focus-visible:border-foreground group-focus-visible:bg-foreground group-focus-visible:text-background"
    >
      <ArrowUpRightIcon className="size-3.5 transition-transform group-hover:translate-x-px group-hover:-translate-y-px" />
    </span>
  )
}

function Serving({ serves }: { serves: Serves }) {
  const { note, icon: Icon } = SERVES[serves]
  return (
    <span className="flex items-center gap-1 justify-self-start rounded-full bg-muted px-2 py-1 text-[11px] leading-none text-muted-foreground">
      <Icon aria-hidden className="size-3" />
      {note}
    </span>
  )
}

export function Toolkit({ pages, total, loading, tools }: { pages: SitePage[] | null; total: number; loading: boolean; tools: Tool[] }) {
  const counts = tally(pages ?? [])
  const states = STATES.filter((s) => counts[s] > 0)
  const drawn = pages?.length ?? 0
  const said = states.map((s) => `${counts[s]} ${STATE_LABEL[s].toLowerCase()}`).join(', ')
  const latest = (pages ?? []).reduce<SitePage | null>((a, p) => (a && a.updatedAt >= p.updatedAt ? a : p), null)
  const rows = lanes(pages ?? [])
  const order = new Map(rows.flatMap((l) => l.pages).map((p, i) => [p.id, i]))
  return (
    <nav aria-label="Everything in the CMS" className="overflow-hidden rounded-xl border bg-background">
      <ul className="-mr-px -mb-px grid grid-cols-2 @2xl:grid-cols-4 @5xl:grid-cols-6">
        <li className="col-span-2 border-r border-b @2xl:col-span-4 @5xl:col-span-2 @5xl:row-span-2">
          <Link
            to="/cms/pages"
            aria-label={pages ? `Pages, ${formatNumber(total)}` : 'Pages'}
            className="group flex h-full flex-col gap-6 p-6 outline-none transition-colors hover:bg-muted/40 focus-visible:bg-muted/40 @2xl:flex-row @2xl:items-end @2xl:justify-between @5xl:flex-col @5xl:items-stretch @5xl:justify-start"
          >
            {pages ? (
              <span className="flex items-baseline gap-3">
                <Figure value={total} className="text-[4.5rem] leading-[0.85] font-extralight tracking-tight tabular-nums text-foreground" />
                <span className="text-lg font-light text-muted-foreground">{total === 1 ? 'page' : 'pages'}</span>
              </span>
            ) : loading ? (
              <span role="status" aria-label="Counting the pages" className="h-14 w-20 animate-pulse rounded-md bg-muted" />
            ) : null}
            {states.length > 0 ? (
              <span className="grid gap-2.5 @2xl:w-72 @5xl:w-auto">
                {drawn <= EACH_MARKED ? (
                  <span role="img" aria-label={said} className="flex flex-wrap gap-x-2 gap-y-1.5">
                    {rows.map((lane) => (
                      <span key={lane.key} className="flex gap-0.5">
                        {lane.pages.map((p) => (
                          <span key={p.id} className={cn('roll-in h-4 w-1.5 rounded-[2px]', STATE_COLOR[stateOf(p)])} style={{ animationDelay: `${Math.min(120 + order.get(p.id)! * 18, 800)}ms` }} />
                        ))}
                      </span>
                    ))}
                  </span>
                ) : (
                  <span role="img" aria-label={said} className="flex h-2 gap-0.5">
                    {states.map((s) => (
                      <span key={s} className={cn('h-full min-w-1 basis-0 rounded-[2px] first:rounded-l-full last:rounded-r-full', STATE_COLOR[s])} style={{ flexGrow: counts[s] }} />
                    ))}
                  </span>
                )}
                <span className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  {states.map((s) => (
                    <span key={s} className="flex items-center gap-1.5">
                      <Mark state={s} />
                      {STATE_LABEL[s]}
                      <span className="tabular-nums text-foreground">{formatNumber(counts[s])}</span>
                    </span>
                  ))}
                  {total > drawn ? <span>of the first {formatNumber(drawn)}</span> : null}
                </span>
                {latest ? (
                  <span className="truncate text-xs text-muted-foreground">
                    Last edit: <span className="text-foreground">{latest.title}</span>, {since(latest.updatedAt)}
                  </span>
                ) : null}
              </span>
            ) : null}
            <span className="flex items-end justify-between gap-4 @5xl:mt-auto">
              <span className="grid gap-0.5">
                <span className="text-sm font-medium text-foreground underline-offset-4 group-hover:underline">Pages</span>
                <span className="text-xs leading-snug text-muted-foreground">Every page as a list, to filter, tag and publish in bulk.</span>
              </span>
              <Opens />
            </span>
          </Link>
        </li>
        {tools.map((t) => (
          <li key={t.to} className="border-r border-b">
            <Link
              to={t.to}
              aria-label={t.failed ? `${t.title}, count unavailable` : t.count == null ? t.title : `${t.title}, ${formatNumber(t.count)}`}
              aria-description={`${SERVES[t.serves].note}. ${t.about}`}
              className="group flex h-full min-h-40 flex-col gap-4 p-4 outline-none transition-colors hover:bg-muted/40 focus-visible:bg-muted/40"
            >
              <span className="flex items-start justify-between gap-3">
                <Glyph kind={t.glyph} />
                {t.failed ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <TriangleAlertIcon aria-hidden className="size-5 text-warning" />
                    </TooltipTrigger>
                    <TooltipContent>Couldn't count them: {t.failed}</TooltipContent>
                  </Tooltip>
                ) : t.count == null ? null : (
                  <Figure value={t.count} className="text-3xl leading-none font-extralight tracking-tight tabular-nums text-foreground" />
                )}
              </span>
              <span className="mt-auto flex items-end justify-between gap-3">
                <span className="grid min-w-0 gap-1">
                  <Serving serves={t.serves} />
                  <span className="text-sm font-medium text-foreground underline-offset-4 group-hover:underline">{t.title}</span>
                  <span className="text-xs leading-snug text-muted-foreground">{t.about}</span>
                </span>
                <Opens />
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  )
}
