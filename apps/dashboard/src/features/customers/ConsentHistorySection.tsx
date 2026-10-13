import { useQuery } from '@apollo/client/react'
import { useMemo } from 'react'
import { type ColumnDef } from '@/components/data-table/tanstack'

import { Badge } from '@/components/ui/badge'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'

import { CustomerConsentHistoryDocument, type CustomerConsentHistoryQuery } from './Customers.gql'

type ConsentEvent = CustomerConsentHistoryQuery['customerConsentHistory'][number]

const SOURCE_LABELS: Record<string, string> = {
  registration: 'Registration',
  profile: 'Account settings',
  checkout: 'Checkout',
  unsubscribe_link: 'Unsubscribe link',
  admin: 'Admin',
  plugin: 'Plugin',
  signup_form: 'Signup form (confirmed)',
}

export function ConsentHistorySection({ customerId, kindName }: { customerId: string; kindName: (kind: string) => string }) {
  const { data, loading } = useQuery(CustomerConsentHistoryDocument, { variables: { customerId } })

  const columns = useMemo<ColumnDef<ConsentEvent>[]>(
    () => [
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Date" />,
        size: 190,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-muted-foreground">{new Date(row.original.createdAt as string).toLocaleString()}</span>
        ),
      },
      { accessorKey: 'kind', header: 'Consent', cell: ({ row }) => kindName(row.original.kind) },
      {
        accessorKey: 'optedIn',
        header: 'Change',
        size: 120,
        cell: ({ row }) =>
          row.original.optedIn ? (
            <Badge variant="outline" className="border-success/40 text-success">
              Opted in
            </Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">
              Opted out
            </Badge>
          ),
      },
      {
        accessorKey: 'source',
        header: 'Source',
        cell: ({ row }) => {
          const e = row.original
          const label = SOURCE_LABELS[e.source] ?? e.source
          return e.source === 'admin' && !e.adminId ? `${label} (integration or deleted admin)` : label
        },
      },
    ],
    [kindName],
  )

  if (loading && !data) return <p className="text-sm text-muted-foreground">Loading…</p>
  const events = data?.customerConsentHistory ?? []
  return (
    <DataTable
      columns={columns}
      data={events}
      getRowId={(row) => row.id}
      pageSize={10}
      emptyState="No consent changes recorded yet."
    />
  )
}
