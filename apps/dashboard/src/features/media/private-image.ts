import { useEffect, useState, type RefCallback } from 'react'
import { fetchWithSession } from '@/lib/authed-download'
import { mediaFileUrl } from '@/lib/api-endpoint'
import { onAdminChange } from '@/shared/auth/auth'
import type { MediaAssetFieldsFragment as MediaAsset } from './MediaLibrary.gql'

// A private file has no address, so its picture comes through the API and is held in memory, within BUDGET bytes, until the admin changes
const BUDGET = 96 << 20
// What the server resizes - media.CanResize on its side, which this has to follow
export const RESIZABLE = ['image/jpeg', 'image/png', 'image/webp']
// Shown whole only while small. An SVG can carry script, and on a blob address it would run as the dashboard
const WHOLE = ['image/gif', 'image/avif', ...RESIZABLE]
export const WHOLE_PICTURE_MAX = 4 << 20

type Kept = { url: Promise<string>; bytes: number }
const held = new Map<string, Kept>()
let total = 0

function drop(key: string) {
  const kept = held.get(key)
  if (!kept) return
  held.delete(key)
  total -= kept.bytes
  void kept.url.then((url) => URL.revokeObjectURL(url)).catch(() => {})
}

onAdminChange(() => {
  for (const key of [...held.keys()]) drop(key)
})

// A picture still on its way frees nothing, and dropping it would hand its tile an address that is already gone
function trim() {
  for (const [key, kept] of held) {
    if (total <= BUDGET) return
    if (kept.bytes > 0 && held.size > 1) drop(key)
  }
}

function fetched(assetId: string, width?: number): Promise<string> {
  const key = `${assetId}@${width ?? 'full'}`
  const known = held.get(key)
  if (known) {
    held.delete(key)
    held.set(key, known)
    return known.url
  }
  const kept: Kept = { url: Promise.resolve(''), bytes: 0 }
  kept.url = fetchWithSession(mediaFileUrl(assetId, width))
    .then((res) => (res.ok ? res.blob() : Promise.reject(new Error(`HTTP ${res.status}`))))
    .then((blob) => {
      if (held.get(key) === kept) {
        kept.bytes = blob.size
        total += blob.size
        trim()
      }
      return URL.createObjectURL(blob)
    })
  held.set(key, kept)
  kept.url.catch(() => held.get(key) === kept && held.delete(key))
  return kept.url
}

export type PrivatePicture = { url: string | null; failed: boolean }

export function usePrivateImage(assetId: string | null, width?: number): PrivatePicture {
  const [shown, setShown] = useState<{ key: string; url: string | null } | null>(null)
  const key = assetId ? `${assetId}@${width ?? 'full'}` : null
  useEffect(() => {
    if (!assetId || !key) return
    let live = true
    fetched(assetId, width)
      .then((url) => live && setShown({ key, url }))
      .catch(() => live && setShown({ key, url: null }))
    return () => {
      live = false
    }
  }, [assetId, width, key])
  const mine = shown?.key === key ? shown : null
  return { url: mine?.url ?? null, failed: !!mine && mine.url === null }
}

type Pictured = Pick<MediaAsset, 'id' | 'visibility' | 'mimeType' | 'sizeBytes'>

// A resized copy where the server makes one, the whole file only while small and safe to draw, and nothing otherwise
export function usePrivatePicture(asset: Pictured, width: number, near = true): PrivatePicture & { shown: boolean } {
  const resized = RESIZABLE.includes(asset.mimeType)
  const shown = asset.visibility === 'PRIVATE' && (resized || (WHOLE.includes(asset.mimeType) && asset.sizeBytes <= WHOLE_PICTURE_MAX))
  return { ...usePrivateImage(shown && near ? asset.id : null, resized ? width : undefined), shown }
}

// Whether an element has come near the screen, and stays true once it has - a wall of private tiles asks the server only
// for the ones someone could see. Without an IntersectionObserver to ask, it says yes at once
export function useNearScreen<T extends Element>(): [RefCallback<T>, boolean] {
  const [near, setNear] = useState(() => typeof IntersectionObserver === 'undefined')
  const [el, setEl] = useState<T | null>(null)
  useEffect(() => {
    if (near || !el) return
    const watch = new IntersectionObserver(([seen]) => seen?.isIntersecting && setNear(true), { rootMargin: '400px' })
    watch.observe(el)
    return () => watch.disconnect()
  }, [el, near])
  return [setEl, near]
}
