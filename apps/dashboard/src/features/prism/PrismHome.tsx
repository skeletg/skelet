import { useMemo, type ReactNode } from 'react'
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
import { cn } from '@/lib/utils'
import { ExperimentsOverviewDocument } from './Prism.gql'

const nfmt = (n: number) => n.toLocaleString()
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
        cell: ({ row }) => <span className="tabular-nums">{nfmt(row.original.totalExposed)}</span>,
      },
      {
        accessorKey: 'conversions',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Conversions" />,
        cell: ({ row }) => <span className="tabular-nums">{nfmt(row.original.conversions)}</span>,
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
                  <span className={r.leaderUplift > 0 ? 'text-emerald-600' : 'text-red-600'}>
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
            <span className={cn('tabular-nums', strong ? 'font-medium text-emerald-600' : 'text-muted-foreground')}>
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
          if (r.winner) return <span className="font-medium text-emerald-600">🏆 {r.winner}</span>
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
                <Tile label="Running" value={nfmt(ov?.running ?? 0)} sub="live experiments" />
                <Tile label="Visitors exposed" value={nfmt(ov?.totalExposed ?? 0)} sub="across running, 30d" />
                <Tile label="Significant winners" value={nfmt(ov?.decided ?? 0)} sub="ready to ship" />
                <Tile label="Drafts" value={nfmt(ov?.draft ?? 0)} sub="not yet live" />
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
                          <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={44} tickFormatter={(v) => nfmt(v as number)} />
                          <Tooltip content={<ExposedTooltip />} cursor={{ fill: 'transparent' }} />
                          <Bar dataKey="exposed" radius={[4, 4, 0, 0]} maxBarSize={64}>
                            {chartRows.map((d, i) => (
                              <Cell key={i} fill={d.isWinner ? '#10b981' : '#6366f1'} />
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
                              <Cell key={i} fill={d.isWinner ? '#10b981' : '#94a3b8'} />
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
                <Button asChild variant="outline" size="sm">
                  <Link to="/prism/new">New experiment</Link>
                </Button>
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
                    No running experiments.{' '}
                    <Link
                      to={(ov?.draft ?? 0) > 0 ? '/prism/experiments' : '/prism/new'}
                      className="text-primary hover:underline"
                    >
                      {(ov?.draft ?? 0) > 0 ? 'Manage drafts' : 'Create one'}
                    </Link>
                    .
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

function Tile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
      <div className="p-3">{children}</div>
    </div>
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
      <div className="tabular-nums">{nfmt(d.exposed)} exposed</div>
      <div className="tabular-nums text-muted-foreground">{nfmt(d.conversions)} conversions</div>
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
