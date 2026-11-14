import { LayoutGridIcon, ListIcon, SearchIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { MediaAssetSort, MediaMimeGroup } from '@/graphql/schema.types'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { MediaCountsQuery } from '../MediaLibrary.gql'
import { ROW_MAX, ROW_MIN, ROW_STEP, SORTS, type LibraryPrefs, type LibraryView } from './prefs'

export type TypeFilter = MediaMimeGroup | 'ALL'
const TYPES: { value: TypeFilter; label: string; count: 'all' | 'images' | 'documents' }[] = [
  { value: 'ALL', label: 'Everything', count: 'all' },
  { value: 'IMAGE', label: 'Images', count: 'images' },
  { value: 'DOCUMENT', label: 'Documents', count: 'documents' },
]
const VIEWS: { value: LibraryView; label: string; icon: typeof ListIcon }[] = [
  { value: 'gallery', label: 'Gallery', icon: LayoutGridIcon },
  { value: 'list', label: 'List', icon: ListIcon },
]

// What the wall shows and how: the kind of file, the words searched for, the order, the picture size and the view
export function Filters({
  type,
  counts,
  search,
  prefs,
  onType,
  onSearch,
  onPrefs,
}: {
  type: TypeFilter
  counts: MediaCountsQuery | undefined
  search: string
  prefs: LibraryPrefs
  onType: (type: TypeFilter) => void
  onSearch: (words: string) => void
  onPrefs: (prefs: LibraryPrefs) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <div role="group" aria-label="Show" className="flex flex-wrap gap-1">
        {TYPES.map((t) => (
          <button
            key={t.value}
            type="button"
            aria-pressed={type === t.value}
            onClick={() => onType(t.value)}
            className={cn(
              'flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
              type === t.value ? 'bg-foreground text-background' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
            )}
          >
            {t.label}
            {counts ? <span className="tabular-nums opacity-70">{formatNumber(counts[t.count].totalCount)}</span> : null}
          </button>
        ))}
      </div>
      <div className="relative min-w-48 flex-1 @3xl:max-w-sm">
        <SearchIcon aria-hidden className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input type="search" aria-label="Search media" placeholder="Search names, titles, alt text and captions" value={search} onChange={(e) => onSearch(e.target.value)} className="h-9 rounded-full pr-8 pl-9" />
        {search ? (
          <button type="button" aria-label="Clear search" onClick={() => onSearch('')} className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground">
            <XIcon className="size-4" />
          </button>
        ) : null}
      </div>
      <div className="ml-auto flex flex-wrap items-center gap-2">
        <Select value={prefs.sort} onValueChange={(v) => onPrefs({ ...prefs, sort: v as MediaAssetSort })}>
          <SelectTrigger size="sm" className="w-[9.5rem] rounded-full" aria-label="Sort">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SORTS.map((s) => (
              <SelectItem key={s.value} value={s.value}>
                {s.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {prefs.view === 'gallery' ? (
          <input
            type="range"
            aria-label="Picture size"
            min={ROW_MIN}
            max={ROW_MAX}
            step={ROW_STEP}
            value={prefs.rowHeight}
            onChange={(e) => onPrefs({ ...prefs, rowHeight: Number(e.target.value) })}
            className="h-1 w-24 cursor-pointer accent-foreground"
          />
        ) : null}
        <div role="group" aria-label="View" className="flex items-center rounded-full border p-0.5">
          {VIEWS.map((v) => (
            <Button key={v.value} type="button" variant={prefs.view === v.value ? 'secondary' : 'ghost'} size="icon" className="size-7 rounded-full" aria-pressed={prefs.view === v.value} aria-label={v.label} onClick={() => onPrefs({ ...prefs, view: v.value })}>
              <v.icon className="size-4" />
            </Button>
          ))}
        </div>
      </div>
    </div>
  )
}
