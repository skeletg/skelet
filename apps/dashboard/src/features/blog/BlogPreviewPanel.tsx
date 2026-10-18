// Inline live preview for a blog post - a sibling of the editor, not an overlay, streaming
// in-flight form state to the channel iframe over postMessage so edits render before Save
// Token re-mints at ~80% of TTL; read expiresAt off the response rather than hardcoding
// PostMessage contract lives in PreviewClient.tsx, discriminator "skelet:blog-preview:update"

import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import {
  ExternalLinkIcon,
  MonitorIcon,
  RefreshCwIcon,
  SmartphoneIcon,
  XIcon,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { STOREFRONT_ORIGIN } from '@/lib/app-origins'
import { MintBlogPostPreviewTokenDocument } from './Blog.gql'

// The iframe renders at this CSS-pixel width then scales to fit - 1024 is Tailwind's `lg:`
// inflection, wide enough that desktop-only styles fire but still legible panel-side
type Viewport = 'mobile' | 'desktop'
const VIEWPORT_WIDTH: Record<Viewport, number> = {
  mobile: 375,
  desktop: 1024,
}

const MESSAGE_TYPE = 'skelet:blog-preview:update'

// Keep in sync with the channel's PreviewPost - the channel merges patches, so an omitted key keeps its previous value
export type BlogPreviewState = {
  id: string
  slug: string
  publishedAt: string | null
  readingTimeMinutes: number | null
  seoNoindex: boolean
  author: { id: string; slug: string; displayName: string } | null
  content: {
    locale: string
    title: string
    excerpt: unknown
    body: unknown
    metaTitle: string | null
    metaDescription: string | null
  } | null
  categories: {
    id: string
    slug: string
    urlPath?: string
    content: { name: string } | null
  }[]
  tags: {
    id: string
    slug: string
    content: { name: string } | null
  }[]
}

type Props = {
  onClose: () => void
  postId: string
  locale?: string
  state: BlogPreviewState
}


export function BlogPreviewPanel({ onClose, postId, locale, state }: Props) {
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<string | null>(null)
  const [iframeReady, setIframeReady] = useState(false)
  const [mintError, setMintError] = useState<string | null>(null)
  const [viewport, setViewport] = useState<Viewport>('desktop')
  // ResizeObserver tracks height too, so the scaled iframe still fills the panel
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [containerSize, setContainerSize] = useState<{ w: number; h: number }>(
    { w: 0, h: 0 },
  )
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const obs = new ResizeObserver((entries) => {
      const r = entries[0]?.contentRect
      if (!r) return
      setContainerSize({ w: r.width, h: r.height })
    })
    obs.observe(el)
    return () => obs.disconnect()
  }, [])
  const [mintMutation, { loading: minting }] = useMutation(MintBlogPostPreviewTokenDocument)

  // Never sets state synchronously, only in the promise callbacks, so it's safe to call
  // from the mount effect - retrying callers clear mintError themselves first
  const mint = useMemo(
    () => () =>
      mintMutation({
        variables: { input: { postId, locale: locale ?? null } },
      })
        .then((res) => {
          const out = res.data?.mintBlogPostPreviewToken
          if (!out) {
            setMintError('Preview token not returned')
            return
          }
          setMintError(null)
          setToken(out.token)
          setExpiresAt(out.expiresAt)
        })
        .catch((err: unknown) => {
          setMintError(err instanceof Error ? err.message : 'Failed to mint token')
        }),
    [mintMutation, postId, locale],
  )

  // A prop-driven re-mint reloads the iframe, so drop ready and wait for the fresh document's signal
  const [prevMint, setPrevMint] = useState({ mint })
  if (prevMint.mint !== mint) {
    setPrevMint({ mint })
    setIframeReady(false)
    setMintError(null)
  }

  useEffect(() => {
    mint()
  }, [locale, mint])

  // Re-mint at ~80% of TTL so the iframe never hits the "session expired" empty state
  useEffect(() => {
    if (!expiresAt) return
    const expiresAtMs = Date.parse(expiresAt)
    const refreshAt = expiresAtMs - (expiresAtMs - Date.now()) * 0.2
    const delay = Math.max(refreshAt - Date.now(), 30_000)
    const t = window.setTimeout(() => {
      setMintError(null)
      mint()
    }, delay)
    return () => window.clearTimeout(t)
  }, [expiresAt, mint])

  // No origin check - the discriminator plus controlling which iframe is mounted is the
  // real boundary, and origin equality broke dev when the two run on different ports
  useEffect(() => {
    function onMessage(ev: MessageEvent) {
      if (!ev.data || typeof ev.data !== 'object') return
      if (ev.data.type !== 'skelet:blog-preview:ready') return
      if (ev.data.postId !== postId) return
      setIframeReady(true)
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [postId])

  // targetOrigin is '*' because this posts to a specific iframe contentWindow handle rather than broadcasting, so only that document can
  // receive it - STOREFRONT_ORIGIN was the bug: when the env-derived origin didn't match the iframe URL the browser dropped the message
  useEffect(() => {
    if (!iframeReady || !iframeRef.current?.contentWindow) return
    iframeRef.current.contentWindow.postMessage(
      { type: MESSAGE_TYPE, postId, post: state },
      '*',
    )
    if (import.meta.env.DEV) {
      console.debug('[blog-preview] push', { postId, contentTitle: state.content?.title })
    }
  }, [iframeReady, postId, state])

  const src =
    token
      ? `${STOREFRONT_ORIGIN}/blog/preview/${postId}?token=${encodeURIComponent(token)}`
      : null

  // Capped at 1 so a wide panel never enlarges the preview - the +24 padding is the
  // bg-muted gutter around mobile, skipped for edge-to-edge desktop
  const targetWidth = VIEWPORT_WIDTH[viewport]
  const availableWidth =
    viewport === 'mobile'
      ? Math.max(containerSize.w - 24, 0)
      : containerSize.w
  const scale =
    containerSize.w === 0 ? 1 : Math.min(1, availableWidth / targetWidth)
  // Sized in unscaled CSS pixels so it fills the container vertically AFTER the transform
  const iframeHeight = scale > 0 ? containerSize.h / scale : containerSize.h

  return (
    <aside className="flex h-full w-[640px] shrink-0 flex-col border-l bg-background">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-semibold">Live preview</div>
          <div className="text-xs text-muted-foreground">
            Edits stream in as you type. Token lasts ~15 min.
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {/* No iframe reload - the constraint is pure container CSS, so live
              updates and form state survive a viewport switch */}
          <div className="mr-1 flex items-center gap-0.5 rounded-md border p-0.5">
            <Button
              variant={viewport === 'desktop' ? 'default' : 'ghost'}
              size="sm"
              className="h-7 px-2"
              onClick={() => setViewport('desktop')}
              title="Desktop preview"
            >
              <MonitorIcon className="size-4" />
            </Button>
            <Button
              variant={viewport === 'mobile' ? 'default' : 'ghost'}
              size="sm"
              className="h-7 px-2"
              onClick={() => setViewport('mobile')}
              title="Mobile preview (375px)"
            >
              <SmartphoneIcon className="size-4" />
            </Button>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setMintError(null)
              mint()
            }}
            disabled={minting}
            title="Refresh preview token"
          >
            <RefreshCwIcon className="size-4" />
          </Button>
          {src && (
            <Button asChild variant="outline" size="sm" title="Open in new tab">
              <a href={src} target="_blank" rel="noopener noreferrer">
                <ExternalLinkIcon className="size-4" />
              </a>
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={onClose}
            title="Close preview panel"
          >
            <XIcon className="size-4" />
          </Button>
        </div>
      </div>

      {mintError && (
        <div className="border-b bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {mintError}
        </div>
      )}

      {/* Rendered at a fixed CSS-pixel width then transform-scaled, so the iframe
          sees its real width natively and its media queries still fire */}
      <div
        ref={containerRef}
        className={`relative flex-1 overflow-hidden ${
          viewport === 'mobile' ? 'bg-muted/30 px-3 py-3' : 'bg-muted/20'
        }`}
      >
        {src ? (
          <div
            className="absolute top-0 origin-top-left"
            style={{
              // Layout width is the viewport target so media queries inside the iframe see real CSS pixels
              width: targetWidth,
              height: iframeHeight,
              transform: `scale(${scale})`,
              // Panel width minus the SCALED iframe width, halved
              left: `${(containerSize.w - targetWidth * scale) / 2}px`,
            }}
          >
            <iframe
              ref={iframeRef}
              src={src}
              title="Blog post preview"
              className={`h-full w-full ${
                viewport === 'mobile'
                  ? 'rounded-md border bg-background shadow-sm'
                  : 'border-0 bg-background'
              }`}
              onLoad={() => {
                // Fallback when the channel's :ready handshake is dropped (HMR, slow
                // first paint) - the load event always fires, so the push effect runs
                setIframeReady(true)
              }}
            />
          </div>
        ) : (
          <div className="flex h-full w-full items-center justify-center text-sm text-muted-foreground">
            Minting preview token…
          </div>
        )}
      </div>
    </aside>
  )
}
