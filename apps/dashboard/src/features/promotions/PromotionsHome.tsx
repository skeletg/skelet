import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  FlaskConicalIcon,
  PlusIcon,
  TagIcon,
  Trash2Icon,
  ZapIcon,
} from 'lucide-react'
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { ColumnDef } from '@/components/data-table/tanstack'

import {
  DetailPageLayout,
  DetailPageHeader,
  DetailPageBody,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { DataTable, DataTableColumnHeader, selectColumn, useListUrlState } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DeletePromotionRuleDocument,
  PromotionsHubDocument,
  ReorderPromotionRulesDocument,
} from './Promotions.gql'
import { ACCENT, formatCents } from './promoFormat'
import { PromotionSimulatorDialog } from './PromotionSimulatorDialog'
import { RedemptionsTrend } from './analyticsShared'
import { usePermissions } from '@/shared/auth/usePermissions'
import { Panel, Tile } from '@/components/layout/panels'

type HubRow = {
  id: string
  slug: string
  name: string
  priority: number
  requiresCode: boolean
  isActive: boolean
  usedCount: number
  maxUses: number | null
  redemptionCount: number
  discountTotalCents: number
  uniqueCustomers: number
  orderRevenueCents: number
}

// PromotionsHome is the merged Promotions hub: KPI tiles + redemption/discount trends over the selected range, then an
// advanced table of every rule joined with its analytics - Each row links to the standalone promotion page
export function PromotionsHome() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  // Creating and deleting promotions need promotions.manage - a viewer's buttons only led to an access-restricted page or a refused delete
  const canManage = usePermissions().has('promotions.manage')
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const variables = useMemo(
    () => ({
      start: start ? new Date(start).toISOString() : null,
      end: end ? new Date(end).toISOString() : null,
    }),
    [start, end],
  )

  const { data, loading, error, refetch } = useQuery(PromotionsHubDocument, {
    variables,
    fetchPolicy: 'cache-and-network',
  })
  const [deleteRule] = useMutation(DeletePromotionRuleDocument)
  const [reorderRules] = useMutation(ReorderPromotionRulesDocument)

  const rules = useMemo(() => data?.promotionRules.items ?? [], [data?.promotionRules.items])
  const totals = data?.promotionAnalyticsTotals
  const daily = data?.promotionAnalyticsDaily ?? []

  // reorderPromotionRules assigns priorities by array position, so the
  // whole priority-ordered id list has to be sent, not just the pair moved
  const moveRule = useCallback(
    async (id: string, delta: number) => {
      const ordered = [...rules].sort((a, b) => a.priority - b.priority).map((r) => r.id)
      const from = ordered.indexOf(id)
      const to = from + delta
      if (from < 0 || to < 0 || to >= ordered.length) return
      ;[ordered[from], ordered[to]] = [ordered[to], ordered[from]]
      await reorderRules({ variables: { ruleIds: ordered } })
      await refetch()
    },
    [rules, reorderRules, refetch],
  )

  const rows = useMemo<HubRow[]>(() => {
    const byId = new Map((data?.promotionAnalytics ?? []).map((a) => [a.ruleId, a]))
    return rules.map((r) => {
      const a = byId.get(r.id)
      return {
        id: r.id,
        slug: r.slug,
        name: r.name,
        priority: r.priority,
        requiresCode: r.requiresCode,
        isActive: r.isActive,
        usedCount: r.usedCount,
        maxUses: r.maxUses ?? null,
        redemptionCount: a?.redemptionCount ?? 0,
        discountTotalCents: a?.discountTotalCents ?? 0,
        uniqueCustomers: a?.uniqueCustomers ?? 0,
        orderRevenueCents: a?.orderRevenueCents ?? 0,
      }
    })
  }, [data?.promotionAnalytics, rules])

  const activeCount = rules.filter((r) => r.isActive).length

  const topByDiscount = useMemo(
    () =>
      rows
        .filter((r) => r.discountTotalCents > 0)
        .sort((a, b) => b.discountTotalCents - a.discountTotalCents)
        .slice(0, 6)
        .map((r) => ({ name: r.name || r.slug, value: r.discountTotalCents })),
    [rows],
  )

  const columns = useMemo<ColumnDef<HubRow>[]>(
    () => [
      // Selection only feeds the bulk delete
      ...(canManage ? [selectColumn<HubRow>()] : []),
      {
        id: 'name',
        accessorFn: (r) => `${r.name} ${r.slug}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Promotion" />,
        cell: ({ row }) => (
          <Link to={`/promotions/${row.original.id}`} className="hover:underline">
            <div className="font-medium">{row.original.name || row.original.slug}</div>
            <div className="font-mono text-xs text-muted-foreground">{row.original.slug}</div>
          </Link>
        ),
      },
      {
        id: 'status',
        accessorFn: (r) => (r.isActive ? 'active' : 'paused'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) =>
          row.original.isActive ? (
            <Badge variant="secondary">Active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">
              Paused
            </Badge>
          ),
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.isActive ? 'active' : 'paused'),
      },
      {
        id: 'type',
        accessorFn: (r) => (r.requiresCode ? 'code' : 'automatic'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Type" />,
        cell: ({ row }) => (
          <span className="inline-flex items-center gap-1.5 text-sm">
            {row.original.requiresCode ? (
              <TagIcon className="size-3.5 text-muted-foreground" />
            ) : (
              <ZapIcon className="size-3.5 text-muted-foreground" />
            )}
            {row.original.requiresCode ? 'Code' : 'Automatic'}
          </span>
        ),
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.requiresCode ? 'code' : 'automatic'),
      },
      {
        accessorKey: 'priority',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Priority" />,
        // Move within priority order, not the table's current sort - the
        // engine evaluates by priority, so that is the order being edited
        cell: ({ row }) => (
          <div className="flex items-center gap-1">
            <span className="tabular-nums">{row.original.priority}</span>
            {canManage && (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  title="Higher priority (runs earlier)"
                  onClick={() => void moveRule(row.original.id, -1)}
                >
                  <ArrowUpIcon className="size-3" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  title="Lower priority (runs later)"
                  onClick={() => void moveRule(row.original.id, 1)}
                >
                  <ArrowDownIcon className="size-3" />
                </Button>
              </>
            )}
          </div>
        ),
      },
      {
        id: 'uses',
        accessorFn: (r) => r.usedCount,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Uses" />,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {row.original.usedCount}
            {row.original.maxUses != null ? ` / ${row.original.maxUses}` : ''}
          </span>
        ),
      },
      {
        accessorKey: 'redemptionCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Redemptions" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.redemptionCount}</span>,
      },
      {
        accessorKey: 'discountTotalCents',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Discount" />,
        cell: ({ row }) => (
          <span className="tabular-nums">{formatCents(row.original.discountTotalCents)}</span>
        ),
      },
      {
        accessorKey: 'uniqueCustomers',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Customers" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.uniqueCustomers}</span>,
      },
      {
        accessorKey: 'orderRevenueCents',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Order revenue" />,
        cell: ({ row }) => (
          <span className="tabular-nums">{formatCents(row.original.orderRevenueCents)}</span>
        ),
      },
    ],
    [moveRule, canManage],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Promotions"
        meta={
          <span>
            Cart price rules with their redemption &amp; discount impact. Lower priority runs first.
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            <PromotionSimulatorDialog
              trigger={
                <Button variant="outline" size="sm">
                  <FlaskConicalIcon className="size-4" />
                  Simulate
                </Button>
              }
            />
            {canManage && (
              <Button asChild size="sm">
                <Link to="/promotions/new">
                  <PlusIcon className="size-4" />
                  New promotion
                </Link>
              </Button>
            )}
          </div>
        }
      />
      <DetailPageBody>
        {error ? (
          <p className="mx-6 mt-4 rounded border border-destructive bg-destructive/10 p-3 text-sm text-destructive">
            {error.message}
          </p>
        ) : null}

        <DetailPageSection
          title="At a glance"
          trailing={
            <div className="flex flex-wrap items-end gap-2">
              <div className="grid gap-1">
                <Label htmlFor="start" className="text-xs">
                  From
                </Label>
                <Input
                  id="start"
                  type="date"
                  value={start}
                  onChange={(e) => setStart(e.target.value)}
                  className="h-8"
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="end" className="text-xs">
                  To
                </Label>
                <Input
                  id="end"
                  type="date"
                  value={end}
                  onChange={(e) => setEnd(e.target.value)}
                  className="h-8"
                />
              </div>
              <Button
                variant="outline"
                size="sm"
                className="h-8"
                onClick={() => {
                  setStart('')
                  setEnd('')
                }}
              >
                All time
              </Button>
            </div>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Tile label="Redemptions" value={(totals?.redemptionCount ?? 0).toLocaleString()} />
            <Tile label="Discount given" value={formatCents(totals?.discountTotalCents ?? 0)} />
            <Tile label="Unique customers" value={(totals?.uniqueCustomers ?? 0).toLocaleString()} />
            <Tile
              label="Active promotions"
              value={String(activeCount)}
              sub={`of ${rules.length} total`}
            />
          </div>
        </DetailPageSection>

        <DetailPageSection title="Trends">
          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title="Redemptions over time">
              <RedemptionsTrend daily={daily} />
            </Panel>

            <Panel title="Top promotions by discount">
              {topByDiscount.length === 0 ? (
                <Empty>No discount recorded in this range.</Empty>
              ) : (
                <div style={{ height: Math.max(160, topByDiscount.length * 34) }}>
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={topByDiscount}
                      layout="vertical"
                      margin={{ top: 0, right: 16, bottom: 0, left: 8 }}
                    >
                      <XAxis
                        type="number"
                        tick={{ fontSize: 11 }}
                        tickFormatter={(v) => formatCents(Number(v))}
                      />
                      <YAxis type="category" dataKey="name" width={130} tick={{ fontSize: 11 }} />
                      <Tooltip content={<DiscountTooltip />} cursor={{ fill: 'transparent' }} />
                      <Bar dataKey="value" fill={ACCENT} radius={[0, 4, 4, 0]} barSize={16} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              )}
            </Panel>
          </div>
        </DetailPageSection>

        <DetailPageSection title="All promotions">
          <DataTable
            urlState={urlState}
            columns={columns}
            data={rows}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(r) => r.id}
            emptyState="No promotions yet."
            searchPlaceholder="Search name or slug…"
            initialSorting={[{ id: 'discountTotalCents', desc: true }]}
            enableRowSelection
            bulkActions={!canManage ? undefined : [
              {
                label: 'Delete',
                icon: Trash2Icon,
                variant: 'destructive',
                confirm: {
                  title: (sel) =>
                    `Delete ${sel.length} promotion${sel.length === 1 ? '' : 's'}?`,
                  description:
                    'This is permanent. Past redemptions stay queryable (the audit rows use ON DELETE SET NULL).',
                  actionLabel: 'Delete',
                },
                perform: async (sel) => {
                  await Promise.all(sel.map((r) => deleteRule({ variables: { id: r.id } })))
                },
                onSuccess: () => {
                  void refetch()
                },
              },
            ]}
            filters={[
              {
                column: 'status',
                title: 'Status',
                options: [
                  { label: 'Active', value: 'active' },
                  { label: 'Paused', value: 'paused' },
                ],
              },
              {
                column: 'type',
                title: 'Type',
                options: [
                  { label: 'Code', value: 'code' },
                  { label: 'Automatic', value: 'automatic' },
                ],
              },
            ]}
          />
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-10 text-center text-sm text-muted-foreground">{children}</p>
}

function DiscountTooltip({
  active,
  payload,
}: {
  active?: boolean
  payload?: Array<{ payload: { name: string; value: number } }>
}) {
  if (!active || !payload?.length) return null
  const p = payload[0].payload
  return (
    <div className="rounded border bg-background px-2.5 py-1.5 text-xs shadow">
      <div className="font-medium">{p.name}</div>
      <div className="text-muted-foreground">{formatCents(p.value)} discount</div>
    </div>
  )
}
