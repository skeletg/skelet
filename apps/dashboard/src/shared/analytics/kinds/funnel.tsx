import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ChevronDownIcon, ChevronUpIcon, FilterIcon, XIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatNumber } from '@/lib/format'
import { eventLabel } from '@/features/synapse/eventLabel'
import { SynapseFunnelDocument } from '@/features/synapse/Synapse.gql'
import { CONDITION_OP_LABEL, type ConditionOp, type ControlsProps, type Kind, type ReportDefinition, type ResultProps } from './definition'
import { KNOWN_KEYS, keyLabel } from './conditions'
import { Note, State, pct, useTopEvents } from './ui'

// The Flow report that follows a step's drop-offs: who did step i and didn't go on to step i+1, held to step i's filters
export function dropOffFlow(d: ReportDefinition, i: number): ReportDefinition {
  const steps = d.steps ?? []
  const where = (d.filters ?? []).filter((f) => f.step === i).map((f) => ({ key: f.key, op: f.op ?? 'eq', values: f.values?.length ? f.values : [f.value] }))
  return { kind: 'FLOW', event: steps[i], depth: 4, notThen: steps[i + 1], notThenWithinHours: d.windowHours, where: where.length ? where : undefined }
}

const WINDOWS: { label: string; hours: number | null }[] = [
  { label: 'Any time in the period', hours: null },
  { label: 'Within 1 hour', hours: 1 },
  { label: 'Within 1 day', hours: 24 },
  { label: 'Within 7 days', hours: 24 * 7 },
  { label: 'Within 30 days', hours: 24 * 30 },
]

// A filter names its step by index, so moving or dropping a step renumbers the filters with it
export function moveStep(d: ReportDefinition, i: number, j: number): ReportDefinition {
  const steps = [...(d.steps ?? [])]
  if (j < 0 || j >= steps.length) return d
  ;[steps[i], steps[j]] = [steps[j], steps[i]]
  const filters = (d.filters ?? []).map((f) => ({ ...f, step: f.step === i ? j : f.step === j ? i : f.step }))
  return { ...d, steps, filters }
}

export function removeStep(d: ReportDefinition, i: number): ReportDefinition {
  const steps = (d.steps ?? []).filter((_, k) => k !== i)
  const filters = (d.filters ?? []).filter((f) => f.step !== i).map((f) => ({ ...f, step: f.step > i ? f.step - 1 : f.step }))
  return { ...d, steps, filters }
}

