import { useState, type ReactNode } from 'react'
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { cn } from '@/lib/utils'

// The assistant can embed lightweight UI widgets by emitting a fenced ```skelet-widget``` block whose
// body is one JSON object, a format taught to the model in the system prompt - Those blocks split
// out into real components and everything else renders as text - parsing is fully defensive, so
// a malformed or unknown block degrades to verbatim text rather than crashing the panel

// Indigo accent, matching the Signals dashboards (PulseHome/SynapseHome) - The
// theme's --chart-1 token is a muted gray, so those pages hardcode this too
const ACCENT = '#6366f1'

type StatItem = { label?: string; value?: string | number; delta?: string; trend?: string }
type BarDatum = { label?: string; value?: number }
type LinePoint = { x?: string | number; y?: number }
type LineSeries = { label?: string; data?: LinePoint[] }

type Widget = {
  type: string
  items?: StatItem[]
  title?: string
  data?: BarDatum[]
  unit?: string
  columns?: string[]
  rows?: (string | number)[][]
  series?: LineSeries[]
  questions?: string[]
  action?: string
  label?: string
  args?: unknown
}

// ActionResult mirrors the runChatAction mutation result
export type ActionResult = { ok: boolean; message: string }
export type OnAction = (action: string, args: unknown) => Promise<ActionResult>

type Segment =
  | { kind: 'text'; text: string }
  | { kind: 'widget'; widget: Widget }
  | { kind: 'truncated' }

const WIDGET_TYPES = new Set(['stat', 'bar', 'table', 'line', 'kv', 'followups', 'action'])

// End index (exclusive) of the balanced JSON object starting at content[start], respecting
// string literals and escapes so braces inside strings don't miscount - -1 if it never balances
function matchObject(content: string, start: number): number {
  let depth = 0
  let inStr = false
  let esc = false
  for (let j = start; j < content.length; j++) {
    const ch = content[j]
    if (inStr) {
      if (esc) esc = false
      else if (ch === '\\') esc = true
      else if (ch === '"') inStr = false
    } else if (ch === '"') {
      inStr = true
    } else if (ch === '{') {
      depth++
    } else if (ch === '}' && --depth === 0) {
      return j + 1
    }
  }
  return -1
}

