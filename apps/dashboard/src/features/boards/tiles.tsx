import { useQuery } from '@apollo/client/react'
import { formatNumber } from '@/lib/format'
import { eventLabel } from '@/features/synapse/eventLabel'
import {
  SynapseFlowDocument,
  SynapseFunnelDocument,
  SynapseInsightsDocument,
  SynapseRetentionDocument,
  SynapseStickinessDocument,
} from '@/features/synapse/Synapse.gql'
import type { ReportDefinition } from './report'

const pct = (n: number, d: number) => (d > 0 ? Math.round((n / d) * 100) : 0)

export type TileProps = { channelId: string; d: ReportDefinition; sinceDays: number }

export function Note({ children }: { children: React.ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}

function State({ loading, error, children }: { loading: boolean; error?: { message: string }; children: React.ReactNode }) {
  if (error) return <p className="py-6 text-center text-sm text-destructive">{error.message}</p>
  if (loading) return <Note>Loading…</Note>
  return <>{children}</>
}

function Bars({ values }: { values: number[] }) {
  const top = Math.max(1, ...values)
  return (
    <div className="flex h-16 items-end gap-px" aria-hidden>
      {values.map((v, i) => (
        <div key={i} className="flex-1 rounded-t-sm bg-series-1" style={{ height: `${Math.max(2, (v / top) * 100)}%` }} />
      ))}
    </div>
  )
}

function Headline({ value, label, delta }: { value: number; label: string; delta?: number | null }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="text-2xl font-semibold tabular-nums">{formatNumber(value)}</span>
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

export function InsightsTile({ channelId, d, sinceDays }: TileProps) {
  const { data, loading, error } = useQuery(SynapseInsightsDocument, {
    variables: { channelId, event: d.event || null, breakdown: d.breakdown || null, measure: d.measure ?? null, unit: d.unit ?? null, sinceDays },
  })
  const ins = data?.synapseInsights
  const unique = d.measure === 'unique'
  const total = unique ? (ins?.uniques ?? 0) : (ins?.total ?? 0)
  const prev = unique ? (ins?.prevUniques ?? 0) : (ins?.prevTotal ?? 0)
  const byBucket = new Map<string, number>()
  for (const p of ins?.points ?? []) byBucket.set(p.bucket, (byBucket.get(p.bucket) ?? 0) + p.value)
  return (
    <State loading={loading && !ins} error={error}>
      <div className="space-y-3">
        <Headline value={total} label={unique ? 'people' : 'events'} delta={prev > 0 ? pct(total - prev, prev) : null} />
        <Bars values={[...byBucket.values()]} />
        {d.breakdown && (ins?.series.length ?? 0) > 0 && (
          <ul className="divide-y text-xs">
            {ins!.series.slice(0, 5).map((s) => (
              <li key={s.key} className="flex justify-between py-1">
                <span className="font-mono">{s.key}</span>
                <span className="tabular-nums">{formatNumber(s.total)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </State>
  )
}

export function FunnelTile({ channelId, d, sinceDays }: TileProps) {
  const { data, loading, error } = useQuery(SynapseFunnelDocument, {
    variables: { channelId, steps: d.steps ?? [], sinceDays, windowHours: d.windowHours ?? null, filters: d.filters ?? [], breakdown: d.breakdown || null },
  })
  const steps = data?.synapseFunnel.steps ?? []
  const top = steps[0]?.count ?? 0
  return (
    <State loading={loading && steps.length === 0} error={error}>
      <div className="space-y-2">
        {steps.map((s, i) => (
          <div key={`${s.eventName}-${i}`}>
            <div className="mb-0.5 flex justify-between text-xs">
              <span>{eventLabel(s.eventName)}</span>
              <span className="tabular-nums">
                {formatNumber(s.count)} <span className="text-muted-foreground">{pct(s.count, top)}%</span>
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-series-1" style={{ width: `${pct(s.count, top)}%` }} />
            </div>
          </div>
        ))}
      </div>
    </State>
  )
}

export function RetentionTile({ channelId, d, sinceDays }: TileProps) {
  const { data, loading, error } = useQuery(SynapseRetentionDocument, {
    variables: { channelId, startEvent: d.startEvent ?? '', returnEvent: d.returnEvent ?? '', sinceDays, periods: d.periods ?? null, unit: d.unit ?? null },
  })
  const r = data?.synapseRetention
  const buckets = (r?.buckets ?? []).filter((b) => b.period > 0)
  return (
    <State loading={loading && !r} error={error}>
      <div className="space-y-3">
        <Headline value={r?.cohort ?? 0} label="people started" />
        <div className="grid grid-cols-4 gap-1 text-center text-xs sm:grid-cols-8">
          {buckets.map((b) => (
            <div key={b.period} className="rounded bg-muted px-1 py-1.5">
              <div className="text-muted-foreground">
                {d.unit === 'week' ? 'W' : 'D'}
                {b.period}
              </div>
              <div className="font-medium tabular-nums">{b.eligible > 0 ? `${pct(b.count, b.eligible)}%` : '-'}</div>
            </div>
          ))}
        </div>
      </div>
    </State>
  )
}

export function StickinessTile({ channelId, d, sinceDays }: TileProps) {
  const { data, loading, error } = useQuery(SynapseStickinessDocument, { variables: { channelId, event: d.event || null, sinceDays } })
  const st = data?.synapseStickiness
  return (
    <State loading={loading && !st} error={error}>
      <div className="space-y-3">
        <Headline value={Math.round(st?.returnRate ?? 0)} label={`% came back within ${st?.windowDays ?? '-'} days`} />
        <div className="flex gap-4 text-xs text-muted-foreground">
          <span>
            Weekly active <span className="font-medium text-foreground tabular-nums">{formatNumber(st?.wau ?? 0)}</span>
          </span>
          <span>
            Monthly active <span className="font-medium text-foreground tabular-nums">{formatNumber(st?.mau ?? 0)}</span>
          </span>
        </div>
        <Bars values={(st?.days ?? []).map((p) => p.count)} />
      </div>
    </State>
  )
}

export function FlowTile({ channelId, d, sinceDays }: TileProps) {
  const { data, loading, error } = useQuery(SynapseFlowDocument, { variables: { channelId, event: d.event ?? '', sinceDays, depth: d.depth ?? null } })
  const f = data?.synapseFlow
  return (
    <State loading={loading && !f} error={error}>
      <div className="space-y-2">
        <Headline value={f?.cohort ?? 0} label={`people · ${pct(f?.continued ?? 0, f?.cohort ?? 0)}% went on`} />
        <ul className="divide-y text-xs">
          {(f?.topPaths ?? []).slice(0, 4).map((p) => (
            <li key={p.events.join('>')} className="flex justify-between gap-2 py-1">
              <span className="truncate">{p.events.map(eventLabel).join(' → ')}</span>
              <span className="shrink-0 tabular-nums">{formatNumber(p.count)}</span>
            </li>
          ))}
        </ul>
      </div>
    </State>
  )
}
