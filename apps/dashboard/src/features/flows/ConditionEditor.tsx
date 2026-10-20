import { useId } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useQuery } from '@apollo/client/react'

import { FlowConditionKindsDocument } from './Flows.gql'
import {
  CONDITION_KINDS,
  LIFECYCLE_STAGES,
  defaultCondition,
  type FlowCondition,
} from './flowTypes'

type SegmentOpt = { id: string; name: string }

// ConditionEditor edits one FlowCondition - shared by the split-node inspector and the flow-level enrollment filter -
// Picks a kind, then renders the fields that kind needs, plus a "negate" toggle to invert the test
export function ConditionEditor({
  value,
  onChange,
  segments,
}: {
  value: FlowCondition
  onChange: (c: FlowCondition) => void
  segments: SegmentOpt[]
}) {
  const fid = useId()
  // 'in_segment' is the wire alias; show it as 'segment' in the picker
  const kind = value.kind === 'in_segment' ? 'segment' : value.kind
  // Only offer what this install registered, unfiltered until the query answers so an existing flow's kind is never dropped
  const { data } = useQuery(FlowConditionKindsDocument)
  const registered = data?.flowConditionKinds
  const kinds = registered
    ? CONDITION_KINDS.filter((c) => registered.includes(c.value) || c.value === kind)
    : CONDITION_KINDS
  const set = (patch: Partial<FlowCondition>) => onChange({ ...value, ...patch })

  const num = (v: string) => {
    const n = parseInt(v, 10)
    return Number.isNaN(n) ? 0 : n
  }

  return (
    <div className="grid gap-2 rounded-md border p-2.5">
      <div className="grid gap-1.5">
        <Label htmlFor={`${fid}-condition`} className="text-xs">Condition</Label>
        <Select value={kind} onValueChange={(k) => onChange(defaultCondition(k))}>
          <SelectTrigger id={`${fid}-condition`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {kinds.map((c) => (
              <SelectItem key={c.value} value={c.value}>
                {c.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {kind === 'segment' && (
        <Select value={value.segmentId || undefined} onValueChange={(v) => set({ segmentId: v })}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Pick a segment…" />
          </SelectTrigger>
          <SelectContent>
            {segments.length === 0 ? (
              <SelectItem value="__none" disabled>
                No segments yet
              </SelectItem>
            ) : (
              segments.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  {s.name}
                </SelectItem>
              ))
            )}
          </SelectContent>
        </Select>
      )}

      {kind === 'lifecycle' && (
        <div className="flex flex-wrap gap-1.5">
          {LIFECYCLE_STAGES.map((stage) => {
            const on = (value.lifecycle ?? []).includes(stage)
            return (
              <button
                key={stage}
                type="button"
                onClick={() =>
                  set({
                    lifecycle: on
                      ? (value.lifecycle ?? []).filter((l) => l !== stage)
                      : [...(value.lifecycle ?? []), stage],
                  })
                }
                className={`rounded-full border px-2.5 py-0.5 text-xs capitalize ${on ? 'border-primary bg-primary text-primary-foreground' : 'text-muted-foreground'}`}
              >
                {stage.replace('_', ' ')}
              </button>
            )
          })}
        </div>
      )}

      {kind === 'rfm' && (
        <div className="grid grid-cols-2 gap-2">
          <NumField label="Min recency (1–5)" value={value.minRecency ?? 0} onChange={(n) => set({ minRecency: n })} />
          <NumField label="Min frequency (1–5)" value={value.minFrequency ?? 0} onChange={(n) => set({ minFrequency: n })} />
          <NumField label="Min monetary (1–5)" value={value.minMonetary ?? 0} onChange={(n) => set({ minMonetary: n })} />
          <NumField label="Min engagement (0–100)" value={value.minEngagement ?? 0} onChange={(n) => set({ minEngagement: n })} />
        </div>
      )}

      {kind === 'email_activity' && (
        <div className="grid grid-cols-2 gap-2">
          <NumField label="Min opens (lifetime)" value={value.minOpens ?? 0} onChange={(n) => set({ minOpens: n })} />
          <NumField label="Min clicks (lifetime)" value={value.minClicks ?? 0} onChange={(n) => set({ minClicks: n })} />
        </div>
      )}

      {kind === 'consent' && (
        <div className="grid gap-2">
          <Select value={value.consentKind || 'marketing_email'} onValueChange={(v) => set({ consentKind: v })}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="marketing_email">Marketing email</SelectItem>
              <SelectItem value="marketing_sms">Marketing SMS</SelectItem>
            </SelectContent>
          </Select>
          <label className="flex items-center gap-2 text-xs">
            <Switch checked={value.optedIn ?? true} onCheckedChange={(v) => set({ optedIn: v })} />
            Must be opted {value.optedIn ?? true ? 'in' : 'out'}
          </label>
        </div>
      )}

      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        <Switch checked={value.negate ?? false} onCheckedChange={(v) => set({ negate: v })} />
        Invert (NOT)
      </label>
    </div>
  )

  function NumField({ label, value: v, onChange: oc }: { label: string; value: number; onChange: (n: number) => void }) {
    const id = `${fid}-${label.replace(/\W+/g, '-')}` // the editor's id, so one per field across its NumFields
    return (
      <div className="grid gap-1">
        <Label htmlFor={id} className="text-[11px] text-muted-foreground">{label}</Label>
        <Input id={id} type="number" value={v} onChange={(e) => oc(num(e.target.value))} />
      </div>
    )
  }
}
