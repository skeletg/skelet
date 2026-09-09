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
import { type LensErrorsQuery, LensErrorsDocument } from './Lens.signals.gql'

type Row = LensErrorsQuery['lensErrors'][number]

function fmtWhen(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

// LensErrors lists JS errors captured on the storefront (window.onerror + unhandledrejection), grouped by message - the
// Clarity-style "what's breaking" view - Errors ride the same custom-event pipeline as the rest of Lens
export function LensErrors() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { data, loading } = useQuery(LensErrorsDocument, {
    variables: { channelId, sinceDays: 30, limit: 100 },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const errors = data?.lensErrors ?? []

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'message',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Error" />,
        cell: ({ row }) => (
          <span className="block max-w-[520px] truncate font-mono text-xs" title={row.original.message}>
            {row.original.message || '(empty)'}
          </span>
        ),
      },
      {
        accessorKey: 'count',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Occurrences" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.count.toLocaleString()}</span>,
      },
      {
        accessorKey: 'pages',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Pages" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.pages.toLocaleString()}</span>,
      },
      {
        accessorKey: 'lastSeen',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Last seen" />,
        cell: ({ row }) => <span className="whitespace-nowrap text-sm text-muted-foreground">{fmtWhen(row.original.lastSeen)}</span>,
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="JS errors"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            <Link to="/lens" className="underline hover:text-foreground">
              Lens
            </Link>
            {' · '}client-side errors captured from the storefront - last 30 days
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a store</AlertTitle>
              <AlertDescription>JS errors are per store - select one from the channel switcher.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection title="Errors by message" description="Grouped JavaScript errors (window.onerror + unhandled promise rejections). Sampled with the rest of Lens.">
            <DataTable
              columns={columns}
              data={errors}
              loading={loading}
              getRowId={(e) => e.message}
              searchPlaceholder="Search errors…"
              initialSorting={[{ id: 'count', desc: true }]}
              emptyState={<p className="py-6 text-sm text-muted-foreground">No JS errors captured yet 🎉</p>}
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
