// Inline live preview for a CMS page - docked beside the editor, or folded to a rail that opens over it when
// the screen is too narrow for both - streaming the in-flight block document to the storefront iframe over postMessage so edits render before Save
//
// Mirror of features/blog/BlogPreviewPanel, differing only in payload: the CMS page body IS a
// block document, so this streams { blocks } rather than post form state
// Token re-mints at ~80% of TTL; contract lives in CmsPreviewClient.tsx, discriminator
// "skelet:cms-preview:update"

import { type PointerEvent as ReactPointerEvent, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import {
  ExternalLinkIcon,
  EyeIcon,
  MonitorIcon,
  PanelRightCloseIcon,
  RefreshCwIcon,
  SmartphoneIcon,
  XIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'

import { Button } from '@/components/ui/button'
import type { CmsBlock } from './page-blocks'
import { STOREFRONT_ORIGIN } from '@/lib/app-origins'
import { MintCmsPagePreviewTokenDocument } from './Cms.gql'

// Same rationale as BlogPreviewPanel - rendered at this CSS-pixel width then scaled, so the
// storefront's media queries fire at the real breakpoint
type Viewport = 'mobile' | 'desktop'
const VIEWPORT_WIDTH: Record<Viewport, number> = {
  mobile: 375,
  desktop: 1024,
}

// The panel width is drag-resizable and persisted - the scale math reads containerSize via
// ResizeObserver, so a wider pane just shows the desktop viewport at a larger scale
const PANEL_WIDTH_KEY = 'skelet:cms-preview:panel-width'
const DEFAULT_PANEL_WIDTH = 640
const MIN_PANEL_WIDTH = 360
// The editor keeps this much: the preview shrinks into what's left, and once even its minimum doesn't fit it
// folds to a rail and opens over the editor on demand
const MIN_EDITOR_WIDTH = 560
// Opened over the editor it leaves a strip of the editor showing, and never narrows past the minimum
const OVERLAY_GUTTER = 48
const OVERLAY_MIN_WIDTH = 320
const COLLAPSED_KEY = 'skelet:cms-preview:collapsed'

function readCollapsed(): boolean {
  try {
    return window.localStorage.getItem(COLLAPSED_KEY) === '1'
  } catch {
    return false
  }
}

const MAX_PANEL_WIDTH = 1280

// Only the preference's own bounds - what the editor needs is taken off the measured row at render
function clampPanelWidth(w: number): number {
  return Math.round(Math.max(MIN_PANEL_WIDTH, Math.min(w, MAX_PANEL_WIDTH)))
}

function readSavedPanelWidth(): number {
  try {
    const raw = Number(window.localStorage.getItem(PANEL_WIDTH_KEY))
    if (Number.isFinite(raw) && raw > 0) return clampPanelWidth(raw)
  } catch {
    // private mode / disabled storage - fall back to the default
  }
  return DEFAULT_PANEL_WIDTH
}

const MESSAGE_TYPE = 'skelet:cms-preview:update'
const SELECT_TYPE = 'skelet:cms-preview:select'

type Props = {
  // The CMS editor renders the preview always-on, so it passes no onClose
  onClose?: () => void
  pageId: string
  locale?: string
  // Lifted by the parent so the canvas and this panel read the same source
  blocks: CmsBlock[]
  // Streamed to the iframe so a clean doc keeps the real server render (data-backed grids
  // included) and only switches to the live client tree once you actually edit
  dirty?: boolean
  // The parent uses this to open and scroll to the clicked block's editor card
  onSelectBlock?: (blockId: string) => void
}


export function CmsPreviewPanel({ onClose, pageId, locale, blocks, dirty, onSelectBlock }: Props) {
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<string | null>(null)
  const [iframeReady, setIframeReady] = useState(false)
  const [mintError, setMintError] = useState<string | null>(null)
  const [viewport, setViewport] = useState<Viewport>('desktop')
  const [panelWidth, setPanelWidth] = useState<number>(() => readSavedPanelWidth())
  const dragStart = useRef<{ x: number; w: number } | null>(null)
  useEffect(() => {
    try {
      window.localStorage.setItem(PANEL_WIDTH_KEY, String(panelWidth))
    } catch {
      // storage unavailable - the width just won't persist across reloads
    }
  }, [panelWidth])

  // Sized off the row it shares with the editor, not the window - the app sidebar takes its share too
  const asideRef = useRef<HTMLElement | null>(null)
  const [rowWidth, setRowWidth] = useState(0)
  useLayoutEffect(() => {
    const row = asideRef.current?.parentElement
    if (!row) return
    setRowWidth(row.getBoundingClientRect().width)
    const obs = new ResizeObserver((entries) => setRowWidth(entries[0]?.contentRect.width ?? 0))
    obs.observe(row)
    return () => obs.disconnect()
  }, [])
  const room = rowWidth > 0 ? rowWidth - MIN_EDITOR_WIDTH : Number.POSITIVE_INFINITY
  const fits = room >= MIN_PANEL_WIDTH
  const dockedWidth = Math.min(panelWidth, room)
  const overlayWidth = Math.min(rowWidth || panelWidth, Math.max(OVERLAY_MIN_WIDTH, Math.min(panelWidth, rowWidth - OVERLAY_GUTTER)))
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const [overlayOpen, setOverlayOpen] = useState(false)
  const mode: 'docked' | 'rail' | 'overlay' = collapsed ? 'rail' : !fits ? (overlayOpen ? 'overlay' : 'rail') : 'docked'
  function rememberCollapsed(next: boolean) {
    setCollapsed(next)
    try {
      window.localStorage.setItem(COLLAPSED_KEY, next ? '1' : '0')
    } catch {
      // the choice just won't survive a reload
    }
  }
  function showPreview() {
    rememberCollapsed(false)
    if (!fits) setOverlayOpen(true)
  }
  function hidePreview() {
    if (mode === 'overlay') setOverlayOpen(false)
    else rememberCollapsed(true)
  }
  useEffect(() => {
    if (mode !== 'overlay') return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOverlayOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [mode])
  // Pointer capture keeps the drag alive over the iframe, which would otherwise swallow the events
  function onResizeDown(e: ReactPointerEvent<HTMLDivElement>) {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    dragStart.current = { x: e.clientX, w: dockedWidth }
  }
  function onResizeMove(e: ReactPointerEvent<HTMLDivElement>) {
    if (!dragStart.current) return
    // The handle is on the LEFT edge, so dragging left (dx < 0) widens the panel
    // Capped at the room too, or dragging past it banks width the pane can't show and dragging back feels dead
    setPanelWidth(clampPanelWidth(Math.min(room, dragStart.current.w - (e.clientX - dragStart.current.x))))
  }
  function onResizeUp(e: ReactPointerEvent<HTMLDivElement>) {
    if (!dragStart.current) return
    dragStart.current = null
    e.currentTarget.releasePointerCapture(e.pointerId)
  }
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [containerSize, setContainerSize] = useState<{ w: number; h: number }>({
    w: 0,
    h: 0,
  })
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
  const [mintMutation, { loading: minting }] = useMutation(MintCmsPagePreviewTokenDocument)

  const mint = useMemo(
    () => async () => {
      setMintError(null)
      try {
        const res = await mintMutation({
          variables: { input: { pageId, locale: locale ?? null } },
        })
        const out = res.data?.mintCMSPagePreviewToken
        if (!out) {
          setMintError('Preview token not returned')
          return
        }
        setToken(out.token)
        setExpiresAt(out.expiresAt)
      } catch (err) {
        setMintError(err instanceof Error ? err.message : 'Failed to mint token')
      }
    },
    [mintMutation, pageId, locale],
  )

  // mint() only sets state once the request resolves, while the synchronous setIframeReady(false) resets the handshake for the
  // about-to-load iframe - both are the intended "drive an external system" use of an effect, not a render-derived cascade
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void mint()
    setIframeReady(false)
  }, [locale, mint])

  // Re-mint at ~80% of TTL so the iframe never hits the "session expired" empty state
  useEffect(() => {
    if (!expiresAt) return
    const expiresAtMs = Date.parse(expiresAt)
    const refreshAt = expiresAtMs - (expiresAtMs - Date.now()) * 0.2
    const delay = Math.max(refreshAt - Date.now(), 30_000)
    const t = window.setTimeout(() => {
      mint()
    }, delay)
    return () => window.clearTimeout(t)
  }, [expiresAt, mint])

  // No origin check, same rationale as BlogPreviewPanel
  useEffect(() => {
    function onMessage(ev: MessageEvent) {
      if (!ev.data || typeof ev.data !== 'object') return
      if (ev.data.type !== 'skelet:cms-preview:ready') return
      if (ev.data.pageId !== pageId) return
      setIframeReady(true)
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [pageId])

  // A click inside the preview posts the block id back, forwarded so the editor opens that card
  useEffect(() => {
    function onSelect(ev: MessageEvent) {
      if (!ev.data || typeof ev.data !== 'object') return
      if (ev.data.type !== SELECT_TYPE) return
      if (ev.data.pageId !== pageId) return
      if (typeof ev.data.blockId !== 'string') return
      // Over the editor, the preview would hide the card it just opened
      setOverlayOpen(false)
      onSelectBlock?.(ev.data.blockId)
    }
    window.addEventListener('message', onSelect)
    return () => window.removeEventListener('message', onSelect)
  }, [pageId, onSelectBlock])

  // Debounced ~400ms so a burst of keystrokes coalesces into one postMessage - targetOrigin
  // is '*' because this posts to a specific iframe contentWindow handle, not a broadcast
  useEffect(() => {
    if (!iframeReady || !iframeRef.current?.contentWindow) return
    const win = iframeRef.current.contentWindow
    const t = window.setTimeout(() => {
      win.postMessage({ type: MESSAGE_TYPE, pageId, blocks, dirty: !!dirty }, '*')
      if (import.meta.env.DEV) {
        console.debug('[cms-preview] push', { pageId, blockCount: blocks.length, dirty: !!dirty })
      }
    }, 400)
    return () => window.clearTimeout(t)
  }, [iframeReady, pageId, blocks, dirty])

  const src = token
    ? `${STOREFRONT_ORIGIN}/cms/preview/${pageId}?token=${encodeURIComponent(token)}`
    : null

  const targetWidth = VIEWPORT_WIDTH[viewport]
  const availableWidth =
    viewport === 'mobile'
      ? Math.max(containerSize.w - 24, 0)
      : containerSize.w
  const scale =
    containerSize.w === 0 ? 1 : Math.min(1, availableWidth / targetWidth)
  const iframeHeight = scale > 0 ? containerSize.h / scale : containerSize.h

  return (
    <aside
      ref={asideRef}
      className={cn('relative flex h-full shrink-0 flex-col border-l bg-background', mode !== 'docked' && 'w-11 items-center')}
      style={mode === 'docked' ? { width: dockedWidth } : undefined}
    >
      {mode !== 'docked' && (
        <button
          type="button"
          onClick={showPreview}
          aria-expanded={mode === 'overlay'}
          className="mt-3 flex flex-col items-center gap-2 rounded-md px-1.5 py-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <EyeIcon className="size-4" />
          <span className="rotate-180 [writing-mode:vertical-rl]">Live preview</span>
        </button>
      )}
      {/* Kept mounted in every mode - hiding it with display:none keeps the iframe's page loaded */}
      <div
        hidden={mode === 'rail'}
        className={cn(
          'flex min-h-0 flex-1 flex-col bg-background',
          mode === 'docked' && 'relative',
          mode === 'rail' && 'hidden',
          mode === 'overlay' && 'absolute inset-y-0 right-0 z-30 border-l shadow-2xl',
        )}
        style={mode === 'overlay' ? { width: overlayWidth } : undefined}
      >
      {/* Drag the left edge to resize the preview; double-click to reset to the default width */}
      {mode === 'docked' && (
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize preview panel"
        onPointerDown={onResizeDown}
        onPointerMove={onResizeMove}
        onPointerUp={onResizeUp}
        onDoubleClick={() => setPanelWidth(DEFAULT_PANEL_WIDTH)}
        title="Drag to resize · double-click to reset"
        className="absolute left-0 top-0 z-20 h-full w-2 -translate-x-1/2 cursor-col-resize touch-none select-none bg-transparent transition-colors hover:bg-primary/40 active:bg-primary/60"
      />
      )}
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-semibold">Live preview</div>
          <div className="text-xs text-muted-foreground">
            Updates as you edit.
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
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
            onClick={() => mint()}
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
          {onClose ? (
            <Button
              variant="outline"
              size="sm"
              onClick={onClose}
              title="Close preview panel"
            >
              <XIcon className="size-4" />
            </Button>
          ) : (
            <Button variant="outline" size="sm" onClick={hidePreview} title="Hide preview" aria-label="Hide preview">
              <PanelRightCloseIcon className="size-4" />
            </Button>
          )}
        </div>
      </div>

      {mintError && (
        <div className="border-b bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {mintError}
        </div>
      )}

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
              width: targetWidth,
              height: iframeHeight,
              transform: `scale(${scale})`,
              left: `${(containerSize.w - targetWidth * scale) / 2}px`,
            }}
          >
            <iframe
              ref={iframeRef}
              src={src}
              title="CMS page preview"
              className={`h-full w-full ${
                viewport === 'mobile'
                  ? 'rounded-md border bg-background shadow-sm'
                  : 'border-0 bg-background'
              }`}
              onLoad={() => {
                // Fallback for a dropped :ready handshake (HMR, slow first paint) - the
                // load event always fires, so the push effect runs with the latest blocks
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
      </div>
    </aside>
  )
}
