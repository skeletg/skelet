import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon, TruckIcon } from 'lucide-react'
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
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeleteProductTypeDocument,
  ProductTypesDocument,
} from './Catalog.gql'
import { ProductTypeFormDialog, type ExistingProductType } from './ProductTypeFormDialog'
import { ProductTypeDetail } from './ProductTypeDetail'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type ProductTypeRow = ExistingProductType & {
  attributes: Array<{ attribute: { id: string; name: string } }>
}

// ProductTypesList is the shell for /catalog/product-types and /catalog/product-types/:id - The list is a table
// (matching the rest of the catalog admin); selecting a row opens the full-width ProductTypeDetail
export function ProductTypesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const activeID = params.id ?? null

  const { has, allowedChannelScopes } = usePermissions()
  const canManage = has('catalog.manage')
  // Types are shared by every channel, so creating one needs the grant everywhere (the resolver's requireGlobal) -
  // which channel the header is viewing doesn't matter
  const canCreate = allowedChannelScopes('catalog.manage').allChannels

  const { data, loading, error } = useQuery(ProductTypesDocument)
  const rows = useMemo(() => ((data?.productTypes ?? []) as ProductTypeRow[]), [data])

  const [confirmingDelete, setConfirmingDelete] = useState<ProductTypeRow | null>(null)

  const [deleteProductType] = useMutation(DeleteProductTypeDocument, {
    refetchQueries: [ProductTypesDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteProductType({ variables: { id: confirmingDelete.id } })
    if (activeID === confirmingDelete.id) {
      navigate('/catalog/product-types', { replace: true })
    }
  }

  const columns = useMemo<ColumnDef<ProductTypeRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Link
              to={`/catalog/product-types/${row.original.id}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
            {row.original.isDigital && (
              <Badge variant="outline" className="text-[10px]">
                Digital
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
        id: 'attributes',
        header: 'Attributes',
        size: 110,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.attributes.length === 0 ? '-' : row.original.attributes.length}
          </span>
        ),
      },
      {
        id: 'shipping',
        header: 'Shipping',
        size: 110,
        cell: ({ row }) =>
          row.original.isShippingRequired ? (
            <Badge variant="secondary" className="gap-1 text-[10px]">
              <TruckIcon className="size-3" /> Required
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
                <Link to={`/catalog/product-types/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setConfirmingDelete(row.original)
                    }}
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

  // Detail renders full-width (reached from the table row)
  if (activeID) return <ProductTypeDetail productTypeId={activeID} />

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No product types yet."
        searchPlaceholder="Search name or slug…"
        toolbarRightSlot={
          canCreate && (
            <ProductTypeFormDialog
              trigger={
                <Button size="sm">
                  <PlusIcon className="size-4" />
                  New product type
                </Button>
              }
            />
          )
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.name}?</>}
        description="This permanently removes the product type and its attribute assignments. Product types with products attached cannot be deleted - the backend will reject those."
        confirmLabel="Delete product type"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
