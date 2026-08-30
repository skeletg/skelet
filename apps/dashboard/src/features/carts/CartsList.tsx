import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef, type Row } from '@/components/data-table/tanstack'
import { PlusIcon } from 'lucide-react'

import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { CartsDocument } from './Carts.gql'
import { NewCartDialog } from './NewCartDialog'

type CartRow = {
  id: string
  customerId?: string | null
  customerEmail?: string | null
  customerName?: string | null
  status: string
  itemCount: number
  subtotal: number
  currencyCode: string
  updatedAt: string
}

// CartsList is the Order Manager's cart console: search/list a channel's carts (active by default; abandoned = old
// active), open one to edit, or start a new cart - Channel-scoped via the header picker
export function CartsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, scope } = useConfigScope()
  const [status, setStatus] = useState<'active' | 'abandoned' | 'converted' | 'all'>('active')
  const [newOpen, setNewOpen] = useState(false)

  const variables = useMemo(
    () => ({
      channelId: effectiveChannelId ?? '',
      status: status === 'all' || status === 'abandoned' ? null : status,
      abandoned: status === 'abandoned' ? true : null,
      limit: 100,
    }),
    [effectiveChannelId, status],
  )

  const { data, loading, error } = useQuery(CartsDocument, {
    variables,
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const rows = (data?.carts.items ?? []) as CartRow[]

  const columns = useMemo<ColumnDef<CartRow>[]>(
    () => [
      {
        id: 'customer',
        accessorFn: (r) => r.customerName ?? r.customerEmail ?? 'Guest',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Customer" />,
        cell: ({ row }) => (
          <Link to={`/carts/${row.original.id}`} className="font-medium hover:underline">
            {row.original.customerName ?? row.original.customerEmail ?? (
              <span className="text-muted-foreground">Guest</span>
            )}
          </Link>
        ),
      },
      {
        accessorKey: 'itemCount',
        header: 'Items',
        size: 90,
        cell: ({ row }) => <span className="tabular-nums">{row.original.itemCount}</span>,
      },
      {
        accessorKey: 'subtotal',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Subtotal" />,
        size: 130,
        cell: ({ row }) => (
          <span className="tabular-nums">{formatCents(row.original.subtotal, row.original.currencyCode)}</span>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        size: 110,
        cell: ({ row }) => (
          <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        size: 160,
        sortingFn: (a: Row<CartRow>, b: Row<CartRow>) => new Date(a.original.updatedAt).getTime() - new Date(b.original.updatedAt).getTime(),
        cell: ({ row }) => (
          <span className="text-muted-foreground">{new Date(row.original.updatedAt).toLocaleString()}</span>
        ),
      },
    ],
    [],
  )

  if (scope.mode === 'GLOBAL' || !effectiveChannelId) {
    return (
      <Card>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          Pick a channel in the header scope picker to browse its carts.
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <DataTable
        urlState={urlState}
        prefsKey="carts.list"
        enableColumnControls
        enableDensityToggle
        key={status}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(r) => r.id}
        emptyState="No carts for this filter."
        searchPlaceholder="Search by customer…"
        toolbarRightSlot={
          <div className="flex items-center gap-2">
            <Select value={status} onValueChange={(v) => setStatus(v as typeof status)}>
              <SelectTrigger className="h-8 w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="abandoned">Abandoned</SelectItem>
                <SelectItem value="converted">Converted</SelectItem>
                <SelectItem value="all">All</SelectItem>
              </SelectContent>
            </Select>
            <Button size="sm" onClick={() => setNewOpen(true)}>
              <PlusIcon className="size-4" />
              New cart
            </Button>
          </div>
        }
      />
      <NewCartDialog open={newOpen} onOpenChange={setNewOpen} channelId={effectiveChannelId} />
    </>
  )
}
