import { useCallback } from 'react'
import { useMutation } from '@apollo/client/react'
import { ADMIN_MEDIA_SVG_ENDPOINT } from '@/lib/api-endpoint'
import { readImageDimensions } from '@/lib/image'
import { putToSignedUrlWithProgress } from '@/lib/upload'
import { getAuthStoredState } from '@/shared/auth/auth'
import { ConfirmMediaAssetUploadDocument, RequestMediaAssetUploadDocument, type MediaAssetFieldsFragment } from './MediaLibrary.gql'

export type UploadedImage = { assetId: string; url: string }

// One file into the library under path - an SVG goes through the server, which sanitizes it, the rest to storage
export function useMediaUpload(path: string) {
  const [requestUpload] = useMutation(RequestMediaAssetUploadDocument)
  const [confirmUpload] = useMutation(ConfirmMediaAssetUploadDocument)
  return useCallback(
    async (file: File, onProgress: (fraction: number) => void = () => {}): Promise<MediaAssetFieldsFragment | { id: string; url: string }> => {
      if (file.type === 'image/svg+xml') {
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
        if (!res.ok || !payload?.id || !payload.url) throw new Error(payload?.error ?? `Upload failed (HTTP ${res.status}).`)
        onProgress(1)
        return { id: payload.id, url: payload.url }
      }
      // Best-effort: an image the browser can't decode still uploads, just without stamped dimensions
      const dims = file.type.startsWith('image/') ? await readImageDimensions(file).catch(() => ({ width: 0, height: 0 })) : null
      const req = await requestUpload({
        variables: { input: { path, filename: file.name, mimeType: file.type || 'application/octet-stream', sizeBytes: file.size, altText: '' } },
      })
      const payload = req.data?.requestMediaAssetUpload
      if (!payload) throw new Error('Upload request did not return a signed URL.')
      await putToSignedUrlWithProgress(payload.uploadUrl, file, onProgress)
      const conf = await confirmUpload({
        variables: { assetId: payload.asset.id, width: dims?.width || undefined, height: dims?.height || undefined },
      })
      const asset = conf.data?.confirmMediaAssetUpload
      if (!asset) throw new Error('Upload confirmation failed.')
      return asset
    },
    [path, requestUpload, confirmUpload],
  )
}

export function useImageUpload(path: string) {
  const upload = useMediaUpload(path)
  return useCallback(
    async (file: File): Promise<UploadedImage> => {
      if (!file.type.startsWith('image/')) throw new Error('Only image files can be uploaded.')
      const asset = await upload(file)
      return { assetId: asset.id, url: asset.url }
    },
    [upload],
  )
}
