import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ImageIcon, SearchIcon } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  SearchMediaAssetsDocument,
  type MediaAsset,
  type SearchMediaAssetsQuery,
  type SearchMediaAssetsQueryVariables,
} from './MediaLibrary.gql'

// MediaAssetPicker is a modal that browses the media library and returns the
// chosen asset - It reuses the existing searchMediaAssets query (filtered to
// images) rather than forking MediaLibrary's full directory UI - a focused
// picker for embedding in field editors - (Extracting a shared MediaBrowser used
// by both this and MediaLibrary is a future refactor.)
export function MediaAssetPicker({
  open,
  onOpenChange,
  onSelect,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (asset: MediaAsset) => void
}) {
  const [query, setQuery] = useState('')
  const { data, loading } = useQuery<SearchMediaAssetsQuery, SearchMediaAssetsQueryVariables>(
    SearchMediaAssetsDocument,
    { variables: { query: query || null, mimeGroup: 'IMAGE', limit: 60, offset: 0 }, skip: !open },
  )
  const items = data?.searchMediaAssets.items ?? []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Choose an image</DialogTitle>
          <DialogDescription>Pick an image from the media library.</DialogDescription>
        </DialogHeader>
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by name…"
            className="h-9 pl-8"
          />
        </div>
        <div className="grid max-h-[60vh] grid-cols-2 gap-2 overflow-y-auto sm:grid-cols-3 md:grid-cols-4">
          {loading && items.length === 0 ? (
            <p className="col-span-full py-8 text-center text-sm text-muted-foreground">Loading…</p>
          ) : items.length === 0 ? (
            <p className="col-span-full py-8 text-center text-sm text-muted-foreground">
              No images match “{query}”.
            </p>
          ) : (
            items.map((a) => (
              <button
                key={a.id}
                type="button"
                onClick={() => {
                  onSelect(a)
                  onOpenChange(false)
                }}
                className="group flex flex-col gap-1 rounded-md border p-1.5 text-left transition-colors hover:border-primary hover:bg-accent"
                title={a.title || a.filename}
              >
                <div className="flex aspect-square w-full items-center justify-center overflow-hidden rounded bg-muted">
                  {a.url ? (
                    <img src={a.url} alt={a.altText} className="size-full object-cover" />
                  ) : (
                    <ImageIcon className="size-6 text-muted-foreground" />
                  )}
                </div>
                <span className="truncate text-[11px] leading-tight">{a.title || a.filename}</span>
              </button>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
