import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  ShieldIcon,
  Trash2Icon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  DataTable,
  DataTableColumnHeader,
  selectColumn,
  type BulkAction,
  useListUrlState,
} from '@/components/data-table'
import { useAuth } from '@/shared/auth/auth'
import { AdminFormDialog, type ExistingAdmin } from './AdminFormDialog'
import { AdminsDocument, DeleteAdminDocument, UpdateAdminDocument } from './Admins.gql'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// the form's shape plus what only the list shows
type AdminRow = ExistingAdmin & { lastLoginAt?: string | null }

type StatusKey = 'active' | 'inactive' | 'superuser'

function statusOf(row: AdminRow): StatusKey {
  if (row.isSuperuser) return 'superuser'
  if (!row.isActive) return 'inactive'
  return 'active'
}

export function AdminsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  // Creating, disabling and deleting admins all need admins.manage - a viewer only met a refused save
  const canManage = usePermissions().has('admins.manage')
  const { auth } = useAuth()
  const myId = auth?.admin.id

  const { data, loading, error } = useQuery(AdminsDocument)
  const rows = useMemo(() => (data?.admins ?? []) as AdminRow[], [data])

  const [confirmingRowDelete, setConfirmingRowDelete] = useState<AdminRow | null>(null)

  const [deleteAdmin] = useMutation(DeleteAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })
  const [updateAdmin] = useMutation(UpdateAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })
  const [deleteAdminBulk] = useMutation(DeleteAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })

  async function onConfirmRowDelete() {
    if (!confirmingRowDelete) return
    await deleteAdmin({ variables: { id: confirmingRowDelete.id } })
  }

  const roleOptions = useMemo(() => {
    const seen = new Set<string>()
    for (const a of rows) for (const r of a.roles) seen.add(r.slug)
    return [...seen]
      .map((slug) => ({ label: slug, value: slug }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [rows])

  // Bulk actions are a config list - DataTable renders buttons + confirm dialogs uniformly - Superusers are ignored by
  // enable/disable since isActive doesn't apply to them in a meaningful way for this flow
  const bulkActions = useMemo<BulkAction<AdminRow>[] | undefined>(
    () => !canManage ? undefined : [
      {
        id: 'disable',
        label: 'Disable',
        icon: PowerOffIcon,
        variant: 'outline',
        disabled: (selected) => selected.every((a) => !a.isActive || a.isSuperuser),
        confirm: {
          title: (selected) =>
            `Disable ${selected.length} admin${selected.length === 1 ? '' : 's'}?`,
          description:
            'Disabled admins cannot sign in. Their existing sessions remain valid until the next token refresh.',
          actionLabel: 'Disable',
        },
        perform: async (selected) => {
          await Promise.all(
            selected
              .filter((a) => !a.isSuperuser && a.isActive)
              .map((a) => updateAdmin({ variables: { id: a.id, input: { isActive: false } } })),
          )
        },
      },
      {
        id: 'enable',
        label: 'Enable',
        icon: PowerIcon,
        variant: 'outline',
        disabled: (selected) => selected.every((a) => a.isActive || a.isSuperuser),
        perform: async (selected) => {
          await Promise.all(
            selected
              .filter((a) => !a.isSuperuser && !a.isActive)
              .map((a) => updateAdmin({ variables: { id: a.id, input: { isActive: true } } })),
          )
        },
      },
      {
        id: 'delete',
        label: 'Delete',
        icon: Trash2Icon,
        variant: 'destructive',
        confirm: {
          title: (selected) =>
            `Delete ${selected.length} admin${selected.length === 1 ? '' : 's'}?`,
          description:
            "Access is revoked immediately and any refresh tokens become invalid. This cannot be undone.",
          actionLabel: 'Delete',
        },
        perform: async (selected) => {
          await Promise.all(selected.map((a) => deleteAdminBulk({ variables: { id: a.id } })))
        },
      },
    ],
    [canManage, updateAdmin, deleteAdminBulk],
  )

  const columns = useMemo<ColumnDef<AdminRow>[]>(
    () => [
      // Selection only feeds the bulk actions
      ...(canManage ? [selectColumn<AdminRow>()] : []),
      {
        id: 'name',
        accessorFn: (row) => `${row.firstName} ${row.lastName}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <span className="inline-flex items-center gap-1.5">
            <Link
              to={`/admins/${row.original.id}`}
              className="font-medium hover:underline"
            >
              {row.original.firstName} {row.original.lastName}
            </Link>
            {row.original.id === myId && (
              <span className="text-xs text-muted-foreground">(you)</span>
            )}
          </span>
        ),
      },
      {
        accessorKey: 'email',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Email" />,
        cell: ({ row }) => <span className="text-muted-foreground">{row.original.email}</span>,
      },
      {
        id: 'status',
        accessorFn: (row) => statusOf(row),
        header: 'Status',
        size: 120,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(statusOf(row.original))
        },
        cell: ({ row }) => {
          const s = statusOf(row.original)
          if (s === 'superuser') return <Badge>superuser</Badge>
          if (s === 'inactive') return <Badge variant="outline" className="text-muted-foreground">inactive</Badge>
          return <Badge variant="secondary">active</Badge>
        },
      },
      {
        id: 'lastSeen',
        accessorFn: (row) => row.lastLoginAt ?? '',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Last seen" />,
        size: 130,
        // never signed in reads as a dash rather than a date - it's the state that matters
        cell: ({ row }) =>
          row.original.lastLoginAt ? (
            <span className="text-muted-foreground">
              {new Date(row.original.lastLoginAt).toLocaleDateString(undefined, { dateStyle: 'medium' })}
            </span>
          ) : (
            <span className="text-muted-foreground">-</span>
          ),
      },
      {
        id: 'roles',
        accessorFn: (row) => row.roles.map((r) => r.slug).join(' '),
        header: 'Roles',
        enableSorting: false,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const slugs = row.original.roles.map((r) => r.slug)
          return slugs.some((s) => value.includes(s))
        },
        cell: ({ row }) => {
          if (row.original.isSuperuser) {
            return <span className="text-xs text-muted-foreground">(bypass)</span>
          }
          if (row.original.roles.length === 0) {
            return <span className="text-xs text-muted-foreground">none</span>
          }
          return (
            <div className="flex flex-wrap gap-1.5">
              {row.original.roles.map((r) => (
                <Badge key={r.slug} variant="secondary">
                  {r.slug}
                </Badge>
              ))}
            </div>
          )
        },
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => {
          const isSelf = row.original.id === myId
          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${row.original.firstName}`}>
                  <MoreHorizontalIcon className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link to={`/admins/${row.original.id}`}>View</Link>
                </DropdownMenuItem>
                {canManage && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      variant="destructive"
                      disabled={isSelf}
                      onSelect={() => {
                        if (isSelf) return
                        setConfirmingRowDelete(row.original)
                      }}
                    >
                      {isSelf ? "Delete (can't delete yourself)" : 'Delete'}
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )
        },
      },
    ],
    [myId, canManage],
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
        enableRowSelection={(row) => row.original.id !== myId}
        emptyState="No admins yet."
        searchPlaceholder="Search name or email…"
        filters={[
          {
            column: 'status',
            title: 'Status',
            options: [
              { label: 'Active', value: 'active', icon: CheckCircle2Icon },
              { label: 'Inactive', value: 'inactive', icon: CircleIcon },
              { label: 'Superuser', value: 'superuser', icon: ShieldIcon },
            ],
          },
          ...(roleOptions.length > 0
            ? [{ column: 'roles', title: 'Roles', options: roleOptions }]
            : []),
        ]}
        bulkActions={bulkActions}
        toolbarRightSlot={
          canManage && (
            <AdminFormDialog
              trigger={
                <Button size="sm">
                  <PlusIcon className="size-4" />
                  New admin
                </Button>
              }
            />
          )
        }
      />

      <ConfirmDialog
        open={!!confirmingRowDelete}
        onOpenChange={(open) => !open && setConfirmingRowDelete(null)}
        title={<>Delete {confirmingRowDelete?.firstName} {confirmingRowDelete?.lastName}?</>}
        description="The admin's access is revoked immediately and any refresh tokens become invalid. This cannot be undone."
        confirmLabel="Delete"
        busyLabel="Deleting…"
        onConfirm={onConfirmRowDelete}
      />
    </>
  )
}
