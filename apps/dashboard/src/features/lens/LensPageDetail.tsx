import { useEffect, useMemo, useRef, useState, type ComponentType, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { createCache, createMirror, rebuildIntoSandboxedIframe } from 'rrweb-snapshot'
import {
  ArrowLeftIcon,
  MonitorIcon,
  MousePointerClickIcon,
  MoveIcon,
  MoveVerticalIcon,
  SmartphoneIcon,
  TabletIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import { useConfigScope } from '@/lib/config-scope'
import { LensHeatmapDocument, LensPagesDocument, LensPageStatsDocument, LensStatesDocument } from './Lens.signals.gql'

const CELL = 10 // must match the backend cellSize
type Kind = 'click' | 'move' | 'scroll'

const VIEWPORT_OPTS = [
  { value: 'desktop', label: 'Desktop', icon: MonitorIcon },
  { value: 'tablet', label: 'Tablet', icon: TabletIcon },
  { value: 'mobile', label: 'Mobile', icon: SmartphoneIcon },
] as const
const KIND_OPTS = [
  { value: 'click', label: 'Clicks', icon: MousePointerClickIcon },
  { value: 'move', label: 'Moves', icon: MoveIcon },
  { value: 'scroll', label: 'Scroll', icon: MoveVerticalIcon },
] as const

function nfmt(n: number): string {
  return n.toLocaleString()
}

// rrweb keeps <script> elements in the snapshot (their content replaced with
// the literal "SCRIPT_PLACEHOLDER") to stay structurally faithful - For a static
// heatmap backdrop they're useless and the placeholder text can render
// visibly, so drop script/noscript nodes before rebuild
type SnapNode = { type?: number; tagName?: string; childNodes?: SnapNode[] }
function stripScripts(n: SnapNode): void {
  if (!n.childNodes) return
  n.childNodes = n.childNodes.filter(
    (c) => !(c.type === 2 && (c.tagName === 'script' || c.tagName === 'noscript')),
  )
  for (const c of n.childNodes) stripScripts(c)
}

export function LensPageDetail() {
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const skip = !effectiveChannelId
  const [params, setParams] = useSearchParams()
  const pageKey = params.get('page') ?? ''
  const viewport = params.get('viewport') ?? 'desktop'
  const state = params.get('state') ?? 'default'
  const [kind, setKind] = useState<Kind>('click')

  const { data: pagesData } = useQuery(LensPagesDocument, {
    variables: { channelId },
    skip,
    fetchPolicy: 'cache-and-network',
  })
  const pages = pagesData?.lensPages ?? []

  const { data: statesData } = useQuery(LensStatesDocument, {
    variables: { channelId, pageKey, viewport },
    skip: skip || !pageKey,
    fetchPolicy: 'cache-and-network',
  })
  const states = statesData?.lensStates ?? []

  const { data: hmData } = useQuery(LensHeatmapDocument, {
    variables: { channelId, pageKey, viewport, state },
    skip: skip || !pageKey,
    fetchPolicy: 'cache-and-network',
  })
  const hm = hmData?.lensHeatmap

  // Rich per-page analytics (totals, timeline, referrers/clients/devices) - independent of the viewport/state toggles,
  // and available even when no snapshot exists yet (so a trafficked page still shows its numbers)
  const { data: statsData } = useQuery(LensPageStatsDocument, {
    variables: { channelId, pageKey },
    skip: skip || !pageKey,
    fetchPolicy: 'cache-and-network',
  })
  const stats = statsData?.lensPageStats

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(params)
    next.set(k, v)
    setParams(next, { replace: true })
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Heatmap"
        badges={pageKey ? <Badge variant="secondary" className="font-mono">{pageKey}</Badge> : undefined}
        meta={
          <Link to="/lens" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:underline">
            <ArrowLeftIcon className="size-3" /> Back to Lens
          </Link>
        }
      />
      <DetailPageBody>
        {!isChannel && (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Heatmaps are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        )}

        {isChannel && (
          <DetailPageSection>
            <div className="mb-4 space-y-3">
              {/* Page search on its own row so a long page key truncates within
                  its box and can never collide with the controls below */}
              <div className="grid max-w-xl gap-1.5">
                <span className="text-xs text-muted-foreground">Page</span>
                <AutocompletePicker
                  options={pages.map((p) => ({ value: p.pageKey, label: p.pageKey || '/', hint: `${nfmt(p.views)} views` }))}
                  value={pageKey}
                  onChange={(v) => setParam('page', String(v))}
                  placeholder="Search pages…"
                  emptyMessage="No pages with traffic."
                />
              </div>

              <div className="flex flex-wrap items-end gap-3">
                <div className="grid gap-1.5">
                  <span className="text-xs text-muted-foreground">Viewport</span>
                  <Segmented value={viewport} onChange={(v) => setParam('viewport', v)} options={VIEWPORT_OPTS} />
                </div>

                {states.length > 1 && (
                  <div className="grid gap-1.5">
                    <span className="text-xs text-muted-foreground">UI state</span>
                    <select
                      className="h-9 rounded-md border bg-background px-2 text-sm"
                      value={state}
                      onChange={(e) => setParam('state', e.target.value)}
                    >
                      {states.map((s) => (
                        <option key={s} value={s}>
                          {s === 'default' ? 'Default view' : s}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            </div>

            {!pageKey ? (
              <p className="py-12 text-center text-sm text-muted-foreground">
                Search for a page above to view its analytics &amp; heatmap.
              </p>
            ) : !stats ? (
              <p className="py-12 text-center text-sm text-muted-foreground">Loading…</p>
            ) : (
              <div className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <Tile label="Page views" value={nfmt(stats.views)} />
                  <Tile label="Unique visitors" value={nfmt(stats.visitors)} />
                  <Tile label="Clicks" value={nfmt(stats.clicks)} sub="all-time" />
                  <Tile label="Avg scroll" value={`${stats.avgScroll}%`} sub="all-time" />
                </div>

                <Panel title="Page views" meta="last 30 days">
                  <Sparkline points={stats.timeline} />
                </Panel>

                <div className="grid gap-4 lg:grid-cols-[1fr_300px]">
                  <Panel
                    title={`${kind} heatmap`}
                    meta={hm ? `${hm.width}×${hm.height}` : undefined}
                    trailing={hm ? <Segmented small value={kind} onChange={(v) => setKind(v as Kind)} options={KIND_OPTS} /> : undefined}
                  >
                    {hm ? (
                      <HeatmapStage
                        snapshotJson={hm.snapshotJson}
                        width={hm.width}
                        height={hm.height}
                        cells={kind === 'click' ? hm.click : kind === 'move' ? hm.move : hm.scroll}
                        kind={kind}
                      />
                    ) : (
                      <p className="py-12 text-center text-sm text-muted-foreground">
                        No snapshot captured for this page + viewport yet - browse it on the storefront (with
                        analytics consent) to capture one. The numbers here are already tracking.
                      </p>
                    )}
                  </Panel>

                  <div className="space-y-4">
                    {hm && (
                      <Panel title="Most-clicked elements">
                        {hm.elements.length === 0 ? (
                          <p className="text-sm text-muted-foreground">No clicks yet.</p>
                        ) : (
                          <ul className="space-y-1.5">
                            {hm.elements.slice(0, 12).map((el) => (
                              <li key={el.selector} className="flex items-center justify-between gap-2 text-sm">
                                <span className="min-w-0 truncate" title={el.selector}>
                                  {el.text || el.selector}
                                </span>
                                <span className="shrink-0 font-medium tabular-nums">{nfmt(el.count)}</span>
                              </li>
                            ))}
                          </ul>
                        )}
                      </Panel>
                    )}
                    {hm && (
                      <Panel title="Scroll reach">
                        <ScrollFunnel cells={hm.scroll} />
                      </Panel>
                    )}
                    <Panel title="Devices">
                      <BarList items={stats.devices} />
                    </Panel>
                    <Panel title="Referrers">
                      <BarList items={stats.referrers} />
                    </Panel>
                    <Panel title="Browsers">
                      <BarList items={stats.clients} />
                    </Panel>
                  </div>
                </div>
              </div>
            )}
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function Tile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

// Segmented control (icon + label), the friendly replacement for the raw
// viewport/kind toggle buttons
function Segmented<T extends string>({
  value,
  onChange,
  options,
  small,
}: {
  value: string
  onChange: (v: T) => void
  options: ReadonlyArray<{ value: T; label: string; icon?: ComponentType<{ className?: string }> }>
  small?: boolean
}) {
  return (
    <div className="inline-flex rounded-md border p-0.5">
      {options.map((o) => {
        const Icon = o.icon
        const active = value === o.value
        return (
          <button
            key={o.value}
            type="button"
            onClick={() => onChange(o.value)}
            className={`inline-flex items-center gap-1.5 rounded px-2.5 py-1 ${small ? 'text-xs' : 'text-sm'} ${active ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground'}`}
          >
            {Icon && <Icon className="size-3.5" />}
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

// A label→count list with proportional bars (devices / referrers / browsers)
function BarList({ items }: { items: ReadonlyArray<{ label: string; views: number }> }) {
  if (items.length === 0) return <p className="text-sm text-muted-foreground">No data yet.</p>
  const max = Math.max(1, ...items.map((i) => i.views))
  return (
    <div className="space-y-2">
      {items.map((i) => (
        <div key={i.label} className="flex items-center gap-2 text-sm">
          <span className="w-24 shrink-0 truncate text-muted-foreground" title={i.label}>
            {i.label}
          </span>
          <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
            <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.round((i.views / max) * 100)}%` }} />
          </span>
          <span className="w-10 shrink-0 text-right tabular-nums text-muted-foreground">{i.views.toLocaleString()}</span>
        </div>
      ))}
    </div>
  )
}

// A compact bar sparkline of daily page views (only days with views are present)
function Sparkline({ points }: { points: ReadonlyArray<{ date: string; views: number }> }) {
  if (points.length === 0) return <p className="text-sm text-muted-foreground">No views in the last 30 days.</p>
  const max = Math.max(1, ...points.map((p) => p.views))
  return (
    <div className="flex h-20 items-end gap-0.5">
      {points.map((p) => (
        <div
          key={p.date}
          className="flex-1 rounded-sm bg-primary/70"
          style={{ height: `${Math.max(4, Math.round((p.views / max) * 100))}%` }}
          title={`${p.date}: ${p.views} views`}
        />
      ))}
    </div>
  )
}

// A bordered panel with a header strip - the flat skelet idiom (no Card)
// Optional `meta` / `trailing` sit on the right of the header
function Panel({
  title,
  meta,
  trailing,
  children,
}: {
  title: string
  meta?: string
  trailing?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="overflow-hidden rounded border">
      <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</span>
        <div className="flex items-center gap-2">
          {meta && <span className="text-xs tabular-nums text-muted-foreground">{meta}</span>}
          {trailing}
        </div>
      </div>
      <div className="p-3">{children}</div>
    </div>
  )
}

// HeatmapStage rebuilds the captured DOM snapshot inside a sandboxed iframe and
// overlays a heat canvas in the same coordinate space, scaled to fit the panel
function HeatmapStage({
  snapshotJson,
  width,
  height,
  cells,
  kind,
}: {
  snapshotJson: string
  width: number
  height: number
  cells: ReadonlyArray<{ x: number; y: number; count: number }>
  kind: Kind
}) {
  const rootRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [stageW, setStageW] = useState(0)
  const [contentH, setContentH] = useState(height)
  const wrapRef = useRef<HTMLDivElement>(null)

  // Fit-to-width scale (never upscale past 1). effH is the stage height: the captured height, grown to the rebuilt
  // content's real height so nothing is clipped and the iframe needs no internal scroll
  const scale = stageW > 0 && width > 0 ? Math.min(1, stageW / width) : 1
  const effH = Math.max(height, contentH)

  useEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => setStageW(entries[0].contentRect.width))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Rebuild the snapshot into a fresh sandboxed iframe whenever it changes - rrweb's rebuild() refuses an
  // arbitrary document (XSS guard), so rebuildIntoSandboxedIframe() makes a script-less same-origin iframe

  // <script> nodes are stripped first (inert, bloaty), then the iframe's own scrollbar goes and the stage grows
  // to real content height, so page + heat overlay scroll as one block rather than under a fixed overlay
  useEffect(() => {
    const root = rootRef.current
    if (!root) return
    root.replaceChildren()
    setContentH(height)
    let node: SnapNode
    try {
      node = JSON.parse(snapshotJson) as SnapNode
    } catch {
      return
    }
    stripScripts(node)
    let iframe: HTMLIFrameElement | undefined
    try {
      ;({ iframe } = rebuildIntoSandboxedIframe(
        node as unknown as Parameters<typeof rebuildIntoSandboxedIframe>[0],
        {
          root,
          iframeAttributes: {
            style: 'position:absolute;inset:0;width:100%;height:100%;border:0;background:#fff',
            scrolling: 'no',
          },
          cache: createCache(),
          mirror: createMirror(),
        },
      ))
    } catch {
      return // fidelity best-effort - overlay still renders on a blank backdrop
    }
    const idoc = iframe?.contentDocument
    if (!idoc) return
    const measure = () => {
      try {
        if (idoc.documentElement) {
          idoc.documentElement.style.overflow = 'hidden'
          idoc.documentElement.scrollTop = 0
          idoc.documentElement.scrollLeft = 0
        }
        if (idoc.body) idoc.body.style.overflow = 'hidden'
        // Anchor the rebuilt doc at its origin (0,0) - rrweb may restore a scroll offset from the capture, which would
        // shift the page out from under the absolute-coordinate heat overlay
        iframe?.contentWindow?.scrollTo(0, 0)
        const h = idoc.documentElement?.scrollHeight ?? 0
        if (h > 0) setContentH((prev) => Math.max(prev, h))
      } catch {
        /* cross-document measurement is best-effort */
      }
    }
    measure()
    // Re-measure after async images/fonts settle so the stage covers them
    const t1 = setTimeout(measure, 500)
    const t2 = setTimeout(measure, 1500)
    return () => {
      clearTimeout(t1)
      clearTimeout(t2)
    }
  }, [snapshotJson, width, height])

  // Draw the heat canvas whenever cells / kind / size change
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (cells.length === 0) return
    const max = cells.reduce((m, c) => Math.max(m, c.count), 0) || 1

    if (kind === 'scroll') {
      // Scroll: shade horizontal bands by reach down the page
      for (const c of cells) {
        const a = Math.min(0.55, 0.12 + (c.count / max) * 0.5)
        ctx.fillStyle = `rgba(220,38,38,${a})`
        const bandTop = (c.y / 100) * effH
        ctx.fillRect(0, bandTop, width, effH / 100 + 1)
      }
      return
    }

    // Click / move: additive radial blobs
    ctx.globalCompositeOperation = 'lighter'
    const radius = CELL * 3
    for (const c of cells) {
      const cx = c.x * CELL + CELL / 2
      const cy = c.y * CELL + CELL / 2
      const intensity = Math.min(1, 0.15 + (c.count / max) * 0.85)
      const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius)
      g.addColorStop(0, `rgba(255,72,0,${0.6 * intensity})`)
      g.addColorStop(1, 'rgba(255,72,0,0)')
      ctx.fillStyle = g
      ctx.beginPath()
      ctx.arc(cx, cy, radius, 0, Math.PI * 2)
      ctx.fill()
    }
    ctx.globalCompositeOperation = 'source-over'
  }, [cells, kind, width, effH])

  return (
    <div ref={wrapRef} className="w-full overflow-hidden rounded-md border bg-white">
      <div
        style={{
          width,
          height: effH,
          transform: `scale(${scale})`,
          transformOrigin: 'top left',
          // Reserve the scaled footprint so siblings flow correctly
          marginBottom: effH * scale - effH,
        }}
        className="relative"
      >
        {/* rrweb appends the sandboxed snapshot iframe here */}
        <div ref={rootRef} className="absolute inset-0" />
        <canvas
          ref={canvasRef}
          width={width}
          height={effH}
          className="pointer-events-none absolute left-0 top-0"
        />
      </div>
    </div>
  )
}

function ScrollFunnel({ cells }: { cells: ReadonlyArray<{ y: number; count: number }> }) {
  const buckets = useMemo(() => {
    const marks = [25, 50, 75, 100]
    const total = cells.reduce((a, c) => a + c.count, 0)
    return marks.map((m) => {
      const reached = cells.filter((c) => c.y >= m).reduce((a, c) => a + c.count, 0)
      return { mark: m, pct: total > 0 ? Math.round((reached / total) * 100) : 0, reached }
    })
  }, [cells])

  if (cells.length === 0) return <p className="text-sm text-muted-foreground">No scroll data yet.</p>
  return (
    <ul className="space-y-2">
      {buckets.map((b) => (
        <li key={b.mark} className="text-sm">
          <div className="mb-0.5 flex justify-between text-xs text-muted-foreground">
            <span>{b.mark}% depth</span>
            <span>{b.pct}%</span>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
            <div className="h-full bg-primary" style={{ width: `${b.pct}%` }} />
          </div>
        </li>
      ))}
    </ul>
  )
}