function FunnelControls({ channelId, value: d, onChange }: ControlsProps) {
  const available = useTopEvents(channelId)
  const steps = d.steps ?? []
  const filters = d.filters ?? []
  const [filtering, setFiltering] = useState<number | null>(null) // the step whose filter form is open
  const [breakdownDraft, setBreakdownDraft] = useState(d.breakdown ?? '')
  const set = (next: ReportDefinition) => {
    setFiltering(null)
    onChange(next)
  }

  return (
    <div className="space-y-2">
      {steps.map((st, i) => (
        <div key={`${st}-${i}`} className="rounded border px-3 py-2 text-sm">
          <div className="flex items-center gap-2">
            <span className="w-5 shrink-0 text-center text-xs font-semibold text-muted-foreground">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate font-medium">{eventLabel(st)}</span>
            <span className="hidden text-xs text-muted-foreground sm:inline">{st}</span>
            <button type="button" aria-label={`Filter step ${i + 1}`} onClick={() => setFiltering(filtering === i ? null : i)} className="rounded p-1 text-muted-foreground hover:bg-muted">
              <FilterIcon className="size-4" />
            </button>
            <button type="button" aria-label={`Move step ${i + 1} up`} onClick={() => set(moveStep(d, i, i - 1))} disabled={i === 0} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
              <ChevronUpIcon className="size-4" />
            </button>
            <button type="button" aria-label={`Move step ${i + 1} down`} onClick={() => set(moveStep(d, i, i + 1))} disabled={i === steps.length - 1} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
              <ChevronDownIcon className="size-4" />
            </button>
            <button type="button" aria-label={`Remove step ${i + 1}`} onClick={() => set(removeStep(d, i))} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
              <XIcon className="size-4" />
            </button>
          </div>
          {filters.some((f) => f.step === i) && (
            <div className="mt-1.5 flex flex-wrap gap-1.5 pl-7">
              {filters.map((f, j) =>
                f.step !== i ? null : (
                  <span key={`${f.key}-${j}`} className="inline-flex items-center gap-1 rounded bg-muted px-2 py-0.5 text-xs">
                    <span>{keyLabel(f.key)}</span> {CONDITION_OP_LABEL[(f.op ?? 'eq') as ConditionOp]} <span className="font-mono">{f.values?.length ? f.values.join(', ') : f.value}</span>
                    <button type="button" aria-label={`Remove filter ${f.key} from step ${i + 1}`} onClick={() => set({ ...d, filters: filters.filter((_, n) => n !== j) })} className="text-muted-foreground hover:text-foreground">
                      <XIcon className="size-3" />
                    </button>
                  </span>
                ),
              )}
            </div>
          )}
          {filtering === i && <StepFilterForm step={i + 1} onAdd={(f) => set({ ...d, filters: [...filters, { step: i, ...f }] })} />}
        </div>
      ))}
      <div className="flex items-center gap-2 pt-1">
        <select
          aria-label="Add a step"
          value=""
          onChange={(e) => e.target.value && set({ ...d, steps: [...steps, e.target.value] })}
          className="rounded border bg-background px-2 py-1.5 text-sm"
        >
          <option value="" disabled>
            + Add step…
          </option>
          {available.map((e) => (
            <option key={e} value={e}>
              {eventLabel(e)}
            </option>
          ))}
        </select>
        {steps.length < 2 && <span className="text-xs text-muted-foreground">Add at least 2 steps.</span>}
      </div>
      <div className="flex flex-wrap items-center gap-4 border-t pt-3">
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Conversion window</span>
          <select
            value={d.windowHours ?? ''}
            onChange={(e) => onChange({ ...d, windowHours: e.target.value ? Number(e.target.value) : undefined })}
            className="rounded border bg-background px-2 py-1.5 text-sm"
          >
            {WINDOWS.map((w) => (
              <option key={w.label} value={w.hours ?? ''}>
                {w.label}
              </option>
            ))}
          </select>
        </label>
        <form
          className="flex items-center gap-2 text-sm"
          onSubmit={(e) => {
            e.preventDefault()
            onChange({ ...d, breakdown: breakdownDraft.trim() })
          }}
        >
          <label className="flex items-center gap-2">
            <span className="text-muted-foreground">Break down by</span>
            <input
              value={breakdownDraft}
              onChange={(e) => setBreakdownDraft(e.target.value)}
              onBlur={() => breakdownDraft.trim() !== (d.breakdown ?? '') && onChange({ ...d, breakdown: breakdownDraft.trim() })}
              placeholder="a property of step 1, e.g. device"
              className="w-56 rounded border bg-background px-2 py-1.5 text-sm"
            />
          </label>
        </form>
      </div>
    </div>
  )
}

// StepFilterForm holds one step to events whose property compares to a value - the property picked, not typed, since a
// typo would quietly filter everything out
function StepFilterForm({ step, onAdd }: { step: number; onAdd: (f: { key: string; value: string; op: ConditionOp; values?: string[] }) => void }) {
  const [key, setKey] = useState(KNOWN_KEYS[0].key)
  const [op, setOp] = useState<ConditionOp>('eq')
  const [value, setValue] = useState('')
  return (
    <form
      className="mt-2 flex flex-wrap items-center gap-2 pl-7"
      onSubmit={(e) => {
        e.preventDefault()
        if (!key.trim() || !value.trim()) return
        const values = value.split(',').map((v) => v.trim()).filter(Boolean)
        onAdd(op === 'in' ? { key: key.trim(), value: values[0], op, values } : { key: key.trim(), value: value.trim(), op })
      }}
    >
      <select aria-label={`Property for step ${step}`} value={key} onChange={(e) => setKey(e.target.value)} className="rounded border bg-background px-2 py-1 text-xs">
        {KNOWN_KEYS.map((k) => (
          <option key={k.key} value={k.key}>
            {k.label}
          </option>
        ))}
      </select>
      <select aria-label={`Operator for step ${step}`} value={op} onChange={(e) => setOp(e.target.value as ConditionOp)} className="rounded border bg-background px-2 py-1 text-xs">
        {(Object.keys(CONDITION_OP_LABEL) as ConditionOp[]).map((o) => (
          <option key={o} value={o}>
            {CONDITION_OP_LABEL[o]}
          </option>
        ))}
      </select>
      <input aria-label={`Value for step ${step}`} value={value} onChange={(e) => setValue(e.target.value)} placeholder={op === 'in' ? 'values, comma separated' : 'value'} className="w-36 rounded border bg-background px-2 py-1 text-xs" />
      <button type="submit" disabled={!key.trim() || !value.trim()} className="rounded border px-2 py-1 text-xs hover:bg-muted disabled:opacity-40">
        Add filter
      </button>
    </form>
  )
}

