import { useRef, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { ImageIcon, LibraryIcon, UploadIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  ConfirmMediaAssetUploadDocument,
  RequestMediaAssetUploadDocument,
  type ConfirmMediaAssetUploadMutation,
  type ConfirmMediaAssetUploadMutationVariables,
  type RequestMediaAssetUploadMutation,
  type RequestMediaAssetUploadMutationVariables,
} from './MediaLibrary.gql'
import { ADMIN_MEDIA_SVG_ENDPOINT } from '@/lib/api-endpoint'
import { getAuthStoredState } from '@/shared/auth/auth'
import { MediaAssetPicker } from './MediaAssetPicker'
import { type AssetRef, assetPreviewUrl, isAssetRef } from './asset-ref'

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

  const [requestUpload] = useMutation<RequestMediaAssetUploadMutation, RequestMediaAssetUploadMutationVariables>(
    RequestMediaAssetUploadDocument,
  )
  const [confirmUpload] = useMutation<ConfirmMediaAssetUploadMutation, ConfirmMediaAssetUploadMutationVariables>(
    ConfirmMediaAssetUploadDocument,
  )

  const ref = isAssetRef(value) ? value : null
  const urlStr = typeof value === 'string' ? value : ''
  const preview = assetPreviewUrl(value)

  // SVG takes its own route: the server sanitizes the markup before storing it, which it can only do
  // if the bytes come through it rather than straight to S3 on a presigned URL
  async function uploadSvg(file: File) {
    const body = new FormData()
    body.append('file', file)
    body.append('path', path)
    const token = getAuthStoredState()?.accessToken
    const res = await fetch(ADMIN_MEDIA_SVG_ENDPOINT, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      body,
    })
    const payload = (await res.json().catch(() => null)) as { id?: string; url?: string; error?: string } | null
    if (!res.ok || !payload?.id || !payload.url) {
      throw new Error(payload?.error ?? `Upload failed (HTTP ${res.status}).`)
    }
    onChange({ type: 'asset', assetId: payload.id, url: payload.url, alt: '' })
  }

  async function handleFile(file: File) {
    setError(null)
    if (!file.type.startsWith('image/')) {
      setError('Only image files can be uploaded.')
      return
    }
    setUploading(true)
    try {
      if (file.type === 'image/svg+xml') {
        await uploadSvg(file)
        return
      }
      const dims = await readImageDimensions(file)
      const req = await requestUpload({
        variables: {
          input: { path, filename: file.name, mimeType: file.type, sizeBytes: file.size, altText: '' },
        },
      })
      const payload = req.data?.requestMediaAssetUpload
      if (!payload) throw new Error('Upload request did not return a signed URL.')
      const put = await fetch(payload.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': file.type },
        body: file,
      })
      if (!put.ok) throw new Error(`Upload failed (HTTP ${put.status}).`)
      const conf = await confirmUpload({
        variables: { assetId: payload.asset.id, width: dims.width, height: dims.height },
      })
      const asset = conf.data?.confirmMediaAssetUpload
      if (!asset) throw new Error('Upload confirmation failed.')
      onChange({ type: 'asset', assetId: payload.asset.id, url: asset.url, alt: '' })
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
        onSelect={(a) => onChange({ type: 'asset', assetId: a.id, url: a.url, alt: a.altText || '' })}
      />
    </div>
  )
}

// readImageDimensions probes a File for its natural size so the confirm step can
// stamp width/height on the asset - Resolves 0×0 on a decode error (best-effort)
function readImageDimensions(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve({ width: img.naturalWidth, height: img.naturalHeight })
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      resolve({ width: 0, height: 0 })
    }
    img.src = url
  })
}
