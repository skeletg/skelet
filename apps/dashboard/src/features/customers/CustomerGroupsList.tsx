import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon, StarIcon } from 'lucide-react'
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
import { CustomerGroupFormDialog, type ExistingCustomerGroup } from './CustomerGroupFormDialog'
import { useCustomerAccess } from './useCustomerAccess'
import {
  CustomerGroupsDocument,
  DeleteCustomerGroupDocument,
  SetDefaultCustomerGroupDocument,
} from './CustomerGroups.gql'

type GroupRow = ExistingCustomerGroup & {
  isDefault: boolean
  memberCount: number
  createdAt: string
}

// CustomerGroupsList: the groups as ONE flat DataTable, a click deep-linking into the standalone
// CustomerGroupDetail page - a master/detail shell would park a second sidebar next to the app
// nav and greeted the operator with an empty "select a group" panel
// "New" keeps opening CustomerGroupFormDialog (slug is required on create - easier as a dialog)
export function CustomerGroupsList() {
  const navigate = useNavigate()
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { canManageGroups } = useCustomerAccess()
  const { data, loading, error } = useQuery(CustomerGroupsDocument)
  const rows = useMemo(() => (data?.customerGroups ?? []) as GroupRow[], [data])

  const [createOpen, setCreateOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState<GroupRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const refetchQueries = [{ query: CustomerGroupsDocument }]
  const [deleteGroup, { loading: deleting }] = useMutation(DeleteCustomerGroupDocument, {
    refetchQueries,
  })
  const [setDefault, { loading: settingDefault }] = useMutation(SetDefaultCustomerGroupDocument, {
    refetchQueries,
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteGroup({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  async function onSetDefault(row: GroupRow) {
    setActionError(null)
    try {
      await setDefault({ variables: { id: row.id } })
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Set default failed.')
    }
  }

  const columns = useMemo<ColumnDef<GroupRow, unknown>[]>(
    () => [
      {
        id: 'group',
        // slug in the accessor so the toolbar search matches it too
        accessorFn: (r) => `${r.name} ${r.slug}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Group" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-sm font-medium leading-tight">{row.original.name}</span>
              {row.original.isDefault && (
                <Badge variant="secondary" className="gap-1 font-normal">
                  <StarIcon className="size-3" />
                  default
                </Badge>
              )}
            </div>
            <code className="block truncate font-mono text-xs text-muted-foreground">
              {row.original.slug}
            </code>
          </div>
        ),
      },
      {
        accessorKey: 'memberCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Members" />,
        size: 100,
        cell: ({ row }) => <span className="text-sm tabular-nums">{row.original.memberCount}</span>,
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Created" />,
        size: 130,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleDateString()}
          </span>
        ),
      },
      ...(canManageGroups
        ? [
            {
              id: 'actions',
              header: '',
              size: 50,
              enableSorting: false,
              cell: ({ row }: { row: { original: GroupRow } }) => {
                const r = row.original
                const canDelete = !r.isDefault && r.memberCount === 0
                return (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7 text-muted-foreground"
                        aria-label={`Actions for ${r.name}`}
                        onClick={(e) => e.stopPropagation()}
                      >
                        <MoreHorizontalIcon className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {!r.isDefault && (
                        <DropdownMenuItem onSelect={() => void onSetDefault(r)} disabled={settingDefault}>
                          <StarIcon className="size-4" />
                          Set as default
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        disabled={!canDelete}
                        onSelect={() => {
                          if (!canDelete) return
                          setDeleteError(null)
                          setConfirmingDelete(r)
                        }}
                      >
                        {r.isDefault
                          ? "Can't delete default"
                          : r.memberCount > 0
                            ? "Can't delete (has members)"
                            : 'Delete'}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )
              },
            } satisfies ColumnDef<GroupRow, unknown>,
          ]
        : []),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps -- handlers are stable enough per render; the column defs only need to rebuild when access or in-flight state changes
    [canManageGroups, settingDefault],
  )

  return (
    <>
      {actionError && (
        <Alert variant="destructive" className="mb-3">
          <AlertTitle>Action failed</AlertTitle>
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      )}
      <DataTable
        urlState={urlState}
        prefsKey="customers.groups"
        enableColumnControls
        enableDensityToggle
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No customer groups yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'memberCount', desc: true }]}
        onRowClick={(row) => navigate(`/customers/groups/${row.id}`)}
        toolbarRightSlot={
          canManageGroups ? (
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <PlusIcon className="size-4" />
              New group
            </Button>
          ) : null
        }
      />

      <CustomerGroupFormDialog open={createOpen} onOpenChange={setCreateOpen} />

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
              The group and its definition will be removed. Groups with members or the
              default group cannot be deleted - the backend will reject those.
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
              {deleting ? 'Deleting…' : 'Delete group'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
