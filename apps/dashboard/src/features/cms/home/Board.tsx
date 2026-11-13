import { useState } from 'react'
import { Link } from 'react-router-dom'
import { PlusIcon, PuzzleIcon, SearchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { BLOCK_CATEGORY_ORDER, blockCategory, blockDescriptor, blockLabel, type BlockCategory } from '../page-blocks'
import { Mark } from './Mark'
import { STATES, STATE_LABEL, addressOf, lanes, lanesByType, matches, since, stateOf, tally, type Lane, type SitePage, type State } from './site'

const LANE_OPENS_ON = 12
const BLOCKS_SHOWN = 9

// The palette's own groups, in its order - the rest of the page never uses these colours for anything else
const CATEGORY: Record<BlockCategory, string> = {
  Layout: 'bg-series-1',
  Content: 'bg-series-2',
  Media: 'bg-series-3',
  'Catalog & blog': 'bg-series-4',
  Advanced: 'bg-series-other',
}

type Show = State | 'all'
type PageType = { id: string; name: string }

export function Board({
  pages,
  types,
  custom,
  canAdd,
  total,
}: {
  pages: SitePage[] | null
  types: PageType[]
  custom: Map<string, string>
  canAdd: boolean
  /** How many pages the site has - more than `pages` holds when the board was handed only the first of them */
  total: number
}) {
  const [show, setShow] = useState<Show>('all')
  const [by, setBy] = useState<'address' | 'type'>('address')
  const [query, setQuery] = useState('')
  const [lit, setLit] = useState<BlockCategory | null>(null)
  const all = pages ?? []
  const counts = tally(all)
  const found = all.filter((p) => matches(p, show, query))
  const narrowed = show !== 'all' || query.trim() !== ''
  const used = BLOCK_CATEGORY_ORDER.filter((c) => all.some((p) => p.outline.some((type) => blockCategory(type) === c)))

  return (
    <section aria-label="Site map" className="overflow-hidden rounded-xl border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3 border-b px-4 py-3">
        <div role="radiogroup" aria-label="Show" className="flex flex-wrap gap-1">
          {(['all', ...STATES.filter((s) => counts[s] > 0)] as Show[]).map((s) => (
            <button
              key={s}
              type="button"
              role="radio"
              aria-checked={show === s}
              onClick={() => setShow(s)}
              className={cn(
                'flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                show === s ? 'bg-foreground text-background' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {s === 'all' ? null : <Mark state={s} />}
              {s === 'all' ? 'All pages' : STATE_LABEL[s]}
              <span className="tabular-nums opacity-70">{formatNumber(s === 'all' ? all.length : counts[s])}</span>
            </button>
          ))}
        </div>
        <div className="flex w-full flex-wrap items-center gap-3 @3xl:w-auto">
          <div role="radiogroup" aria-label="Arrange by" className="grid auto-cols-fr grid-flow-col gap-1 rounded-md bg-muted p-1">
            {(['address', 'type'] as const).map((v) => (
              <button
                key={v}
                type="button"
                role="radio"
                aria-checked={by === v}
                onClick={() => setBy(v)}
                className={cn(
                  'rounded px-2.5 py-1 text-xs whitespace-nowrap outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                  by === v ? 'bg-background font-medium text-foreground shadow-xs' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {v === 'address' ? 'By address' : 'By page type'}
              </button>
            ))}
          </div>
          <div className="relative min-w-48 flex-1 @3xl:w-64 @3xl:flex-none">
            <SearchIcon aria-hidden className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input type="search" aria-label="Find a page" placeholder="Find by title or address" value={query} onChange={(e) => setQuery(e.target.value)} className="h-9 pl-8" />
          </div>
        </div>
      </div>

      {used.length > 0 ? (
        <div className="flex flex-wrap items-center gap-x-1 gap-y-1 border-b px-4 py-2 text-xs text-muted-foreground">
          <span className="mr-2">Blocks by kind</span>
          {used.map((c) => (
            <button
              key={c}
              type="button"
              aria-pressed={lit === c}
              onClick={() => setLit(lit === c ? null : c)}
              className={cn(
                'flex items-center gap-1.5 rounded-md px-2 py-1 outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring',
                lit === c && 'bg-muted text-foreground',
              )}
            >
              <span aria-hidden className={cn('h-3 w-1 rounded-full', CATEGORY[c])} />
              {c}
            </button>
          ))}
        </div>
      ) : null}
      {pages == null || total <= pages.length ? null : (
        <p className="border-b bg-warning-soft px-4 py-2 text-xs text-foreground">
          These are the first {formatNumber(pages.length)} of {formatNumber(total)} pages. The{' '}
          <Link to="/cms/pages" className="font-medium underline underline-offset-2">
            pages list
          </Link>{' '}
          has every one.
        </p>
      )}

      <div className="site-board grid gap-9 px-5 py-6">
        {pages == null ? (
          <Waiting />
        ) : all.length === 0 ? (
          <Empty canAdd={canAdd} />
        ) : found.length === 0 ? (
          <div className="grid justify-items-center gap-3 py-14 text-center">
            <p className="text-sm text-muted-foreground">No page matches that.</p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setShow('all')
                setQuery('')
              }}
            >
              Show every page
            </Button>
          </div>
        ) : (
          (by === 'type' ? lanesByType(found, types) : lanes(found)).map((lane, i) => (
            <LaneOfPages key={`${by}-${lane.key}`} lane={lane} whole={narrowed} custom={custom} lit={lit} adds={canAdd && i === 0 && !narrowed} />
          ))
        )}
      </div>
    </section>
  )
}

function Waiting() {
  return (
    <div role="status" aria-label="Loading the site" className="grid animate-pulse grid-cols-[repeat(auto-fill,minmax(11.5rem,1fr))] gap-3">
      {[5, 3, 6, 2, 4].map((rows, i) => (
        <span key={i} className="grid content-start gap-2 rounded-lg border bg-background p-3">
          <span className="h-3 w-2/3 rounded bg-muted" />
          <span className="h-3 w-1/2 rounded bg-muted" />
          {Array.from({ length: rows }, (_, r) => (
            <span key={r} className="h-6 rounded-[5px] bg-muted" />
          ))}
        </span>
      ))}
    </div>
  )
}

function Empty({ canAdd }: { canAdd: boolean }) {
  return (
    <div className="grid justify-items-center gap-3 py-14 text-center">
      <p className="text-base font-medium text-foreground">No pages yet</p>
      <p className="max-w-md text-sm text-muted-foreground">
        A page type says what a page holds. Set one up under{' '}
        <Link to="/cms/page-types" className="font-medium text-foreground underline underline-offset-4">
          Page types
        </Link>
        , then write the first page and it shows up here, block by block.
      </p>
      {canAdd ? (
        <Button asChild size="sm">
          <Link to="/cms/pages/new">
            <PlusIcon />
            New page
          </Link>
        </Button>
      ) : null}
    </div>
  )
}

function LaneOfPages({ lane, whole, custom, lit, adds }: { lane: Lane; whole: boolean; custom: Map<string, string>; lit: BlockCategory | null; adds: boolean }) {
  const [open, setOpen] = useState(false)
  const { name, pages } = lane
  const all = whole || open || pages.length <= LANE_OPENS_ON
  const drawn = all ? pages : pages.slice(0, LANE_OPENS_ON)
  return (
    <section aria-label={name} className="grid gap-3">
      <h2 className="flex items-baseline gap-2.5">
        <span className="text-xl font-light tracking-tight text-foreground">
          {lane.path ? (
            <>
              <span className="text-muted-foreground">/</span>
              {name.slice(1)}
            </>
          ) : (
            name
          )}
        </span>
        <span className="text-xs text-muted-foreground">
          {formatNumber(pages.length)} {pages.length === 1 ? 'page' : 'pages'}
        </span>
      </h2>
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(11.5rem,1fr))] items-start gap-3">
        {drawn.map((p) => (
          <li key={p.id} className="min-w-0">
            <Blueprint page={p} custom={custom} lit={lit} />
          </li>
        ))}
        {adds ? (
          <li className="min-w-0">
            <Link
              to="/cms/pages/new"
              className="grid min-h-28 place-items-center rounded-lg border border-dashed border-foreground/25 text-sm text-muted-foreground outline-none transition-colors hover:border-foreground/50 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              <span className="flex items-center gap-1.5">
                <PlusIcon className="size-4" />
                Add a page
              </span>
            </Link>
          </li>
        ) : null}
      </ul>
      {all ? null : (
        <Button type="button" variant="ghost" size="sm" className="justify-self-start" onClick={() => setOpen(true)}>
          Show all {formatNumber(pages.length)} in {name}
        </Button>
      )}
    </section>
  )
}

function Blueprint({ page, custom, lit }: { page: SitePage; custom: Map<string, string>; lit: BlockCategory | null }) {
  const state = stateOf(page)
  const blocks = page.outline.slice(0, BLOCKS_SHOWN)
  const more = page.outline.length - blocks.length
  return (
    <Link
      to={`/cms/pages/${page.id}`}
      aria-label={`${page.title}, ${addressOf(page)}, ${STATE_LABEL[state].toLowerCase()}, ${page.outline.length} ${page.outline.length === 1 ? 'block' : 'blocks'}`}
      className={cn(
        'group grid content-start gap-2.5 rounded-lg border bg-background p-3 outline-none transition-[border-color,box-shadow] hover:border-foreground/35 hover:shadow-sm focus-visible:ring-2 focus-visible:ring-ring',
        state !== 'live' && 'border-dashed border-foreground/25',
      )}
    >
      <span className="flex items-center justify-between gap-2 text-[11px] leading-none text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <Mark state={state} />
          {STATE_LABEL[state]}
        </span>
        <span className="truncate">{since(page.updatedAt)}</span>
      </span>
      <span className="grid gap-0.5">
        <span className="truncate text-sm font-medium text-foreground underline-offset-4 group-hover:underline">{page.title}</span>
        <span className="truncate text-xs text-muted-foreground">{addressOf(page)}</span>
      </span>
      {blocks.length === 0 ? (
        <span className="grid h-12 place-items-center rounded-[5px] border border-dashed text-[11px] text-muted-foreground">No blocks</span>
      ) : (
        <span className="grid gap-1">
          {blocks.map((type, i) => {
            const Icon = blockDescriptor(type)?.icon ?? PuzzleIcon
            const category = blockCategory(type)
            return (
              <span
                key={i}
                className={cn('flex h-6 items-center gap-1.5 overflow-hidden rounded-[5px] bg-muted pr-2 text-[11px] text-foreground/85 transition-opacity', lit && lit !== category && 'opacity-30')}
              >
                <span aria-hidden className={cn('h-full w-1 shrink-0', CATEGORY[category])} />
                <Icon aria-hidden className="size-3 shrink-0 text-muted-foreground" />
                <span className="truncate">{custom.get(type) ?? blockLabel(type)}</span>
              </span>
            )
          })}
          {more > 0 ? <span className="pt-0.5 pl-1 text-[11px] text-muted-foreground">and {more} more</span> : null}
        </span>
      )}
    </Link>
  )
}
