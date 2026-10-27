import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ImageIcon, PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { MediaAssetPicker } from '@/features/media/MediaAssetPicker'
import {
  MediaAssetByIdDocument,
  type MediaAssetByIdQuery,
  type MediaAssetByIdQueryVariables,
} from '@/features/media/MediaLibrary.gql'

// Asset attribute fields: a media-library picker for the `asset` (single) and
// `assets` (ordered list) attribute input types - The stored value stays a media
// asset id string / array of ids (unchanged contract) - only the editing UX
// improves (browse + click instead of pasting UUIDs) - Thumbnails for already-
// stored ids resolve via the mediaAsset(id) query - Reorder is deferred

function AssetThumb({ id, className = 'size-16' }: { id: string; className?: string }) {
  const { data } = useQuery<MediaAssetByIdQuery, MediaAssetByIdQueryVariables>(MediaAssetByIdDocument, {
    variables: { id },
    skip: !id,
    // A thumbnail's url and alt barely change, and the app default (network-only) refetched every image each time a
    // block or gallery field remounted - cache-first fetches each asset once
    fetchPolicy: 'cache-first',
  })
  const a = data?.mediaAsset
  return (
    <div className={`flex items-center justify-center overflow-hidden rounded border bg-muted ${className}`}>
      {a?.url ? (
        <img src={a.url} alt={a.altText} className="size-full object-cover" title={a.title || a.filename} />
      ) : (
        <ImageIcon className="size-5 text-muted-foreground" />
      )}
    </div>
  )
}

export function AssetField({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string | undefined) => void
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="flex items-center gap-3">
      {value ? (
        <AssetThumb id={value} />
      ) : (
        <span className="text-sm text-muted-foreground">No image selected.</span>
      )}
      <div className="flex gap-2">
        <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => setOpen(true)}>
          {value ? 'Replace' : 'Choose from library'}
        </Button>
        {value && (
          <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => onChange(undefined)}>
            Remove
          </Button>
        )}
      </div>
      <MediaAssetPicker open={open} onOpenChange={setOpen} uploadPath="cms/pages" selectedIds={value ? [value] : []} onSelect={(a) => onChange(a.id)} />
    </div>
  )
}

export function AssetsField({
  value,
  onChange,
  disabled,
}: {
  value: string[]
  onChange: (v: string[]) => void
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const ids = Array.isArray(value) ? value : []
  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {ids.map((id, i) => (
          <div key={`${id}-${i}`} className="relative">
            <AssetThumb id={id} />
            {!disabled && (
              <button
                type="button"
                onClick={() => onChange(ids.filter((_, j) => j !== i))}
                className="absolute -right-1.5 -top-1.5 rounded-full border bg-background p-0.5 text-muted-foreground shadow-sm hover:text-destructive"
                aria-label="Remove image"
              >
                <XIcon className="size-3" />
              </button>
            )}
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          className="h-16"
          onClick={() => setOpen(true)}
        >
          <PlusIcon className="size-4" /> Add
        </Button>
      </div>
      <MediaAssetPicker
        open={open}
        onOpenChange={setOpen}
        uploadPath="cms/pages"
        multiple
        onSelectMany={(assets) => onChange([...ids, ...assets.map((a) => a.id).filter((id) => !ids.includes(id))])}
      />
    </div>
  )
}
