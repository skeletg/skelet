import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ImageOffIcon, ImageIcon, XIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { MediaAssetPicker } from '@/features/media/MediaAssetPicker'
import { MediaPickerAssetDocument } from './MediaPicker.gql'

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

export function MediaPicker({
  value,
  onChange,
  label,
  description,
  previewClassName,
}: {
  // Current asset id (nullable) - When set, the picker fetches the asset on mount so the preview thumbnail renders even
  // before the user opens the dialog
  value: string | null
  onChange: (assetId: string | null) => void
  label?: string
  description?: string
  previewClassName?: string
}) {
  // Hydrate the currently-selected asset so the preview thumbnail
  // shows immediately - Skipped when value is null
  const { data: currentData } = useQuery(
    MediaPickerAssetDocument,
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

      <MediaAssetPicker open={open} onOpenChange={setOpen} selectedIds={current ? [current.id] : []} onSelect={(asset) => onChange(asset.id)} />
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
}: {
  onPick: (url: string) => void
  label?: string
}) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button type="button" variant="outline" size="sm" className="shrink-0" onClick={() => setOpen(true)}>
        <ImageIcon className="size-4" />
        {label}
      </Button>
      <MediaAssetPicker open={open} onOpenChange={setOpen} onSelect={(asset) => onPick(asset.url)} />
    </>
  )
}
