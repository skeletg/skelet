import { useRef, useState } from 'react'
import { ImageIcon, ImagePlusIcon, LibraryIcon, Trash2Icon, UploadIcon } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { MediaAssetPicker } from '@/features/media/MediaAssetPicker'
import { useImageUpload } from '@/features/media/useImageUpload'
import type { OptionImageRef } from '../productOptionsStaging'

// A picture on an option or a choice - a thumbnail that opens: pick from the library, upload one, or remove it
export function OptionPicture({
  value,
  onChange,
  label,
  disabled,
  size = 'md',
}: {
  value: OptionImageRef | null
  onChange: (next: OptionImageRef | null) => void
  // Names the control for screen readers - "Gift box" reads as "Gift box picture"
  label: string
  disabled?: boolean
  size?: 'sm' | 'md'
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const upload = useImageUpload('option-pictures')
  const box = size === 'sm' ? 'size-8' : 'size-20'

  return (
    <div className="grid justify-items-start gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild disabled={disabled || uploading}>
          <button
            type="button"
            aria-label={`${label} picture`}
            className={cn(
              'flex shrink-0 items-center justify-center overflow-hidden rounded-md border bg-muted/30 text-muted-foreground transition-colors hover:bg-muted disabled:opacity-50',
              !value && 'border-dashed',
              box,
            )}
          >
            {value ? (
              <img src={value.url} alt={value.altText ?? ''} className="size-full object-cover" />
            ) : uploading ? (
              <ImageIcon className="size-4 animate-pulse" />
            ) : (
              <ImagePlusIcon className={size === 'sm' ? 'size-4' : 'size-5'} />
            )}
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem onSelect={() => setPickerOpen(true)}>
            <LibraryIcon className="size-4" />
            Choose from the library
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => fileRef.current?.click()}>
            <UploadIcon className="size-4" />
            Upload a picture
          </DropdownMenuItem>
          {value ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => onChange(null)}>
                <Trash2Icon className="size-4" />
                Remove the picture
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        className="hidden"
        aria-label={`Upload ${label} picture`}
        onChange={async (e) => {
          const file = e.target.files?.[0]
          e.target.value = ''
          if (!file) return
          setError(null)
          setUploading(true)
          try {
            onChange(await upload(file))
          } catch (err) {
            setError(err instanceof Error ? err.message : 'Upload failed.')
          } finally {
            setUploading(false)
          }
        }}
      />
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
      <MediaAssetPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        title={`Picture for ${label}`}
        uploadPath="option-pictures"
        selectedIds={value ? [value.assetId] : []}
        onSelect={(a) => onChange({ assetId: a.id, url: a.url, altText: a.altText || '' })}
      />
    </div>
  )
}
