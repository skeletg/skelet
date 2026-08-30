// What a CMS image field stores when it points at a media-library asset rather than a plain URL -
// the backend re-resolves by assetId at the edge, so `url` is only a dashboard thumbnail hint
export type AssetRef = {
  type: 'asset'
  assetId: string
  url: string
  alt?: string
  focalX?: number
  focalY?: number
}

export function isAssetRef(v: unknown): v is AssetRef {
  return (
    typeof v === 'object' &&
    v !== null &&
    (v as { type?: unknown }).type === 'asset' &&
    typeof (v as { assetId?: unknown }).assetId === 'string'
  )
}

export function assetPreviewUrl(v: unknown): string {
  if (isAssetRef(v)) return v.url
  if (typeof v === 'string') return v
  return ''
}
