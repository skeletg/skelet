import { useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef, type Row } from '@/components/data-table/tanstack'
import {
  BanIcon,
  CheckCircle2Icon,
  CircleCheckIcon,
  CircleIcon,
  CogIcon,
  PackageCheckIcon,
  PackageIcon,
  WalletIcon,
} from 'lucide-react'
import { PlusIcon, DownloadIcon } from 'lucide-react'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { formatCents } from '@/lib/format'
import type { OrderStatus } from '@/graphql/schema.types'
import { OrderStatusBadge } from './OrderStatusBadge'
import { PaymentStatusBadge } from './PaymentStatusBadge'
import { OrdersPagedDocument } from './Orders.gql'

type OrderRow = {
  id: string
  number: string
  status: OrderStatus
  paymentStatus: 'UNPAID' | 'PAID' | 'REFUNDED'
  total: number
  currencyCode: string
  createdAt: string
  shippingAddress: { firstName: string; lastName: string }
}

export function OrdersList({ defaultStatus }: { defaultStatus?: OrderStatus[] } = {}) {
  // Server-driven paging/search/filtering - orders is the one table that grows forever,
  // so the browser must never hold more than a page - URL-backed so views survive navigation
  const {
    globalFilter: search,
    setGlobalFilter: setSearch,
    columnFilters,
    setColumnFilters,
    pagination,
    setPagination,
  } = useListUrlState()
  const debouncedSearch = useDebounced(search.trim(), 300)

  // Pre-seed the status filter when a preset is passed (e.g. /orders/pending) - once, so the user can still clear it
  const [seeded, setSeeded] = useState(false)
  if (!seeded && defaultStatus?.length) {
    setSeeded(true)
    if (columnFilters.length === 0) {
      setColumnFilters([{ id: 'status', value: defaultStatus }])
    }
  }

  const statusValues = columnFilters.find((f) => f.id === 'status')?.value as string[] | undefined
  const paymentValues = columnFilters.find((f) => f.id === 'paymentStatus')?.value as
    | string[]
    | undefined

  const { data, loading, error } = useQuery(OrdersPagedDocument, {
    variables: {
      search: debouncedSearch || null,
      statuses: statusValues?.length ? statusValues : null,
      paymentStatuses: paymentValues?.length ? paymentValues : null,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
  })
  const rows = (data?.ordersPaged.items ?? []) as OrderRow[]
  const totalCount = data?.ordersPaged.totalCount ?? 0

  const columns = useMemo<ColumnDef<OrderRow>[]>(
    () => [
      {
        accessorKey: 'number',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Order #" />,
        cell: ({ row }) => (
          <Link
            to={`/orders/${row.original.id}`}
            className="font-mono font-medium hover:underline"
          >
            {row.original.number}
          </Link>
        ),
      },
      {
        id: 'customer',
        accessorFn: (row) => `${row.shippingAddress.firstName} ${row.shippingAddress.lastName}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Customer" />,
        cell: ({ row }) => (
          <span>
            {row.original.shippingAddress.firstName} {row.original.shippingAddress.lastName}
          </span>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        size: 130,
        filterFn: (row, _col, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.status)
        },
        cell: ({ row }) => <OrderStatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'paymentStatus',
        header: 'Payment',
        size: 130,
        filterFn: (row, _col, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.paymentStatus)
        },
        cell: ({ row }) => <PaymentStatusBadge status={row.original.paymentStatus} />,
      },
      {
        accessorKey: 'total',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Total" />,
        size: 140,
        cell: ({ row }) => (
          <span className="font-medium tabular-nums">
            {formatCents(row.original.total, row.original.currencyCode)}
          </span>
        ),
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Placed" />,
        size: 160,
        sortingFn: (a: Row<OrderRow>, b: Row<OrderRow>) =>
          new Date(a.original.createdAt).getTime() - new Date(b.original.createdAt).getTime(),
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleDateString()}
          </span>
        ),
      },
    ],
    [],
  )

  return (
    <DataTable
      key={defaultStatus ? defaultStatus.join(',') : 'all'}
      prefsKey="orders.list"
      viewsKey="orders.list"
      enableColumnControls
      enableDensityToggle
      columns={columns}
      data={rows}
      loading={loading && rows.length === 0}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState="No orders yet."
      searchPlaceholder="Search number, email or name…"
      manual={{
        rowCount: totalCount,
        pagination,
        onPaginationChange: setPagination,
        globalFilter: search,
        onGlobalFilterChange: setSearch,
        columnFilters,
        onColumnFiltersChange: setColumnFilters,
      }}
      toolbarRightSlot={
        <>
          <Button variant="outline" size="sm" asChild>
            <Link to="/data?resource=order_lines">
              <DownloadIcon className="size-4" />
              Export CSV
            </Link>
          </Button>
          <Button asChild size="sm">
            <Link to="/orders/new">
              <PlusIcon className="size-4" />
              New order
            </Link>
          </Button>
        </>
      }
      filters={[
        {
          column: 'status',
          title: 'Status',
          options: [
            { label: 'Draft',      value: 'DRAFT',      icon: CircleIcon },
            { label: 'Pending',    value: 'PENDING',    icon: CircleIcon },
            { label: 'Confirmed',  value: 'CONFIRMED',  icon: CheckCircle2Icon },
            { label: 'Processing', value: 'PROCESSING', icon: CogIcon },
            { label: 'Fulfilled',  value: 'FULFILLED',  icon: PackageCheckIcon },
            { label: 'Cancelled',  value: 'CANCELLED',  icon: BanIcon },
          ],
        },
        {
          column: 'paymentStatus',
          title: 'Payment',
          options: [
            { label: 'Unpaid',   value: 'UNPAID',   icon: WalletIcon },
            { label: 'Paid',     value: 'PAID',     icon: CircleCheckIcon },
            { label: 'Refunded', value: 'REFUNDED', icon: PackageIcon },
          ],
        },
      ]}
    />
  )
}

