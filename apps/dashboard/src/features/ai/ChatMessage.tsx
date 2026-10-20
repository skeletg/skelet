import { lazy, Suspense, useState, type ReactNode } from 'react'
import type { BarDatum, LineSeries } from './ChatCharts'
import { cn } from '@/lib/utils'

// The assistant can embed lightweight UI widgets by emitting a fenced ```skelet-widget``` block whose
// body is one JSON object, a format taught to the model in the system prompt - Those blocks split
// out into real components and everything else renders as text - parsing is fully defensive, so
// a malformed or unknown block degrades to verbatim text rather than crashing the panel

type StatItem = { label?: string; value?: string | number; delta?: string; trend?: string }

// recharts is the heaviest thing the chat could pull in, and most answers never draw a chart - so the two chart
// widgets load on first use rather than with the signed-in shell, which mounts this panel on every page
const BarWidget = lazy(() => import('./ChatCharts').then((m) => ({ default: m.BarWidget })))
const LineWidget = lazy(() => import('./ChatCharts').then((m) => ({ default: m.LineWidget })))

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
                    ? 'text-success'
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
    return <p className="text-xs font-medium text-success">✓ {msg || 'Done.'}</p>
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
      return (
        <Suspense fallback={null}>
          <BarWidget title={widget.title} data={widget.data} unit={widget.unit} />
        </Suspense>
      )
    case 'line':
      return (
        <Suspense fallback={null}>
          <LineWidget title={widget.title} series={widget.series} unit={widget.unit} />
        </Suspense>
      )
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
