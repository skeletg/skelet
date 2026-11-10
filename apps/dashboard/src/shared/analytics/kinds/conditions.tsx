import { useId, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { PlusIcon, XIcon } from 'lucide-react'
import { SynapseInsightsDocument } from '@/features/synapse/Synapse.gql'
import type { SynapsePropertyFilterInput } from '@/graphql/schema.types'
import { CONDITION_OP_LABEL, type Condition, type ConditionOp } from './definition'

// The properties the spine's events carry, named - picking one beats typing it, since a typo filters everything out
export const KNOWN_KEYS: { key: string; label: string }[] = [
  { key: 'sku', label: 'Product (SKU)' },
  { key: 'campaign_id', label: 'Campaign' },
  { key: 'flow_id', label: 'Flow' },
  { key: 'utm_source', label: 'Source (UTM)' },
  { key: 'utm_medium', label: 'Medium (UTM)' },
  { key: 'utm_campaign', label: 'Campaign (UTM)' },
  { key: 'device', label: 'Device' },
  { key: 'os', label: 'OS' },
  { key: 'client', label: 'Browser' },
  { key: 'country', label: 'Country' },
  { key: 'page_key', label: 'Page' },
  { key: 'status', label: 'Status' },
]
export const keyLabel = (k: string) => KNOWN_KEYS.find((x) => x.key === k)?.label ?? k

// The most common values of a property on these events, for the value box to suggest - an Insights breakdown already
// answers exactly that
function useTopValues(channelId: string, events: string[], key: string): string[] {
  const { data } = useQuery(SynapseInsightsDocument, {
    variables: { channelId, events, breakdown: key, measure: 'total', unit: 'week', sinceDays: 90 },
    skip: !channelId || !key,
    fetchPolicy: 'cache-first',
  })
  return (data?.synapseInsights.series ?? []).map((s) => s.key).filter((k) => k !== '(none)')
}

// ConditionsEditor edits a list of conditions, each a property, an operator and its values. It takes them as a
// definition holds them, where the API types op as any string - every one this editor wrote is a ConditionOp
export function ConditionsEditor({ channelId, events, label, value: held, onChange }: { channelId: string; events: string[]; label: string; value: readonly SynapsePropertyFilterInput[] | null | undefined; onChange: (next: Condition[]) => void }) {
  const value = (held ?? []) as Condition[]
  const set = (i: number, c: Condition) => onChange(value.map((x, k) => (k === i ? c : x)))
  return (
    <div className="space-y-2">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      {value.map((c, i) => (
        <ConditionRow key={i} n={i + 1} channelId={channelId} events={events} c={c} onChange={(next) => set(i, next)} onRemove={() => onChange(value.filter((_, k) => k !== i))} />
      ))}
      <button type="button" onClick={() => onChange([...value, { key: 'sku', op: 'eq', values: [''] }])} className="inline-flex items-center gap-1 rounded border px-2 py-1 text-xs hover:bg-muted">
        <PlusIcon className="size-3" />
        Add a condition
      </button>
    </div>
  )
}

function ConditionRow({ n, channelId, events, c, onChange, onRemove }: { n: number; channelId: string; events: string[]; c: Condition; onChange: (c: Condition) => void; onRemove: () => void }) {
  const listId = useId()
  const known = KNOWN_KEYS.some((k) => k.key === c.key)
  const [custom, setCustom] = useState(!known)
  const suggestions = useTopValues(channelId, events, c.key)
  const text = c.op === 'in' ? c.values.join(', ') : (c.values[0] ?? '')
  const setText = (t: string) => onChange({ ...c, values: c.op === 'in' ? t.split(',').map((v) => v.trim()) : [t] })
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-sm">
      {custom ? (
        <input aria-label={`Property of condition ${n}`} value={c.key} onChange={(e) => onChange({ ...c, key: e.target.value })} placeholder="property" className="w-32 rounded border bg-background px-2 py-1 text-xs" />
      ) : (
        <select
          aria-label={`Property of condition ${n}`}
          value={c.key}
          onChange={(e) => (e.target.value === '' ? setCustom(true) : onChange({ ...c, key: e.target.value, values: [''] }))}
          className="rounded border bg-background px-2 py-1 text-xs"
        >
          {KNOWN_KEYS.map((k) => (
            <option key={k.key} value={k.key}>
              {k.label}
            </option>
          ))}
          <option value="">Other property…</option>
        </select>
      )}
      <select
        aria-label={`Operator of condition ${n}`}
        value={c.op}
        onChange={(e) => {
          const op = e.target.value as ConditionOp
          onChange({ ...c, op, values: op === 'in' ? c.values : [c.values[0] ?? ''] })
        }}
        className="rounded border bg-background px-2 py-1 text-xs"
      >
        {(Object.keys(CONDITION_OP_LABEL) as ConditionOp[]).map((op) => (
          <option key={op} value={op}>
            {CONDITION_OP_LABEL[op]}
          </option>
        ))}
      </select>
      <input
        aria-label={`Value of condition ${n}`}
        list={listId}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={c.op === 'in' ? 'values, comma separated' : 'value'}
        className="w-40 rounded border bg-background px-2 py-1 text-xs"
      />
      <datalist id={listId}>
        {suggestions.map((v) => (
          <option key={v} value={v} />
        ))}
      </datalist>
      <button type="button" aria-label={`Remove condition ${n}`} onClick={onRemove} className="rounded p-1 text-muted-foreground hover:bg-muted">
        <XIcon className="size-3.5" />
      </button>
    </div>
  )
}
