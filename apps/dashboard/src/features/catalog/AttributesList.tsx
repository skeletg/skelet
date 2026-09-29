import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
import { AttributeFormDialog, type ExistingAttribute } from './AttributeFormDialog'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import {
  AttributesDocument,
  DeleteAttributeDocument,
} from './Catalog.gql'

type AttributeRow = ExistingAttribute & {
  values: Array<{ id: string }>
}

export function AttributesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has, allowedChannelScopes } = usePermissions()
  const canManage = has('catalog.manage')
  // Attributes are shared by every channel, so creating one needs the grant everywhere (the resolver's
  // requireGlobal) - which channel the header is viewing doesn't matter
  const canCreate = allowedChannelScopes('catalog.manage').allChannels

  const { data, loading, error } = useQuery(AttributesDocument)
  const rows = (data?.attributes ?? []) as AttributeRow[]

  const [confirmingDelete, setConfirmingDelete] = useState<AttributeRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteAttribute, { loading: deleting }] = useMutation(DeleteAttributeDocument, {
    refetchQueries: [{ query: AttributesDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteAttribute({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<AttributeRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Link
              to={`/catalog/attributes/${row.original.id}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
            <AttributeInputTypeBadge type={row.original.inputType} />
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
        id: 'flags',
        header: 'Flags',
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {row.original.valueRequired && (
              <Badge variant="secondary" className="text-[10px]">
                Required
              </Badge>
            )}
            {row.original.filterable && (
              <Badge variant="secondary" className="text-[10px]">
                Filterable
              </Badge>
            )}
            {row.original.filterable && row.original.facetCollapsed && (
              <Badge variant="outline" className="text-[10px]">
                Collapsed
              </Badge>
            )}
            {row.original.searchable && (
              <Badge variant="secondary" className="text-[10px]">
                Searchable
              </Badge>
            )}
          </div>
        ),
      },
      {
        id: 'values',
        header: 'Values',
        size: 100,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.values.length === 0 ? '-' : row.original.values.length}
          </span>
        ),
      },
      {
        accessorKey: 'sortOrder',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Order" />,
        size: 80,
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
                <Link to={`/catalog/attributes/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setDeleteError(null)
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

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No attributes yet."
        searchPlaceholder="Search name or slug…"
        toolbarRightSlot={
          canCreate && (
            <AttributeFormDialog
              trigger={
                <Button size="sm">
                  <PlusIcon className="size-4" />
                  New attribute
                </Button>
              }
            />
          )
        }
      />

      <AlertDialog
        open={!!confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {confirmingDelete?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Removes the attribute, all of its values, and its assignments to any product types.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                onConfirmDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete attribute'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
