import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { CHART_ACCENT, CHART_SUCCESS, SERIES_OTHER } from '@/lib/chart-colors'
import { cn } from '@/lib/utils'
import { ExperimentsOverviewDocument } from './Prism.gql'
import { usePermissions } from '@/shared/auth/usePermissions'
import { Panel, Tile } from '@/components/layout/panels'
import { formatNumber } from '@/lib/format'

const pct = (x: number) => `${(x * 100).toFixed(1)}%`

type SummaryRow = {
  id: string
  key: string
  name: string
  variations: number
  coverage: number
  totalExposed: number
  conversions: number
  leader: string
  leaderRate: number
  leaderUplift: number
  leaderChanceToWin: number
  leaderIsControl: boolean
  winner: string
  sampleRatioMismatch: boolean
}

export function PrismHome() {
  // Creating an experiment needs experiments.manage - without it the button only led to an access-restricted page or a refused save
  const canCreate = usePermissions().has('experiments.manage')
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { data, loading } = useQuery(ExperimentsOverviewDocument, {
    variables: { channelId: effectiveChannelId ?? '', sinceDays: 30 },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const ov = data?.experimentsOverview
  const rows = useMemo(() => (ov?.experiments ?? []) as SummaryRow[], [ov?.experiments])

  const chartRows = useMemo<ChartRow[]>(
    () =>
      [...rows]
        .sort((a, b) => b.totalExposed - a.totalExposed)
        .slice(0, 8)
        .map((r) => ({
          name: r.name || r.key,
          exposed: r.totalExposed,
          conversions: r.conversions,
          upliftPct: +(r.leaderUplift * 100).toFixed(1),
          isWinner: !!r.winner,
        })),
    [rows],
  )

  const columns = useMemo<ColumnDef<SummaryRow>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (r) => `${r.name} ${r.key}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Experiment" />,
        cell: ({ row }) => (
          <div className="grid min-w-0 gap-0.5">
            <Link to={`/prism/${row.original.id}`} className="truncate font-medium hover:underline">
              {row.original.name || row.original.key}
            </Link>
            <span className="font-mono text-[11px] text-muted-foreground">{row.original.key}</span>
          </div>
        ),
      },
      {
        accessorKey: 'totalExposed',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Exposed" />,
        cell: ({ row }) => <span className="tabular-nums">{formatNumber(row.original.totalExposed)}</span>,
      },
      {
        accessorKey: 'conversions',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Conversions" />,
        cell: ({ row }) => <span className="tabular-nums">{formatNumber(row.original.conversions)}</span>,
      },
      {
        id: 'leader',
        accessorFn: (r) => r.leaderRate,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Leader" />,
        cell: ({ row }) => {
          const r = row.original
          if (r.totalExposed === 0 || !r.leader) return <span className="text-muted-foreground">-</span>
          return (
            <div className="grid gap-0.5">
              <span className="font-medium">
                {r.leader}
                {r.leaderIsControl ? (
                  <span className="ml-1 text-xs font-normal text-muted-foreground">control</span>
                ) : null}
              </span>
              <span className="tabular-nums text-xs text-muted-foreground">
                {pct(r.leaderRate)}
                {!r.leaderIsControl && r.leaderUplift !== 0 ? (
                  <span className={r.leaderUplift > 0 ? 'text-success' : 'text-destructive'}>
                    {' '}
                    ({r.leaderUplift > 0 ? '+' : ''}
                    {pct(r.leaderUplift)})
                  </span>
                ) : null}
              </span>
            </div>
          )
        },
      },
      {
        id: 'chance',
        accessorFn: (r) => r.leaderChanceToWin,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Chance to win" />,
        cell: ({ row }) => {
          const r = row.original
          if (r.leaderIsControl || r.totalExposed === 0) return <span className="text-muted-foreground">-</span>
          const strong = r.leaderChanceToWin >= 0.975
          return (
            <span className={cn('tabular-nums', strong ? 'font-medium text-success' : 'text-muted-foreground')}>
              {pct(r.leaderChanceToWin)}
            </span>
          )
        },
      },
      {
        id: 'verdict',
        enableSorting: false,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Verdict" />,
        cell: ({ row }) => {
          const r = row.original
          if (r.sampleRatioMismatch) return <Badge variant="destructive">Check setup</Badge>
          if (r.winner) return <span className="font-medium text-success">🏆 {r.winner}</span>
          if (r.totalExposed === 0) return <span className="text-muted-foreground">No data</span>
          return <span className="text-muted-foreground">No winner yet</span>
        },
      },
      {
        accessorKey: 'coverage',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Coverage" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.coverage}%</span>,
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Prism"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Frontend A/B testing - split storefront traffic, render the assigned variation, and measure what converts.
            Last 30 days.
          </span>
        }
        actions={
          isChannel ? (
            <Button asChild variant="outline" size="sm">
              <Link to="/prism/experiments">All experiments</Link>
            </Button>
          ) : undefined
        }
      />
      <DetailPageBody>
        {!isChannel && (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Experiments are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        )}

        {isChannel && (
          <>
            <DetailPageSection title="At a glance">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Tile label="Running" value={formatNumber(ov?.running ?? 0)} sub="live experiments" />
                <Tile label="Visitors exposed" value={formatNumber(ov?.totalExposed ?? 0)} sub="across running, 30d" />
                <Tile label="Significant winners" value={formatNumber(ov?.decided ?? 0)} sub="ready to ship" />
                <Tile label="Drafts" value={formatNumber(ov?.draft ?? 0)} sub="not yet live" />
              </div>
            </DetailPageSection>

            {rows.length > 0 && (
              <DetailPageSection
                title="Traffic & impact"
                description="Visitors entered into each running experiment, and how far the leading variation is beating its control."
              >
                <div className="grid gap-4 lg:grid-cols-2">
                  <Panel title="Visitors exposed by experiment">
                    <div className="h-56">
                      <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={chartRows} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                          <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-muted" />
                          <XAxis dataKey="name" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={truncate} />
                          <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={44} tickFormatter={(v) => formatNumber(v as number)} />
                          <Tooltip content={<ExposedTooltip />} cursor={{ fill: 'transparent' }} />
                          <Bar dataKey="exposed" radius={[4, 4, 0, 0]} maxBarSize={64}>
                            {chartRows.map((d, i) => (
                              <Cell key={i} fill={d.isWinner ? CHART_SUCCESS : CHART_ACCENT} />
                            ))}
                          </Bar>
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                  </Panel>
                  <Panel title="Leader uplift vs control">
                    <div className="h-56">
                      <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={chartRows} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                          <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-muted" />
                          <XAxis dataKey="name" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={truncate} />
                          <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={44} tickFormatter={(v) => `${v}%`} />
                          <Tooltip content={<UpliftTooltip />} cursor={{ fill: 'transparent' }} />
                          <Bar dataKey="upliftPct" radius={[4, 4, 0, 0]} maxBarSize={64}>
                            {chartRows.map((d, i) => (
                              <Cell key={i} fill={d.isWinner ? CHART_SUCCESS : SERIES_OTHER} />
                            ))}
                          </Bar>
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                  </Panel>
                </div>
              </DetailPageSection>
            )}

            <DetailPageSection
              title="Running experiments"
              description="Verdicts use the same stats as the detail page - a winner is only called at ≥97.5% chance to beat the control."
              trailing={
                canCreate && (
                  <Button asChild variant="outline" size="sm">
                    <Link to="/prism/new">New experiment</Link>
                  </Button>
                )
              }
            >
              <DataTable
                urlState={urlState}
                columns={columns}
                data={rows}
                loading={loading && !ov}
                getRowId={(r) => r.id}
                searchPlaceholder="Search experiments…"
                initialSorting={[{ id: 'totalExposed', desc: true }]}
                emptyState={
                  <span>
                    No running experiments.
                    {((ov?.draft ?? 0) > 0 || canCreate) && (
                      <>
                        {' '}
                        <Link
                          to={(ov?.draft ?? 0) > 0 ? '/prism/experiments' : '/prism/new'}
                          className="text-primary hover:underline"
                        >
                          {(ov?.draft ?? 0) > 0 ? 'Manage drafts' : 'Create one'}
                        </Link>
                        .
                      </>
                    )}
                  </span>
                }
              />
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type ChartRow = { name: string; exposed: number; conversions: number; upliftPct: number; isWinner: boolean }

const truncate = (v: string) => (v.length > 14 ? `${v.slice(0, 13)}…` : v)

function ExposedTooltip({ active, payload }: { active?: boolean; payload?: Array<{ payload: ChartRow }> }) {
  if (!active || !payload?.length) return null
  const d = payload[0].payload
  return (
    <div className="rounded border bg-background px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium">{d.name}</div>
      <div className="tabular-nums">{formatNumber(d.exposed)} exposed</div>
      <div className="tabular-nums text-muted-foreground">{formatNumber(d.conversions)} conversions</div>
    </div>
  )
}

function UpliftTooltip({ active, payload }: { active?: boolean; payload?: Array<{ payload: ChartRow }> }) {
  if (!active || !payload?.length) return null
  const d = payload[0].payload
  return (
    <div className="rounded border bg-background px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium">{d.name}</div>
      <div className="tabular-nums">
        {d.upliftPct > 0 ? '+' : ''}
        {d.upliftPct}% uplift{d.isWinner ? ' · winner' : ''}
      </div>
    </div>
  )
}