// Strip markdown code-fence marker lines (```skelet-widget / ```json / ```) so
// that - whether or not the model wrapped a widget in a fence - no orphaned
// backticks are left behind in the surrounding prose
function stripFences(s: string): string {
  return s.replace(/```[^\n]*\n?/g, '').trim()
}

// Split a reply into ordered text / widget / truncated segments - A widget is any
// JSON object carrying a known "type" - detected straight from its braces, so it
// renders whether the model fenced it (```skelet-widget) or emitted it bare
function parseSegments(content: string): Segment[] {
  const segments: Segment[] = []
  let buf = ''
  const flush = () => {
    const t = stripFences(buf)
    if (t) segments.push({ kind: 'text', text: t })
    buf = ''
  }
  let i = 0
  while (i < content.length) {
    if (content[i] === '{') {
      const end = matchObject(content, i)
      if (end === -1) {
        // Object never closes → reply was cut off mid-widget - If it looks like a
        // widget, drop the partial JSON and flag it; otherwise keep it as text
        if (/^\{\s*"type"\s*:/.test(content.slice(i))) {
          flush()
          segments.push({ kind: 'truncated' })
          return segments
        }
      } else {
        try {
          const obj = JSON.parse(content.slice(i, end)) as Widget
          if (obj && typeof obj.type === 'string' && WIDGET_TYPES.has(obj.type)) {
            flush()
            segments.push({ kind: 'widget', widget: obj })
            i = end
            continue
          }
        } catch {
          /* balanced but not valid JSON - fall through and render as text */
        }
      }
    }
    buf += content[i]
    i++
  }
  flush()
  return segments.length ? segments : [{ kind: 'text', text: content }]
}

function StatWidget({ items }: { items?: StatItem[] }) {
  const list = Array.isArray(items) ? items.filter((i) => i && (i.label || i.value != null)) : []
  if (!list.length) return null
  return (
    <div className="flex flex-wrap gap-2">
      {list.map((it, i) => (
        <div key={i} className="min-w-[6.5rem] flex-1 rounded-xl border bg-muted/40 px-3 py-2">
          <div className="truncate text-xs text-muted-foreground">{it.label}</div>
          <div className="mt-0.5 text-lg font-semibold tabular-nums">{String(it.value ?? '-')}</div>
          {it.delta ? (
            <div
              className={cn(
                'mt-0.5 text-xs font-medium tabular-nums',
                it.trend === 'down'
                  ? 'text-destructive'
                  : it.trend === 'up'
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : 'text-muted-foreground',
              )}
            >
              {it.delta}
            </div>
          ) : null}
        </div>
      ))}
    </div>
  )
}

// KvWidget renders a record's fields as a labelled detail list (e.g. a product's name/SKU/price) - the structured
// alternative to a markdown key/value table Reuses StatItem (label + value) for its items
function KvWidget({ title, items }: { title?: string; items?: StatItem[] }) {
  const list = Array.isArray(items) ? items.filter((i) => i && (i.label || i.value != null)) : []
  if (!list.length) return null
  return (
    <div className="overflow-hidden rounded-xl border">
      {title ? (
        <div className="border-b bg-muted/40 px-3 py-1.5 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {title}
        </div>
      ) : null}
      <dl className="divide-y text-sm">
        {list.map((it, i) => (
          <div key={i} className="flex gap-3 px-3 py-1.5">
            <dt className="w-28 shrink-0 text-muted-foreground">{it.label}</dt>
            <dd className="min-w-0 flex-1 break-words font-medium">{String(it.value ?? '-')}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function BarWidget({ title, data, unit }: { title?: string; data?: BarDatum[]; unit?: string }) {
  const rows = Array.isArray(data)
    ? data.filter((d): d is BarDatum => !!d && d.label != null && typeof d.value === 'number')
    : []
  if (!rows.length) return null
  const height = Math.max(96, rows.length * 34 + 24)
  const fmt = (v: number | string | undefined) => {
    const n = typeof v === 'number' ? v.toLocaleString() : String(v ?? '')
    if (unit === '$') return `$${n}`
    return unit ? `${n} ${unit}` : n
  }
  return (
    <div className="rounded-xl border bg-muted/30 p-3">
      {title ? <div className="mb-2 text-xs font-medium text-muted-foreground">{title}</div> : null}
      <ChartContainer height={height}>
        <BarChart data={rows} layout="vertical" margin={{ top: 0, right: 12, bottom: 0, left: 4 }}>
          <CartesianGrid horizontal={false} strokeDasharray="3 3" stroke="var(--color-border)" />
          <XAxis type="number" tick={{ fontSize: 11 }} allowDecimals={false} />
          <YAxis
            type="category"
            dataKey="label"
            width={96}
            tick={{ fontSize: 11 }}
            tickFormatter={(v: string) => (v.length > 16 ? `${v.slice(0, 15)}…` : v)}
          />
          <Tooltip cursor={{ fill: 'var(--color-muted)' }} content={<ChartTooltipContent valueFormatter={fmt} />} />
          <Bar dataKey="value" name={title || 'Value'} fill={ACCENT} radius={[0, 4, 4, 0]} barSize={14} />
        </BarChart>
      </ChartContainer>
    </div>
  )
}

// LineWidget renders one or two time-series as a line chart (trends, retention curves) - Series are merged into one
// row[] keyed by x (first-seen order), with each series as its own dataKey
function LineWidget({ title, series, unit }: { title?: string; series?: LineSeries[]; unit?: string }) {
  const list = Array.isArray(series) ? series.filter((s) => s && Array.isArray(s.data) && s.data.length) : []
  if (!list.length) return null
  const rows: Record<string, number | string>[] = []
  const byX = new Map<string | number, Record<string, number | string>>()
  const keys = list.map((s, si) => s.label || `Series ${si + 1}`)
  list.forEach((s, si) => {
    const key = keys[si]
    const data = s.data ?? []
    data.forEach((pt) => {
      if (!pt || pt.x == null || typeof pt.y !== 'number') return
      let row = byX.get(pt.x)
      if (!row) {
        row = { x: pt.x }
        byX.set(pt.x, row)
        rows.push(row)
      }
      row[key] = pt.y
    })
  })
  if (!rows.length) return null
  const colors = [ACCENT, 'var(--color-muted-foreground)']
  const fmt = (v: number | string | undefined) => {
    const n = typeof v === 'number' ? v.toLocaleString() : String(v ?? '')
    if (unit === '$') return `$${n}`
    return unit ? `${n} ${unit}` : n
  }
  return (
    <div className="rounded-xl border bg-muted/30 p-3">
      {title ? <div className="mb-2 text-xs font-medium text-muted-foreground">{title}</div> : null}
      <ChartContainer height={200}>
        <LineChart data={rows} margin={{ top: 8, right: 12, bottom: 0, left: -8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
          <XAxis dataKey="x" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
          <YAxis tick={{ fontSize: 11 }} width={44} tickFormatter={(v: number) => v.toLocaleString()} />
          <Tooltip content={<ChartTooltipContent valueFormatter={fmt} />} />
          {keys.length > 1 ? <Legend wrapperStyle={{ fontSize: 11 }} /> : null}
          {keys.map((k, i) => (
            <Line key={k} type="monotone" dataKey={k} name={k} stroke={colors[i % colors.length]} strokeWidth={2} dot={false} />
          ))}
        </LineChart>
      </ChartContainer>
    </div>
  )
}

function TableWidget({ columns, rows }: { columns?: string[]; rows?: (string | number)[][] }) {
  const cols = Array.isArray(columns) ? columns : []
  const body = Array.isArray(rows) ? rows.filter((r): r is (string | number)[] => Array.isArray(r)) : []
  if (!cols.length && !body.length) return null
  return (
    <div className="overflow-x-auto rounded-xl border">
      <table className="w-full text-sm">
        {cols.length ? (
          <thead>
            <tr className="border-b bg-muted/40">
              {cols.map((c, i) => (
                <th key={i} className="px-3 py-1.5 text-left font-medium text-muted-foreground">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
        ) : null}
        <tbody>
          {body.map((r, i) => (
            <tr key={i} className="border-b last:border-0">
              {r.map((cell, j) => (
                <td key={j} className="px-3 py-1.5 tabular-nums">
                  {String(cell)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ActionWidget renders a confirm button for an action the assistant proposed - It runs only on click (and only when onAction
// is provided - the latest answer); shows running/done/error inline - Nothing executes automatically
function ActionWidget({ action, label, args, onAction }: { action?: string; label?: string; args?: unknown; onAction?: OnAction }) {
  const [status, setStatus] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const [msg, setMsg] = useState('')
  if (!action || !onAction) return null
  if (status === 'done') {
    return <p className="text-xs font-medium text-emerald-600 dark:text-emerald-400">✓ {msg || 'Done.'}</p>
  }
  return (
    <div className="space-y-1.5">
      <button
        type="button"
        disabled={status === 'running'}
        onClick={async () => {
          setStatus('running')
          try {
            const res = await onAction(action, args)
            if (res.ok) {
              setStatus('done')
              setMsg(res.message)
            } else {
              setStatus('error')
              setMsg(res.message || 'Action failed.')
            }
          } catch (e) {
            setStatus('error')
            setMsg(e instanceof Error ? e.message : 'Action failed.')
          }
        }}
        className="rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50"
      >
        {status === 'running' ? 'Running…' : label || 'Confirm'}
      </button>
      {status === 'error' ? <p className="text-xs text-destructive">{msg}</p> : null}
    </div>
  )
}

function WidgetView({ widget, onAction }: { widget: Widget; onAction?: OnAction }) {
  switch (widget.type) {
    case 'stat':
      return <StatWidget items={widget.items} />
    case 'kv':
      return <KvWidget title={widget.title} items={widget.items} />
    case 'bar':
      return <BarWidget title={widget.title} data={widget.data} unit={widget.unit} />
    case 'line':
      return <LineWidget title={widget.title} series={widget.series} unit={widget.unit} />
    case 'table':
      return <TableWidget columns={widget.columns} rows={widget.rows} />
    case 'action':
      return <ActionWidget action={widget.action} label={widget.label} args={widget.args} onAction={onAction} />
    default:
      return null
  }
}

// ---- section grouping (markdown-lite) ----

// A heading line: markdown ATX (#..######) or a whole-line bold (**Title:**)
// Inline bold mid-sentence is NOT a heading (the bold must span the full line)
const HEADING = /^\s*(?:#{1,6}\s+(.*\S)|\*\*(.+?)\*\*)\s*:?\s*$/

function headingTitle(line: string): string | null {
  const m = HEADING.exec(line)
  if (!m) return null
  return (m[1] ?? m[2] ?? '').replace(/:\s*$/, '').trim() || null
}

type Section = { title: string; items: Segment[] }
type Doc = { preamble: Segment[]; sections: Section[] }

// Group flat segments into an optional preamble plus titled sections, splitting
// text segments at heading lines - Mirrors the detail pages, where each area is a
// bordered Panel with an uppercase header strip - A reply with no headings keeps
// everything in the preamble, so plain answers render flat exactly as before
function groupDoc(segments: Segment[]): Doc {
  const doc: Doc = { preamble: [], sections: [] }
  let current = doc.preamble
  const pushText = (t: string) => {
    if (t.trim()) current.push({ kind: 'text', text: t.trim() })
  }
  for (const seg of segments) {
    if (seg.kind !== 'text') {
      current.push(seg)
      continue
    }
    let buf = ''
    for (const line of seg.text.split('\n')) {
      const title = headingTitle(line)
      if (title) {
        pushText(buf)
        buf = ''
        const section: Section = { title, items: [] }
        doc.sections.push(section)
        current = section.items
      } else {
        buf += buf ? `\n${line}` : line
      }
    }
    pushText(buf)
  }
  return doc
}

// Render inline **bold** spans within a text run; everything else is plain text
function renderInline(text: string): ReactNode {
  const out: ReactNode[] = []
  const re = /\*\*(.+?)\*\*/g
  let last = 0
  let k = 0
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push(text.slice(last, m.index))
    out.push(
      <strong key={k++} className="font-semibold">
        {m[1]}
      </strong>,
    )
    last = m.index + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function SegmentView({ seg, onAction }: { seg: Segment; onAction?: OnAction }) {
  if (seg.kind === 'text')
    return <p className="whitespace-pre-wrap text-sm leading-relaxed">{renderInline(seg.text)}</p>
  if (seg.kind === 'truncated')
    return <p className="text-xs italic text-muted-foreground">…response was cut off - ask me to continue.</p>
  return <WidgetView widget={seg.widget} onAction={onAction} />
}

// FollowupsWidget renders the model's suggested next questions as clickable chips - Only shown when onAsk is provided
// (i.e. for the latest answer), so old suggestions don't accumulate; clicking sends that question
function FollowupsWidget({ questions, onAsk }: { questions?: string[]; onAsk?: (q: string) => void }) {
  const list = Array.isArray(questions) ? questions.map((q) => String(q).trim()).filter(Boolean) : []
  if (!list.length || !onAsk) return null
  return (
    <div className="flex flex-wrap gap-2 pt-1">
      {list.map((q, i) => (
        <button
          key={i}
          type="button"
          onClick={() => onAsk(q)}
          className="rounded-full border bg-muted/40 px-3 py-1 text-left text-xs text-foreground transition-colors hover:bg-muted"
        >
          {q}
        </button>
      ))}
    </div>
  )
}

// MessageContent renders an assistant reply: a free-text preamble, titled
// sections (bordered panels matching the detail pages), and - when onAsk is
// provided (the latest answer) - the model's suggested follow-up questions as
// chips at the very bottom - The followups widget is pulled out of the flow so it
// always renders last, outside any section card
export function MessageContent({
  content,
  onAsk,
  onAction,
}: {
  content: string
  onAsk?: (q: string) => void
  onAction?: OnAction
}) {
  const segments = parseSegments(content)
  const followups = segments.find((s) => s.kind === 'widget' && s.widget.type === 'followups')
  const rest = segments.filter((s) => !(s.kind === 'widget' && s.widget.type === 'followups'))
  const doc = groupDoc(rest)
  return (
    <div className="min-w-0 flex-1 space-y-3">
      {doc.preamble.map((seg, i) => (
        <SegmentView key={i} seg={seg} onAction={onAction} />
      ))}
      {doc.sections.map((s, i) => (
        <section key={i} className="rounded border">
          <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {s.title}
          </div>
          <div className="space-y-3 p-3">
            {s.items.map((seg, j) => (
              <SegmentView key={j} seg={seg} onAction={onAction} />
            ))}
          </div>
        </section>
      ))}
      {followups?.kind === 'widget' ? <FollowupsWidget questions={followups.widget.questions} onAsk={onAsk} /> : null}
    </div>
  )
}
