// Pure helpers for the page editor's SEO panel: an on-page lint and a body-image scan (shared by the social-card
// fallback) - No React, no network - the panel runs these against the editor's live (unsaved) title/meta/block state
import { isAssetRef } from '@/features/media/asset-ref'
import { type CmsBlock } from './page-blocks'

export type LintLevel = 'pass' | 'warn' | 'fail'
export type LintResult = { id: string; level: LintLevel; label: string; detail: string }

export type LintInput = {
  title: string
  metaTitle: string
  metaDescription: string
  slug: string
  seoNoindex: boolean
  blocks: CmsBlock[]
}

const MAX_DEPTH = 8
// Image prop keys, in fallback priority order - Matches the backend's body-image scan (graph/front/cms_helpers.go) so
// the editor preview and the storefront og:image agree on which image a page falls back to
const IMAGE_KEYS = ['imageUrl', 'image', 'src', 'backgroundImage', 'logo']

export function truncate(s: string, max: number): string {
  const t = s.trim()
  return t.length <= max ? t : `${t.slice(0, max - 1).trimEnd()}…`
}

// Pass/warn/fail checklist computed from the page's current meta and block layout
export function runSeoLint(input: LintInput): LintResult[] {
  const out: LintResult[] = []
  const effectiveTitle = (input.metaTitle || input.title).trim()
  const titleLen = effectiveTitle.length
  if (titleLen === 0) {
    out.push({ id: 'title', level: 'fail', label: 'Title tag', detail: 'No meta title or page title set.' })
  } else if (titleLen < 30) {
    out.push({ id: 'title', level: 'warn', label: 'Title length', detail: `${titleLen} chars - aim for 30–60.` })
  } else if (titleLen > 60) {
    out.push({ id: 'title', level: 'warn', label: 'Title length', detail: `${titleLen} chars - over ~60 may be truncated.` })
  } else {
    out.push({ id: 'title', level: 'pass', label: 'Title length', detail: `${titleLen} chars.` })
  }

  const descLen = input.metaDescription.trim().length
  if (descLen === 0) {
    out.push({ id: 'desc', level: 'fail', label: 'Meta description', detail: 'Empty - add a 120–160 char summary.' })
  } else if (descLen < 120) {
    out.push({ id: 'desc', level: 'warn', label: 'Meta description', detail: `${descLen} chars - aim for 120–160.` })
  } else if (descLen > 160) {
    out.push({ id: 'desc', level: 'warn', label: 'Meta description', detail: `${descLen} chars - over ~160 may be truncated.` })
  } else {
    out.push({ id: 'desc', level: 'pass', label: 'Meta description', detail: `${descLen} chars.` })
  }

  if (input.seoNoindex) {
    out.push({ id: 'noindex', level: 'warn', label: 'Noindex', detail: 'This page is excluded from search engines.' })
  }

  const heroes = countType(input.blocks, 'hero', 1)
  if (heroes > 1) {
    out.push({ id: 'hero', level: 'warn', label: 'Multiple heroes', detail: `${heroes} hero blocks - a page usually needs one.` })
  }

  const missingAlt = countMissingAlt(input.blocks, 1)
  if (missingAlt > 0) {
    out.push({ id: 'alt', level: 'warn', label: 'Image alt text', detail: `${missingAlt} image(s) missing alt text.` })
  } else {
    out.push({ id: 'alt', level: 'pass', label: 'Image alt text', detail: 'No images missing alt text.' })
  }

  if (input.slug.length > 60) {
    out.push({ id: 'slug', level: 'warn', label: 'Slug length', detail: `${input.slug.length} chars - shorter is friendlier.` })
  }
  return out
}

// firstBlockImage finds the page's first body image URL (the social-card fallback when no OG image is set), checking
// IMAGE_KEYS per block plus repeater item arrays
export function firstBlockImage(blocks: CmsBlock[], depth = 1): string {
  if (depth > MAX_DEPTH) return ''
  for (const b of blocks) {
    const url = imageInProps(b.props)
    if (url) return url
    if (b.blocks?.length) {
      const child = firstBlockImage(b.blocks, depth + 1)
      if (child) return child
    }
  }
  return ''
}

function imageValue(v: unknown): string {
  if (typeof v === 'string') return v.trim()
  if (isAssetRef(v)) return v.url
  return ''
}

function imageInProps(props: Record<string, unknown>): string {
  for (const k of IMAGE_KEYS) {
    const url = imageValue(props[k])
    if (url) return url
  }
  for (const v of Object.values(props)) {
    if (Array.isArray(v)) {
      for (const it of v) {
        if (it && typeof it === 'object') {
          const url = imageInProps(it as Record<string, unknown>)
          if (url) return url
        }
      }
    }
  }
  return ''
}

function countType(blocks: CmsBlock[], type: string, depth: number): number {
  if (depth > MAX_DEPTH) return 0
  let n = 0
  for (const b of blocks) {
    if (b.type === type) n++
    if (b.blocks?.length) n += countType(b.blocks, type, depth + 1)
  }
  return n
}

// countMissingAlt counts image-bearing scopes that HAVE an `alt` field but leave it blank (e.g. gallery items) - Scopes
// with no alt field (decorative hero backgrounds) aren't counted - the editor can't fix what it can't set
function countMissingAlt(blocks: CmsBlock[], depth: number): number {
  if (depth > MAX_DEPTH) return 0
  let n = 0
  for (const b of blocks) {
    n += altIssuesInProps(b.props)
    if (b.blocks?.length) n += countMissingAlt(b.blocks, depth + 1)
  }
  return n
}

function altIssuesInProps(props: Record<string, unknown>): number {
  let n = 0
  const hasImage = IMAGE_KEYS.concat('url').some((k) => imageValue(props[k]) !== '')
  if (hasImage && 'alt' in props && !String(props.alt ?? '').trim()) n++
  for (const v of Object.values(props)) {
    if (Array.isArray(v)) {
      for (const it of v) {
        if (it && typeof it === 'object') n += altIssuesInProps(it as Record<string, unknown>)
      }
    }
  }
  return n
}
