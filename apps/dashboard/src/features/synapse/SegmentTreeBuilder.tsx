import { useId } from 'react'
import { XIcon, PlusIcon, FolderPlusIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { eventLabel } from './eventLabel'
import {
  type SegNode,
  type TraitLeaf,
  type FieldKind,
  LIFECYCLES,
  FIELD_LABELS,
  emptyTrait,
  newEvent,
  newGroup,
  newCondition,
  conditionField,
} from './segment-tree'

export type TrackedAttribute = { slug: string; name: string; values: string[] }

// SegmentTreeBuilder is the recursive editor for a definition tree
export function SegmentTreeBuilder({
  node,
  onChange,
  events,
  attributes,
}: {
  node: SegNode
  onChange: (n: SegNode) => void
  events: string[]
  attributes: TrackedAttribute[]
}) {
  return <NodeEditor node={node} onChange={onChange} events={events} attributes={attributes} isRoot />
}

type EditorProps = {
  node: SegNode
  onChange: (n: SegNode) => void
  onRemove?: () => void
  events: string[]
  attributes: TrackedAttribute[]
  isRoot?: boolean
}

function NodeEditor({ node, onChange, onRemove, events, attributes, isRoot }: EditorProps) {
  if (node.kind === 'GROUP') {
    const children = node.children ?? []
    const op = node.op ?? 'AND'
    const setChild = (i: number, c: SegNode) => onChange({ ...node, children: children.map((x, k) => (k === i ? c : x)) })
    const removeChild = (i: number) => onChange({ ...node, children: children.filter((_, k) => k !== i) })
    return (
      <div className={cn('rounded-lg border p-2', !isRoot && 'bg-muted/30')}>
        {/* Operator only matters with 2+ conditions - hidden for a single one */}
        {children.length >= 2 ? (
          <div className="mb-2 flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Match</span>
            <div className="inline-flex rounded-md border bg-background p-0.5">
              {(['AND', 'OR'] as const).map((o) => (
                <button
                  key={o}
                  type="button"
                  onClick={() => onChange({ ...node, op: o })}
                  className={cn('rounded px-2 py-0.5 text-xs font-medium', op === o ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-muted')}
                >
                  {o === 'AND' ? 'all' : 'any'}
                </button>
              ))}
            </div>
            <span className="text-muted-foreground">of these:</span>
          </div>
        ) : null}
        <div className={cn('space-y-2', children.length >= 2 && 'border-l-2 border-border/60 pl-2')}>
          {children.length === 0 ? (
            <p className="px-1 py-1 text-xs text-muted-foreground">Empty group - add a condition.</p>
          ) : (
            children.map((c, i) => <NodeEditor key={i} node={c} onChange={(x) => setChild(i, x)} onRemove={() => removeChild(i)} events={events} attributes={attributes} />)
          )}
        </div>
        <div className="mt-2 flex items-center gap-1">
          <button type="button" onClick={() => onChange({ ...node, children: [...children, newEvent(events[0])] })} className="inline-flex items-center gap-1 rounded border bg-background px-2 py-0.5 text-xs hover:bg-muted">
            <PlusIcon className="size-3" /> Condition
          </button>
          <button type="button" onClick={() => onChange({ ...node, children: [...children, newGroup(op === 'AND' ? 'OR' : 'AND')] })} className="inline-flex items-center gap-1 rounded border bg-background px-2 py-0.5 text-xs text-muted-foreground hover:bg-muted">
            <FolderPlusIcon className="size-3" /> Group
          </button>
          {!isRoot && onRemove ? (
            <button type="button" onClick={onRemove} className="ml-auto rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground" title="Remove group">
              <XIcon className="size-3.5" />
            </button>
          ) : null}
        </div>
      </div>
    )
  }

  const field = conditionField(node)
  const fieldOpts = FIELD_LABELS.filter((f) => f.value !== 'affinity' || attributes.length > 0)
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-background p-2">
      <select value={field} onChange={(e) => onChange(newCondition(e.target.value as FieldKind, events))} className="rounded border bg-background px-1.5 py-1 text-xs">
        {fieldOpts.map((f) => (
          <option key={f.value} value={f.value}>
            {f.label}
          </option>
        ))}
      </select>
      <div className="flex flex-1 flex-wrap items-center gap-2">
        {field === 'event' ? (
          <EventLeaf node={node} onChange={onChange} events={events} />
        ) : field === 'affinity' ? (
          <AffinityLeaf node={node} onChange={onChange} attributes={attributes} />
        ) : field === 'lifecycle' ? (
          <LifecycleLeaf node={node} onChange={onChange} />
        ) : field === 'engagement' ? (
          <EngagementLeaf node={node} onChange={onChange} />
        ) : (
          <ScoreLeaf node={node} onChange={onChange} field={field} />
        )}
      </div>
      {onRemove ? (
        <button type="button" onClick={onRemove} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground" title="Remove condition">
          <XIcon className="size-3.5" />
        </button>
      ) : null}
    </div>
  )
}

function EventLeaf({ node, onChange, events }: { node: SegNode; onChange: (n: SegNode) => void; events: string[] }) {
  const opts = node.event && !events.includes(node.event) ? [node.event, ...events] : events
  return (
    <>
      <select value={node.negate ? 'no' : 'yes'} onChange={(e) => onChange({ ...node, negate: e.target.value === 'no' })} className="rounded border bg-background px-1.5 py-1 text-xs">
        <option value="yes">did</option>
        <option value="no">did not</option>
      </select>
      <select value={node.event} onChange={(e) => onChange({ ...node, event: e.target.value })} className="rounded border bg-background px-2 py-1 text-sm">
        {opts.map((ev) => (
          <option key={ev} value={ev}>
            {eventLabel(ev)}
          </option>
        ))}
      </select>
      {!node.negate ? (
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          at least
          <input type="number" min={1} value={node.count ?? 1} onChange={(e) => onChange({ ...node, count: Math.max(1, Number(e.target.value) || 1) })} className="w-12 rounded border bg-background px-1.5 py-1 text-sm tabular-nums" />
          time(s)
        </label>
      ) : null}
      <label className="flex items-center gap-1 text-xs text-muted-foreground">
        in the last
        <input type="number" min={1} value={node.days ?? 30} onChange={(e) => onChange({ ...node, days: Math.max(1, Number(e.target.value) || 1) })} className="w-16 rounded border bg-background px-1.5 py-1 text-sm tabular-nums" />
        days
      </label>
    </>
  )
}

function LifecycleLeaf({ node, onChange }: { node: SegNode; onChange: (n: SegNode) => void }) {
  const t = node.trait ?? emptyTrait()
  const toggle = (l: string) => onChange({ ...node, trait: { ...t, lifecycle: t.lifecycle.includes(l) ? t.lifecycle.filter((x) => x !== l) : [...t.lifecycle, l] } })
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">is</span>
      {LIFECYCLES.map((l) => {
        const on = t.lifecycle.includes(l)
        return (
          <button
            key={l}
            type="button"
            onClick={() => toggle(l)}
            className={cn('rounded-full border px-2 py-0.5 text-xs capitalize', on ? 'border-primary bg-muted font-medium' : 'text-muted-foreground hover:bg-muted')}
          >
            {l.replace('_', ' ')}
          </button>
        )
      })}
    </div>
  )
}

function ScoreLeaf({ node, onChange, field }: { node: SegNode; onChange: (n: SegNode) => void; field: 'recency' | 'frequency' | 'monetary' }) {
  const t = node.trait ?? emptyTrait()
  const key: keyof TraitLeaf = field === 'recency' ? 'minRecencyScore' : field === 'frequency' ? 'minFrequencyScore' : 'minMonetaryScore'
  const val = (t[key] as number) || 3
  return (
    <label className="flex items-center gap-1 text-xs text-muted-foreground">
      is at least
      <select value={val} onChange={(e) => onChange({ ...node, trait: { ...t, [key]: Number(e.target.value) } })} className="rounded border bg-background px-1.5 py-1 text-sm tabular-nums">
        {[1, 2, 3, 4, 5].map((n) => (
          <option key={n} value={n}>
            {n}
          </option>
        ))}
      </select>
      out of 5
    </label>
  )
}

function EngagementLeaf({ node, onChange }: { node: SegNode; onChange: (n: SegNode) => void }) {
  const t = node.trait ?? emptyTrait()
  return (
    <label className="flex items-center gap-1 text-xs text-muted-foreground">
      is at least
      <input
        type="number"
        min={0}
        max={100}
        value={t.minEngagement || 0}
        onChange={(e) => onChange({ ...node, trait: { ...t, minEngagement: Math.max(0, Math.min(100, Number(e.target.value) || 0)) } })}
        className="w-16 rounded border bg-background px-1.5 py-1 text-sm tabular-nums"
      />
      out of 100
    </label>
  )
}

function AffinityLeaf({ node, onChange, attributes }: { node: SegNode; onChange: (n: SegNode) => void; attributes: TrackedAttribute[] }) {
  const a = node.affinity ?? { attr: '', value: '', days: 90 }
  const set = (patch: Partial<{ attr: string; value: string; days: number }>) => onChange({ ...node, affinity: { ...a, ...patch } })
  const listId = useId()
  const selected = attributes.find((x) => x.slug === a.attr)
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <select value={a.attr} onChange={(e) => set({ attr: e.target.value, value: '' })} className="rounded border bg-background px-1.5 py-1 text-sm">
        <option value="">attribute…</option>
        {attributes.map((at) => (
          <option key={at.slug} value={at.slug}>
            {at.name}
          </option>
        ))}
      </select>
      <span>=</span>
      <input
        list={listId}
        value={a.value}
        onChange={(e) => set({ value: e.target.value })}
        placeholder={selected ? 'pick or type a value' : 'choose an attribute first'}
        disabled={!a.attr}
        className="w-40 rounded border bg-background px-2 py-1 text-sm disabled:opacity-50"
      />
      <datalist id={listId}>
        {(selected?.values ?? []).map((v) => (
          <option key={v} value={v} />
        ))}
      </datalist>
      <label className="flex items-center gap-1">
        in the last
        <input type="number" min={1} value={a.days} onChange={(e) => set({ days: Math.max(1, Number(e.target.value) || 1) })} className="w-14 rounded border bg-background px-1.5 py-1 text-sm tabular-nums" />
        days
      </label>
    </div>
  )
}
