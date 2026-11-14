import { useState, type ComponentProps } from 'react'
import { ImageIcon } from 'lucide-react'
import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import { cn } from '@/lib/utils'
import type { MediaAssetFieldsFragment as MediaAsset } from './MediaLibrary.gql'
import { useNearScreen, usePrivatePicture } from './private-image'

const GRID_WIDTH = 640

function reachable(url: string) {
  if (!url.startsWith('/')) return url
  try {
    return new URL(url, new URL(ADMIN_GRAPHQL_ENDPOINT, window.location.href)).href
  } catch {
    return url
  }
}

type Pictured = Pick<MediaAsset, 'id' | 'visibility' | 'mimeType' | 'sizeBytes' | 'url' | 'thumbnailUrl' | 'altText'>

// A library picture at grid size: its small copy, then the original should that not load. A private one has no address, so it comes through the API
export function AssetImage({ asset, ...img }: { asset: Pictured } & Omit<ComponentProps<'img'>, 'src' | 'alt'>) {
  const [failed, setFailed] = useState<string | null>(null)
  const [watch, near] = useNearScreen<HTMLSpanElement>()
  const picture = usePrivatePicture(asset, GRID_WIDTH, near)
  if (asset.visibility === 'PRIVATE') {
    if (picture.url && failed !== picture.url) return <img {...img} src={picture.url} alt={asset.altText} onError={() => setFailed(picture.url)} />
    const waiting = picture.shown && !picture.failed && !picture.url
    return (
      <span ref={watch} aria-hidden className={cn(img.className, 'grid place-items-center text-muted-foreground')}>
        {waiting ? null : <ImageIcon className="size-6" />}
      </span>
    )
  }
  const small = asset.thumbnailUrl
  return <img {...img} src={failed === small ? asset.url : reachable(small)} alt={asset.altText} loading="lazy" onError={() => setFailed(small)} />
}
