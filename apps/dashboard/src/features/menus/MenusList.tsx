import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, Trash2Icon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'

import {
  DeleteMenuDocument,
  MenusListDocument,
} from './Menus.gql'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type MenuRow = {
  id: string
  slug: string
  name: string
  itemCount: number
  channelIds: string[]
}

// MenusList lists navigation menus in a catalog-style advanced table (search / sort / paginate) - A row links to the
// standalone menu editor; "New menu" opens the create page
export function MenusList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('cms.manage')

  const { data, loading, error } = useQuery(MenusListDocument)
  const rows = useMemo(
    () => ((data?.menus ?? []) as MenuRow[]).map((m) => ({ ...m })),
    [data],
  )

  const [confirmingDelete, setConfirmingDelete] = useState<MenuRow | null>(null)
  const [deleteMenu] = useMutation(DeleteMenuDocument, {
    refetchQueries: [{ query: MenusListDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteMenu({ variables: { id: confirmingDelete.id } })
  }

  const columns = useMemo<ColumnDef<MenuRow>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (m) => m.name,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/menus/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'items',
        accessorFn: (m) => m.itemCount,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Items" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.itemCount}</span>,
      },
      {
        id: 'channels',
        accessorFn: (m) => m.channelIds.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Channels" />,
        cell: ({ row }) => {
          const n = row.original.channelIds.length
          return <span className="text-sm text-muted-foreground">{n === 0 ? 'Unassigned' : `${n} channel${n === 1 ? '' : 's'}`}</span>
        },
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) =>
          canManage ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-muted-foreground hover:text-destructive"
              onClick={() => {
                setConfirmingDelete(row.original)
              }}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          ) : null,
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
        getRowId={(m) => m.id}
        emptyState="No menus yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'name', desc: false }]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/menus/new">
                <PlusIcon className="size-4" />
                New menu
              </Link>
            </Button>
          )
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.name}?</>}
        description="This removes the menu and every item inside it. Channels referencing the slug fall back to their empty state."
        confirmLabel="Delete menu"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
