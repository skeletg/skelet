import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { Area, AreaChart, CartesianGrid, Legend, Tooltip, XAxis, YAxis } from 'recharts'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ChartContainer } from '@/components/ui/chart'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DetailPageBody, DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { ReviewInsightsDocument } from './Reviews.gql'

type Bucket = 'day' | 'week' | 'month'

const COLORS = ['#0ea5e9', '#10b981', '#8b5cf6', '#f59e0b', '#ef4444', '#14b8a6', '#ec4899', '#6366f1']
const UNCATEGORIZED_COLOR = '#94a3b8'
const MAX_TREND_SERIES = 8

// Status display: order + color for the share bars + legend
const STATUS_META: Record<string, { label: string; color: string }> = {
  APPROVED: { label: 'Approved', color: '#10b981' },
  PENDING: { label: 'Pending', color: '#f59e0b' },
  REJECTED: { label: 'Rejected', color: '#ef4444' },
}

const nfmt = (n: number) => n.toLocaleString()

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10)
}
function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return isoDay(d)
}
function formatPeriod(iso: string, bucket: Bucket): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  if (bucket === 'month') return d.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

const UNCATEGORIZED_KEY = '__uncategorized__'
// Category slug → stable series/color key ("" = uncategorized bucket)
const keyOf = (slug: string) => slug || UNCATEGORIZED_KEY

