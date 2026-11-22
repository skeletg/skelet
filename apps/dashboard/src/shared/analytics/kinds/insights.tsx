import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { SERIES, SERIES_OTHER } from '@/lib/chart-colors'
import { cn } from '@/lib/utils'
import { formatCents, formatNumber, formatShortDay } from '@/lib/format'
import { eventLabel } from '@/features/synapse/eventLabel'
import { aggregateNote } from '@/features/synapse/aggregateNote'
import { SynapseInsightsDocument } from '@/features/synapse/Synapse.gql'
import { ConditionsEditor } from './conditions'
import { Bars, EventSelect, Field, Headline, Note, State, StatTile, Toggle, pct, useTopEvents } from './ui'
import { NET_SALES, describeConditions, isNetSales, readyConditions, type ControlsProps, type Kind, type ReportDefinition, type ResultProps } from './definition'

// What a sum can add up - integer properties the spine's commerce events carry
export const SUM_KEYS: { key: string; label: string }[] = [
  { key: 'net_base_cents', label: 'Net sales (base currency)' },
  { key: 'total_base_cents', label: 'Order total (base currency)' },
  { key: 'revenue_base_cents', label: 'Attributed revenue (base currency)' },
  { key: 'qty', label: 'Units' },
]
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
// What an unset measure and unit read as
const DEFAULTS = { measure: 'total', unit: 'day' } as const

function InsightsControls({ channelId, value: d, onChange }: ControlsProps) {
  const events = useTopEvents(channelId)
  const net = isNetSales(d)
  // Net sales is a measure of its own: the sum of every line's net over the lines placed, edited and refunded
  const setMeasure = (m: string) =>
    onChange(
      m === 'net'
        ? { ...d, event: undefined, events: [...NET_SALES.events], measure: NET_SALES.measure, sumKey: NET_SALES.sumKey }
        : { ...d, measure: m, sumKey: m === 'sum' ? (net ? SUM_KEYS[0].key : (d.sumKey ?? SUM_KEYS[0].key)) : undefined, ...(net ? { events: undefined } : {}) },
    )
  return (
    <div className="space-y-3">
    <div className="flex flex-wrap items-end gap-3 text-sm">
      <Field label="Measure">
        <Toggle
          label="Measure"
          options={[
            { v: 'total', l: 'Total' },
            { v: 'unique', l: 'Unique' },
            { v: 'sum', l: 'Sum' },
            { v: 'net', l: 'Net sales' },
          ]}
          value={net ? 'net' : (d.measure ?? DEFAULTS.measure)}
          onChange={setMeasure}
        />
      </Field>
      {!net && (
        <Field label="Event">
          <EventSelect label="Event" value={d.event ?? ''} onChange={(event) => onChange({ ...d, event })} events={events} anyLabel="Any event" />
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
        <Toggle label="By" options={[{ v: 'day', l: 'Day' }, { v: 'week', l: 'Week' }]} value={d.unit ?? DEFAULTS.unit} onChange={(unit) => onChange({ ...d, unit })} />
      </Field>
    </div>
    <ConditionsEditor channelId={channelId} events={eventsOf(d)} label="Where" value={d.where} onChange={(where) => onChange({ ...d, where })} />
    </div>
  )
}

function InsightsResult({ channelId, d, sinceDays, compact }: ResultProps) {
  const measure = d.measure ?? DEFAULTS.measure
  const unit = d.unit ?? DEFAULTS.unit
  const breakdown = d.breakdown ?? ''
  const { data, loading, error } = useQuery(SynapseInsightsDocument, {
    variables: { channelId, event: d.event || null, events: d.events ?? null, sumKey: d.sumKey ?? null, filters: readyConditions(d.where), breakdown: breakdown || null, measure, unit, sinceDays },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const ins = data?.synapseInsights
  const unique = measure === 'unique'
  const sum = measure === 'sum'
  // Every figure reads the measure's amount, which no sum in cents outgrows - unique people have their own headline
  const cur = unique ? (ins?.uniques ?? 0) : (ins?.totalAmount ?? 0)
  const prev = unique ? (ins?.prevUniques ?? 0) : (ins?.prevTotalAmount ?? 0)
  const money = sum && (d.sumKey ?? '').endsWith('_cents')
  const fmt = (n: number) => (money ? formatCents(n, ins?.baseCurrency ?? 'USD') : formatNumber(n))
  const measureLabel = sum ? (isNetSales(d) ? 'Net sales' : `Sum of ${d.sumKey}`) : unique ? 'Unique people' : 'Total events'
  const delta = prev > 0 ? pct(cur - prev, prev) : null
  const seriesKeys = useMemo(() => (ins?.series ?? []).map((s) => s.key), [ins])

  // Pivot (bucket, key, value) points into recharts rows: { bucket, <key>: value, … }, missing series as 0 so lines connect
  const chartData = useMemo(() => {
    const byBucket = new Map<string, Record<string, number | string>>()
    for (const p of ins?.points ?? []) {
      const label = formatShortDay(p.bucket)
      const row = byBucket.get(label) ?? { bucket: label }
      row[p.key] = p.amount
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
                  <span className="tabular-nums">{fmt(s.amount)}</span>
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
  const seriesTotal = (ins?.series ?? []).reduce((s, x) => s + x.amount, 0) || 1
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
                    const share = Math.round((s.amount / seriesTotal) * 100)
                    return (
                      <tr key={s.key} className="border-b last:border-0">
                        <td className="px-3 py-1.5">
                          <span className="mr-2 inline-block size-2 rounded-full align-middle" style={{ background: COLORS[i % COLORS.length] }} />
                          {s.key}
                        </td>
                        <td className="px-3 py-1.5 text-right tabular-nums">{fmt(s.amount)}</td>
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

export const insights: Kind = {
  page: '/synapse/insights',
  label: 'Insights',
  menu: { label: 'Insights', hint: 'Count any event over time' },
  describe: (d) => {
    const events = eventsOf(d).join(', ') || 'any event'
    const what = isNetSales(d) ? 'Net sales' : d.measure === 'sum' ? `Sum of ${d.sumKey} on ${events}` : `${d.measure === 'unique' ? 'People doing' : 'Count of'} ${events}`
    const where = describeConditions(d.where)
    return `${what}${where ? ` where ${where}` : ''}${d.breakdown ? ` by ${d.breakdown}` : ''}`
  },
  title: (d) => (isNetSales(d) ? 'Net sales' : d.event ? eventLabel(d.event) : 'All events'),
  start: { kind: 'INSIGHTS', event: 'page_view', ...DEFAULTS },
  Controls: InsightsControls,
  Result: InsightsResult,
}
