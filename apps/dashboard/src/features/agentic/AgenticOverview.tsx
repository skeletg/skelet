import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChevronDownIcon, ChevronRightIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { Empty, Panel, PickStore } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { LoadingState } from '@/components/ui/loading-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CHART_ACCENT } from '@/lib/chart-colors'
import { useConfigScope } from '@/lib/config-scope'
import type { AgenticState } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import { AgenticReportDocument, AgenticStatusDocument, type AgenticReportQuery, type AgenticStatusQuery } from './Agentic.gql'
import { AssistantTiles } from './AssistantTiles'
import { AuroraStage, Presence, Streamed } from './aurora'
import { useAssistantName, useStreamed } from './useAssistant'

type Day = AgenticReportQuery['assistantReport']['daily'][number]
type Status = AgenticStatusQuery['agenticStatus'][number]

const WINDOWS = ['7', '30', '90'] as const

// The report's days are UTC dates, so they're named in UTC - the local zone would put some on the day before
const dayName = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })
const dayLabel = (day: string) => dayName.format(new Date(`${day}T00:00:00Z`))

export function AgenticOverview() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const [days, setDays] = useState<(typeof WINDOWS)[number]>('30')
  const channelId = effectiveChannelId ?? ''
  const { canAccessChannelScope } = usePermissions()
  const { enabled } = useFeatures()
  const canSetUp = !!channelId && canAccessChannelScope('config.view', channelId)
  const report = useQuery(AgenticReportDocument, { variables: { channelId, days: Number(days) }, skip: !channelId, fetchPolicy: 'cache-and-network' })
  const status = useQuery(AgenticStatusDocument, { variables: { channelId }, skip: !canSetUp, fetchPolicy: 'cache-and-network' })
  const r = report.data?.assistantReport
  const rows = status.data?.agenticStatus
  // Conversations are there to open only while the store keeps them - unknown without settings access, so the link stays
  const kept = canSetUp ? rows?.some((s) => s.key === 'transcripts' && s.state === 'ON') : true
  const off = rows ? rows.some((s) => s.key === 'assistant' && s.state !== 'ON') : undefined
  const opens = kept && enabled('signals') && !!channelId && canAccessChannelScope('assistant.view', channelId)
  const opensSearch = !!channelId && canAccessChannelScope('catalog.view', channelId)
  const opensInsights = enabled('signals') && !!channelId && canAccessChannelScope('assistant.view', channelId)
  const name = useAssistantName(effectiveChannelId)

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Agentic Commerce"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">How the shopping assistant and shoppers' own AI agents are doing on this store</span>}
        actions={
          <Select value={days} onValueChange={(v) => setDays(v as (typeof WINDOWS)[number])}>
            <SelectTrigger className="h-8 w-36" aria-label="Time range">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {WINDOWS.map((w) => (
                <SelectItem key={w} value={w}>
                  Last {w} days
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <PickStore why="The assistant runs per store" />
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection>
              <OverviewStage report={report.error ? undefined : r} failed={!!report.error} days={Number(days)} name={name} off={off} />
            </DetailPageSection>
            <DetailPageSection title="How it's doing" description={`What the assistant answered and sold, and what shoppers' agents ordered, over the last ${days} days.`}>
              {report.error ? (
                <Alert variant="destructive">
                  <AlertTitle>Couldn't load the numbers</AlertTitle>
                  <AlertDescription>{report.error.message}</AlertDescription>
                </Alert>
              ) : !r ? (
                <LoadingState />
              ) : (
                <div className="space-y-4">
                  <AssistantTiles report={r} unanswered={opens ? '/conversations?unanswered=any' : undefined} />
                  {r.searchOverviews > 0 ? (
                    <p className="text-sm text-muted-foreground">
                      It also wrote <span className="tabular-nums text-foreground">{r.searchOverviews.toLocaleString()}</span>{' '}
                      {opensSearch ? (
                        <Link to="/agentic/hybrid-search" className="text-foreground underline underline-offset-2">
                          {r.searchOverviews === 1 ? 'AI overview' : 'AI overviews'}
                        </Link>
                      ) : r.searchOverviews === 1 ? (
                        'AI overview'
                      ) : (
                        'AI overviews'
                      )}{' '}
                      above search results - counted in the model use, not in the messages.
                    </p>
                  ) : null}
                  <DailyChart daily={r.daily} />
                  {opensInsights ? (
                    <p className="text-sm text-muted-foreground">
                      How shoppers who chatted compare with the rest, what it recommends and shoppers' own agents are in{' '}
                      <Link to="/ai-insights" className="text-foreground underline underline-offset-2">
                        AI insights
                      </Link>
                      .
                    </p>
                  ) : null}
                </div>
              )}
            </DetailPageSection>
            {canSetUp ? (
              <DetailPageSection title="What's switched on" description={rows ? setupSummary(rows) : 'Each piece, in the order it builds up, and where it is set.'}>
                {status.error ? (
                  <Alert variant="destructive">
                    <AlertTitle>Couldn't load the setup</AlertTitle>
                    <AlertDescription>{status.error.message}</AlertDescription>
                  </Alert>
                ) : !rows ? (
                  <LoadingState />
                ) : (
                  <StatusList rows={rows} />
                )}
              </DetailPageSection>
            ) : null}
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

const counted = (n: number) => (n === 0 ? 'none' : n.toLocaleString())

// The window in one sentence, written out as the assistant would - and the orb thinking while the numbers load
function OverviewStage({ report: r, failed, days, name, off }: { report?: AgenticReportQuery['assistantReport']; failed: boolean; days: number; name: string; off?: boolean }) {
  const headline = !r
    ? ''
    : r.turns === 0
      ? `${name} hasn't answered anyone in the last ${days} days.`
      : `${name} answered ${r.turns.toLocaleString()} ${r.turns === 1 ? 'message' : 'messages'} in the last ${days} days.`
  const line = useStreamed(headline, `${days}|${headline}`)
  const assisted = r?.orders.reduce((n, o) => n + o.assistantOrders, 0) ?? 0
  const detail = !r
    ? ''
    : r.turns === 0
      ? off
        ? "It's not answering on this store yet - what it still needs is listed below."
        : off === false
          ? 'Shoppers find it in the storefront header.'
          : "Once it's on, shoppers find it in the storefront header."
      : `${counted(r.cartTurns)} filled a cart, ${counted(assisted)} led to an order, and ${counted(r.unansweredTurns)} went unanswered.`
  return (
    <AuroraStage>
      <div className="flex items-start gap-5 p-6 md:p-8">
        <Presence speaking={(!r && !failed) || line.streaming} />
        <div className="min-w-0 pt-1">
          <p className="text-balance text-2xl font-semibold tracking-tight md:text-3xl">
            {r ? <Streamed {...line} /> : failed ? "The numbers didn't load." : <span className="text-muted-foreground">Reading the numbers…</span>}
          </p>
          {detail ? <p className={`mt-2 text-muted-foreground transition-opacity duration-500 ${line.streaming ? 'opacity-0' : 'opacity-100'}`}>{detail}</p> : null}
        </div>
      </div>
    </AuroraStage>
  )
}

function DailyChart({ daily }: { daily: Day[] }) {
  const points = daily.map((d) => ({ ...d, label: dayLabel(d.day) }))
  const quiet = daily.every((d) => d.turns === 0 && d.assistantOrders === 0 && d.agentOrders === 0)
  return (
    <Panel title="Messages answered per day">
      {quiet ? (
        <Empty>No messages in this window.</Empty>
      ) : (
        <>
          <div className="h-56" role="img" aria-label={`Messages answered per day, ${daily.length} days`}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: 0 }} barCategoryGap={2}>
                <CartesianGrid vertical={false} className="stroke-muted" />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} tickLine={false} interval="preserveStartEnd" minTickGap={24} />
                <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} width={40} />
                <Tooltip cursor={{ className: 'fill-muted/60' }} content={(props) => <DayTooltip {...props} />} />
                <Bar dataKey="turns" name="Messages answered" fill={CHART_ACCENT} radius={[4, 4, 0, 0]} maxBarSize={28} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <DayTable daily={daily} />
        </>
      )}
    </Panel>
  )
}

