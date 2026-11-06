import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import { DetailPageSection } from '@/components/layout/detail-page'
import type { LensPlanIssueKind } from '@/graphql/schema.types'
import { formatNumber } from '@/lib/format'
import type { LensOverviewQuery } from './Lens.gql'

type IssueRow = LensOverviewQuery['lensOverview']['planIssues'][number]

const issueLabel: Record<LensPlanIssueKind, string> = {
  UNKNOWN_EVENT: 'Unknown event, dropped',
  UNKNOWN_PROP: 'Unknown property, dropped',
  BAD_TYPE: 'Wrong type, dropped',
  SERVER_PROP: 'Server-only property, dropped',
}

const columns: ColumnDef<IssueRow>[] = [
  {
    accessorKey: 'kind',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Issue" />,
    cell: ({ row }) => <span className="text-warning">{issueLabel[row.original.kind]}</span>,
  },
  {
    accessorKey: 'event',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Event" />,
    cell: ({ row }) => <span className="font-mono text-xs">{row.original.event}</span>,
  },
  {
    accessorKey: 'prop',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Property" />,
    cell: ({ row }) => <span className="font-mono text-xs">{row.original.prop || '-'}</span>,
  },
  {
    accessorKey: 'count',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Count" />,
    cell: ({ row }) => <span className="tabular-nums">{formatNumber(row.original.count)}</span>,
  },
]

// DataHealthSection lists what ingest stripped against the tracking plan, so a storefront tracker that drifts from it
// shows up here instead of quietly losing data
export function DataHealthSection({ issues }: { issues: IssueRow[] }) {
  return (
    <DetailPageSection
      title="Data health"
      description={
        <>
          Browser events and properties the tracking plan doesn&apos;t allow, dropped at ingest. The plan is in{' '}
          <code className="text-xs">docs/tracking-plan.md</code>.
        </>
      }
    >
      <DataTable
        columns={columns}
        data={issues}
        getRowId={(i) => `${i.kind}:${i.event}:${i.prop}`}
        searchPlaceholder="Search events and properties…"
        pageSize={10}
        initialSorting={[{ id: 'count', desc: true }]}
        emptyState={<p className="py-6 text-sm text-muted-foreground">Every browser event matched the tracking plan.</p>}
      />
    </DetailPageSection>
  )
}
