import { useRef, type KeyboardEvent } from 'react'
import { CheckIcon, FileTextIcon, ImageIcon, LoaderIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatBytes } from '@/lib/format'
import type { MediaAssetFieldsFragment as MediaAsset } from '../MediaLibrary.gql'

export type GridSize = 'small' | 'large'

const GRID_COLUMNS: Record<GridSize, string> = {
  small: 'grid-cols-[repeat(auto-fill,minmax(7.5rem,1fr))]',
  large: 'grid-cols-[repeat(auto-fill,minmax(11rem,1fr))]',
}

const typeLabel = (a: MediaAsset) => (a.mimeType.split('/')[1] ?? a.mimeType).replace(/\+xml$/, '').toUpperCase()
const dims = (a: MediaAsset) => (a.width && a.height ? `${a.width}×${a.height}` : null)

// Transparent images read better on a checkerboard than on flat grey
const CHECKER =
  'bg-[length:16px_16px] bg-[linear-gradient(45deg,var(--color-muted)_25%,transparent_25%),linear-gradient(-45deg,var(--color-muted)_25%,transparent_25%),linear-gradient(45deg,transparent_75%,var(--color-muted)_75%),linear-gradient(-45deg,transparent_75%,var(--color-muted)_75%)] bg-[position:0_0,0_8px,8px_-8px,-8px_0]'

