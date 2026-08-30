import { useState, useEffect } from 'react'
import { gql } from '@apollo/client'
import { useLazyQuery, useQuery } from '@apollo/client/react'
import { ImageOffIcon, ImageIcon, SearchIcon, XIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

// MediaPicker is the dashboard's single-asset picker - Used wherever a
// domain row references one media asset by id (promotion label /
// banner, blog cover, etc.) - Renders as a thumbnail preview with
// Browse / Replace / Remove affordances; clicking "Browse" opens a
// dialog that searches the media library by filename and lets the
// admin click an asset to bind it
//
// Selection emits a (assetId, url) tuple so the caller can render
// the thumbnail without a second roundtrip - URL is also exposed on
// the parent's stored model when GraphQL hydrates the asset
// separately

const PICKER_SEARCH_QUERY = gql`
  query MediaPickerSearch($query: String, $mimeGroup: MediaMimeGroup, $limit: Int) {
    searchMediaAssets(query: $query, mimeGroup: $mimeGroup, limit: $limit) {
      totalCount
      items {
        id
        url
        filename
        title
        altText
        mimeType
        width
        height
      }
    }
  }
`

const PICKER_ASSET_QUERY = gql`
  query MediaPickerAsset($id: ID!) {
    mediaAsset(id: $id) {
      id
      url
      filename
      title
      altText
    }
  }
`

type PickerAsset = {
  id: string
  url: string
  filename: string
  title: string
  altText: string
}

export function MediaPicker({
  value,
  onChange,
  label,
  description,
  previewClassName,
  mimeGroup,
}: {
  // Current asset id (nullable) - When set, the picker fetches the asset on mount so the preview thumbnail renders even
  // before the user opens the dialog
  value: string | null
  onChange: (assetId: string | null) => void
  label?: string
  description?: string
  previewClassName?: string
  // Optional MIME-group filter for the search dialog - Pass "IMAGE"
  // for image-only pickers (the most common case)
  mimeGroup?: 'IMAGE' | 'VIDEO' | 'DOCUMENT' | 'OTHER'
}) {
  // Hydrate the currently-selected asset so the preview thumbnail
  // shows immediately - Skipped when value is null
  const { data: currentData } = useQuery<{ mediaAsset: PickerAsset | null }>(
    PICKER_ASSET_QUERY,
    {
      variables: { id: value ?? '' },
      skip: !value,
    },
  )
  const current = currentData?.mediaAsset ?? null

  const [open, setOpen] = useState(false)

  return (
    <div className="grid gap-2">
      {label ? <p className="text-sm font-medium">{label}</p> : null}
      {description ? (
        <p className="text-xs text-muted-foreground">{description}</p>
      ) : null}

      <div className="flex items-start gap-3 rounded border bg-card p-3">
        <div
          className={cn(
            'grid size-24 shrink-0 place-items-center overflow-hidden rounded border bg-muted text-muted-foreground',
            previewClassName,
          )}
        >
          {current ? (
            <img
              src={current.url}
              alt={current.altText || current.title || current.filename}
              className="size-full object-cover"
            />
          ) : (
            <ImageOffIcon className="size-6 opacity-50" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          {current ? (
            <>
              <p className="truncate text-sm font-medium">
                {current.title || current.filename}
              </p>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {current.filename}
              </p>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">No image selected</p>
          )}
          <div className="mt-2 flex gap-2">
            <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
              <ImageIcon className="size-4" />
              {current ? 'Replace' : 'Browse'}
            </Button>
            {current ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => onChange(null)}
              >
                <XIcon className="size-4" /> Remove
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      <PickerDialog
        open={open}
        onOpenChange={setOpen}
        mimeGroup={mimeGroup}
        onPick={(asset) => {
          onChange(asset.id)
          setOpen(false)
        }}
      />
    </div>
  )
}

// MediaPickerButton is a compact "browse the library" trigger that emits the
// chosen asset's URL (not its id) - Used where a field stores a raw URL - e.g.
// the email builder's image / video-thumbnail blocks, which accept either a
// library URL or a {{merge.tag}} typed alongside it
export function MediaPickerButton({
  onPick,
  label = 'Library',
  mimeGroup = 'IMAGE',
}: {
  onPick: (url: string) => void
  label?: string
  mimeGroup?: 'IMAGE' | 'VIDEO' | 'DOCUMENT' | 'OTHER'
}) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button type="button" variant="outline" size="sm" className="shrink-0" onClick={() => setOpen(true)}>
        <ImageIcon className="size-4" />
        {label}
      </Button>
      <PickerDialog
        open={open}
        onOpenChange={setOpen}
        mimeGroup={mimeGroup}
        onPick={(asset) => {
          onPick(asset.url)
          setOpen(false)
        }}
      />
    </>
  )
}

function PickerDialog({
  open,
  onOpenChange,
  mimeGroup,
  onPick,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  mimeGroup?: 'IMAGE' | 'VIDEO' | 'DOCUMENT' | 'OTHER'
  onPick: (asset: PickerAsset) => void
}) {
  const [query, setQuery] = useState('')
  const [runSearch, { data, loading }] = useLazyQuery<{
    searchMediaAssets: { totalCount: number; items: PickerAsset[] }
  }>(PICKER_SEARCH_QUERY)

  // Re-fetch when the dialog opens / search query changes - The search query is the asset filename / title; without it the backend returns
  // the most-recent assets, which is the right default for "I just uploaded the sticker, find it" flows
  useEffect(() => {
    if (!open) return
    const handle = window.setTimeout(() => {
      runSearch({
        variables: { query: query || null, mimeGroup: mimeGroup ?? null, limit: 60 },
      })
    }, 150) // debounce typing
    return () => window.clearTimeout(handle)
  }, [open, query, mimeGroup, runSearch])

  const items = data?.searchMediaAssets.items ?? []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Pick an image</DialogTitle>
          <DialogDescription>
            Search by filename or title. The 60 newest matching assets are
            shown.
          </DialogDescription>
        </DialogHeader>
        <div className="relative">
          <SearchIcon className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search…"
            className="pl-9"
            autoFocus
          />
        </div>
        <div className="grid max-h-96 grid-cols-2 gap-2 overflow-y-auto sm:grid-cols-3 md:grid-cols-4">
          {loading && items.length === 0 ? (
            <p className="col-span-full p-4 text-center text-sm text-muted-foreground">
              Loading…
            </p>
          ) : items.length === 0 ? (
            <p className="col-span-full p-4 text-center text-sm text-muted-foreground">
              No assets found.
            </p>
          ) : (
            items.map((asset) => (
              <button
                type="button"
                key={asset.id}
                onClick={() => onPick(asset)}
                className="grid gap-1 rounded border p-1.5 text-left transition hover:border-foreground/40 hover:bg-accent/50"
              >
                <div className="grid aspect-square place-items-center overflow-hidden rounded bg-muted">
                  <img
                    src={asset.url}
                    alt={asset.altText || asset.title || asset.filename}
                    className="size-full object-cover"
                  />
                </div>
                <p className="truncate text-xs">{asset.title || asset.filename}</p>
              </button>
            ))
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
