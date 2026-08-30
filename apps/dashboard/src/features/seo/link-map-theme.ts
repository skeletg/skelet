export type MapNodeType =
  | 'PRODUCT'
  | 'CATEGORY'
  | 'COLLECTION'
  | 'CMS_PAGE'
  | 'BLOG_POST'
  | 'BLOG_CATEGORY'
  | 'BLOG_TAG'
  | 'BLOG_AUTHOR'
  | 'OTHER'

export const NODE_COLORS: Record<MapNodeType, string> = { // fixed hues, not theme tokens - these carry meaning and the palette is user-customisable
  CMS_PAGE: '#34d399',
  CATEGORY: '#a78bfa',
  COLLECTION: '#e879f9',
  PRODUCT: '#38bdf8',
  BLOG_POST: '#fbbf24',
  BLOG_CATEGORY: '#fb923c',
  BLOG_TAG: '#fb7185',
  BLOG_AUTHOR: '#2dd4bf',
  OTHER: '#94a3b8',
}

export const NODE_LABELS: Record<MapNodeType, string> = {
  CMS_PAGE: 'CMS page',
  CATEGORY: 'Category',
  COLLECTION: 'Collection',
  PRODUCT: 'Product',
  BLOG_POST: 'Blog post',
  BLOG_CATEGORY: 'Blog category',
  BLOG_TAG: 'Blog tag',
  BLOG_AUTHOR: 'Blog author',
  OTHER: 'Other page',
}

// The root of the whole graph - every depth is measured from it, so it gets its own colour in
// EVERY colour mode plus a bigger radius and a halo ring in the canvas
export const HOMEPAGE_COLOR = '#fde047'

export const BROKEN_COLOR = '#ef4444'
export const ORPHAN_COLOR = '#f97316'
export const REDIRECT_COLOR = '#eab308'
export const OK_COLOR = '#22c55e'
export const DISCOVERED_COLOR = '#64748b'

const DEPTH_SCALE = ['#22c55e', '#84cc16', '#eab308', '#f97316', '#ef4444', '#b91c1c']

export function depthColor(depth: number): string {
  if (depth < 0) return '#64748b'
  return DEPTH_SCALE[Math.min(depth, DEPTH_SCALE.length - 1)]
}

export type NodeState = 'CRAWLED' | 'DISCOVERED' | 'UNSEEN'

// Linked-but-never-fetched is grey, not orange - it is unmeasured, and colouring it as a finding made a truncated crawl look like a broken site
export function healthColor(node: { state: NodeState; statusCode: number; inLinks: number; noindex: boolean }): string {
  if (node.state === 'DISCOVERED') return DISCOVERED_COLOR
  if (node.state === 'UNSEEN') return ORPHAN_COLOR
  if (node.statusCode >= 400 || node.statusCode === 0) return BROKEN_COLOR
  if (node.statusCode >= 300) return REDIRECT_COLOR
  if (node.inLinks === 0) return ORPHAN_COLOR
  if (node.noindex) return '#94a3b8'
  return OK_COLOR
}

// Memoised because the render loop asks for the same handful of colours thousands of times a frame, and each
// miss is a string build plus a CSS colour parse inside canvas
const alphaCache = new Map<string, string>()

export function withAlpha(hex: string, alpha: number): string {
  const key = `${hex}:${alpha}`
  const hit = alphaCache.get(key)
  if (hit) return hit
  const n = parseInt(hex.slice(1), 16)
  const value = `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
  alphaCache.set(key, value)
  return value
}
