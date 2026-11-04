import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { SERIES, SERIES_OTHER } from '@/lib/chart-colors'
import { cn } from '@/lib/utils'
import { formatCents, formatNumber, formatShortDay } from '@/lib/format'
import { aggregateNote } from '@/features/synapse/aggregateNote'
import { SynapseInsightsDocument } from '@/features/synapse/Synapse.gql'
import { NET_SALES, conditionsIn, isNetSales, type ReportDefinition } from '../report'
import { eventLabel } from '@/features/synapse/eventLabel'
import { ConditionsEditor } from './conditions'
import { Bars, Field, Headline, Note, State, StatTile, Toggle, pct, useTopEvents, type ControlsProps, type ResultProps } from './ui'

// What a sum can add up - integer properties the spine's commerce events carry
export const SUM_KEYS: { key: string; label: string }[] = [
  { key: 'net_base_cents', label: 'Net sales (base currency)' },
  { key: 'total_base_cents', label: 'Order total (base currency)' },
  { key: 'revenue_base_cents', label: 'Attributed revenue (base currency)' },
  { key: 'qty', label: 'Units' },
]
const NET = '__net_sales'
const eventsOf = (d: ReportDefinition) => d.events ?? (d.event ? [d.event] : [])

// Breakdown properties the spine's events carry. '' = none
export const BREAKDOWNS: { value: string; label: string }[] = [
  { value: '', label: 'No breakdown' },
  { value: 'device', label: 'Device' },
  { value: 'os', label: 'OS' },
  { value: 'client', label: 'Browser' },
  { value: 'country', label: 'Country' },
  { value: 'utm_source', label: 'Source (UTM)' },
  { value: 'sku', label: 'SKU' },
  { value: 'status', label: 'Status' },
  { value: 'page_key', label: 'Page' },
]
const COLORS = [...SERIES, SERIES_OTHER]

export function InsightsControls({ channelId, value: d, onChange }: ControlsProps) {
  const events = useTopEvents(channelId)
  const net = isNetSales(d)
  const current = net ? NET : (d.event ?? '')
  const options = !current || current === NET || events.includes(current) ? events : [current, ...events]
  return (
    <div className="space-y-3">
    <div className="flex flex-wrap items-end gap-3 text-sm">
      <Field label="Event">
        <select
          aria-label="Event"
          value={current}
          onChange={(e) => {
            const v = e.target.value
            if (v === NET) onChange({ ...d, event: undefined, events: [...NET_SALES.events], measure: 'sum', sumKey: NET_SALES.sumKey })
            else onChange({ ...d, event: v, events: undefined, ...(net ? { measure: 'total', sumKey: undefined } : {}) })
          }}
          className="rounded border bg-background px-2 py-1.5 text-sm"
        >
          <option value={NET}>Net sales</option>
          <option value="">Any event</option>
          {options.map((e) => (
            <option key={e} value={e}>
              {eventLabel(e)}
            </option>
          ))}
        </select>
      </Field>
      {!net && (
        <Field label="Measure">
          <Toggle
            label="Measure"
            options={[
              { v: 'total', l: 'Total' },
              { v: 'unique', l: 'Unique' },
              { v: 'sum', l: 'Sum' },
            ]}
            value={d.measure ?? 'total'}
            onChange={(measure) => onChange({ ...d, measure, sumKey: measure === 'sum' ? (d.sumKey ?? SUM_KEYS[0].key) : undefined })}
          />
        </Field>
      )}
      {!net && d.measure === 'sum' && (
        <Field label="Sum of">
          <select aria-label="Sum of" value={d.sumKey ?? ''} onChange={(e) => onChange({ ...d, sumKey: e.target.value })} className="rounded border bg-background px-2 py-1.5 text-sm">
            {SUM_KEYS.map((k) => (
              <option key={k.key} value={k.key}>
                {k.label}
              </option>
            ))}
          </select>
        </Field>
      )}
      <Field label="Break down by">
        <select aria-label="Break down by" value={d.breakdown ?? ''} onChange={(e) => onChange({ ...d, breakdown: e.target.value })} className="rounded border bg-background px-2 py-1.5 text-sm">
          {BREAKDOWNS.map((b) => (
            <option key={b.value} value={b.value}>
              {b.label}
            </option>
          ))}
        </select>
      </Field>
      <Field label="By">
        <Toggle label="By" options={[{ v: 'day', l: 'Day' }, { v: 'week', l: 'Week' }]} value={d.unit ?? 'day'} onChange={(unit) => onChange({ ...d, unit })} />
      </Field>
    </div>
    <ConditionsEditor channelId={channelId} events={eventsOf(d)} label="Where" value={d.where ?? []} onChange={(where) => onChange({ ...d, where })} />
    </div>
  )
}