function FunnelResult({ channelId, d, sinceDays, compact, onAddReport }: ResultProps) {
  const steps = d.steps ?? []
  const breakdown = d.breakdown ?? ''
  const { data, loading, error } = useQuery(SynapseFunnelDocument, {
    variables: { channelId, steps, sinceDays, windowHours: d.windowHours ?? null, filters: d.filters ?? [], breakdown: breakdown || null },
    skip: !channelId || steps.length < 2,
    fetchPolicy: 'cache-and-network',
  })
  if (steps.length < 2) return <Note>Add at least two steps to see the funnel.</Note>
  const result = data?.synapseFunnel.steps ?? []
  const split = data?.synapseFunnel.breakdown ?? []
  const top = result[0]?.count ?? 0

  return (
    <State loading={loading && result.length === 0} error={error}>
      <div className={compact ? 'space-y-2' : 'space-y-4'}>
        <div className="space-y-2">
          {result.map((s, i) => {
            const overall = pct(s.count, top)
            const fromPrev = pct(s.count, i > 0 ? result[i - 1].count : s.count)
            return (
              <div key={`${s.eventName}-${i}`} className={compact ? '' : 'rounded border p-3'}>
                <div className="mb-1 flex items-baseline justify-between gap-3 text-xs">
                  <span className={compact ? '' : 'text-sm font-medium'}>
                    {!compact && <span className="mr-2 text-xs text-muted-foreground">{i + 1}</span>}
                    {eventLabel(s.eventName)}
                  </span>
                  <span className="tabular-nums">
                    {formatNumber(s.count)} <span className="text-muted-foreground">{overall}%</span>
                  </span>
                </div>
                <div className={cn('overflow-hidden rounded-full bg-muted', compact ? 'h-1.5' : 'h-2')}>
                  <div className="h-full rounded-full bg-series-1" style={{ width: `${overall}%` }} />
                </div>
                {!compact && i > 0 && (
                  <div className={cn('mt-1 flex flex-wrap items-center gap-2 text-xs', fromPrev < 50 ? 'text-warning' : 'text-muted-foreground')}>
                    <span>
                      {fromPrev}% from previous step · {100 - fromPrev}% drop-off
                    </span>
                    {onAddReport && fromPrev < 100 && (
                      <button
                        type="button"
                        onClick={() => onAddReport(`After ${eventLabel(steps[i - 1])}, not ${eventLabel(steps[i])}`, dropOffFlow(d, i - 1))}
                        className="rounded border px-1.5 py-0.5 text-foreground hover:bg-muted"
                      >
                        See where the {formatNumber(result[i - 1].count - s.count)} went
                      </button>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
        {!compact && breakdown && (
          <div>
            <h3 className="mb-2 text-sm font-semibold">By {breakdown}</h3>
            {split.length === 0 ? (
              <Note>No first-step events carry {breakdown}.</Note>
            ) : (
              <div className="overflow-x-auto rounded border">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left text-muted-foreground">
                      <th className="px-3 py-2 font-medium">{breakdown}</th>
                      {steps.map((s, i) => (
                        <th key={`${s}-${i}`} className="px-3 py-2 text-right font-medium">
                          {i + 1}. {eventLabel(s)}
                        </th>
                      ))}
                      <th className="px-3 py-2 text-right font-medium">Converted</th>
                    </tr>
                  </thead>
                  <tbody>
                    {split.map((b) => (
                      <tr key={b.value} className="border-b last:border-0">
                        <td className="px-3 py-2 font-mono text-xs">{b.value}</td>
                        {b.counts.map((n, i) => (
                          <td key={i} className="px-3 py-2 text-right tabular-nums">
                            {formatNumber(n)}
                          </td>
                        ))}
                        <td className="px-3 py-2 text-right tabular-nums">{pct(b.counts[b.counts.length - 1] ?? 0, b.counts[0] ?? 0)}%</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
    </State>
  )
}

export const funnel: Kind = {
  label: 'Funnel',
  menu: { label: 'Funnel', hint: 'Conversion through steps, in order' },
  describe: (d) => (d.steps ?? []).join(' → ') + (d.windowHours ? ` within ${d.windowHours} h` : ''),
  title: (d) => (d.steps ?? []).map(eventLabel).join(' → '),
  start: { kind: 'FUNNEL', steps: ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed'] },
  Controls: FunnelControls,
  Result: FunnelResult,
}
