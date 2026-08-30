import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { AlertTriangleIcon, PackageOpenIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { LowStockLevelsDocument, type LowStockLevelsQuery } from './Inventory.gql'
import { BulkStockSyncDialog } from './BulkStockSyncDialog'

type LowStockRow = LowStockLevelsQuery['lowStockLevels'][number]

export function LowStockList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { data, loading, error, refetch } = useQuery(LowStockLevelsDocument, {
    fetchPolicy: 'cache-and-network',
  })
  const rows = data?.lowStockLevels ?? []

  const columns = useMemo<ColumnDef<LowStockRow>[]>(
    () => [
      {
        accessorKey: 'productSlug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Product" />,
        cell: ({ row }) => (
          <Link
            to={`/catalog/products/${row.original.productId}`}
            className="font-medium hover:underline"
          >
            {row.original.productSlug}
          </Link>
        ),
      },
      {
        accessorKey: 'productSku',
        header: 'SKU',
        size: 160,
        cell: ({ row }) => (
          <code className="font-mono text-xs">{row.original.productSku}</code>
        ),
      },
      {
        accessorKey: 'locationName',
        header: 'Location',
        cell: ({ row }) => {
          const unstocked = row.original.locationId == null
          return unstocked ? (
            <span className="text-sm italic text-muted-foreground">No locations stocked</span>
          ) : (
            <Link
              to={`/inventory/locations/${row.original.locationId}`}
              className="text-sm hover:underline"
            >
              {row.original.locationName}
            </Link>
          )
        },
      },
      {
        accessorKey: 'quantity',
        header: ({ column }) => <DataTableColumnHeader column={column} title="On hand" />,
        size: 96,
        cell: ({ row }) => {
          const unstocked = row.original.locationId == null
          const fmt = (n: number) =>
            row.original.quantityDecimals > 0
              ? n.toFixed(row.original.quantityDecimals)
              : String(Math.trunc(n))
          return (
            <div className="text-right tabular-nums">
              {unstocked ? <span className="text-muted-foreground">-</span> : fmt(row.original.quantity)}
            </div>
          )
        },
      },
      {
        accessorKey: 'lowStockThreshold',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Threshold" />,
        size: 112,
        cell: ({ row }) => {
          const fmt = (n: number) =>
            row.original.quantityDecimals > 0
              ? n.toFixed(row.original.quantityDecimals)
              : String(Math.trunc(n))
          return (
            <div className="text-right tabular-nums text-muted-foreground">
              {fmt(row.original.lowStockThreshold)}
            </div>
          )
        },
      },
      {
        id: 'status',
        header: 'Status',
        size: 128,
        enableSorting: false,
        cell: ({ row }) => {
          const unstocked = row.original.locationId == null
          const out = row.original.quantity <= 0
          return (
            <>
              {unstocked ? (
                <Badge variant="destructive" className="gap-1">
                  <AlertTriangleIcon className="size-3" />
                  No stock recorded
                </Badge>
              ) : out ? (
                <Badge variant="destructive" className="gap-1">
                  <AlertTriangleIcon className="size-3" />
                  Out of stock
                </Badge>
              ) : (
                <Badge variant="secondary" className="gap-1">
                  <AlertTriangleIcon className="size-3" />
                  Low
                </Badge>
              )}
              {row.original.backorderable && (
                <Badge variant="outline" className="ml-1 text-xs">
                  Backorder
                </Badge>
              )}
            </>
          )
        },
      },
    ],
    [],
  )

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="grid gap-1">
        <h1 className="text-2xl font-bold tracking-tight">Low stock</h1>
        <p className="text-sm text-muted-foreground">
          Tracked items whose physical quantity has fallen at or below the
          configured low-stock threshold. Items with threshold = 0 are
          excluded - set a threshold on a product's inventory section to
          have it appear here.
        </p>
      </div>
        <BulkStockSyncDialog onSynced={() => void refetch()} />
      </div>

      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => `${row.inventoryItemId}:${row.locationId ?? 'no-loc'}`}
        searchPlaceholder="Search products…"
        emptyState={
          <div className="grid gap-3 p-4 text-center">
            <PackageOpenIcon className="mx-auto size-8 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              Nothing's low. Every tracked SKU with a threshold has stock
              above it.
            </p>
          </div>
        }
      />
    </div>
  )
}
