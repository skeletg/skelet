import { useCallback, useMemo, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useProductSaveSection, type SectionDiffEntry } from '../productSaveRegistry'
import { useProductListingWrite } from './productListing'

// Per-store URL override editor, shown inside the SEO section - A product can have a different URL
// path on a specific store; blank inherits the product's own URL. The override rides on the store
// listing (there's no standalone per-channel URL mutation) - the listing writer keeps its published
// and price values as they are

type ListingLite = {
  isPublished: boolean
  price?: number | null
  url?: string | null
  channel: { id: string; name: string }
}

type Row = {
  channelId: string
  name: string
  isPublished: boolean
  price: number | null
  url: string
  originalUrl: string
}

export function ChannelUrlOverridesEditor({
  productId,
  listings,
  canManage,
}: {
  productId: string
  listings: ListingLite[]
  canManage: boolean
}) {
  const initial = useMemo<Row[]>(
    () =>
      listings.map((l) => ({
        channelId: l.channel.id,
        name: l.channel.name,
        isPublished: l.isPublished,
        price: l.price ?? null,
        url: l.url ?? '',
        originalUrl: l.url ?? '',
      })),
    [listings],
  )

  const [rows, setRows] = useState<Row[]>(initial)
  // Re-seed the draft when the server listings change (post-save refetch)
  const key = JSON.stringify(initial.map((r) => [r.channelId, r.originalUrl]))
  const [prevKey, setPrevKey] = useState(key)
  if (prevKey !== key) {
    setPrevKey(key)
    setRows(initial)
  }

  const writeListing = useProductListingWrite(productId)

  const patch = (channelId: string, url: string) =>
    setRows((prev) => prev.map((r) => (r.channelId === channelId ? { ...r, url } : r)))

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    for (const r of rows) {
      if (r.url.trim() === r.originalUrl.trim()) continue
      entries.push({
        label: `${r.name} URL`,
        before: r.originalUrl || 'inherit',
        after: r.url.trim() || 'inherit',
      })
    }
    return entries
  }, [rows])

  const save = useCallback(async () => {
    for (const r of rows) {
      if (r.url.trim() === r.originalUrl.trim()) continue
      await writeListing(r.channelId, { url: r.url.trim() === '' ? null : r.url.trim() })
    }
  }, [rows, writeListing])

  useProductSaveSection({ id: 'channel-urls', title: 'Per-store URL overrides', diff, save })

  return (
    <div className="grid gap-3 rounded-md border p-4">
      <div>
        <div className="text-sm font-medium">Per-store URL overrides</div>
        <p className="text-xs text-muted-foreground">
          Optional. Give this product a different URL path on a specific store. Blank inherits the
          product&rsquo;s own URL set above.
        </p>
      </div>
      {rows.length === 0 ? (
        <p className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">
          Not listed on any store yet. Publish this product on a store to set a per-store URL.
        </p>
      ) : (
        <div className="grid gap-2">
          {rows.map((row) => (
            <div key={row.channelId} className="grid grid-cols-[minmax(7rem,1fr)_2fr] items-center gap-3">
              <Label htmlFor={`churl-${row.channelId}`} className="truncate text-sm font-normal text-muted-foreground">
                {row.name}
              </Label>
              <Input
                id={`churl-${row.channelId}`}
                className="h-8 font-mono text-xs"
                placeholder="inherit product URL"
                value={row.url}
                onChange={(e) => patch(row.channelId, e.target.value)}
                disabled={!canManage}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