// Tiles or rows - a click picks, a double click or Enter uses it, the arrows move through it like a grid
export function AssetGrid({
  assets,
  view,
  size,
  selectedIds,
  focusedId,
  pickable,
  onFocus,
  onToggle,
  onUse,
}: {
  assets: MediaAsset[]
  view: 'grid' | 'list'
  size: GridSize
  selectedIds: ReadonlySet<string>
  focusedId: string | null
  // Why an asset can't be picked here, or null when it can
  pickable: (a: MediaAsset) => string | null
  onFocus: (id: string) => void
  onToggle: (a: MediaAsset, extend: { range: boolean; additive: boolean }) => void
  onUse: (a: MediaAsset) => void
}) {
  const root = useRef<HTMLDivElement>(null)

  const move = (from: number, e: KeyboardEvent) => {
    const cols = view === 'list' ? 1 : (root.current ? getComputedStyle(root.current).gridTemplateColumns.split(' ').length : 1) || 1
    const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols, Home: -from, End: assets.length - 1 - from }[e.key]
    if (step !== undefined) {
      e.preventDefault()
      const next = assets[Math.max(0, Math.min(assets.length - 1, from + step))]
      onFocus(next.id)
      root.current?.querySelector<HTMLElement>(`[data-asset="${next.id}"]`)?.focus()
      return
    }
    const a = assets[from]
    if (e.key === 'Enter' && !pickable(a)) {
      e.preventDefault()
      onUse(a)
    } else if (e.key === ' ' && !pickable(a)) {
      e.preventDefault()
      onToggle(a, { range: e.shiftKey, additive: true })
    }
  }

  const tabStop = focusedId && assets.some((a) => a.id === focusedId) ? focusedId : assets[0]?.id

  if (view === 'list') {
    return (
      <div ref={root} role="listbox" aria-label="Media" aria-multiselectable className="divide-y rounded-md border">
        <div className="grid grid-cols-[3rem_minmax(0,1fr)_5rem_6rem_5rem] gap-3 bg-muted/40 px-3 py-1.5 text-xs font-medium text-muted-foreground" aria-hidden>
          <span />
          <span>Name</span>
          <span>Type</span>
          <span>Size</span>
          <span className="text-right">Added</span>
        </div>
        {assets.map((a, i) => {
          const reason = pickable(a)
          const selected = selectedIds.has(a.id)
          return (
            <div
              key={a.id}
              data-asset={a.id}
              role="option"
              aria-selected={selected}
              aria-disabled={!!reason}
              title={reason ?? undefined}
              tabIndex={a.id === tabStop ? 0 : -1}
              onFocus={() => onFocus(a.id)}
              onClick={(e) => !reason && onToggle(a, { range: e.shiftKey, additive: e.metaKey || e.ctrlKey })}
              onDoubleClick={() => !reason && onUse(a)}
              onKeyDown={(e) => move(i, e)}
              className={cn(
                'grid cursor-pointer grid-cols-[3rem_minmax(0,1fr)_5rem_6rem_5rem] items-center gap-3 px-3 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
                selected ? 'bg-primary/10' : a.id === focusedId ? 'bg-muted/60' : 'hover:bg-muted/40',
                reason && 'cursor-not-allowed opacity-50',
              )}
            >
              <Thumb asset={a} className="size-10 rounded" />
              <span className="grid min-w-0">
                <span className="truncate font-medium">{a.title || a.filename}</span>
                <span className="truncate text-xs text-muted-foreground">{a.directoryPath || 'Library'}{dims(a) ? ` · ${dims(a)}` : ''}</span>
              </span>
              <span className="text-xs text-muted-foreground">{typeLabel(a)}</span>
              <span className="text-xs tabular-nums text-muted-foreground">{formatBytes(a.sizeBytes)}</span>
              <span className="text-right text-xs tabular-nums text-muted-foreground">{new Date(a.createdAt).toLocaleDateString()}</span>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <div ref={root} role="listbox" aria-label="Media" aria-multiselectable className={cn('grid gap-3', GRID_COLUMNS[size])}>
      {assets.map((a, i) => {
        const reason = pickable(a)
        const selected = selectedIds.has(a.id)
        return (
          <div
            key={a.id}
            data-asset={a.id}
            role="option"
            aria-selected={selected}
            aria-disabled={!!reason}
            aria-label={a.title || a.filename}
            title={reason ?? (a.title || a.filename)}
            tabIndex={a.id === tabStop ? 0 : -1}
            onFocus={() => onFocus(a.id)}
            onClick={(e) => !reason && onToggle(a, { range: e.shiftKey, additive: e.metaKey || e.ctrlKey })}
            onDoubleClick={() => !reason && onUse(a)}
            onKeyDown={(e) => move(i, e)}
            className={cn(
              'group relative grid cursor-pointer gap-1.5 rounded-md border p-1.5 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
              selected ? 'border-primary bg-primary/5 ring-1 ring-primary' : a.id === focusedId ? 'border-foreground/30' : 'hover:border-foreground/30',
              reason && 'cursor-not-allowed opacity-50',
            )}
          >
            <Thumb asset={a} className="aspect-square w-full rounded" />
            <span
              aria-hidden
              className={cn(
                'absolute left-2.5 top-2.5 grid size-5 place-items-center rounded-full border shadow-sm transition-opacity',
                selected ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background/90 opacity-0 group-hover:opacity-100',
              )}
            >
              {selected ? <CheckIcon className="size-3" /> : null}
            </span>
            <span className="grid px-0.5">
              <span className="truncate text-xs font-medium">{a.title || a.filename}</span>
              <span className="truncate text-[11px] text-muted-foreground">
                {[dims(a), formatBytes(a.sizeBytes)].filter(Boolean).join(' · ')}
              </span>
            </span>
          </div>
        )
      })}
    </div>
  )
}

export function Thumb({ asset, className }: { asset: MediaAsset; className?: string }) {
  if (asset.status !== 'READY') {
    return (
      <span className={cn('grid place-items-center bg-muted text-muted-foreground', className)}>
        <LoaderIcon className="size-5 animate-spin" />
      </span>
    )
  }
  if (asset.mimeGroup !== 'IMAGE') {
    return (
      <span className={cn('grid place-items-center bg-muted text-muted-foreground', className)}>
        <FileTextIcon className="size-6" />
      </span>
    )
  }
  return (
    <span className={cn('block overflow-hidden', CHECKER, className)}>
      {asset.url ? (
        <img src={asset.url} alt={asset.altText} loading="lazy" className="size-full object-cover" />
      ) : (
        <ImageIcon className="m-auto size-6 text-muted-foreground" />
      )}
    </span>
  )
}
