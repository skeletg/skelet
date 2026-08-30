import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PackageCheckIcon, PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeleteInventoryLocationDocument,
  InventoryLocationsPagedDocument,
} from './Inventory.gql'
import {
  InventoryLocationCreatePage,
  InventoryLocationEditPage,
} from './InventoryLocationForm'

type LocationRow = {
  id: string
  slug: string
  name: string
  isActive: boolean
  type: string
  city: string
  region: string
  countryCode: string
  pickupEnabled: boolean
}

// InventoryLocationsList is the shell for /inventory/locations, /inventory/locations/new and /inventory/locations/:id -
// The list is a table (matching the rest of the catalog admin); selecting a row opens the full-width location form
export function InventoryLocationsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  // Server-driven paging + search - the grid stays light no matter how many locations exist
  const {
    globalFilter: search,
    setGlobalFilter: setSearch,
    columnFilters,
    setColumnFilters,
    pagination,
    setPagination,
  } = useListUrlState()
  const debouncedSearch = useDebounced(search.trim(), 300)
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  // /inventory/locations/new is its own URL but routes to this same
  // component - Disambiguate by raw param
  const rawId = params.id ?? null
  const isCreate = rawId === 'new'
  const activeID = !isCreate && rawId ? rawId : null

  const { has } = usePermissions()
  const canManage = has('inventory.manage')

  const { data, loading, error } = useQuery(InventoryLocationsPagedDocument, {
    variables: {
      search: debouncedSearch || null,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
  })
  const rows = useMemo<LocationRow[]>(
    () =>
      (data?.inventoryLocations ?? []).map((l) => ({
        id: l.id,
        slug: l.slug,
        name: l.name,
        isActive: l.isActive,
        type: l.type,
        city: l.city,
        region: l.region,
        countryCode: l.countryCode,
        pickupEnabled: l.pickupEnabled,
      })),
    [data],
  )
  const totalCount = data?.inventoryLocationsCount ?? 0

  const [confirmingDelete, setConfirmingDelete] = useState<LocationRow | null>(null)

  const [deleteLocation] = useMutation(DeleteInventoryLocationDocument, {
    refetchQueries: ['InventoryLocationsPaged'],
  })

  // Throws on failure - ConfirmDialog keeps itself open and shows the message
  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteLocation({ variables: { id: confirmingDelete.id } })
    if (activeID === confirmingDelete.id) {
      navigate('/inventory/locations', { replace: true })
    }
    setConfirmingDelete(null)
  }

  const columns = useMemo<ColumnDef<LocationRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Link
              to={`/inventory/locations/${row.original.id}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
            {!row.original.isActive && (
              <Badge variant="outline" className="text-[10px]">
                Inactive
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'slug',
        header: 'Slug',
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{row.original.slug}</code>
        ),
      },
      {
        accessorKey: 'type',
        header: 'Type',
        size: 120,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{row.original.type || '-'}</span>
        ),
      },
      {
        id: 'address',
        header: 'Location',
        // accessorFn not just cell - the global filter only sees columns that resolve a value, and city has to stay searchable
        accessorFn: (row) => formatLocation(row),
        enableSorting: false,
        cell: ({ getValue }) => (
          <span className="text-xs text-muted-foreground">{(getValue() as string) || '-'}</span>
        ),
      },
      {
        id: 'pickup',
        header: 'Pickup',
        size: 110,
        cell: ({ row }) =>
          row.original.pickupEnabled ? (
            <Badge variant="secondary" className="gap-1 text-[10px]">
              <PackageCheckIcon className="size-3" /> Enabled
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label={`Actions for ${row.original.name}`}
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/inventory/locations/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => setConfirmingDelete(row.original)}
                  >
                    Delete
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [canManage],
  )

  // Create + edit render full-width (reached from the table)
  if (isCreate) return <InventoryLocationCreatePage />
  if (activeID) return <InventoryLocationEditPage locationId={activeID} />

  return (
    <>
      <DataTable
        columns={columns}
        data={rows}
        loading={loading && rows.length === 0}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No locations yet."
        searchPlaceholder="Search name, slug or city…"
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
          canManage && (
            <Button size="sm" onClick={() => navigate('/inventory/locations/new')}>
              <PlusIcon className="size-4" />
              New location
            </Button>
          )
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(v) => !v && setConfirmingDelete(null)}
        title={`Delete ${confirmingDelete?.name}?`}
        description="This permanently removes the location and any stock levels recorded against it. Reservation rows referencing the location are preserved for audit but the FK on order_id is set to NULL."
        confirmLabel="Delete location"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}

function formatLocation(row: { city: string; region: string; countryCode: string }): string {
  return [row.city, row.region, row.countryCode].filter((s) => s && s.trim() !== '').join(', ')
}
