import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Simulation, type ForceEdge, type ForceNode } from './link-force'
import {
  BROKEN_COLOR,
  HOMEPAGE_COLOR,
  NODE_COLORS,
  ORPHAN_COLOR,
  depthColor,
  healthColor,
  withAlpha,
  type MapNodeType,
  type NodeState,
} from './link-map-theme'

export type MapNode = {
  key: string
  entityId: string | null
  type: MapNodeType
  label: string
  path: string
  kind: 'PAGE' | 'FILTER' | 'PARAM'
  state: NodeState
  statusCode: number
  noindex: boolean
  isHomepage: boolean
  inLinks: number
  outLinks: number
  depth: number
}

export type MapEdge = {
  from: string
  to: string
  nofollow: boolean
  boilerplate: boolean
  weight?: number
}

export type ColorMode = 'type' | 'depth' | 'health'

type Props = {
  nodes: MapNode[]
  edges: MapEdge[]
  colorMode: ColorMode
  search: string
  selectedKey: string | null
  onSelect: (key: string | null) => void
  maxDepth: number
  height?: string
}

const UNREACHABLE_RING_GAP = 1
const MIN_SCREEN_RADIUS = 2.5
const MAX_SCREEN_RADIUS = 30
const MAX_LABELS = 60

function nodeRadius(n: MapNode): number {
  if (n.isHomepage) return 22
  if (n.state !== 'CRAWLED') return 3.5
  return 4.5 + Math.min(9, Math.sqrt(n.inLinks + n.outLinks) * 2.1)
}

