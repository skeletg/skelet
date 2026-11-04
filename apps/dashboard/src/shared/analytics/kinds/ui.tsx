import { useQuery } from '@apollo/client/react'
import { cn } from '@/lib/utils'
import { formatNumber } from '@/lib/format'
import { eventLabel } from '@/features/synapse/eventLabel'
import { SynapseOverviewDocument } from '@/features/synapse/Synapse.gql'
import type { ReportDefinition } from '../report'

// What every kind's controls and result take - the board's sheet and the report pages hand them the same props
export type ControlsProps = { channelId: string; value: ReportDefinition; onChange: (next: ReportDefinition) => void }
export type ResultProps = {
  channelId: string
  d: ReportDefinition
  sinceDays: number
  compact?: boolean
  // Where a result can put a report it leads to - "see where the people who dropped off went" - on the board or a page
  onAddReport?: (title: string, d: ReportDefinition) => void
}

export const pct = (n: number, d: number) => (d > 0 ? Math.round((n / d) * 100) : 0)

// The event names this shop has recorded lately, busiest first - what a step or a focus can be chosen from
export function useTopEvents(channelId: string): string[] {
  const { data } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !channelId, fetchPolicy: 'cache-first' })
  return (data?.synapseOverview.topEvents ?? []).map((e) => e.name)
}

// A caption over a control - a div, not a label, since a label would fold its text into a toggle's first button's name;
// each control names itself with aria-label instead
export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <span aria-hidden className="text-xs uppercase tracking-wider text-muted-foreground">
        {label}
      </span>
      {children}
    </div>
  )
}

// Toggle is a pick-one of a few options; aria-pressed tells a screen reader which one is on
export function Toggle<T extends string>({ label, options, value, onChange }: { label: string; options: { v: T; l: string }[]; value: T; onChange: (v: T) => void }) {
  return (
    <div role="group" aria-label={label} className="inline-flex w-fit rounded-md border p-0.5">
      {options.map((o) => (
        <button
          key={o.v}
          type="button"
          aria-pressed={value === o.v}
          onClick={() => onChange(o.v)}
          className={cn('rounded px-2.5 py-1 text-xs', value === o.v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
        >
          {o.l}
        </button>
      ))}
    </div>
  )
}

// EventSelect keeps the current value listed even when it hasn't happened lately, so a saved report never shows blank
export function EventSelect({ label, value, onChange, events, anyLabel }: { label: string; value: string; onChange: (v: string) => void; events: string[]; anyLabel?: string }) {
  const options = !value || events.includes(value) ? events : [value, ...events]
  return (
    <select aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} className="rounded border bg-background px-2 py-1.5 text-sm">
      {anyLabel !== undefined && <option value="">{anyLabel}</option>}
      {options.map((e) => (
        <option key={e} value={e}>
          {eventLabel(e)}
        </option>
      ))}
    </select>
  )
}

export function Note({ children }: { children: React.ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}

// State shows a query's error or first load in place of its result; a refetch keeps the last result on screen
export function State({ loading, error, children }: { loading: boolean; error?: { message: string }; children: React.ReactNode }) {
  if (error) return <p className="py-6 text-center text-sm text-destructive">{error.message}</p>
  if (loading) return <Note>Loading…</Note>
  return <>{children}</>
}

export function Bars({ values }: { values: number[] }) {
  const top = Math.max(1, ...values)
  return (
    <div className="flex h-16 items-end gap-px" aria-hidden>
      {values.map((v, i) => (
        <div key={i} className="flex-1 rounded-t-sm bg-series-1" style={{ height: `${Math.max(2, (v / top) * 100)}%` }} />
      ))}
    </div>
  )
}

export function Headline({ value, label, delta }: { value: number | string; label: string; delta?: number | null }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-2">
      <span className="text-2xl font-semibold tabular-nums">{typeof value === 'number' ? formatNumber(value) : value}</span>
      <span className="text-xs text-muted-foreground">{label}</span>
      {delta != null && (
        <span className={delta >= 0 ? 'text-xs text-success' : 'text-xs text-destructive'}>
          {delta >= 0 ? '+' : ''}
          {delta}% vs the period before
        </span>
      )}
    </div>
  )
}

export function StatTile({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      {sub && <div className="mt-0.5 text-xs text-muted-foreground">{sub}</div>}
    </div>
  )
}
