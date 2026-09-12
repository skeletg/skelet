import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
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
import {
  type SynapseSegmentQuery,
  SynapseSegmentDocument,
  SynapseSaveSegmentDocument,
  SynapseOverviewDocument,
  SynapseTrackedAttributesDocument,
} from './Synapse.gql'
import { customerLabel, useCustomerNames } from '@/shared/signals/useCustomerNames'
import { SegmentTreeBuilder } from './SegmentTreeBuilder'
import { definitionToTree, nodeComplete, nodeToInput, summarizeNode, type SegNode, type SummaryNode } from './segment-tree'

const ACCENT = 'hsl(var(--primary))'
// Money values on the segment detail are in the segment channel's currency
const money = (cents: number, currency: string) => (cents / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
const fmtDay = (v: string) => {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
const fmtDate = (v?: string | null) => {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString(undefined, { dateStyle: 'medium' })
}

// Derived from the query, so a schema change breaks the build instead of reading undefined at runtime
type Member = NonNullable<SynapseSegmentQuery['synapseSegment']>['members'][number]

const LIFECYCLE_STYLE: Record<string, string> = {
  new: 'bg-sky-500/10 text-sky-600 border-sky-500/30',
  active: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30',
  at_risk: 'bg-amber-500/10 text-amber-600 border-amber-500/30',
  lapsed: 'bg-red-500/10 text-red-600 border-red-500/30',
  prospect: 'bg-muted text-muted-foreground border-border',
}
// Solid fills for the lifecycle stacked bar (keyed by stage; '' = uncomputed)
const LIFECYCLE_BAR: Record<string, string> = {
  new: 'bg-sky-500',
  active: 'bg-emerald-500',
  at_risk: 'bg-amber-500',
  lapsed: 'bg-red-500',
  prospect: 'bg-muted-foreground/40',
  '': 'bg-muted-foreground/20',
}
const RFM_COLORS: Record<string, string> = { recency: '#0ea5e9', frequency: '#10b981', monetary: '#f59e0b' }
const stageLabel = (s: string) => (s === '' ? 'uncomputed' : s.replace('_', ' '))

export function SynapseSegment() {
  const { id = '' } = useParams()
  const { data, loading, error: segError } = useQuery(SynapseSegmentDocument, { variables: { id }, skip: !id, fetchPolicy: 'cache-and-network' })
  const d = data?.synapseSegment
  const seg = d?.segment
  const currency = d?.currency ?? 'USD'

  // Editing reuses the segment builder; events + tracked attributes are loaded for
  // the segment's own channel so a save can't accidentally re-scope it
  const channelId = seg?.channelId ?? ''
  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !channelId, fetchPolicy: 'cache-first' })
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)
  const { data: attrData, error: attrError } = useQuery(SynapseTrackedAttributesDocument, { variables: { channelId }, skip: !channelId, fetchPolicy: 'cache-first' })
  const attributes = attrData?.synapseTrackedAttributes ?? []
  const error = segError ?? ovError ?? attrError
  const [editing, setEditing] = useState(false)
  const [tree, setTree] = useState<SegNode>(() => ({ kind: 'GROUP', op: 'AND', children: [] }))
  const [save, { loading: saving }] = useMutation(SynapseSaveSegmentDocument, { refetchQueries: [SynapseSegmentDocument] })
  const [saveError, setSaveError] = useState<string | null>(null)
  const startEdit = () => {
    setTree(definitionToTree((seg?.definition ?? null) as unknown as SummaryNode | null))
    setSaveError(null)
    setEditing(true)
  }
  const onSaveEdit = async () => {
    if (!seg || !nodeComplete(tree)) return
    setSaveError(null)
    try {
      await save({ variables: { channelId: seg.channelId, name: seg.name, segmentId: seg.id, root: nodeToInput(tree) } })
      setEditing(false)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e))
    }
  }

  const growth = (d?.growth ?? []).map((g) => ({ day: fmtDay(g.day as string), count: g.count }))
  const mix = d?.lifecycleMix ?? []
  const mixTotal = mix.reduce((s, m) => s + m.count, 0) || 1
  // Pivot the RFM distribution to one row per score (0–5) with R/F/M columns
  const rfmRows = useMemo(() => {
    const rows = [0, 1, 2, 3, 4, 5].map((score) => ({ score, recency: 0, frequency: 0, monetary: 0 }) as Record<string, number>)
    for (const b of d?.rfmDistribution ?? []) {
      const row = rows[b.score]
      if (row && (b.dimension in row)) row[b.dimension] = b.count
    }
    return rows
  }, [d?.rfmDistribution])
  const affMax = Math.max(1, ...(d?.topAffinities ?? []).map((a) => a.members))
  const memberIDs = useMemo(() => (d?.members ?? []).map((m) => m.customerId), [d])
  const nameById = useCustomerNames(memberIDs)

  const columns = useMemo<ColumnDef<Member>[]>(
    () => [
      {
        id: 'customer',
        accessorFn: (m) => customerLabel(nameById.get(m.customerId), m.customerId),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Customer" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <Link to={`/synapse/customers/${row.original.customerId}`} className="block truncate font-medium hover:underline" title="Open behavioural profile">
              {customerLabel(nameById.get(row.original.customerId), row.original.customerId)}
            </Link>
            <div className="truncate text-xs text-muted-foreground">{nameById.get(row.original.customerId)?.email ?? ''}</div>
          </div>
        ),
      },
      {
        accessorKey: 'lifecycle',
        header: 'Lifecycle',
        cell: ({ row }) =>
          row.original.lifecycle ? (
            <span className={`rounded-full border px-2 py-0.5 text-xs font-medium capitalize ${LIFECYCLE_STYLE[row.original.lifecycle] ?? LIFECYCLE_STYLE.prospect}`}>
              {row.original.lifecycle.replace('_', ' ')}
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        accessorKey: 'orders',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Orders" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.orders}</span>,
      },
      {
        accessorKey: 'spendCents',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Spend" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.spendCents ? money(row.original.spendCents, currency) : '-'}</span>,
      },
      {
        accessorKey: 'lastOrderAt',
        header: 'Last order',
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{fmtDate(row.original.lastOrderAt)}</span>,
      },
    ],
    [currency, nameById],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/synapse/segments"
        title={seg?.name ?? 'Segment'}
        badges={seg ? <Badge variant="secondary">{seg.memberCount.toLocaleString()} members</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            {seg?.slug ? `${seg.slug} · ` : ''}
            {seg ? summarizeNode((seg.definition ?? null) as unknown as SummaryNode | null) : ''}
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load segment</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!d ? (
          !loading && !error ? (
            <DetailPageSection title="Segment">
              <p className="text-sm text-muted-foreground">Segment not found.</p>
            </DetailPageSection>
          ) : loading ? (
            <DetailPageSection title="Segment">
              <p className="text-sm text-muted-foreground">Loading…</p>
            </DetailPageSection>
          ) : null
        ) : (
          <>
            <DetailPageSection
              title="Definition"
              description="The conditions that decide who belongs. Saving re-evaluates membership immediately."
              trailing={
                !editing ? (
                  <button type="button" onClick={startEdit} className="rounded border px-2 py-1 text-xs text-muted-foreground hover:bg-muted">
                    Edit
                  </button>
                ) : undefined
              }
            >
              {editing ? (
                <div className="space-y-3">
                  <SegmentTreeBuilder node={tree} onChange={setTree} events={events} attributes={attributes} />
                  <div className="flex flex-wrap items-center gap-2">
                    <button type="button" onClick={onSaveEdit} disabled={!nodeComplete(tree) || saving} className="rounded bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50">
                      {saving ? 'Saving…' : 'Save & re-evaluate'}
                    </button>
                    <button type="button" onClick={() => setEditing(false)} className="rounded border px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted">
                      Cancel
                    </button>
                    {!nodeComplete(tree) ? <span className="text-xs text-muted-foreground">Complete every condition to save.</span> : null}
                  </div>
                  {saveError ? <p className="text-xs text-destructive">Couldn't save segment: {saveError}</p> : null}
                </div>
              ) : (
                <p className="text-sm">{summarizeNode((seg!.definition ?? null) as unknown as SummaryNode | null)}</p>
              )}
            </DetailPageSection>

            <DetailPageSection title="Overview">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Tile label="Members" value={seg!.memberCount.toLocaleString()} />
                <Tile label="Member revenue" value={money(d.revenueCents, currency)} sub={`${d.orders.toLocaleString()} orders`} />
                <Tile label="Avg order value" value={d.orders > 0 ? money(d.aovCents, currency) : '-'} />
                <Tile label="Lifecycle windows" value={seg!.lifecycleWindows ? `${seg!.lifecycleWindows.newDays}/${seg!.lifecycleWindows.activeDays}/${seg!.lifecycleWindows.atRiskDays}d` : 'Channel default'} />
              </div>
            </DetailPageSection>

            <DetailPageSection title="Composition" description="Who's in this segment - lifecycle mix, RFM score spread, and the attributes they're most into.">
              <div className="grid gap-6 lg:grid-cols-2">
                <div>
                  <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">Lifecycle mix</p>
                  {mix.length === 0 ? (
                    <p className="text-sm text-muted-foreground">No members.</p>
                  ) : (
                    <>
                      <div className="flex h-3 w-full overflow-hidden rounded-full">
                        {mix.map((m) => (
                          <span
                            key={m.stage}
                            className={LIFECYCLE_BAR[m.stage] ?? 'bg-muted-foreground/40'}
                            style={{ width: `${(m.count / mixTotal) * 100}%` }}
                            title={`${stageLabel(m.stage)}: ${m.count}`}
                          />
                        ))}
                      </div>
                      <ul className="mt-3 grid gap-1.5 text-sm sm:grid-cols-2">
                        {mix.map((m) => (
                          <li key={m.stage} className="flex items-center gap-2">
                            <span className={`size-2.5 rounded-full ${LIFECYCLE_BAR[m.stage] ?? 'bg-muted-foreground/40'}`} />
                            <span className="capitalize">{stageLabel(m.stage)}</span>
                            <span className="ml-auto tabular-nums text-muted-foreground">
                              {m.count} · {Math.round((m.count / mixTotal) * 100)}%
                            </span>
                          </li>
                        ))}
                      </ul>
                    </>
                  )}
                </div>

                <div>
                  <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">RFM score distribution</p>
                  <div className="h-44 w-full">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={rfmRows} margin={{ top: 4, right: 8, bottom: 0, left: -16 }}>
                        <CartesianGrid strokeDasharray="3 3" className="stroke-border" vertical={false} />
                        <XAxis dataKey="score" tick={{ fontSize: 11 }} />
                        <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={32} />
                        <Tooltip content={<ChartTooltipContent />} />
                        <Legend wrapperStyle={{ fontSize: 11 }} />
                        <Bar dataKey="recency" name="Recency" fill={RFM_COLORS.recency} />
                        <Bar dataKey="frequency" name="Frequency" fill={RFM_COLORS.frequency} />
                        <Bar dataKey="monetary" name="Monetary" fill={RFM_COLORS.monetary} />
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                  <p className="mt-1 text-[11px] text-muted-foreground">Score 0 = never ordered; 5 = top quintile. Members without computed traits are excluded.</p>
                </div>
              </div>

              {(d.topAffinities ?? []).length > 0 ? (
                <div className="mt-4">
                  <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">Top affinities</p>
                  <ul className="grid gap-2 sm:grid-cols-2">
                    {d.topAffinities.map((a) => (
                      <li key={`${a.attr}:${a.value}`} className="grid gap-1 text-sm">
                        <div className="flex items-center justify-between gap-2">
                          <span className="truncate">
                            <span className="text-muted-foreground">{a.attr}</span> <span className="font-medium">{a.value}</span>
                          </span>
                          <span className="shrink-0 tabular-nums text-muted-foreground">{a.members}</span>
                        </div>
                        <span className="block h-1.5 overflow-hidden rounded-full bg-muted">
                          <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.max(3, (a.members / affMax) * 100)}%` }} />
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </DetailPageSection>

            <DetailPageSection title="Growth" description="Daily member count. Accrues forward from when the segment first materialised - back-history isn't reconstructed.">
              {growth.length === 0 ? (
                <p className="text-sm text-muted-foreground">No snapshots yet - the count is recorded each time membership refreshes.</p>
              ) : (
                <div className="h-56 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={growth} margin={{ top: 8, right: 12, bottom: 0, left: -12 }}>
                      <defs>
                        <linearGradient id="seg-growth" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="0%" stopColor={ACCENT} stopOpacity={0.3} />
                          <stop offset="100%" stopColor={ACCENT} stopOpacity={0} />
                        </linearGradient>
                      </defs>
                      <CartesianGrid strokeDasharray="3 3" className="stroke-border" vertical={false} />
                      <XAxis dataKey="day" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
                      <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={40} />
                      <Tooltip content={<ChartTooltipContent />} />
                      <Area type="monotone" dataKey="count" name="Members" stroke={ACCENT} strokeWidth={2} fill="url(#seg-growth)" />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              )}
            </DetailPageSection>

            <DetailPageSection
              title="Members"
              description={d.membersTruncated ? `Top ${d.members.length} by spend (segment has more).` : 'All members.'}
            >
              <DataTable
                columns={columns}
                data={d.members}
                loading={loading}
                getRowId={(m) => m.customerId}
                searchPlaceholder="Search members…"
                emptyState={<p className="py-6 text-sm text-muted-foreground">No members.</p>}
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
      {sub ? <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p> : null}
    </div>
  )
}
