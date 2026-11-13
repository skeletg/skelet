import { useState, type ComponentProps } from 'react'
import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import type { MediaAssetFieldsFragment as MediaAsset } from './MediaLibrary.gql'

function reachable(url: string) {
  if (!url.startsWith('/')) return url
  try {
    return new URL(url, new URL(ADMIN_GRAPHQL_ENDPOINT, window.location.href)).href
  } catch {
    return url
  }
}

// A library picture at grid size: its small copy, which may be a path on the API's origin, and the original should that not load
export function AssetImage({ asset, ...img }: { asset: Pick<MediaAsset, 'url' | 'thumbnailUrl' | 'altText'> } & Omit<ComponentProps<'img'>, 'src' | 'alt'>) {
  const [failed, setFailed] = useState<string | null>(null)
  const small = asset.thumbnailUrl
  return <img {...img} src={failed === small ? asset.url : reachable(small)} alt={asset.altText} loading="lazy" onError={() => setFailed(small)} />
}