export function LinkMapCanvas({
  nodes,
  edges,
  colorMode,
  search,
  selectedKey,
  onSelect,
  maxDepth,
  height,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef({ x: 0, y: 0, scale: 1, ready: false })
  const dragRef = useRef({ active: false, moved: false, lastX: 0, lastY: 0 })
  const hoverRef = useRef<number | null>(null)
  const dirtyRef = useRef(true)
  const nodesRef = useRef(nodes)
  const colorModeRef = useRef(colorMode)
  const matchesRef = useRef<Set<number> | null>(null)
  const selectedRef = useRef<string | null>(selectedKey)
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)

  const model = useMemo(() => {
    const index = new Map<string, number>()
    nodes.forEach((n, i) => index.set(n.key, i))
    const unreachableRing = maxDepth + UNREACHABLE_RING_GAP
    const simNodes: ForceNode[] = nodes.map((n) => ({
      key: n.key,
      ring: n.depth >= 0 ? n.depth : unreachableRing,
      x: 0,
      y: 0,
      vx: 0,
      vy: 0,
      radius: nodeRadius(n),
      fixed: n.isHomepage,
      seedAngle: 0,
    }))
    const simEdges: ForceEdge[] = []
    const neighbours: Set<number>[] = nodes.map(() => new Set<number>())
    for (const e of edges) {
      const a = index.get(e.from)
      const b = index.get(e.to)
      if (a === undefined || b === undefined) continue
      simEdges.push({ source: a, target: b, length: 130 })
      neighbours[a].add(b)
      neighbours[b].add(a)
    }
    const perRing = new Map<number, number>()
    for (const n of simNodes) perRing.set(n.ring, (perRing.get(n.ring) ?? 0) + 1)
    const ringRadii: number[] = [0]
    for (let ring = 1; ring <= unreachableRing; ring++) {
      const spacing = nodes.length > 200 ? 14 : 46
      const needed = ((perRing.get(ring) ?? 0) * spacing) / (Math.PI * 2)
      ringRadii[ring] = ringRadii[ring - 1] + Math.max(220, needed)
    }
    return { index, simNodes, simEdges, neighbours, unreachableRing, ringRadii }
  }, [nodes, edges, maxDepth])

  const matches = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return null
    const hit = new Set<number>()
    nodes.forEach((n, i) => {
      if (n.label.toLowerCase().includes(q) || n.path.toLowerCase().includes(q)) hit.add(i)
    })
    return hit
  }, [nodes, search])

  useEffect(() => {
    nodesRef.current = nodes
    colorModeRef.current = colorMode
    matchesRef.current = matches
    selectedRef.current = selectedKey
    dirtyRef.current = true
  }, [nodes, colorMode, matches, selectedKey])

  const fitToView = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    // Frame the linked site - the unlinked band is often ten times larger and would shrink the real map to nothing
    for (const n of model.simNodes) {
      if (n.ring >= model.unreachableRing) continue
      minX = Math.min(minX, n.x)
      minY = Math.min(minY, n.y)
      maxX = Math.max(maxX, n.x)
      maxY = Math.max(maxY, n.y)
    }
    if (!Number.isFinite(minX)) {
      for (const n of model.simNodes) {
        minX = Math.min(minX, n.x)
        minY = Math.min(minY, n.y)
        maxX = Math.max(maxX, n.x)
        maxY = Math.max(maxY, n.y)
      }
    }
    if (!Number.isFinite(minX)) return
    const spanX = Math.max(1, maxX - minX)
    const spanY = Math.max(1, maxY - minY)
    const scale = Math.min(w / (spanX + 200), h / (spanY + 200), 1.6)
    viewRef.current.scale = scale
    viewRef.current.x = w / 2 - ((minX + maxX) / 2) * scale
    viewRef.current.y = h / 2 - ((minY + maxY) / 2) * scale
    dirtyRef.current = true
  }, [model])

  useEffect(() => {
    const canvas = canvasRef.current
    const wrap = wrapRef.current
    if (!canvas || !wrap) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const sim = new Simulation(model.simNodes, model.simEdges, { ringRadii: model.ringRadii })
    viewRef.current.ready = false
    let raf = 0
    const start = performance.now()

    const resize = () => {
      dirtyRef.current = true
      const dpr = window.devicePixelRatio || 1
      canvas.width = Math.max(1, Math.floor(wrap.clientWidth * dpr))
      canvas.height = Math.max(1, Math.floor(wrap.clientHeight * dpr))
      canvas.style.width = `${wrap.clientWidth}px`
      canvas.style.height = `${wrap.clientHeight}px`
    }
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(wrap)

    // Registered non-passively: React's onWheel cannot preventDefault, so the page scrolled underneath the zoom
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      dirtyRef.current = true
      const rect = canvas.getBoundingClientRect()
      const view = viewRef.current
      const next = Math.min(6, Math.max(0.05, view.scale * Math.exp(-e.deltaY * 0.0016)))
      const px = e.clientX - rect.left
      const py = e.clientY - rect.top
      view.x = px - ((px - view.x) / view.scale) * next
      view.y = py - ((py - view.y) / view.scale) * next
      view.scale = next
    }
    canvas.addEventListener('wheel', onWheel, { passive: false })

    const draw = (now: number) => {
      raf = requestAnimationFrame(draw)
      const elapsed = now - start
      const nodes = nodesRef.current
      const colorMode = colorModeRef.current
      const matches = matchesRef.current
      const selectedKey = selectedRef.current
      const settled = sim.alpha < 0.005
      if (settled && !dirtyRef.current && hoverRef.current === null && !selectedKey) return
      if (settled) dirtyRef.current = false
      for (let i = 0; i < 3; i++) sim.tick()
      if (!viewRef.current.ready && (sim.alpha < 0.35 || elapsed > 2600)) {
        fitToView()
        viewRef.current.ready = true
      }

      const dpr = window.devicePixelRatio || 1
      const dark = document.documentElement.classList.contains('dark')
      const w = canvas.width / dpr
      const h = canvas.height / dpr
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, w, h)

      const view = viewRef.current
      const scale = view.scale
      // Everything is sized in screen pixels and converted back - magnifying blobs costs fill rate and reads worse
      const px = (screenPixels: number) => screenPixels / scale

      ctx.save()
      ctx.translate(view.x, view.y)
      ctx.scale(scale, scale)

      const margin = px(160)
      const viewLeft = -view.x / scale - margin
      const viewTop = -view.y / scale - margin
      const viewRight = viewLeft + w / scale + margin * 2
      const viewBottom = viewTop + h / scale + margin * 2
      const onScreen = (x: number, y: number) => x >= viewLeft && x <= viewRight && y >= viewTop && y <= viewBottom

      const ringInk = dark ? 'rgba(148,163,184,0.16)' : 'rgba(100,116,139,0.16)'
      const ringText = dark ? 'rgba(148,163,184,0.7)' : 'rgba(71,85,105,0.7)'
      ctx.lineWidth = px(1)
      for (let ring = 1; ring <= model.unreachableRing; ring++) {
        const r = model.ringRadii[ring]
        ctx.beginPath()
        ctx.strokeStyle = ring === model.unreachableRing ? withAlpha(ORPHAN_COLOR, 0.22) : ringInk
        ctx.setLineDash(ring === model.unreachableRing ? [px(6), px(6)] : [])
        ctx.arc(0, 0, r, 0, Math.PI * 2)
        ctx.stroke()
      }
      ctx.setLineDash([])
      ctx.font = `${px(12)}px ui-sans-serif, system-ui`
      ctx.fillStyle = ringText
      ctx.textAlign = 'center'
      for (let ring = 1; ring <= model.unreachableRing; ring++) {
        const label = ring === model.unreachableRing ? 'nothing links here' : `${ring} click${ring === 1 ? '' : 's'}`
        ctx.fillText(label, 0, -model.ringRadii[ring] - px(8))
      }

      const hovered = hoverRef.current
      const selectedIdx = selectedKey ? (model.index.get(selectedKey) ?? null) : null
      const focus = hovered ?? selectedIdx
      const near = focus === null ? null : model.neighbours[focus]
      const appear = Math.min(1, elapsed / 900)

      // Only the focused node's links are drawn - every edge at once is a hairball that hides the structure it
      // is meant to show, and hover is how you read one page's connections anyway
      if (focus !== null) {
        ctx.lineWidth = px(1.6)
        ctx.strokeStyle = withAlpha('#0ea5e9', 0.75)
        ctx.beginPath()
        for (const e of model.simEdges) {
          if (e.source !== focus && e.target !== focus) continue
          const a = model.simNodes[e.source]
          const b = model.simNodes[e.target]
          ctx.moveTo(a.x, a.y)
          ctx.lineTo(b.x, b.y)
        }
        ctx.stroke()
        const t = (now / 1100) % 1
        ctx.fillStyle = withAlpha('#0ea5e9', 0.95)
        ctx.beginPath()
        for (const e of model.simEdges) {
          if (e.source !== focus && e.target !== focus) continue
          const a = model.simNodes[e.source]
          const b = model.simNodes[e.target]
          const cx = a.x + (b.x - a.x) * t
          const cy = a.y + (b.y - a.y) * t
          ctx.moveTo(cx + px(2.6), cy)
          ctx.arc(cx, cy, px(2.6), 0, Math.PI * 2)
        }
        ctx.fill()
      }

      // Nodes batched by fill colour - one path per colour instead of one per node
      const buckets = new Map<string, number[]>()
      const visible: number[] = []
      for (let i = 0; i < model.simNodes.length; i++) {
        const sn = model.simNodes[i]
        if (!onScreen(sn.x, sn.y)) continue
        const n = nodes[i]
        const grown = Math.min(1, Math.max(0, appear * 1.4 - sn.ring * 0.06))
        if (grown <= 0) continue
        visible.push(i)
        const dimmed = (focus !== null && i !== focus && !near?.has(i)) || (matches !== null && !matches.has(i))
        // The homepage keeps its own colour in every mode - it's the reference point the rest
        // of the map is measured against, so it must never blend into a hue bucket
        const base = n.isHomepage
          ? HOMEPAGE_COLOR
          : colorMode === 'type'
            ? NODE_COLORS[n.type]
            : colorMode === 'depth'
              ? depthColor(n.depth)
              : healthColor(n)
        const fill = withAlpha(base, dimmed ? 0.3 : 1)
        const bucket = buckets.get(fill)
        if (bucket) bucket.push(i)
        else buckets.set(fill, [i])
      }

      const screenRadius = (i: number) => {
        const sn = model.simNodes[i]
        const grown = Math.min(1, Math.max(0, appear * 1.4 - sn.ring * 0.06))
        const raw = sn.radius * scale * grown * (i === focus ? 1.35 : 1)
        // the homepage never shrinks into the crowd, whatever the zoom
        const floor = nodes[i].isHomepage ? 9 : MIN_SCREEN_RADIUS
        return Math.max(floor, Math.min(MAX_SCREEN_RADIUS, raw))
      }

      for (const [fill, members] of buckets) {
        ctx.fillStyle = fill
        ctx.beginPath()
        for (const i of members) {
          const sn = model.simNodes[i]
          const r = px(screenRadius(i))
          ctx.moveTo(sn.x + r, sn.y)
          ctx.arc(sn.x, sn.y, r, 0, Math.PI * 2)
        }
        ctx.fill()
      }

      // Homepage halo - a double ring outside the fill, so it reads as the root even
      // when zoomed out to dots
      for (const i of visible) {
        if (!nodes[i].isHomepage) continue
        const sn = model.simNodes[i]
        const r = px(screenRadius(i))
        ctx.strokeStyle = withAlpha(HOMEPAGE_COLOR, 0.85)
        ctx.lineWidth = px(2)
        ctx.beginPath()
        ctx.arc(sn.x, sn.y, r + px(3), 0, Math.PI * 2)
        ctx.stroke()
        ctx.strokeStyle = withAlpha(HOMEPAGE_COLOR, 0.35)
        ctx.lineWidth = px(1.5)
        ctx.beginPath()
        ctx.arc(sn.x, sn.y, r + px(6.5), 0, Math.PI * 2)
        ctx.stroke()
      }

      const markers: Record<string, number[]> = { broken: [], orphan: [] }
      const slashes: number[] = []
      for (const i of visible) {
        const n = nodes[i]
        const dimmed = (focus !== null && i !== focus && !near?.has(i)) || (matches !== null && !matches.has(i))
        if (dimmed) continue
        const broken = n.state === 'CRAWLED' && (n.statusCode >= 400 || n.statusCode === 0)
        const orphan = n.state === 'UNSEEN' || (n.state === 'CRAWLED' && n.inLinks === 0 && !n.isHomepage)
        if (broken) markers.broken.push(i)
        else if (orphan && !n.isHomepage) markers.orphan.push(i)
        if (n.noindex) slashes.push(i)
      }
      for (const [kind, members] of Object.entries(markers)) {
        if (members.length === 0) continue
        ctx.strokeStyle = withAlpha(kind === 'broken' ? BROKEN_COLOR : ORPHAN_COLOR, 0.95)
        ctx.lineWidth = px(1.6)
        ctx.beginPath()
        for (const i of members) {
          const sn = model.simNodes[i]
          const r = px(screenRadius(i) + 3.5)
          ctx.moveTo(sn.x + r, sn.y)
          ctx.arc(sn.x, sn.y, r, 0, Math.PI * 2)
        }
        ctx.stroke()
      }
      if (slashes.length > 0) {
        ctx.strokeStyle = dark ? 'rgba(226,232,240,0.85)' : 'rgba(15,23,42,0.75)'
        ctx.lineWidth = px(1.4)
        ctx.beginPath()
        for (const i of slashes) {
          const sn = model.simNodes[i]
          const r = px(screenRadius(i))
          ctx.moveTo(sn.x - r, sn.y - r)
          ctx.lineTo(sn.x + r, sn.y + r)
        }
        ctx.stroke()
      }

      if (selectedIdx !== null && onScreen(model.simNodes[selectedIdx].x, model.simNodes[selectedIdx].y)) {
        const sn = model.simNodes[selectedIdx]
        const pulse = 1 + Math.sin(now / 320) * 0.25
        ctx.beginPath()
        ctx.strokeStyle = withAlpha('#0ea5e9', 0.6)
        ctx.lineWidth = px(2)
        ctx.arc(sn.x, sn.y, px(screenRadius(selectedIdx) + 9 * pulse), 0, Math.PI * 2)
        ctx.stroke()
      }

      const labelCells = new Set<number>()
      let drawn = 0
      ctx.fillStyle = dark ? 'rgba(241,245,249,0.94)' : 'rgba(15,23,42,0.9)'
      ctx.font = `${px(12)}px ui-sans-serif, system-ui`
      ctx.textAlign = 'center'
      const ordered = visible.slice().sort((a, b) => screenRadius(b) - screenRadius(a))
      for (const i of ordered) {
        if (drawn >= MAX_LABELS) break
        const n = nodes[i]
        const sn = model.simNodes[i]
        const pinned = i === focus || i === selectedIdx || n.isHomepage
        const dimmed = (focus !== null && !pinned && !near?.has(i)) || (matches !== null && !matches.has(i))
        if (dimmed) continue
        if (!pinned && screenRadius(i) < 9) continue
        const cell = Math.round((sn.x * scale + view.x) / 120) * 1000 + Math.round((sn.y * scale + view.y) / 16)
        if (!pinned && labelCells.has(cell)) continue
        labelCells.add(cell)
        drawn++
        const text = n.label.length > 30 ? `${n.label.slice(0, 29)}…` : n.label
        ctx.fillText(text, sn.x, sn.y - px(screenRadius(i) + 7))
      }
      ctx.restore()
    }

    raf = requestAnimationFrame(draw)
    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
      canvas.removeEventListener('wheel', onWheel)
    }
  }, [model, fitToView])

  const hitTest = (clientX: number, clientY: number): number | null => {
    const canvas = canvasRef.current
    if (!canvas) return null
    const rect = canvas.getBoundingClientRect()
    const view = viewRef.current
    const wx = (clientX - rect.left - view.x) / view.scale
    const wy = (clientY - rect.top - view.y) / view.scale
    let best: number | null = null
    let bestDist = Infinity
    for (let i = 0; i < model.simNodes.length; i++) {
      const n = model.simNodes[i]
      const dx = n.x - wx
      const dy = n.y - wy
      const d = dx * dx + dy * dy
      const screen = Math.max(MIN_SCREEN_RADIUS, Math.min(MAX_SCREEN_RADIUS, n.radius * view.scale))
      const reach = (screen / view.scale + 5 / view.scale) ** 2
      if (d < reach && d < bestDist) {
        best = i
        bestDist = d
      }
    }
    return best
  }

  const hoverNode = hoverIndex === null ? null : nodes[hoverIndex]
  const hoverDrawn = hoverIndex === null ? 0 : (model.neighbours[hoverIndex]?.size ?? 0)

  return (
    <div
      ref={wrapRef}
      className="relative w-full overflow-hidden rounded-md border bg-muted/20"
      style={{ height: height ?? 'calc(100vh - 22rem)', minHeight: 440 }}
    >
      <canvas
        ref={canvasRef}
        className="block h-full w-full cursor-grab active:cursor-grabbing"
        onMouseDown={(e) => {
          dragRef.current = { active: true, moved: false, lastX: e.clientX, lastY: e.clientY }
        }}
        onMouseMove={(e) => {
          const drag = dragRef.current
          if (drag.active) {
            const dx = e.clientX - drag.lastX
            const dy = e.clientY - drag.lastY
            if (Math.abs(dx) + Math.abs(dy) > 2) drag.moved = true
            dirtyRef.current = true
            viewRef.current.x += dx
            viewRef.current.y += dy
            drag.lastX = e.clientX
            drag.lastY = e.clientY
            return
          }
          const hit = hitTest(e.clientX, e.clientY)
          if (hit !== hoverRef.current) {
            dirtyRef.current = true
            hoverRef.current = hit
            setHoverIndex(hit)
          }
        }}
        onMouseUp={(e) => {
          const drag = dragRef.current
          dragRef.current = { ...drag, active: false }
          if (drag.moved) return
          const hit = hitTest(e.clientX, e.clientY)
          onSelect(hit === null ? null : nodes[hit].key)
        }}
        onMouseLeave={() => {
          dragRef.current = { ...dragRef.current, active: false }
          dirtyRef.current = true
          hoverRef.current = null
          setHoverIndex(null)
        }}
      />
      {hoverNode ? (
        <div className="pointer-events-none absolute bottom-3 left-3 max-w-[min(28rem,80%)] rounded-md border bg-background/95 px-3 py-2 text-xs shadow-sm">
          <div className="truncate font-medium">{hoverNode.label}</div>
          <div className="truncate font-mono text-muted-foreground">{hoverNode.path}</div>
          <div className="mt-1 text-muted-foreground">
            {hoverNode.state === 'CRAWLED'
              ? `HTTP ${hoverNode.statusCode} · ${hoverNode.inLinks} in · ${hoverNode.outLinks} out · ${hoverDrawn} drawn · ${hoverNode.depth} clicks deep${hoverNode.kind === 'FILTER' ? ' · facet URL' : hoverNode.kind === 'PARAM' ? ' · has parameters' : ''}`
              : hoverNode.state === 'DISCOVERED'
                ? `${hoverNode.inLinks} pages link here, but the crawl ran out of budget before fetching it`
                : 'published with a URL that never turned up as a link on any crawled page'}
          </div>
        </div>
      ) : null}
      <button
        type="button"
        onClick={fitToView}
        className="absolute right-3 top-3 rounded-md border bg-background/95 px-2 py-1 text-xs shadow-sm hover:bg-accent"
      >
        Fit
      </button>
    </div>
  )
}
