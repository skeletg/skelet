import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { eventLabel } from './eventLabel'
import { SynapseInsightsDocument, SynapseOverviewDocument } from './Synapse.gql'

type Measure = 'total' | 'unique'
type Unit = 'day' | 'week'

// Breakdown properties we expose (stored on the spine's event props). '' = none
const BREAKDOWNS: { value: string; label: string }[] = [
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
const COLORS = ['#0ea5e9', '#10b981', '#8b5cf6', '#f59e0b', '#ef4444', '#14b8a6', '#ec4899', '#64748b', '#a3a3a3']
const nfmt = (n: number) => n.toLocaleString()
function fmtBucket(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function SynapseInsights() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [event, setEvent] = useState('')
  const [breakdown, setBreakdown] = useState('')
  const [measure, setMeasure] = useState<Measure>('total')
  const [unit, setUnit] = useState<Unit>('day')

  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)

  const { data, loading, error: insError } = useQuery(SynapseInsightsDocument, {
    variables: { channelId, event, breakdown, measure, unit, sinceDays: 30 },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const ins = data?.synapseInsights
  const error = ovError ?? insError
  const seriesKeys = (ins?.series ?? []).map((s) => s.key)

  // Pivot (bucket, key, value) points into recharts rows: { bucket, <key>: value, … }
  const chartData = useMemo(() => {
    const byBucket = new Map<string, Record<string, number | string>>()
    for (const p of ins?.points ?? []) {
      const label = fmtBucket(p.bucket)
      const row = byBucket.get(label) ?? { bucket: label }
      row[p.key] = p.value
      byBucket.set(label, row)
    }
    // fill missing series with 0 so lines connect
    const rows = [...byBucket.values()]
    for (const row of rows) for (const k of seriesKeys) if (row[k] === undefined) row[k] = 0
    return rows
  }, [ins, seriesKeys])

  const seriesLabel = (k: string) => (breakdown ? k : measure === 'unique' ? 'Unique users' : 'Total events')

  // Headline measures + period-over-period delta (exact window figures from the
  // backend; the prior equal-length window drives the delta)
  const measureLabel = measure === 'unique' ? 'Unique people' : 'Total events'
  const unitWord = unit === 'day' ? 'day' : 'week'
  const cur = measure === 'unique' ? (ins?.uniques ?? 0) : (ins?.total ?? 0)
  const prev = measure === 'unique' ? (ins?.prevUniques ?? 0) : (ins?.prevTotal ?? 0)
  const delta = prev > 0 ? Math.round(((cur - prev) / prev) * 100) : null
  const bucketTotals = chartData.map((r) => seriesKeys.reduce((s, k) => s + (Number(r[k]) || 0), 0))
  const avgPerUnit = chartData.length ? Math.round(bucketTotals.reduce((a, b) => a + b, 0) / chartData.length) : 0
  const peakIdx = bucketTotals.reduce((bi, v, i, arr) => (v > arr[bi] ? i : bi), 0)
  const peakVal = bucketTotals.length ? bucketTotals[peakIdx] : 0
  const peakLabel = chartData[peakIdx]?.bucket as string | undefined
  const seriesTotal = (ins?.series ?? []).reduce((s, x) => s + x.total, 0) || 1

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Insights"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Ad-hoc breakdown - measure any event over time, split by a property. People resolved across anonymous +
            signed-in activity. Last 30 days.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load insights</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Insights are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="Query">
              <div className="flex flex-wrap items-end gap-3 text-sm">
                <Field label="Event">
                  <select value={event} onChange={(e) => setEvent(e.target.value)} className="rounded border bg-background px-2 py-1.5 text-sm">
                    <option value="">Any event</option>
                    {events.map((e) => (
                      <option key={e} value={e}>
                        {eventLabel(e)}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Measure">
                  <Toggle options={[{ v: 'total', l: 'Total' }, { v: 'unique', l: 'Unique' }]} value={measure} onChange={setMeasure} />
                </Field>
                <Field label="Break down by">
                  <select value={breakdown} onChange={(e) => setBreakdown(e.target.value)} className="rounded border bg-background px-2 py-1.5 text-sm">
                    {BREAKDOWNS.map((b) => (
                      <option key={b.value} value={b.value}>
                        {b.label}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="By">
                  <Toggle options={[{ v: 'day', l: 'Day' }, { v: 'week', l: 'Week' }]} value={unit} onChange={setUnit} />
                </Field>
              </div>
            </DetailPageSection>

            <DetailPageSection title="Trend">
              {!ins ? (
                <p className="py-6 text-sm text-muted-foreground">{loading ? 'Computing…' : 'No data.'}</p>
              ) : chartData.length === 0 ? (
                <p className="py-6 text-sm text-muted-foreground">No events match in this window.</p>
              ) : (
                <>
                  <div className="mb-4 grid gap-3 sm:grid-cols-3">
                    <div className="rounded border p-4">
                      <p className="text-xs uppercase tracking-wider text-muted-foreground">{measureLabel}</p>
                      <div className="mt-1 flex items-baseline gap-2">
                        <span className="text-2xl font-semibold tabular-nums">{nfmt(cur)}</span>
                        {delta !== null ? (
                          <span className={cn('text-xs font-medium tabular-nums', delta > 0 ? 'text-emerald-600' : delta < 0 ? 'text-red-600' : 'text-muted-foreground')}>
                            {delta > 0 ? '▲' : delta < 0 ? '▼' : ''} {Math.abs(delta)}%
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">{delta !== null ? `vs ${nfmt(prev)} prior period` : 'no prior-period data'}</p>
                    </div>
                    <div className="rounded border p-4">
                      <p className="text-xs uppercase tracking-wider text-muted-foreground">Avg per {unitWord}</p>
                      <p className="mt-1 text-2xl font-semibold tabular-nums">{nfmt(avgPerUnit)}</p>
                      <p className="mt-0.5 text-xs text-muted-foreground">across {chartData.length} {unitWord}s</p>
                    </div>
                    <div className="rounded border p-4">
                      <p className="text-xs uppercase tracking-wider text-muted-foreground">Peak {unitWord}</p>
                      <p className="mt-1 text-2xl font-semibold tabular-nums">{nfmt(peakVal)}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{peakLabel ?? '-'}</p>
                    </div>
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
                    <div className="mt-3 overflow-x-auto rounded border">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                            <th className="px-3 py-2">{BREAKDOWNS.find((b) => b.value === breakdown)?.label ?? breakdown}</th>
                            <th className="px-3 py-2 text-right">{measure === 'unique' ? 'Unique users' : 'Total events'}</th>
                            <th className="w-2/5 px-3 py-2">Share</th>
                          </tr>
                        </thead>
                        <tbody>
                          {ins.series.map((s, i) => {
                            const share = Math.round((s.total / seriesTotal) * 100)
                            return (
                              <tr key={s.key} className="border-b last:border-0">
                                <td className="px-3 py-1.5">
                                  <span className="mr-2 inline-block size-2 rounded-full align-middle" style={{ background: COLORS[i % COLORS.length] }} />
                                  {s.key}
                                </td>
                                <td className="px-3 py-1.5 text-right tabular-nums">{nfmt(s.total)}</td>
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
                  {measure === 'unique' && breakdown && (
                    <p className="mt-2 text-xs text-muted-foreground">
                      Unique counts are exact per point; totals across series/time may slightly over-count a person
                      seen in multiple buckets or values.
                    </p>
                  )}
                </>
              )}
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs uppercase tracking-wider text-muted-foreground">{label}</span>
      {children}
    </label>
  )
}

function Toggle<T extends string>({ options, value, onChange }: { options: { v: T; l: string }[]; value: T; onChange: (v: T) => void }) {
  return (
    <div className="inline-flex rounded-md border p-0.5">
      {options.map((o) => (
        <button
          key={o.v}
          type="button"
          onClick={() => onChange(o.v)}
          className={cn('rounded px-2.5 py-1 text-xs', value === o.v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
        >
          {o.l}
        </button>
      ))}
    </div>
  )
}
