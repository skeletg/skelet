import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
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
import { type LensSessionsQuery, LensSessionsDocument } from './Lens.signals.gql'
import { LensCustomerNamesDocument } from './CustomerNames.gql'

type Row = LensSessionsQuery['lensSessions'][number]

function fmtDur(ms: number): string {
  const s = Math.round(ms / 1000)
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`
}
function fmtWhen(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

// LensRecordings is the channel-wide session-replay browser - a catalog-style data table (search / sort / paginate)
// with a faceted filter to split logged-in customers from anonymous visitors - A row opens the rrweb player
export function LensRecordings() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { data, loading } = useQuery(LensSessionsDocument, {
    variables: { channelId, limit: 200 },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const sessions = useMemo(() => data?.lensSessions ?? [], [data])

  // The Signals API returns customer ids and no names, so the labels come from core in one batched follow-up
  const customerIds = useMemo(
    () => [...new Set(sessions.flatMap((s) => (s.customerId ? [s.customerId] : [])))],
    [sessions],
  )
  const { data: nameData } = useQuery(LensCustomerNamesDocument, {
    variables: { ids: customerIds },
    skip: customerIds.length === 0,
    fetchPolicy: 'cache-first',
  })
  const nameById = useMemo(() => {
    const m = new Map<string, string>()
    for (const c of nameData?.customersByIds ?? []) {
      m.set(c.id, [c.firstName, c.lastName].filter(Boolean).join(' ').trim() || c.email)
    }
    return m
  }, [nameData])

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        id: 'visitor',
        accessorFn: (r) => (r.customerId ? 'customer' : 'anon'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Visitor" />,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.customerId ? 'customer' : 'anon')
        },
        cell: ({ row }) => {
          const s = row.original
          const cust = !!s.customerId
          // Tie through to the visitor's Synapse profile (customer or anon)
          const to = cust ? `/synapse/customers/${s.customerId}` : `/synapse/visitors/${s.anonId}`
          const label = cust ? nameById.get(s.customerId!) || s.customerId!.slice(0, 8) : `${s.anonId.slice(0, 8)}…`
          return (
            <div className="flex min-w-0 items-center gap-2">
              <Badge variant={cust ? 'default' : 'secondary'} className="shrink-0 text-[10px]">
                {cust ? 'Logged-in' : 'Anonymous'}
              </Badge>
              <Link to={to} className="truncate hover:underline" title="Open Synapse profile">
                {label}
              </Link>
            </div>
          )
        },
      },
      {
        accessorKey: 'startedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Started" />,
        cell: ({ row }) => <span className="whitespace-nowrap text-sm">{fmtWhen(row.original.startedAt)}</span>,
      },
      {
        id: 'duration',
        accessorFn: (r) => r.durationMs,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Duration" />,
        cell: ({ row }) => <span className="tabular-nums">{fmtDur(row.original.durationMs)}</span>,
      },
      {
        accessorKey: 'eventCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Events" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.eventCount.toLocaleString()}</span>,
      },
      {
        id: 'pages',
        accessorFn: (r) => `${r.entryPage} ${r.exitPage}`,
        header: 'Pages',
        enableSorting: false,
        cell: ({ row }) => {
          const s = row.original
          return (
            <span className="font-mono text-xs text-muted-foreground">
              {s.entryPage || '/'}
              {s.exitPage && s.exitPage !== s.entryPage ? ` → ${s.exitPage}` : ''}
            </span>
          )
        },
      },
      {
        accessorKey: 'viewport',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Device" />,
        filterFn: (row, _colId, value: string[]) => !value?.length || value.includes(row.original.viewport),
        cell: ({ row }) => <span className="capitalize">{row.original.viewport || '-'}</span>,
      },
      {
        id: 'signals',
        // Array accessor (tags present on the row) so the faceted filter can compute option counts - a column with no
        // accessor crashes getFacetedUniqueValues
        accessorFn: (r) => {
          const tags: string[] = []
          if (r.rageCount > 0) tags.push('rage')
          if (r.errorCount > 0) tags.push('error')
          return tags
        },
        header: 'Frustration',
        enableSorting: false,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const r = row.original
          return value.some((v) => (v === 'rage' && r.rageCount > 0) || (v === 'error' && r.errorCount > 0))
        },
        cell: ({ row }) => {
          const r = row.original
          if (r.rageCount === 0 && r.errorCount === 0) return <span className="text-xs text-muted-foreground">-</span>
          return (
            <div className="flex items-center gap-1.5">
              {r.rageCount > 0 ? (
                <span className="rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-400" title="rage clicks">
                  😡 {r.rageCount}
                </span>
              ) : null}
              {r.errorCount > 0 ? (
                <span className="rounded-full bg-red-500/15 px-1.5 py-0.5 text-[11px] font-medium text-red-700 dark:text-red-400" title="JS errors">
                  ⚠ {r.errorCount}
                </span>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) => (
          <Link to={`/lens/sessions/${row.original.id}`} className="text-xs font-medium text-primary hover:underline">
            ▶ Replay
          </Link>
        ),
      },
    ],
    [nameById],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Session recordings"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            <Link to="/lens" className="underline hover:text-foreground">
              Lens
            </Link>
            {' · '}rrweb replays of recent storefront sessions
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a store</AlertTitle>
              <AlertDescription>Session recordings are scoped to a single store - select one from the channel switcher.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection
            title="Recordings"
            description="Logged-in customers are always recorded; anonymous visitors are sampled. Filter by visitor type, then click a row to replay."
          >
            <DataTable
              columns={columns}
              data={sessions}
              loading={loading}
              getRowId={(s) => s.id}
              searchPlaceholder="Search by page…"
              filters={[
                {
                  column: 'visitor',
                  title: 'Visitor',
                  options: [
                    { label: 'Logged-in', value: 'customer' },
                    { label: 'Anonymous', value: 'anon' },
                  ],
                },
                {
                  column: 'signals',
                  title: 'Frustration',
                  options: [
                    { label: 'Has rage clicks', value: 'rage' },
                    { label: 'Has errors', value: 'error' },
                  ],
                },
                {
                  column: 'viewport',
                  title: 'Device',
                  options: [
                    { label: 'Desktop', value: 'desktop' },
                    { label: 'Tablet', value: 'tablet' },
                    { label: 'Mobile', value: 'mobile' },
                  ],
                },
              ]}
              initialSorting={[{ id: 'startedAt', desc: true }]}
              emptyState={<p className="py-6 text-sm text-muted-foreground">No recordings captured yet for this store.</p>}
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