// ReviewInsightsContent is the Insights tab body (no page chrome - the Reviews
// shell provides the header + tabs) - Date-ranged + channel-scoped issue
// analytics: volume, category mix, status split, and moderation-flag counts,
// each with a prior equal-length window for comparison
export function ReviewInsightsContent() {
  const { effectiveChannelId, selectedChannel } = useConfigScope()

  const [start, setStart] = useState(() => daysAgo(30))
  const [end, setEnd] = useState(() => isoDay(new Date()))
  const [bucket, setBucket] = useState<Bucket>('day')

  const variables = useMemo(
    () => ({
      channelId: effectiveChannelId ?? null,
      startDate: start ? new Date(`${start}T00:00:00.000Z`).toISOString() : null,
      endDate: end ? new Date(`${end}T23:59:59.999Z`).toISOString() : null,
      bucket,
    }),
    [effectiveChannelId, start, end, bucket],
  )

  const { data, loading, error } = useQuery(ReviewInsightsDocument, {
    variables,
    fetchPolicy: 'cache-and-network',
  })

  const ins = data?.reviewInsights
  const total = ins?.total ?? 0
  const prevTotal = ins?.prevTotal ?? 0
  const totalDelta = prevTotal > 0 ? Math.round(((total - prevTotal) / prevTotal) * 100) : null

  const breakdown = useMemo(() => {
    const rows = [...(ins?.byCategory ?? [])]
    rows.sort((a, b) => {
      const au = !a.slug
      const bu = !b.slug
      if (au !== bu) return au ? 1 : -1
      return b.count - a.count
    })
    return rows
  }, [ins])

  const mover = useMemo(() => {
    let best: { name: string; change: number } | null = null
    for (const r of breakdown) {
      const change = r.count - r.prevCount
      if (best === null || Math.abs(change) > Math.abs(best.change)) {
        best = { name: r.slug ? r.name : 'Uncategorized', change }
      }
    }
    return best && best.change !== 0 ? best : null
  }, [breakdown])

  const colorByKey = useMemo(() => {
    const m = new Map<string, string>()
    let ci = 0
    for (const r of breakdown) {
      const k = keyOf(r.slug)
      if (!r.slug) m.set(k, UNCATEGORIZED_COLOR)
      else m.set(k, COLORS[ci++ % COLORS.length])
    }
    return m
  }, [breakdown])

  const labelByKey = useMemo(() => {
    const m = new Map<string, string>()
    for (const r of breakdown) m.set(keyOf(r.slug), r.slug ? r.name : 'Uncategorized')
    return m
  }, [breakdown])

  // Only real categories get a trend band (the category timeseries never carries
  // the uncategorized bucket - those reviews just aren't unnested)
  const trendKeys = useMemo(
    () => breakdown.filter((r) => r.slug).slice(0, MAX_TREND_SERIES).map((r) => keyOf(r.slug)),
    [breakdown],
  )
  const hasOther = breakdown.filter((r) => r.slug).length > trendKeys.length

  // Stacked-area rows by category (a multi-category review appears in each band)
  const chartData = useMemo(() => {
    const trendSet = new Set(trendKeys)
    const byPeriod = new Map<string, Record<string, number | string>>()
    const periodOrder: string[] = []
    for (const p of ins?.timeseries ?? []) {
      const label = formatPeriod(String(p.period), bucket)
      if (!byPeriod.has(label)) {
        byPeriod.set(label, { period: label })
        periodOrder.push(label)
      }
      const row = byPeriod.get(label)!
      const k = keyOf(p.slug)
      const seriesKey = trendSet.has(k) ? (labelByKey.get(k) ?? k) : 'Other'
      row[seriesKey] = (Number(row[seriesKey]) || 0) + p.count
    }
    const allSeries = [...trendKeys.map((k) => labelByKey.get(k) ?? k), ...(hasOther ? ['Other'] : [])]
    const rows = periodOrder.map((label) => byPeriod.get(label)!)
    for (const row of rows) for (const s of allSeries) if (row[s] === undefined) row[s] = 0
    return rows
  }, [ins, trendKeys, labelByKey, bucket, hasOther])

  // True review volume per period (one row per review) - correct under multi-category
  const volumeData = useMemo(
    () => (ins?.volume ?? []).map((v) => ({ period: formatPeriod(String(v.period), bucket), count: v.count })),
    [ins, bucket],
  )

  const trendSeries = useMemo(
    () => [
      ...trendKeys.map((k) => ({ key: k, name: labelByKey.get(k) ?? k, color: colorByKey.get(k) ?? COLORS[0] })),
      ...(hasOther ? [{ key: 'Other', name: 'Other', color: UNCATEGORIZED_COLOR }] : []),
    ],
    [trendKeys, labelByKey, colorByKey, hasOther],
  )

  const avgPerBucket = volumeData.length ? Math.round(total / volumeData.length) : 0
  const maxCount = breakdown.reduce((m, r) => Math.max(m, r.count), 0)

  // Status breakdown rows (display order from STATUS_META)
  const statusRows = useMemo(() => {
    const rows = [...(ins?.byStatus ?? [])]
    const rank = (s: string) => Object.keys(STATUS_META).indexOf(s)
    rows.sort((a, b) => rank(a.status) - rank(b.status))
    return rows
  }, [ins])
  const statusTotal = statusRows.reduce((s, r) => s + r.count, 0) || 1

  const tagRows = ins?.byTag ?? []
  const maxTagCount = tagRows.reduce((m, t) => Math.max(m, t.count), 0)

  const scopeLabel = selectedChannel ? selectedChannel.name : 'all channels'

  return (
    <DetailPageBody>
      <DetailPageSection
        title="Period"
        description={`What customers are raising in ${scopeLabel}, with the prior equal-length period for comparison.`}
      >
        <div className="flex flex-wrap items-end gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="ri-start">From</Label>
            <Input id="ri-start" type="date" value={start} onChange={(e) => setStart(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="ri-end">To</Label>
            <Input id="ri-end" type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
          </div>
          <div className="flex gap-1.5">
            <Button variant="outline" size="sm" onClick={() => { setStart(daysAgo(7)); setEnd(isoDay(new Date())) }}>7d</Button>
            <Button variant="outline" size="sm" onClick={() => { setStart(daysAgo(30)); setEnd(isoDay(new Date())) }}>30d</Button>
            <Button variant="outline" size="sm" onClick={() => { setStart(daysAgo(90)); setEnd(isoDay(new Date())) }}>90d</Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                const now = new Date()
                setStart(`${now.getFullYear()}-01-01`)
                setEnd(isoDay(now))
              }}
            >
              YTD
            </Button>
          </div>
          <Tabs value={bucket} onValueChange={(v) => setBucket(v as Bucket)} className="ml-auto">
            <TabsList>
              <TabsTrigger value="day">Day</TabsTrigger>
              <TabsTrigger value="week">Week</TabsTrigger>
              <TabsTrigger value="month">Month</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </DetailPageSection>

      {error ? (
        <DetailPageSection>
          <Alert variant="destructive">
            <AlertTitle>Couldn't load insights</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </DetailPageSection>
      ) : null}

      {!ins ? (
        <DetailPageSection>
          <p className="py-6 text-sm text-muted-foreground">{loading ? 'Computing…' : 'No data.'}</p>
        </DetailPageSection>
      ) : total === 0 ? (
        <DetailPageSection>
          <p className="py-6 text-sm text-muted-foreground">No reviews submitted in this period.</p>
        </DetailPageSection>
      ) : (
        <>
          <DetailPageSection title="At a glance">
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div className="rounded border p-4">
                <p className="text-xs uppercase tracking-wider text-muted-foreground">Total reviews</p>
                <div className="mt-1 flex items-baseline gap-2">
                  <span className="text-2xl font-semibold tabular-nums">{nfmt(total)}</span>
                  {totalDelta !== null ? (
                    <span
                      className={cn(
                        'text-xs font-medium tabular-nums',
                        totalDelta > 0 ? 'text-emerald-600' : totalDelta < 0 ? 'text-red-600' : 'text-muted-foreground',
                      )}
                    >
                      {totalDelta > 0 ? '▲' : totalDelta < 0 ? '▼' : ''} {Math.abs(totalDelta)}%
                    </span>
                  ) : null}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {totalDelta !== null ? `vs ${nfmt(prevTotal)} prior period` : 'no prior-period data'}
                </p>
              </div>
              <div className="rounded border p-4">
                <p className="text-xs uppercase tracking-wider text-muted-foreground">Avg per {bucket}</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums">{nfmt(avgPerBucket)}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">across {volumeData.length} {bucket}s</p>
              </div>
              <div className="rounded border p-4">
                <p className="text-xs uppercase tracking-wider text-muted-foreground">Top category</p>
                <p className="mt-1 truncate text-2xl font-semibold">{breakdown[0] ? (breakdown[0].slug ? breakdown[0].name : 'Uncategorized') : '-'}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{breakdown[0] ? `${nfmt(breakdown[0].count)} reviews` : ''}</p>
              </div>
              <div className="rounded border p-4">
                <p className="text-xs uppercase tracking-wider text-muted-foreground">Biggest mover</p>
                <p className="mt-1 truncate text-2xl font-semibold">{mover ? mover.name : '-'}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {mover ? (
                    <span className={mover.change > 0 ? 'text-emerald-600' : 'text-red-600'}>
                      {mover.change > 0 ? '▲ +' : '▼ '}
                      {nfmt(Math.abs(mover.change))} vs prior
                    </span>
                  ) : (
                    'no change'
                  )}
                </p>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection title="Review volume" description={`Reviews submitted per ${bucket}.`}>
            {volumeData.length === 0 ? (
              <p className="py-6 text-sm text-muted-foreground">No reviews in this window.</p>
            ) : (
              <ChartContainer height={240}>
                <AreaChart data={volumeData} margin={{ top: 8, right: 16, bottom: 0, left: -8 }}>
                  <defs>
                    <linearGradient id="volFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="var(--color-chart-1)" stopOpacity={0.5} />
                      <stop offset="100%" stopColor="var(--color-chart-1)" stopOpacity={0.05} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" vertical={false} />
                  <XAxis dataKey="period" tick={{ fill: 'var(--color-muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} minTickGap={24} />
                  <YAxis tick={{ fill: 'var(--color-muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} width={36} />
                  <Tooltip content={<SimpleTooltip />} />
                  <Area type="monotone" dataKey="count" name="Reviews" stroke="var(--color-chart-1)" strokeWidth={2} fill="url(#volFill)" />
                </AreaChart>
              </ChartContainer>
            )}
          </DetailPageSection>

          <DetailPageSection
            title="By category"
            description="Share of this period's reviews, with the change vs the prior equal-length window."
          >
            <ShareTable
              rows={breakdown.map((r) => {
                const k = keyOf(r.slug)
                return {
                  key: k,
                  label: r.slug ? r.name : 'Uncategorized',
                  muted: !r.slug,
                  count: r.count,
                  change: r.count - r.prevCount,
                  color: colorByKey.get(k) ?? UNCATEGORIZED_COLOR,
                }
              })}
              total={total}
              maxCount={maxCount}
              showDelta
            />
          </DetailPageSection>

          <DetailPageSection title="By status" description="Distribution across pending / approved / rejected.">
            <div className="space-y-2">
              {statusRows.map((s) => {
                const meta = STATUS_META[s.status] ?? { label: s.status, color: UNCATEGORIZED_COLOR }
                const share = Math.round((s.count / statusTotal) * 100)
                return (
                  <div key={s.status} className="grid gap-1">
                    <div className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2">
                        <span className="inline-block size-2 rounded-full" style={{ background: meta.color }} />
                        {meta.label}
                      </span>
                      <span className="tabular-nums text-muted-foreground">{nfmt(s.count)} · {share}%</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                      <div className="h-full rounded-full" style={{ width: `${share}%`, background: meta.color }} />
                    </div>
                  </div>
                )
              })}
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Moderation flags"
            description="AI concern tags raised this period (a review may carry several), with the change vs the prior window."
          >
            {tagRows.length === 0 ? (
              <p className="text-sm text-muted-foreground">No moderation concerns flagged in this window.</p>
            ) : (
              <ShareTable
                rows={tagRows.map((t, i) => ({
                  key: t.slug,
                  label: t.name,
                  muted: false,
                  count: t.count,
                  change: t.count - t.prevCount,
                  color: COLORS[i % COLORS.length],
                }))}
                total={tagRows.reduce((s, t) => s + t.count, 0)}
                maxCount={maxTagCount}
                showDelta
                countLabel="flags"
              />
            )}
          </DetailPageSection>

          <DetailPageSection title="Categories over time" description={`Reviews per ${bucket} by category.`}>
            {chartData.length === 0 ? (
              <p className="py-6 text-sm text-muted-foreground">No reviews in this window.</p>
            ) : (
              <ChartContainer height={320}>
                <AreaChart data={chartData} margin={{ top: 8, right: 16, bottom: 0, left: -8 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" vertical={false} />
                  <XAxis dataKey="period" tick={{ fill: 'var(--color-muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} minTickGap={24} />
                  <YAxis tick={{ fill: 'var(--color-muted-foreground)', fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} width={36} />
                  <Tooltip content={<TrendTooltip />} />
                  {trendSeries.length > 1 && <Legend wrapperStyle={{ fontSize: 12 }} />}
                  {trendSeries.map((s) => (
                    <Area key={s.name} type="monotone" dataKey={s.name} stackId="reviews" stroke={s.color} fill={s.color} fillOpacity={0.25} strokeWidth={2} />
                  ))}
                </AreaChart>
              </ChartContainer>
            )}
          </DetailPageSection>
        </>
      )}
    </DetailPageBody>
  )
}

// ShareTable renders count + share bar (+ optional vs-prior delta) rows - used
// for both the category and moderation-flag breakdowns
function ShareTable({
  rows,
  total,
  maxCount,
  showDelta,
  countLabel = 'reviews',
}: {
  rows: { key: string; label: string; muted: boolean; count: number; change: number; color: string }[]
  total: number
  maxCount: number
  showDelta?: boolean
  countLabel?: string
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2 text-right">{countLabel[0].toUpperCase() + countLabel.slice(1)}</th>
            <th className="w-2/5 px-3 py-2">Share</th>
            {showDelta ? <th className="px-3 py-2 text-right">vs prior</th> : null}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const share = total > 0 ? Math.round((r.count / total) * 100) : 0
            return (
              <tr key={r.key} className="border-b last:border-0">
                <td className="px-3 py-1.5">
                  <span className="mr-2 inline-block size-2 rounded-full align-middle" style={{ background: r.color }} />
                  {r.muted ? <span className="text-muted-foreground">{r.label}</span> : r.label}
                </td>
                <td className="px-3 py-1.5 text-right tabular-nums">{nfmt(r.count)}</td>
                <td className="px-3 py-1.5">
                  <div className="flex items-center gap-2">
                    <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                      <span className="block h-full rounded-full" style={{ width: `${maxCount > 0 ? Math.max(2, (r.count / maxCount) * 100) : 0}%`, background: r.color }} />
                    </span>
                    <span className="w-9 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{share}%</span>
                  </div>
                </td>
                {showDelta ? (
                  <td className="px-3 py-1.5 text-right tabular-nums">
                    {r.change === 0 ? (
                      <span className="text-muted-foreground">-</span>
                    ) : (
                      <span className={r.change > 0 ? 'text-emerald-600' : 'text-red-600'}>
                        {r.change > 0 ? `+${nfmt(r.change)}` : nfmt(r.change)}
                      </span>
                    )}
                  </td>
                ) : null}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function SimpleTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  label?: string | number
  payload?: Array<{ value?: number }>
  // `label` prop name retained for the recharts API; `_label` placeholder unused
  _label?: string
}) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-xs shadow-md">
      <div className="font-medium">{String(label ?? '')}</div>
      <div className="mt-1 text-muted-foreground tabular-nums">{nfmt(payload[0].value ?? 0)} reviews</div>
    </div>
  )
}

// TrendTooltip lists each non-zero category band for the hovered period + total
function TrendTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  label?: string | number
  payload?: Array<{ name?: string; value?: number; color?: string }>
}) {
  if (!active || !payload?.length) return null
  const rows = payload.filter((p) => (p.value ?? 0) > 0).sort((a, b) => (b.value ?? 0) - (a.value ?? 0))
  const total = payload.reduce((s, p) => s + (p.value ?? 0), 0)
  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-xs shadow-md">
      <div className="font-medium">{String(label ?? '')}</div>
      <div className="mt-1 grid gap-0.5">
        {rows.map((p) => (
          <div key={p.name} className="flex items-center gap-2">
            <span className="inline-block size-2 rounded-full" style={{ background: p.color }} />
            <span className="flex-1 text-muted-foreground">{p.name}</span>
            <span className="tabular-nums">{p.value}</span>
          </div>
        ))}
        <div className="mt-0.5 flex items-center gap-2 border-t pt-0.5 font-medium">
          <span className="flex-1">Total</span>
          <span className="tabular-nums">{total}</span>
        </div>
      </div>
    </div>
  )
}
