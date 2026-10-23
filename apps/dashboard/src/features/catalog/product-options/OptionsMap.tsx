import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import {
  BaseEdge,
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  useStore,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
  type ReactFlowState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { EyeIcon, LayersIcon, PlayIcon, ShoppingCartIcon, WorkflowIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { ProductOption } from '../productOptionsStaging'
import { buildOptionsModel, type LinkKind, type OptionNodeData, type RuleNodeData, type StepNodeData } from './options-graph'
import { estimateBoxes, headerRoom, layoutOptionsMap, NODE_W, roundedPath, RULE_W, slotCounts, STEP_W, type Box, type Point } from './options-layout'
import type { OptionRule } from './rules'

// Read-only: a click opens the editor for what was clicked, a hover lights up that card's lines

const hidden = '!size-px !min-w-0 !border-0 !opacity-0'
const PAD = 32

type Slots = { slots: number }

function OptionNode({ data, selected }: NodeProps<Node<OptionNodeData & Slots>>) {
  return (
    <div
      style={{ width: NODE_W }}
      className={cn('rounded-md border bg-card text-left shadow-sm', data.when && 'border-dashed', selected && 'ring-2 ring-ring')}
    >
      <Handle type="target" position={Position.Top} id="in" className={hidden} />
      <Handle type="source" position={Position.Bottom} id="out" className={hidden} />
      <div className="px-3 pt-2 pb-1" style={{ minHeight: headerRoom(data.slots) }}>
        <div className="flex items-center gap-2">
          <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-semibold tabular-nums">
            {data.step}
          </span>
          <span className="truncate text-sm font-medium">{data.title}</span>
        </div>
        <p className="mt-0.5 text-[11px] text-muted-foreground">
          {data.type}
          {data.required ? ' · required' : ''}
        </p>
        {data.when ? (
          <p className="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground">
            <EyeIcon className="mr-1 inline size-3 align-[-2px]" aria-hidden />
            Only when {data.when}
          </p>
        ) : null}
        {data.template ? (
          <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
            <LayersIcon className="mr-1 inline size-3 align-[-2px]" aria-hidden />
            from {data.template}
          </p>
        ) : null}
      </div>
      {data.choices.length > 0 ? (
        <ul className="pb-2">
          {data.choices.map((c) => (
            <li key={c.id} className="relative flex items-center gap-2 px-3 text-[11px]" style={{ height: 20 }}>
              <span
                aria-hidden
                className={cn('size-1.5 shrink-0 rounded-full border border-muted-foreground', data.linked.includes(c.id) && 'border-foreground bg-foreground')}
              />
              <span className="truncate">{c.title}</span>
              <Handle type="source" position={Position.Right} id={`v:${c.id}`} className={hidden} />
            </li>
          ))}
          {data.more > 0 ? (
            <li className="flex items-center px-3 pl-[1.6rem] text-[11px] text-muted-foreground" style={{ height: 20 }}>
              +{data.more} more
            </li>
          ) : null}
        </ul>
      ) : null}
    </div>
  )
}

function RuleNode({ data, selected }: NodeProps<Node<RuleNodeData & Slots>>) {
  return (
    <div
      style={{ width: RULE_W, minHeight: headerRoom(data.slots) }}
      className={cn('rounded-md border border-dashed bg-background px-3 py-2 text-left shadow-sm', selected && 'ring-2 ring-ring')}
    >
      <Handle type="target" position={Position.Right} id="in" className={hidden} />
      <Handle type="source" position={Position.Left} id="out" className={hidden} />
      <div className="flex items-center gap-1.5 text-xs font-semibold">
        <WorkflowIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{data.title}</span>
      </div>
      <p className="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground">When {data.when}</p>
      <ul className="mt-1 flex flex-wrap gap-1">
        {data.effects.map((e) => (
          <li key={e} className="rounded border px-1.5 py-0.5 text-[11px]">{e}</li>
        ))}
      </ul>
    </div>
  )
}

function StepNode({ data }: NodeProps<Node<StepNodeData>>) {
  const Icon = data.kind === 'start' ? PlayIcon : ShoppingCartIcon
  return (
    <div style={{ width: STEP_W }} className="flex items-center justify-center gap-1.5 whitespace-nowrap rounded-full border bg-muted px-3 py-1.5 text-xs font-medium">
      <Handle type="target" position={Position.Top} id="in" className={hidden} />
      <Handle type="source" position={Position.Bottom} id="out" className={hidden} />
      <Icon className="size-3.5 text-muted-foreground" />
      {data.label}
    </div>
  )
}

type RoutedData = { points: Point[]; dot: boolean }

function RoutedEdge({ data, style, markerEnd }: EdgeProps<Edge<RoutedData>>) {
  if (!data || data.points.length < 2) return null
  const first = data.points[0]
  return (
    <>
      <BaseEdge path={roundedPath(data.points)} style={style} markerEnd={markerEnd} />
      {data.dot ? <circle cx={first.x} cy={first.y} r={2.5} style={{ fill: style?.stroke, opacity: style?.opacity }} /> : null}
    </>
  )
}

const nodeTypes = { option: OptionNode, rule: RuleNode, step: StepNode }
const edgeTypes = { routed: RoutedEdge }

const MUTED = 'var(--color-muted-foreground)'
const INK = 'var(--color-foreground)'
const LINE: Record<LinkKind, CSSProperties> = {
  path: { stroke: MUTED, strokeWidth: 1.5 },
  chain: { stroke: MUTED, strokeWidth: 1.5, strokeDasharray: '2 3' },
  rejoin: { stroke: MUTED, strokeWidth: 1.5, strokeDasharray: '2 3' },
  show: { stroke: INK, strokeWidth: 1.5 },
  read: { stroke: MUTED, strokeWidth: 1.25, strokeDasharray: '5 3' },
  act: { stroke: INK, strokeWidth: 1.25, strokeDasharray: '5 3' },
  fx: { stroke: INK, strokeWidth: 1.25, strokeDasharray: '5 3' },
}

function Legend() {
  const row = 'flex items-center gap-2'
  return (
    <ul className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground" aria-label="Map key">
      <li className={row}><span className="w-5 border-t-[1.5px] border-muted-foreground" />Customer path</li>
      <li className={row}><span className="w-5 border-t-[1.5px] border-dotted border-muted-foreground" />Back from a branch</li>
      <li className={row}><span className="w-5 border-t-[1.5px] border-foreground" />Answer shows an option</li>
      <li className={row}><span className="w-5 border-t border-dashed border-muted-foreground" />Rule reads an answer</li>
      <li className={row}><span className="w-5 border-t border-dashed border-foreground" />Rule changes something</li>
    </ul>
  )
}

// Sizes and choice lines as the browser drew them, as one string so an unchanged measure doesn't re-render
const selectMeasured = (s: ReactFlowState) => {
  const out: string[] = []
  for (const [id, n] of s.nodeLookup) {
    if (!n.measured.width || !n.measured.height) continue
    const ports = (n.internals.handleBounds?.source ?? [])
      .filter((h) => h.id?.startsWith('v:'))
      .map((h) => `${h.id?.slice(2)}=${Math.round(h.y + h.height / 2)}`)
    out.push(`${id}|${Math.round(n.measured.width)}|${Math.round(n.measured.height)}|${ports.join(',')}`)
  }
  return out.sort().join(';')
}

function parseMeasured(raw: string) {
  const out = new Map<string, Box>()
  for (const row of raw ? raw.split(';') : []) {
    const [id, w, h, ports] = row.split('|')
    out.set(id, { w: Number(w), h: Number(h), ports: Object.fromEntries((ports ? ports.split(',') : []).map((p) => [p.slice(0, p.lastIndexOf('=')), Number(p.slice(p.lastIndexOf('=') + 1))])) })
  }
  return out
}

type MapProps = {
  options: ProductOption[]
  rules: OptionRule[]
  locale: string
  defaultLocale: string
  currency: string
  onOpenOption: (id: string) => void
  onOpenRule: (id: string) => void
}

function MapCanvas({ options, rules, locale, defaultLocale, currency, onOpenOption, onOpenRule }: MapProps) {
  const model = useMemo(() => buildOptionsModel(options, rules, locale, defaultLocale, currency), [options, rules, locale, defaultLocale, currency])
  const slots = useMemo(() => slotCounts(model), [model])
  const estimates = useMemo(() => estimateBoxes(model), [model])
  const measuredRaw = useStore(selectMeasured)
  const measured = useMemo(() => parseMeasured(measuredRaw), [measuredRaw])
  const ready = [...estimates.keys()].every((id) => measured.has(id))
  const layout = useMemo(
    () => layoutOptionsMap(model, (id) => measured.get(id) ?? estimates.get(id) ?? { w: 0, h: 0, ports: {} }),
    [model, measured, estimates],
  )
  const [hovered, setHovered] = useState<string | null>(null)

  // A node handed back without its measure reads as never measured to React Flow, and stays hidden
  const nodes = useMemo((): Node[] => {
    const place = (id: string) => {
      const m = measured.get(id)
      return { position: layout.positions.get(id) ?? { x: 0, y: 0 }, ...(m ? { measured: { width: m.w, height: m.h } } : {}) }
    }
    return [
      { id: 'start', type: 'step', ...place('start'), selectable: false, data: { kind: 'start', label: 'Customer opens the product' } },
      ...model.options.map((o) => ({ id: o.id, type: 'option', ...place(o.id), data: { ...o.data, slots: slots.get(o.id) ?? 0 } })),
      ...model.rules.map((r) => ({ id: r.id, type: 'rule', ...place(r.id), data: { ...r.data, slots: slots.get(r.id) ?? 0 } })),
      { id: 'cart', type: 'step', ...place('cart'), selectable: false, data: { kind: 'cart', label: 'Add to cart' } },
    ]
  }, [model, layout, slots, measured])
  const edges = useMemo(
    (): Edge[] =>
      layout.routes.map((r) => {
        const lit = hovered !== null && (r.from === hovered || r.to === hovered)
        const style = { ...LINE[r.kind], ...(hovered === null ? {} : lit ? { strokeWidth: 2.25 } : { opacity: 0.15 }) }
        return {
          id: r.id, type: 'routed', source: r.from, target: r.to, data: { points: r.points, dot: r.dot }, style,
          markerEnd: r.kind === 'chain' ? undefined : { type: MarkerType.ArrowClosed, width: 11, height: 11, color: String(style.stroke) },
        }
      }),
    [layout, hovered],
  )

  // As tall as the graph at the zoom that fits its width, so the page scrolls it like a document
  const box = useRef<HTMLDivElement>(null)
  const [boxWidth, setBoxWidth] = useState<number | null>(null)
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    setBoxWidth(el.getBoundingClientRect().width || 900)
    const watch = new ResizeObserver(([entry]) => entry && entry.contentRect.width > 0 && setBoxWidth(entry.contentRect.width))
    watch.observe(el)
    return () => watch.disconnect()
  }, [])
  const zoom = boxWidth ? Math.max(0.45, Math.min(1, (boxWidth - 2 * PAD) / layout.width)) : 1
  const { setViewport } = useReactFlow()
  useEffect(() => {
    if (boxWidth) void setViewport({ x: (boxWidth - layout.width * zoom) / 2, y: PAD * zoom, zoom })
  }, [boxWidth, layout.width, zoom, setViewport])

  return (
    <div className="grid gap-2">
      <Legend />
      <div
        ref={box}
        className="rounded-md border transition-opacity"
        style={{ height: Math.max(360, Math.min(6000, (layout.height + 2 * PAD) * zoom)), opacity: ready ? 1 : 0 }}
        data-testid="options-map"
      >
        {boxWidth === null ? null : (
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            nodesConnectable={false}
            nodesDraggable={false}
            edgesFocusable={false}
            onNodeClick={(_, n) => {
              const [kind, id] = [n.id.slice(0, n.id.indexOf(':')), n.id.slice(n.id.indexOf(':') + 1)]
              if (kind === 'opt') onOpenOption(id)
              else if (kind === 'rule') onOpenRule(id)
            }}
            onNodeMouseEnter={(_, n) => setHovered(n.id)}
            onNodeMouseLeave={() => setHovered(null)}
            minZoom={0.3}
            zoomOnScroll={false}
            preventScrolling={false}
            proOptions={{ hideAttribution: true }}
          >
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
        )}
      </div>
    </div>
  )
}

export default function OptionsMap(props: MapProps) {
  return (
    <ReactFlowProvider>
      <MapCanvas {...props} />
    </ReactFlowProvider>
  )
}
