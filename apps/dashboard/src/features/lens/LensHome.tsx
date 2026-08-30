import { useMemo, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { BotIcon, BugIcon, FlameIcon, GaugeIcon, SearchIcon, VideoIcon, type LucideIcon } from 'lucide-react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { type LensOverviewQuery, LensFrustrationDocument, LensOverviewDocument, LensVitalsSummaryDocument } from './Lens.gql'
import { VITAL_ORDER, isVitalMetric } from './vitals'
import { VitalGauge } from './VitalGauge'

type PageRow = LensOverviewQuery['lensOverview']['topPages'][number]

function nfmt(n: number): string {
  return n.toLocaleString()
}

export function LensHome() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const skip = !effectiveChannelId

  const { data } = useQuery(LensOverviewDocument, {
    variables: { channelId, sinceDays: 30 },
    skip,
    fetchPolicy: 'cache-and-network',
  })
  const ov = data?.lensOverview

  const deviceTotal = (ov?.devices ?? []).reduce((a, d) => a + d.views, 0)
  const totalClicks = (ov?.topPages ?? []).reduce((a, p) => a + p.clicks, 0)
  // Engagement ratios derived from the page rows (scroll is view-weighted)
  const scrollViews = (ov?.topPages ?? []).reduce((a, p) => a + p.views, 0)
  const avgScroll = scrollViews > 0 ? Math.round((ov?.topPages ?? []).reduce((a, p) => a + p.avgScroll * p.views, 0) / scrollViews) : 0
  const viewsPerVisitor = ov && ov.visitors > 0 ? ov.views / ov.visitors : 0
  const maxReferrer = Math.max(1, ...(ov?.referrers ?? []).map((r) => r.views))

  const totalViews = ov?.views ?? 0
  const pageColumns = useMemo<ColumnDef<PageRow>[]>(
    () => [
      {
        accessorKey: 'pageKey',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Page" />,
        cell: ({ row }) => (
          <Link
            to={`/lens/heatmap?page=${encodeURIComponent(row.original.pageKey || '/')}&viewport=desktop`}
            className="block max-w-[340px] truncate font-mono text-sm hover:underline"
            title="View heatmap"
          >
            {row.original.pageKey || '/'}
          </Link>
        ),
      },
      {
        accessorKey: 'views',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Views" />,
        cell: ({ row }) => {
          const pct = totalViews > 0 ? Math.round((row.original.views / totalViews) * 100) : 0
          return (
            <div className="flex items-center gap-2">
              <span className="w-14 shrink-0 tabular-nums">{nfmt(row.original.views)}</span>
              <Bar value={pct} suffix={`${pct}%`} />
            </div>
          )
        },
      },
      {
        accessorKey: 'clicks',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Clicks" />,
        cell: ({ row }) => <span className="tabular-nums">{nfmt(row.original.clicks)}</span>,
      },
      {
        accessorKey: 'avgScroll',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Avg scroll" />,
        cell: ({ row }) => <Bar value={row.original.avgScroll} suffix={`${row.original.avgScroll}%`} />,
      },
    ],
    [totalViews],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Lens"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Heatmaps, scroll depth &amp; on-page interactions - see how visitors actually use your storefront. Last 30 days.
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel && (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel for web analytics</AlertTitle>
              <AlertDescription>
                Lens stats are per channel - pick one in the header scope picker.
              </AlertDescription>
            </Alert>
          </DetailPageSection>
        )}

        {isChannel && ov && (
          <>
            <DetailPageSection title="At a glance" description="Daily page views over the last 30 days.">
              <div className="rounded-lg border p-4">
                <div className="mb-1 flex flex-wrap items-baseline justify-between gap-2">
                  <span className="text-sm font-medium">Traffic</span>
                  <span className="text-xs text-muted-foreground">
                    {nfmt(ov.views)} views · {nfmt(ov.visitors)} visitors
                  </span>
                </div>
                <TrafficTrend daily={ov.daily} />
              </div>
              <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <Tile label="Page views" value={nfmt(ov.views)} sub="last 30 days" />
                <Tile label="Unique visitors" value={nfmt(ov.visitors)} sub="distinct anonymous ids" />
                <Tile label="Views / visitor" value={viewsPerVisitor.toFixed(1)} sub="browsing depth" />
                <Tile label="Clicks" value={nfmt(totalClicks)} sub="across tracked pages" />
                <Tile label="Avg scroll depth" value={`${avgScroll}%`} sub="view-weighted" />
                <Tile label="Pages" value={nfmt(ov.topPages.length)} sub="with traffic" />
              </div>
            </DetailPageSection>

            <CoreWebVitalsSection channelId={channelId} />

            <DetailPageSection title="Explore" description="Recordings, heatmaps, performance, errors and on-site search - one click away.">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <NavCard
                  to="/lens/sessions"
                  icon={VideoIcon}
                  title="Session recordings"
                  desc="Replay real visitor sessions - cursor, clicks, scrolls & navigation."
                />
                <NavCard
                  to={`/lens/heatmap?page=${encodeURIComponent(ov.topPages[0]?.pageKey || '/')}&viewport=desktop`}
                  icon={FlameIcon}
                  title="Heatmaps"
                  desc="Click, move & scroll density per page. Or pick a page below."
                />
                <NavCard
                  to="/lens/bots"
                  icon={BotIcon}
                  title="Bot & crawler traffic"
                  desc="AI/search crawlers vs. humans, broken down by agent."
                />
                <NavCard
                  to="/lens/errors"
                  icon={BugIcon}
                  title="JS errors"
                  desc="Client-side errors breaking the storefront, grouped by message."
                />
                <NavCard
                  to="/lens/performance"
                  icon={GaugeIcon}
                  title="Performance"
                  desc="Core Web Vitals (LCP / INP / CLS / TTFB) per page, p75."
                />
                <NavCard
                  to="/lens/search"
                  icon={SearchIcon}
                  title="Search analytics"
                  desc="Top search terms + zero-result searches (merchandising gaps)."
                />
              </div>
            </DetailPageSection>

            <FrustrationSection channelId={channelId} />

            <DetailPageSection
              title="Top pages"
              description="Views are last-30-days; clicks and avg scroll are cumulative. Click a page to open its heatmap."
            >
              <DataTable
                columns={pageColumns}
                data={ov.topPages}
                getRowId={(p) => p.pageKey || '/'}
                searchPlaceholder="Search pages…"
                pageSize={10}
                initialSorting={[{ id: 'views', desc: true }]}
                emptyState={<p className="py-6 text-sm text-muted-foreground">No page views yet.</p>}
              />
            </DetailPageSection>

            <DetailPageSection title="Traffic sources" description="What people browse on and where they arrive from.">
              <div className="grid gap-4 lg:grid-cols-2">
                <Panel title="Devices">
                  {ov.devices.length === 0 ? (
                    <Empty>No data yet.</Empty>
                  ) : (
                    <ul className="space-y-2">
                      {ov.devices.map((d) => {
                        const p = deviceTotal > 0 ? Math.round((d.views / deviceTotal) * 100) : 0
                        return (
                          <li key={d.label} className="flex items-center gap-3 text-sm">
                            <span className="w-20 shrink-0 capitalize text-muted-foreground">{d.label}</span>
                            <Bar value={p} />
                            <span className="w-12 shrink-0 text-right tabular-nums">{nfmt(d.views)}</span>
                            <span className="w-9 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{p}%</span>
                          </li>
                        )
                      })}
                    </ul>
                  )}
                </Panel>

                <Panel title="Top referrers">
                  {ov.referrers.length === 0 ? (
                    <Empty>No data yet.</Empty>
                  ) : (
                    <ul className="space-y-2">
                      {ov.referrers.slice(0, 8).map((r) => (
                        <li key={r.label} className="flex items-center gap-3 text-sm">
                          <span className="w-32 shrink-0 truncate text-muted-foreground" title={r.label}>{r.label}</span>
                          <Bar value={Math.round((r.views / maxReferrer) * 100)} />
                          <span className="w-12 shrink-0 text-right tabular-nums">{nfmt(r.views)}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </Panel>
              </div>
            </DetailPageSection>

            <DetailPageSection
              title="Interactions"
              description={
                <>
                  Named UI events - sidebars, modals, accordions (auto-captured from accessible toggles), plus
                  anything tagged <code className="text-xs">data-lens-event</code> or sent via{' '}
                  <code className="text-xs">window.lens.track()</code>. Counted by name, so dynamic elements register
                  even when they aren&apos;t in a snapshot.
                </>
              }
            >
              {ov.interactions.length === 0 ? (
                <Empty>
                  No interactions tracked yet. Accessible toggles (<code className="text-xs">aria-expanded</code>) are
                  captured automatically; tag elements with <code className="text-xs">data-lens-event=&quot;name&quot;</code> for custom ones.
                </Empty>
              ) : (
                <div className="overflow-x-auto rounded border">
                  <table className="w-full text-sm">
                    <tbody>
                      {ov.interactions.map((i) => (
                        <tr key={i.label} className="border-b last:border-0">
                          <td className="px-3 py-2">{i.label}</td>
                          <td className="px-3 py-2 text-right tabular-nums">{nfmt(i.views)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// CoreWebVitalsSection shows site-wide p75 Core Web Vitals as Lighthouse-style
// gauges (LCP / INP / CLS / TTFB), scored green/amber/red against Google's
// thresholds - Always rendered (with an empty state) so the panel is discoverable
// even before vitals land - Click through to /lens/performance for per-page detail
function CoreWebVitalsSection({ channelId }: { channelId: string }) {
  const { data } = useQuery(LensVitalsSummaryDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const byMetric = useMemo(() => {
    const m = new Map<string, { p75: number; samples: number }>()
    for (const r of data?.lensVitalsSummary ?? []) {
      if (isVitalMetric(r.metric)) m.set(r.metric, { p75: r.p75, samples: r.samples })
    }
    return m
  }, [data])

  return (
    <DetailPageSection
      title="Core Web Vitals"
      description={
        <>
          Real-user p75 over the last 30 days, scored against Google&apos;s thresholds. Captured on full page loads.{' '}
          <Link to="/lens/performance" className="underline hover:text-foreground">
            Per-page breakdown →
          </Link>
        </>
      }
    >
      {byMetric.size === 0 ? (
        <Empty>
          No web vitals captured yet. Vitals are recorded on full page loads - hard-refresh a storefront page to start
          collecting.
        </Empty>
      ) : (
        <div className="grid grid-cols-2 gap-4 rounded-lg border p-5 sm:grid-cols-4">
          {VITAL_ORDER.map((m) => {
            const v = byMetric.get(m)
            return v ? (
              <VitalGauge key={m} metric={m} value={v.p75} samples={v.samples} />
            ) : (
              <div key={m} className="flex flex-col items-center gap-1.5 text-center opacity-50">
                <div className="flex h-[104px] w-[104px] items-center justify-center rounded-full border-[9px] border-muted">
                  <span className="text-lg font-semibold text-muted-foreground">-</span>
                </div>
                <div>
                  <p className="text-sm font-semibold">{m}</p>
                  <p className="text-[11px] leading-tight text-muted-foreground">No samples</p>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </DetailPageSection>
  )
}

// FrustrationSection surfaces rage/dead clicks - confusing or broken UI - Renders
// nothing until there's at least one signal, so a healthy store stays uncluttered
function FrustrationSection({ channelId }: { channelId: string }) {
  const { data } = useQuery(LensFrustrationDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const f = data?.lensFrustration
  if (!f || (f.ragePages.length === 0 && f.deadPages.length === 0 && f.rageElements.length === 0)) return null
  const maxRage = Math.max(1, ...f.ragePages.map((p) => p.count))
  const maxDead = Math.max(1, ...f.deadPages.map((p) => p.count))
  const pageList = (rows: { pageKey: string; count: number }[], max: number) =>
    rows.length === 0 ? (
      <Empty>None 🎉</Empty>
    ) : (
      <ul className="space-y-2">
        {rows.slice(0, 8).map((p) => (
          <li key={p.pageKey} className="flex items-center gap-3 text-sm">
            <span className="w-32 shrink-0 truncate font-mono text-xs text-muted-foreground" title={p.pageKey}>{p.pageKey || '/'}</span>
            <Bar value={Math.round((p.count / max) * 100)} />
            <span className="w-10 shrink-0 text-right tabular-nums">{nfmt(p.count)}</span>
          </li>
        ))}
      </ul>
    )
  return (
    <DetailPageSection
      title="Frustration"
      description="Rage clicks (rapid repeated clicks) and dead clicks (clicks that did nothing) - signals of confusing or broken UI."
    >
      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Top pages - rage clicks">{pageList(f.ragePages, maxRage)}</Panel>
        <Panel title="Top pages - dead clicks">{pageList(f.deadPages, maxDead)}</Panel>
        <Panel title="Most rage-clicked elements">
          {f.rageElements.length === 0 ? (
            <Empty>None</Empty>
          ) : (
            <ul className="space-y-1.5 text-sm">
              {f.rageElements.slice(0, 8).map((el, i) => (
                <li key={i} className="flex items-center justify-between gap-2">
                  <span className="min-w-0">
                    <span className="block truncate">{el.text || el.selector}</span>
                    <span className="block truncate font-mono text-[11px] text-muted-foreground">{el.selector}</span>
                  </span>
                  <span className="shrink-0 tabular-nums">{nfmt(el.count)}</span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </DetailPageSection>
  )
}

// The current theme's primary colour. recharts resolves CSS custom properties in
// its colour attributes (see the chart.tsx note), so the trend
// recolours automatically across themes (default / rose / ocean / dark) - Note:
// reference the token directly - wrapping it (e.g. hsl(var(--primary))) breaks,
// since --primary is already a full oklch() colour, not bare HSL channels
const CHART_ACCENT = 'var(--color-primary)'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
// fmtDay turns "2026-06-26" into "Jun 26" without going through Date() (which
// would shift the day across timezones for a bare YYYY-MM-DD)
function fmtDay(d: string): string {
  const p = d.split('-')
  return p.length === 3 ? `${MONTHS[Number(p[1]) - 1]} ${Number(p[2])}` : d
}

// TrafficTooltip is a theme-aware replacement for recharts' default tooltip, which renders fixed light colours that
// wash out in dark mode - Built from the app's popover tokens so the date + count stay legible in either theme
function TrafficTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  payload?: { value?: number }[]
  label?: string | number
}) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow-md">
      <p className="font-medium text-popover-foreground">{fmtDay(String(label))}</p>
      <p className="text-muted-foreground">{Number(payload[0]?.value ?? 0).toLocaleString()} page views</p>
    </div>
  )
}

// TrafficTrend is the daily page-views area chart for the overview - the app
// primary colour with a soft gradient fill, matching the dashboard chart style
function TrafficTrend({ daily }: { daily: { date: string; views: number }[] }) {
  if (daily.length === 0) {
    return <p className="flex h-56 items-center justify-center text-sm text-muted-foreground">No page views in this window yet.</p>
  }
  return (
    <div className="h-56 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={daily} margin={{ top: 8, right: 8, bottom: 0, left: -16 }}>
          <defs>
            <linearGradient id="lensTraffic" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={CHART_ACCENT} stopOpacity={0.3} />
              <stop offset="100%" stopColor={CHART_ACCENT} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
          <XAxis dataKey="date" tick={{ fontSize: 11 }} tickFormatter={fmtDay} minTickGap={28} tickMargin={6} />
          <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={40} />
          <Tooltip content={<TrafficTooltip />} cursor={{ stroke: CHART_ACCENT, strokeOpacity: 0.25 }} />
          <Area type="monotone" dataKey="views" stroke={CHART_ACCENT} strokeWidth={2} fill="url(#lensTraffic)" />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

// NavCard is a clickable destination tile (recordings / heatmaps / bots / …) Icon-led, matching the Pulse hub cards: a
// tinted glyph square + title + description, with a quiet border/tint shift on hover
function NavCard({ to, icon: Icon, title, desc }: { to: string; icon: LucideIcon; title: string; desc: string }) {
  return (
    <Link
      to={to}
      className="group flex items-start gap-3 rounded-lg border p-4 transition-colors hover:border-primary/40 hover:bg-muted/40"
    >
      <span className="grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
        <Icon className="size-5" />
      </span>
      <span className="min-w-0">
        <span className="block font-medium group-hover:text-primary">{title}</span>
        <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{desc}</span>
      </span>
    </Link>
  )
}

function Tile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

// A bordered panel with a header strip - the flat alternative to a Card
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </div>
      <div className="p-3">{children}</div>
    </div>
  )
}

function Bar({ value, suffix }: { value: number; suffix?: string }) {
  return (
    <span className="flex flex-1 items-center gap-2">
      <span className="h-1.5 min-w-16 flex-1 overflow-hidden rounded-full bg-muted">
        <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
      </span>
      {suffix && <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{suffix}</span>}
    </span>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}
