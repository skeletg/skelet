import { useCallback, useState } from 'react'
import { randomId } from '@/lib/random-id'
import { useMediaUpload } from '../useImageUpload'

export type UploadItem = {
  key: string
  name: string
  progress: number
  status: 'uploading' | 'done' | 'error'
  error?: string
  assetId?: string
}

// Several files at once, each with its own progress - a failure stays in the list with its reason until dismissed
export function useUploadQueue(path: string, onUploaded: (assetIds: string[]) => void) {
  const upload = useMediaUpload(path)
  const [items, setItems] = useState<UploadItem[]>([])
  const patch = (key: string, p: Partial<UploadItem>) => setItems((list) => list.map((i) => (i.key === key ? { ...i, ...p } : i)))

  const add = useCallback(
    async (files: File[], accept: (file: File) => string | null) => {
      const batch = files.map((file) => ({ file, key: randomId(), reason: accept(file) }))
      setItems((list) => [
        ...list.filter((i) => i.status !== 'done'),
        ...batch.map((b): UploadItem => ({ key: b.key, name: b.file.name, progress: 0, status: b.reason ? 'error' : 'uploading', error: b.reason ?? undefined })),
      ])
      const results = await Promise.all(
        batch
          .filter((b) => !b.reason)
          .map(async (b) => {
            try {
              const asset = await upload(b.file, (progress) => patch(b.key, { progress }))
              patch(b.key, { status: 'done', progress: 1, assetId: asset.id })
              return asset.id
            } catch (e) {
              patch(b.key, { status: 'error', error: e instanceof Error ? e.message : 'Upload failed.' })
              return null
            }
          }),
      )
      const done = results.filter((id): id is string => id !== null)
      if (done.length > 0) onUploaded(done)
    },
    [upload, onUploaded],
  )

  const dismiss = useCallback((key: string) => setItems((list) => list.filter((i) => i.key !== key)), [])
  const clearFinished = useCallback(() => setItems((list) => list.filter((i) => i.status === 'uploading')), [])
  return { items, add, dismiss, clearFinished, busy: items.some((i) => i.status === 'uploading') }
}