export function InsightsResult({ channelId, d, sinceDays, compact }: ResultProps) {
  const measure = d.measure ?? 'total'
  const unit = d.unit ?? 'day'
  const breakdown = d.breakdown ?? ''
  const { data, loading, error } = useQuery(SynapseInsightsDocument, {
    variables: { channelId, event: d.event || null, events: d.events ?? null, sumKey: d.sumKey ?? null, filters: conditionsIn(d.where), breakdown: breakdown || null, measure, unit, sinceDays },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const ins = data?.synapseInsights
  const unique = measure === 'unique'
  const sum = measure === 'sum' // amounts ride beside the clamped counts, which a sum in cents would overflow
  const cur = sum ? (ins?.totalAmount ?? 0) : unique ? (ins?.uniques ?? 0) : (ins?.total ?? 0)
  const prev = sum ? (ins?.prevTotalAmount ?? 0) : unique ? (ins?.prevUniques ?? 0) : (ins?.prevTotal ?? 0)
  const money = sum && (d.sumKey ?? '').endsWith('_cents')
  const fmt = (n: number) => (money ? formatCents(n, ins?.baseCurrency ?? 'USD') : formatNumber(n))
  const valueOf = (p: { value: number; amount: number }) => (sum ? p.amount : p.value)
  const delta = prev > 0 ? pct(cur - prev, prev) : null
  const seriesKeys = useMemo(() => (ins?.series ?? []).map((s) => s.key), [ins])

  // Pivot (bucket, key, value) points into recharts rows: { bucket, <key>: value, … }, missing series as 0 so lines connect
  const chartData = useMemo(() => {
    const byBucket = new Map<string, Record<string, number | string>>()
    for (const p of ins?.points ?? []) {
      const label = formatShortDay(p.bucket)
      const row = byBucket.get(label) ?? { bucket: label }
      row[p.key] = valueOf(p)
      byBucket.set(label, row)
    }
    const rows = [...byBucket.values()]
    for (const row of rows) for (const k of seriesKeys) if (row[k] === undefined) row[k] = 0
    return rows
  }, [ins, seriesKeys])
  const bucketTotals = chartData.map((r) => seriesKeys.reduce((s, k) => s + (Number(r[k]) || 0), 0))

  if (compact) {
    return (
      <State loading={loading && !ins} error={error}>
        <div className="space-y-3">
          <Headline value={fmt(cur)} label={sum ? (isNetSales(d) ? 'net sales' : `sum of ${d.sumKey}`) : unique ? 'people' : 'events'} delta={delta} />
          <Bars values={bucketTotals} />
          {breakdown && seriesKeys.length > 0 && (
            <ul className="divide-y text-xs">
              {ins!.series.slice(0, 5).map((s) => (
                <li key={s.key} className="flex justify-between py-1">
                  <span className="font-mono">{s.key}</span>
                  <span className="tabular-nums">{fmt(sum ? s.amount : s.total)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </State>
    )
  }

  const unitWord = unit === 'day' ? 'day' : 'week'
  const note = aggregateNote(measure, ins?.aggregate ?? 0)
  const avgPerUnit = chartData.length ? Math.round(bucketTotals.reduce((a, b) => a + b, 0) / chartData.length) : 0
  const peakIdx = bucketTotals.reduce((bi, v, i, arr) => (v > arr[bi] ? i : bi), 0)
  const seriesTotal = (ins?.series ?? []).reduce((s, x) => s + (sum ? x.amount : x.total), 0) || 1
  const measureLabel = sum ? (isNetSales(d) ? 'Net sales' : `Sum of ${d.sumKey}`) : unique ? 'Unique people' : 'Total events'
  const seriesLabel = (k: string) => (breakdown ? k : measureLabel)

  return (
    <State loading={loading && !ins} error={error}>
      {chartData.length === 0 ? (
        <Note>No events match in this window.</Note>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatTile
              label={measureLabel}
              value={
                <span className="flex items-baseline gap-2">
                  {fmt(cur)}
                  {delta !== null && (
                    <span className={cn('text-xs font-medium', delta > 0 ? 'text-success' : delta < 0 ? 'text-destructive' : 'text-muted-foreground')}>
                      {delta > 0 ? '▲' : delta < 0 ? '▼' : ''} {Math.abs(delta)}%
                    </span>
                  )}
                </span>
              }
              sub={
                <>
                  {delta !== null ? `vs ${fmt(prev)} prior period` : 'no prior-period data'}
                  {note && <span className="block">{note}</span>}
                </>
              }
            />
            <StatTile label={`Avg per ${unitWord}`} value={fmt(avgPerUnit)} sub={`across ${chartData.length} ${unitWord}s`} />
            <StatTile label={`Peak ${unitWord}`} value={fmt(bucketTotals[peakIdx] ?? 0)} sub={(chartData[peakIdx]?.bucket as string | undefined) ?? '-'} />
          </div>
          <div className="h-72 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData} margin={{ top: 8, right: 16, bottom: 0, left: -8 }}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                <XAxis dataKey="bucket" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
                <Tooltip content={<ChartTooltipContent />} />
                {seriesKeys.length > 1 && <Legend wrapperStyle={{ fontSize: 12 }} />}
                {seriesKeys.map((k, i) => (
                  <Line key={k} type="monotone" dataKey={k} name={seriesLabel(k)} stroke={COLORS[i % COLORS.length]} dot={false} strokeWidth={2} />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
          {breakdown && (
            <div className="overflow-x-auto rounded border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                    <th className="px-3 py-2">{BREAKDOWNS.find((b) => b.value === breakdown)?.label ?? breakdown}</th>
                    <th className="px-3 py-2 text-right">{measureLabel}</th>
                    <th className="w-2/5 px-3 py-2">Share</th>
                  </tr>
                </thead>
                <tbody>
                  {ins!.series.map((s, i) => {
                    const share = Math.round(((sum ? s.amount : s.total) / seriesTotal) * 100)
                    return (
                      <tr key={s.key} className="border-b last:border-0">
                        <td className="px-3 py-1.5">
                          <span className="mr-2 inline-block size-2 rounded-full align-middle" style={{ background: COLORS[i % COLORS.length] }} />
                          {s.key}
                        </td>
                        <td className="px-3 py-1.5 text-right tabular-nums">{fmt(sum ? s.amount : s.total)}</td>
                        <td className="px-3 py-1.5">
                          <div className="flex items-center gap-2">
                            <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                              <span className="block h-full rounded-full" style={{ width: `${Math.max(2, share)}%`, background: COLORS[i % COLORS.length] }} />
                            </span>
                            <span className="w-9 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{share}%</span>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
          {unique && breakdown && (
            <p className="text-xs text-muted-foreground">
              Unique counts are exact per point; totals across series or time may slightly over-count a person seen in several
              buckets or values.
            </p>
          )}
        </div>
      )}
    </State>
  )
}
