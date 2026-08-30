import { useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MailCheckIcon,
  MailXIcon,
  MoreHorizontalIcon,
  ArrowUpDownIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  Trash2Icon,
} from 'lucide-react'
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
import {
  DataTable,
  DataTableColumnHeader,
  selectColumn,
  useListUrlState,
  type BulkAction,
} from '@/components/data-table'
type ExistingCustomer = {
  id: string
  email: string
  firstName: string
  lastName: string
  phone?: string | null
  isActive: boolean
  isEmailVerified: boolean
  group: { id: string; slug: string; name: string }
}
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CustomersPagedDocument,
  DeleteCustomerDocument,
  SetCustomerActiveDocument,
} from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'

type CustomerRow = ExistingCustomer & { createdAt: string }

// Reads the tri-state the server wants out of a multi-select facet - both or neither selected
// means "don't filter"
function activeFilter(values: string[] | undefined): boolean | null {
  if (!values || values.length !== 1) return null
  return values[0] === 'active'
}

export function CustomersList() {
  // Header scope is the sole source of truth: in CHANNEL mode the list is
  // scoped to that store's customers; in GLOBAL mode every customer across
  // every store is shown
  // Channel scope rides the `X-Channel-Id` header (see lib/apollo.ts), not a
  // query var. effectiveChannelId is still read below for UI affordances
  const { effectiveChannelId } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('customers.manage')

  // Server-driven paging/search/filtering - the full table is far too big to slice in the browser
  // URL-backed, so a filtered list survives a detail-page round trip and can be shared
  const {
    globalFilter: search,
    setGlobalFilter: setSearch,
    columnFilters,
    setColumnFilters,
    pagination,
    setPagination,
  } = useListUrlState()
  const debouncedSearch = useDebounced(search.trim(), 300)

  const statusValues = columnFilters.find((f) => f.id === 'status')?.value as string[] | undefined
  const groupValues = columnFilters.find((f) => f.id === 'group')?.value as string[] | undefined

  const variables = useMemo(
    () => ({
      search: debouncedSearch || null,
      groupSlug: groupValues?.[0] ?? null,
      isActive: activeFilter(statusValues),
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    }),
    [debouncedSearch, groupValues, statusValues, pagination.pageIndex, pagination.pageSize],
  )

  const { data, loading, error } = useQuery(CustomersPagedDocument, {
    variables,
    fetchPolicy: 'cache-and-network',
    notifyOnNetworkStatusChange: true,
  })
  const rows = (data?.customersPaged.items ?? []) as CustomerRow[]
  const totalCount = data?.customersPaged.totalCount ?? 0

  // A page of results can't enumerate the groups, so the facet reads the group list itself
  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groupOptions = useMemo(
    () =>
      (groupsQuery.data?.customerGroups ?? [])
        .map((g) => ({ label: g.name, value: g.slug }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [groupsQuery.data],
  )

  const [confirmingRowDelete, setConfirmingRowDelete] = useState<CustomerRow | null>(null)
  const [rowDeleteError, setRowDeleteError] = useState<string | null>(null)

  const refetchQueries = [{ query: CustomersPagedDocument, variables }]
  const [setActive] = useMutation(SetCustomerActiveDocument, { refetchQueries })
  const [deleteCustomer, { loading: deletingRow }] = useMutation(DeleteCustomerDocument, { refetchQueries })
  const [deleteCustomerBulk] = useMutation(DeleteCustomerDocument, { refetchQueries })

  async function onConfirmRowDelete() {
    if (!confirmingRowDelete) return
    setRowDeleteError(null)
    try {
      await deleteCustomer({ variables: { id: confirmingRowDelete.id } })
      setConfirmingRowDelete(null)
    } catch (err) {
      setRowDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const bulkActions = useMemo<BulkAction<CustomerRow>[]>(
    () =>
      canManage
        ? [
            {
              id: 'disable',
              label: 'Disable',
              icon: PowerOffIcon,
              variant: 'outline',
              disabled: (selected) => selected.every((c) => !c.isActive),
              confirm: {
                title: (selected) => `Disable ${selected.length} customer${selected.length === 1 ? '' : 's'}?`,
                description: 'Disabled customers cannot sign in until re-enabled.',
                actionLabel: 'Disable',
              },
              perform: async (selected) => {
                await Promise.all(
                  selected
                    .filter((c) => c.isActive)
                    .map((c) => setActive({ variables: { id: c.id, active: false } })),
                )
              },
            },
            {
              id: 'enable',
              label: 'Enable',
              icon: PowerIcon,
              variant: 'outline',
              disabled: (selected) => selected.every((c) => c.isActive),
              perform: async (selected) => {
                await Promise.all(
                  selected
                    .filter((c) => !c.isActive)
                    .map((c) => setActive({ variables: { id: c.id, active: true } })),
                )
              },
            },
            {
              id: 'delete',
              label: 'Delete',
              icon: Trash2Icon,
              variant: 'destructive',
              confirm: {
                title: (selected) => `Delete ${selected.length} customer${selected.length === 1 ? '' : 's'}?`,
                description:
                  "Addresses and carts cascade. Customers with orders will fail to delete - the backend rejects those to protect order history.",
                actionLabel: 'Delete',
              },
              perform: async (selected) => {
                await Promise.all(selected.map((c) => deleteCustomerBulk({ variables: { id: c.id } })))
              },
            },
          ]
        : [],
    [canManage, setActive, deleteCustomerBulk],
  )

  const columns = useMemo<ColumnDef<CustomerRow>[]>(
    () => [
      ...(canManage ? [selectColumn<CustomerRow>()] : []),
      {
        id: 'name',
        accessorFn: (row) => `${row.firstName} ${row.lastName}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link
            to={`/customers/${row.original.id}`}
            className="font-medium hover:underline"
          >
            {row.original.firstName} {row.original.lastName}
          </Link>
        ),
      },
      {
        accessorKey: 'email',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Email" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-1.5 text-muted-foreground">
            <span>{row.original.email}</span>
            {row.original.isEmailVerified ? (
              <MailCheckIcon className="size-3.5" aria-label="Verified" />
            ) : (
              <MailXIcon className="size-3.5" aria-label="Unverified" />
            )}
          </div>
        ),
      },
      {
        id: 'group',
        accessorFn: (row) => row.group.slug,
        header: 'Group',
        size: 140,
        cell: ({ row }) => (
          <Badge variant="secondary" title={row.original.group.slug}>
            {row.original.group.name}
          </Badge>
        ),
      },
      {
        id: 'status',
        accessorFn: (row) => (row.isActive ? 'active' : 'inactive'),
        header: 'Status',
        size: 110,
        cell: ({ row }) =>
          row.original.isActive ? (
            <Badge variant="secondary">active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">
              inactive
            </Badge>
          ),
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Joined" />,
        size: 160,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleDateString()}
          </span>
        ),
      },
      ...(canManage
        ? [
            {
              id: 'actions',
              size: 48,
              enableSorting: false,
              enableHiding: false,
              cell: ({ row }) => (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${row.original.firstName}`}>
                      <MoreHorizontalIcon className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem asChild>
                      <Link to={`/customers/${row.original.id}`}>View</Link>
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() =>
                        setActive({ variables: { id: row.original.id, active: !row.original.isActive } })
                      }
                    >
                      {row.original.isActive ? (
                        <>
                          <PowerOffIcon className="size-4" />
                          Disable
                        </>
                      ) : (
                        <>
                          <PowerIcon className="size-4" />
                          Enable
                        </>
                      )}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      variant="destructive"
                      onSelect={() => {
                        setRowDeleteError(null)
                        setConfirmingRowDelete(row.original)
                      }}
                    >
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ),
            } satisfies ColumnDef<CustomerRow>,
          ]
        : []),
    ],
    [canManage, setActive],
  )

  return (
    <>
      <DataTable
        prefsKey="customers.list"
        viewsKey="customers.list"
        enableColumnControls
        enableDensityToggle
        columns={columns}
        data={rows}
        loading={loading && rows.length === 0}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No customers yet."
        searchPlaceholder="Search name or email…"
        manual={{
          rowCount: totalCount,
          pagination,
          onPaginationChange: setPagination,
          globalFilter: search,
          onGlobalFilterChange: setSearch,
          columnFilters,
          // The server takes a single groupSlug, so the group facet behaves single-select -
          // keep the most recently picked slug
          onColumnFiltersChange: (next) =>
            setColumnFilters(
              next.map((f) =>
                f.id === 'group' && Array.isArray(f.value) && f.value.length > 1
                  ? { ...f, value: [f.value[f.value.length - 1]] }
                  : f,
              ),
            ),
        }}
        filters={[
          {
            column: 'status',
            title: 'Status',
            options: [
              { label: 'Active', value: 'active', icon: CheckCircle2Icon },
              { label: 'Inactive', value: 'inactive', icon: CircleIcon },
            ],
          },
          ...(groupOptions.length > 0
            ? [{ column: 'group', title: 'Group', options: groupOptions }]
            : []),
        ]}
        bulkActions={bulkActions}
        toolbarRightSlot={
          <>
          {canManage && (
            <Button variant="outline" size="sm" asChild>
              <Link to="/data?resource=customers">
                <ArrowUpDownIcon className="size-4" />
                Import / export
              </Link>
            </Button>
          )}
          {canManage &&
            (effectiveChannelId ? (
              <Button size="sm" asChild>
                <Link to="/customers/new">
                  <PlusIcon className="size-4" />
                  New customer
                </Link>
              </Button>
            ) : (
              <Button size="sm" disabled title="Pick a channel in the header to create a customer.">
                <PlusIcon className="size-4" />
                New customer
              </Button>
            ))}
          </>
        }
      />

      <AlertDialog
        open={!!confirmingRowDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingRowDelete(null)
            setRowDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {confirmingRowDelete?.firstName} {confirmingRowDelete?.lastName}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Addresses and carts cascade. Customers with orders can't be deleted - the backend
              rejects the request to preserve order history.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {rowDeleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{rowDeleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingRow}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deletingRow}
              onClick={(e) => {
                e.preventDefault()
                onConfirmRowDelete()
              }}
            >
              {deletingRow ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
