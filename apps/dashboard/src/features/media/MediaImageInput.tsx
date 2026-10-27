import { useRef, useState } from 'react'
import { ImageIcon, LibraryIcon, UploadIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { MediaAssetPicker } from './MediaAssetPicker'
import { type AssetRef, assetPreviewUrl, isAssetRef } from './asset-ref'
import { useImageUpload } from './useImageUpload'

type ImageValue = string | AssetRef | undefined

// MediaImageInput is a reusable image field with three ways to set an image:
//  • Library - pick an existing media asset (stores an AssetRef → carries alt +
//    dimensions to the storefront and survives renames),
//  • Upload - push a new file into the library (stores an AssetRef too),
//  • URL - paste an external/manual URL (a plain string)
// When the value is a library ref, an inline "Alt text" override is offered
// (falls back to the asset's own altText when blank)
export function MediaImageInput({
  value,
  onChange,
  disabled,
  path = 'cms/pages',
  placeholder,
}: {
  value: ImageValue
  onChange: (value: ImageValue) => void
  disabled?: boolean
  /** Media-library folder uploads are scoped under (for later bulk cleanup) */
  path?: string
  placeholder?: string
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  const upload = useImageUpload(path)

  const ref = isAssetRef(value) ? value : null
  const urlStr = typeof value === 'string' ? value : ''
  const preview = assetPreviewUrl(value)

  async function handleFile(file: File) {
    setError(null)
    setUploading(true)
    try {
      const asset = await upload(file)
      onChange({ type: 'asset', ...asset, alt: '' })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Upload failed.')
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  return (
    <div className="grid gap-2">
      <div className="flex gap-2">
        {ref ? (
          <div className="flex flex-1 items-center gap-2 rounded-md border bg-muted/40 px-2.5 py-1.5 text-sm">
            <ImageIcon className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate text-muted-foreground">Library image</span>
            <button
              type="button"
              onClick={() => onChange(undefined)}
              disabled={disabled}
              className="ml-auto rounded-sm text-muted-foreground hover:text-foreground"
              aria-label="Remove image"
            >
              <XIcon className="size-4" />
            </button>
          </div>
        ) : (
          <Input
            type="url"
            value={urlStr}
            placeholder={placeholder ?? 'https://… or pick from library →'}
            disabled={disabled || uploading}
            onChange={(e) => onChange(e.target.value || undefined)}
            className="flex-1"
          />
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || uploading}
          onClick={() => setPickerOpen(true)}
        >
          <LibraryIcon className="size-4" />
          Library
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) void handleFile(f)
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || uploading}
          onClick={() => fileRef.current?.click()}
        >
          <UploadIcon className="size-4" />
          {uploading ? 'Uploading…' : 'Upload'}
        </Button>
      </div>

      {ref && (
        <Input
          value={ref.alt ?? ''}
          onChange={(e) => onChange({ ...ref, alt: e.target.value })}
          placeholder="Alt text (optional - defaults to the asset's)"
          disabled={disabled}
          className="h-8 text-xs"
        />
      )}

      {error && <p className="text-xs text-destructive">{error}</p>}
      {preview && (
        <img
          src={preview}
          alt=""
          className="h-20 w-full rounded-md border object-cover"
          onError={(e) => {
            e.currentTarget.style.display = 'none'
          }}
        />
      )}

      <MediaAssetPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        uploadPath={path}
        selectedIds={ref ? [ref.assetId] : []}
        onSelect={(a) => onChange({ type: 'asset', assetId: a.id, url: a.url, alt: a.altText || '' })}
      />
    </div>
  )
}