const dayFacts: [keyof Day, string][] = [
  ['turns', 'Messages answered'],
  ['unansweredTurns', "Couldn't answer"],
  ['cartTurns', 'Filled a cart'],
  ['checkoutTurns', 'Sent to checkout'],
  ['assistantOrders', 'Assisted orders'],
  ['agentOrders', "Agents' orders"],
]

function DayTooltip({ active, payload }: { active?: boolean; payload?: ReadonlyArray<{ payload?: unknown }> }) {
  const d = active ? (payload?.[0]?.payload as (Day & { label: string }) | undefined) : undefined
  if (!d) return null
  return (
    <div className="rounded-md border bg-background px-3 py-2 text-xs shadow-md">
      <p className="mb-1 font-medium">{d.label}</p>
      <dl className="grid grid-cols-[auto_auto] gap-x-4 gap-y-0.5">
        {dayFacts.map(([key, name]) => (
          <div key={key} className="contents">
            <dt className="text-muted-foreground">{name}</dt>
            <dd className="text-right tabular-nums">{Number(d[key]).toLocaleString()}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

// The chart's numbers as a table, for anyone who can't hover or read the bars - the days with something in them, newest first
function DayTable({ daily }: { daily: Day[] }) {
  const busy = daily.filter((d) => d.turns > 0 || d.assistantOrders > 0 || d.agentOrders > 0).reverse()
  return (
    <Collapsible className="mt-3 rounded-md border">
      <CollapsibleTrigger className="group flex w-full items-center justify-between px-3 py-2 text-left text-sm">
        Day by day
        <ChevronDownIcon className="size-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent className="overflow-x-auto border-t">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-muted-foreground">
              <th className="px-3 py-2 text-left font-normal">Day</th>
              {dayFacts.map(([key, name]) => (
                <th key={key} className="px-3 py-2 text-right font-normal">
                  {name}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y">
            {busy.map((d) => (
              <tr key={d.day}>
                <td className="px-3 py-1.5">{dayLabel(d.day)}</td>
                {dayFacts.map(([key]) => (
                  <td key={key} className="px-3 py-1.5 text-right tabular-nums">
                    {Number(d[key]).toLocaleString()}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </CollapsibleContent>
    </Collapsible>
  )
}

const STATE: Record<AgenticState, { label: string; badge: 'success' | 'secondary' | 'warning' }> = {
  ON: { label: 'On', badge: 'success' },
  OFF: { label: 'Off', badge: 'secondary' },
  ATTENTION: { label: 'Needs attention', badge: 'warning' },
}

function setupSummary(rows: Status[]): string {
  const on = rows.filter((r) => r.state === 'ON').length
  const attention = rows.filter((r) => r.state === 'ATTENTION').length
  return `${on} of ${rows.length} on${attention > 0 ? `, ${attention} needing attention` : ''} - each in the order it builds up. Open one to change it.`
}

function StatusList({ rows }: { rows: Status[] }) {
  return (
    <ul className="divide-y rounded-md border">
      {rows.map((row) => {
        const s = STATE[row.state]
        return (
          <li key={row.key}>
            <Link to={row.settingsPath} className="group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/40">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">{row.label}</span>
                  <Badge variant={s.badge}>{s.label}</Badge>
                </div>
                <p className="mt-0.5 break-words text-sm text-muted-foreground">{row.detail}</p>
              </div>
              <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" aria-hidden />
            </Link>
          </li>
        )
      })}
    </ul>
  )
}
