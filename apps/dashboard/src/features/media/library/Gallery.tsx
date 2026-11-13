import { useRef, type KeyboardEvent } from 'react'
import { CheckIcon, FileTextIcon, LoaderIcon } from 'lucide-react'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { MediaAssetFieldsFragment as MediaAsset } from '../MediaLibrary.gql'
import { CHECKER } from '../picker/AssetGrid'

// A picture keeps its own shape within these bounds; past them it is fitted whole, as a wide logo needs
const RATIO_MIN = 0.5
const RATIO_MAX = 2.4
// A row short of pictures stretches the ones it has; past this it leaves the row's end empty instead
const STRETCH_MAX = 1.35

function shape(a: MediaAsset): { ratio: number; whole: boolean } {
  if (a.mimeGroup !== 'IMAGE' || a.status !== 'READY') return { ratio: 1, whole: false }
  if (!a.width || !a.height) return { ratio: 1.2, whole: true }
  const real = a.width / a.height
  const ratio = Math.min(RATIO_MAX, Math.max(RATIO_MIN, real))
  return { ratio, whole: ratio !== real }
}

const kind = (a: MediaAsset) => (a.mimeType.split('/')[1] ?? a.mimeType).replace(/\+xml$/, '').toUpperCase()

// A wall of pictures: rows of one height, each picture as wide as its shape makes it. Arrows walk it, Enter opens
export function Gallery({
  assets,
  rowHeight,
  selectedIds,
  focusedId,
  onFocus,
  onToggle,
  onOpen,
}: {
  assets: MediaAsset[]
  rowHeight: number
  selectedIds: ReadonlySet<string>
  focusedId: string | null
  onFocus: (id: string) => void
  onToggle: (a: MediaAsset, extend: { range: boolean; additive: boolean }) => void
  onOpen: (a: MediaAsset) => void
}) {
  const root = useRef<HTMLDivElement>(null)

  // Up and down land on the tile nearest above or below, since the rows don't share columns
  const neighbour = (from: number, key: string): number => {
    if (key === 'ArrowRight') return Math.min(assets.length - 1, from + 1)
    if (key === 'ArrowLeft') return Math.max(0, from - 1)
    if (key === 'Home') return 0
    if (key === 'End') return assets.length - 1
    const tiles = Array.from(root.current?.querySelectorAll<HTMLElement>('[data-asset]') ?? [])
    const here = tiles[from]?.getBoundingClientRect()
    if (!here) return from
    const centre = here.left + here.width / 2
    let best = from
    let bestGap = Infinity
    let row: number | null = null
    const down = key === 'ArrowDown'
    for (let i = down ? from + 1 : from - 1; i >= 0 && i < tiles.length; i += down ? 1 : -1) {
      const r = tiles[i]!.getBoundingClientRect()
      if (Math.abs(r.top - here.top) < 4) continue
      if (row === null) row = r.top
      if (Math.abs(r.top - row) > 4) break
      const gap = Math.abs(r.left + r.width / 2 - centre)
      if (gap < bestGap) {
        best = i
        bestGap = gap
      }
    }
    return best
  }

  const onKey = (from: number, e: KeyboardEvent) => {
    const a = assets[from]!
    if (['ArrowRight', 'ArrowLeft', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(e.key)) {
      e.preventDefault()
      const next = assets[neighbour(from, e.key)]!
      onFocus(next.id)
      root.current?.querySelector<HTMLElement>(`[data-asset="${next.id}"]`)?.focus()
    } else if (e.key === 'Enter') {
      e.preventDefault()
      onOpen(a)
    } else if (e.key === ' ') {
      e.preventDefault()
      onToggle(a, { range: e.shiftKey, additive: true })
    }
  }

  const tabStop = focusedId && assets.some((a) => a.id === focusedId) ? focusedId : assets[0]?.id

  return (
    <div ref={root} role="listbox" aria-label="Media" aria-multiselectable className="flex flex-wrap gap-2 after:flex-[1000_1_0%] after:content-['']">
      {assets.map((a, i) => {
        const { ratio, whole } = shape(a)
        const selected = selectedIds.has(a.id)
        const name = a.title || a.filename
        return (
          <div
            key={a.id}
            data-asset={a.id}
            role="option"
            aria-selected={selected}
            aria-label={name}
            tabIndex={a.id === tabStop ? 0 : -1}
            onFocus={() => onFocus(a.id)}
            onClick={(e) => onToggle(a, { range: e.shiftKey, additive: e.metaKey || e.ctrlKey })}
            onDoubleClick={() => onOpen(a)}
            onKeyDown={(e) => onKey(i, e)}
            style={{ flexGrow: ratio, flexBasis: `${Math.round(ratio * rowHeight)}px`, maxWidth: `${Math.round(ratio * rowHeight * STRETCH_MAX)}px` }}
            className={cn(
              'group relative cursor-pointer overflow-hidden rounded-lg outline-none transition-shadow focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
              whole ? 'bg-background' : 'bg-muted',
              selected ? 'ring-2 ring-foreground ring-offset-2 ring-offset-background' : a.id === focusedId && 'ring-1 ring-foreground/40',
            )}
          >
            <i aria-hidden className="block" style={{ paddingBottom: `${100 / ratio}%` }} />
            {a.status !== 'READY' ? (
              <span className="absolute inset-0 grid place-items-center text-muted-foreground">
                <LoaderIcon className="size-6 animate-spin" />
              </span>
            ) : a.mimeGroup === 'IMAGE' ? (
              <span className={cn('absolute inset-0', whole && cn(CHECKER, 'rounded-lg ring-1 ring-foreground/10 ring-inset'))}>
                <img
                  src={a.url}
                  alt={a.altText}
                  loading="lazy"
                  draggable={false}
                  className={cn('size-full transition-transform duration-300 group-hover:scale-[1.03]', whole ? 'object-contain p-3' : 'object-cover')}
                />
              </span>
            ) : (
              <span className="absolute inset-0 grid place-items-center content-center gap-2 text-muted-foreground">
                <FileTextIcon className="size-9" />
                <span className="rounded bg-background px-1.5 py-0.5 text-[11px] font-medium text-foreground">{kind(a)}</span>
              </span>
            )}
            <span
              aria-hidden
              className={cn(
                'absolute top-2 left-2 grid size-5 place-items-center rounded-full border shadow-sm transition-opacity',
                selected ? 'border-foreground bg-foreground text-background' : 'border-border bg-background/90 opacity-0 group-hover:opacity-100',
              )}
            >
              {selected ? <CheckIcon className="size-3" /> : null}
            </span>
            <span className="pointer-events-none absolute inset-x-0 bottom-0 grid gap-0.5 bg-gradient-to-t from-black/75 via-black/35 to-transparent px-2.5 pt-8 pb-2 text-white opacity-0 transition-opacity duration-200 group-hover:opacity-100 group-focus-visible:opacity-100">
              <span className="truncate text-xs font-medium">{name}</span>
              <span className="truncate text-[11px] text-white/75">
                {[a.width && a.height ? `${a.width}×${a.height}` : kind(a), formatBytes(a.sizeBytes)].join(', ')}
              </span>
            </span>
          </div>
        )
      })}
    </div>
  )
}
