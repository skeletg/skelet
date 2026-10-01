import { type ReactNode, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import {
  ActivityIcon,
  FilterIcon,
  LayersIcon,
  LineChartIcon,
  RepeatIcon,
  WaypointsIcon,
} from 'lucide-react'
import { Area, AreaChart, Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { CHART_ACCENT } from '@/lib/chart-colors'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { customerLabel, useCustomerNames } from '@/shared/signals/useCustomerNames'
import { eventLabel } from './eventLabel'
import { SynapseOverviewDocument, SynapseStickinessDocument } from './Synapse.gql'
import { Empty, Panel, Tile } from '@/components/layout/panels'
import { formatCents, formatDate, formatNumber, formatShortDay } from '@/lib/format'

// The Synapse tools, described so the hub explains what each one is
const REPORTS: { to: string; icon: ReactNode; title: string; desc: string }[] = [
  { to: '/synapse/insights', icon: <LineChartIcon className="size-5" />, title: 'Insights', desc: 'Measure any event over time, broken down by any property.' },
  { to: '/synapse/funnels', icon: <FilterIcon className="size-5" />, title: 'Funnels', desc: 'Step-by-step conversion and where people drop off.' },
  { to: '/synapse/retention', icon: <RepeatIcon className="size-5" />, title: 'Retention', desc: 'Do they come back? Cohort retention curves over time.' },
  { to: '/synapse/flows', icon: <WaypointsIcon className="size-5" />, title: 'Flows', desc: 'The most common paths people take after an event.' },
  { to: '/synapse/stickiness', icon: <ActivityIcon className="size-5" />, title: 'Engagement', desc: 'Return rate - how many people come back, on your store’s cadence.' },
  { to: '/synapse/segments', icon: <LayersIcon className="size-5" />, title: 'Segments', desc: 'Build behavioural cohorts; target them in campaigns & flows.' },
]

export function SynapseHome() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const skip = !effectiveChannelId

  const { data, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip, fetchPolicy: 'cache-and-network' })
  const { data: stick, error: stickError } = useQuery(SynapseStickinessDocument, { variables: { channelId, event: '', sinceDays: 30 }, skip, fetchPolicy: 'cache-and-network' })
  const error = ovError ?? stickError
  const ov = data?.synapseOverview
  const revenue = (ov?.revenue ?? []).map((r) => formatCents(r.cents, r.currency)).join(' · ')

  const activity = (stick?.synapseStickiness.days ?? []).map((d) => ({ day: formatShortDay(d.day), count: d.count }))
  const eventMix = (ov?.topEvents ?? []).slice(0, 10).map((e) => ({ label: eventLabel(e.name), count: e.count }))
  const audience = (ov?.identifiedCustomers ?? 0) + (ov?.anonymousVisitors ?? 0)
  const maxCust = Math.max(1, ...(ov?.activeCustomers ?? []).map((c) => c.events))
  const activeCustomerIDs = useMemo(() => (ov?.activeCustomers ?? []).map((c) => c.customerId), [ov])
  const nameById = useCustomerNames(activeCustomerIDs)
  const maxVis = Math.max(1, ...(ov?.activeVisitors ?? []).map((v) => v.events))

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Synapse"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">Customer behaviour & profiles - every signal stitched to a person. Figures below cover the last 30 days.</span>}
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load overview</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        <DetailPageSection title="Explore" description="Pick a report. Each works per channel and resolves people across their anonymous + signed-in activity.">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {REPORTS.map((r) => (
              <Link key={r.to} to={r.to} className="group flex items-start gap-3 rounded-lg border p-4 transition hover:border-primary/40 hover:bg-muted/40">
                <span className="grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">{r.icon}</span>
                <span className="min-w-0">
                  <span className="block font-medium group-hover:text-primary">{r.title}</span>
                  <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{r.desc}</span>
                </span>
              </Link>
            ))}
          </div>
        </DetailPageSection>

        {!isChannel ? (
          <DetailPageSection title="Overview">
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>The overview below is per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : ov ? (
          <>
            <DetailPageSection title="At a glance">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
                <Tile label="Identified customers" value={formatNumber(ov.identifiedCustomers)} sub="stitched to a profile" />
                <Tile label="Anonymous visitors" value={formatNumber(ov.anonymousVisitors)} sub="not signed in yet" />
                <Tile label="Total events" value={formatNumber(ov.totalEvents)} sub="web + email + commerce" />
                <Tile label="Page views" value={formatNumber(ov.pageViews)} />
                <Tile label="Conversions" value={formatNumber(ov.conversions)} sub={revenue || 'no attributed revenue'} />
              </div>
              {audience > 0 && (
                <div className="mt-3 rounded border p-3">
                  <div className="mb-1.5 flex items-center justify-between text-xs text-muted-foreground">
                    <span>Audience - identified vs anonymous</span>
                    <span>{Math.round(((ov.identifiedCustomers ?? 0) / audience) * 100)}% identified</span>
                  </div>
                  <div className="flex h-2.5 overflow-hidden rounded-full bg-muted">
                    <div className="bg-primary" style={{ width: `${((ov.identifiedCustomers ?? 0) / audience) * 100}%` }} title={`Identified: ${formatNumber(ov.identifiedCustomers)}`} />
                    <div className="bg-warning" style={{ width: `${((ov.anonymousVisitors ?? 0) / audience) * 100}%` }} title={`Anonymous: ${formatNumber(ov.anonymousVisitors)}`} />
                  </div>
                </div>
              )}
            </DetailPageSection>

            <DetailPageSection title="Activity" description="Daily active people and the mix of what they're doing - this channel, last 30 days.">
              <div className="grid gap-4 lg:grid-cols-2">
                <Panel title="Daily active people">
                  {activity.length === 0 ? (
                    <Empty>No activity yet.</Empty>
                  ) : (
                    <div className="h-56">
                      <ResponsiveContainer width="100%" height="100%">
                        <AreaChart data={activity} margin={{ top: 8, right: 12, bottom: 0, left: -12 }}>
                          <defs>
                            <linearGradient id="syn-act" x1="0" y1="0" x2="0" y2="1">
                              <stop offset="0%" stopColor={CHART_ACCENT} stopOpacity={0.35} />
                              <stop offset="100%" stopColor={CHART_ACCENT} stopOpacity={0} />
                            </linearGradient>
                          </defs>
                          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                          <XAxis dataKey="day" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
                          <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={32} />
                          <Tooltip content={<ChartTooltipContent />} />
                          <Area type="monotone" dataKey="count" name="Active people" stroke={CHART_ACCENT} strokeWidth={2} fill="url(#syn-act)" />
                        </AreaChart>
                      </ResponsiveContainer>
                    </div>
                  )}
                </Panel>
                <Panel title="Event mix">
                  {eventMix.length === 0 ? (
                    <Empty>No events yet.</Empty>
                  ) : (
                    <div style={{ height: Math.max(160, eventMix.length * 28) }}>
                      <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={eventMix} layout="vertical" margin={{ top: 0, right: 16, bottom: 0, left: 8 }}>
                          <XAxis type="number" tick={{ fontSize: 11 }} allowDecimals={false} />
                          <YAxis type="category" dataKey="label" width={120} tick={{ fontSize: 11 }} />
                          <Tooltip content={<ChartTooltipContent />} />
                          <Bar dataKey="count" name="Events" fill={CHART_ACCENT} radius={[0, 4, 4, 0]} barSize={14} />
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                  )}
                </Panel>
              </div>
            </DetailPageSection>

            <DetailPageSection title="Most active customers" description="Ranked by total activity (web visits via stitched identity + email engagement). Open one for their full profile.">
              <ActorTable
                rows={ov.activeCustomers.map((c) => ({
                  key: c.customerId,
                  to: `/synapse/customers/${c.customerId}`,
                  title: customerLabel(nameById.get(c.customerId), c.customerId),
                  sub: nameById.get(c.customerId)?.email ?? '',
                  events: c.events,
                  lastSeen: c.lastSeen,
                  max: maxCust,
                }))}
                head="Customer"
                empty="No customer activity yet."
              />
            </DetailPageSection>

            <DetailPageSection title="Most active visitors" description="Not-yet-identified visitors (by browser id). Their history folds into a customer profile automatically once they sign in.">
              <ActorTable
                rows={ov.activeVisitors.map((v) => ({
                  key: v.anonId,
                  to: `/synapse/visitors/${encodeURIComponent(v.anonId)}`,
                  title: `Visitor ${v.anonId.slice(0, 8)}`,
                  sub: v.lastPage || '',
                  events: v.events,
                  lastSeen: v.lastSeen,
                  max: maxVis,
                }))}
                head="Visitor"
                empty="No anonymous activity yet."
              />
            </DetailPageSection>
          </>
        ) : null}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type ActorRow = { key: string; to: string; title: string; sub: string; events: number; lastSeen?: string | null; max: number }

// ActorTable: a ranked activity table with an inline relative-activity bar
function ActorTable({ rows, head, empty }: { rows: ActorRow[]; head: string; empty: string }) {
  if (rows.length === 0) return <Empty>{empty}</Empty>
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
            <th className="w-8 px-3 py-2 text-right">#</th>
            <th className="px-3 py-2">{head}</th>
            <th className="px-3 py-2">Activity</th>
            <th className="px-3 py-2">Last seen</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.key} className="border-b last:border-0 hover:bg-muted/40">
              <td className="px-3 py-2 text-right text-xs tabular-nums text-muted-foreground">{i + 1}</td>
              <td className="px-3 py-2">
                <Link to={r.to} className="hover:underline">
                  <div className="font-medium">{r.title}</div>
                  {r.sub ? <div className="truncate text-xs text-muted-foreground">{r.sub}</div> : null}
                </Link>
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-2">
                  <span className="h-1.5 w-24 overflow-hidden rounded-full bg-muted">
                    <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.round((r.events / r.max) * 100)}%` }} />
                  </span>
                  <span className="tabular-nums text-muted-foreground">{formatNumber(r.events)}</span>
                </div>
              </td>
              <td className="px-3 py-2 text-muted-foreground">{formatDate(r.lastSeen)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
