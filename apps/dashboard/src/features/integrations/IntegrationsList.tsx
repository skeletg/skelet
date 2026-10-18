import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef, type Row } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  RefreshCwIcon,
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
import { Switch } from '@/components/ui/switch'
import {
  DataTable,
  DataTableColumnHeader,
  selectColumn,
  type BulkAction,
  useListUrlState,
} from '@/components/data-table'
import { IntegrationFormDialog, type ExistingIntegration } from './IntegrationFormDialog'
import { NewTokenDialog } from './NewTokenDialog'
import {
  DeleteIntegrationDocument,
  IntegrationsDocument,
  RegenerateIntegrationTokenDocument,
  SetIntegrationActiveDocument,
} from './Integrations.gql'

type IntegrationRow = Omit<ExistingIntegration, 'permissions'> & {
  isActive: boolean
  createdAt: string
  permissions: { id: string; slug: string; name: string }[]
}

function formatExpiry(iso: string | null | undefined): { label: string; stale: boolean } {
  if (!iso) return { label: 'never', stale: false }
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return { label: iso, stale: false }
  const stale = d.getTime() < Date.now()
  return { label: d.toLocaleString(), stale }
}

export function IntegrationsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { data, loading, error } = useQuery(IntegrationsDocument)

  const [confirmingRowDelete, setConfirmingRowDelete] = useState<IntegrationRow | null>(null)
  const [rowDeleteError, setRowDeleteError] = useState<string | null>(null)
  const [newToken, setNewToken] = useState<{ token: string; name: string } | null>(null)

  const [setActive, { loading: toggling }] = useMutation(SetIntegrationActiveDocument, {
    refetchQueries: [IntegrationsDocument],
  })
  const [regenerate] = useMutation(RegenerateIntegrationTokenDocument, {
    refetchQueries: [IntegrationsDocument],
  })
  const [deleteIntegration, { loading: deletingRow }] = useMutation(DeleteIntegrationDocument, {
    refetchQueries: [IntegrationsDocument],
  })

  const onToggleActive = useCallback(
    async (row: IntegrationRow, active: boolean) => {
      try {
        await setActive({ variables: { id: row.id, active } })
      } catch {
        /* noop */
      }
    },
    [setActive],
  )

  const onRegenerate = useCallback(
    async (row: IntegrationRow) => {
      try {
        const { data } = await regenerate({ variables: { id: row.id } })
        if (data) {
          setNewToken({
            token: data.regenerateIntegrationToken.rawToken,
            name: data.regenerateIntegrationToken.integration.name,
          })
        }
      } catch {
        /* noop */
      }
    },
    [regenerate],
  )

  async function onConfirmRowDelete() {
    if (!confirmingRowDelete) return
    setRowDeleteError(null)
    try {
      await deleteIntegration({ variables: { id: confirmingRowDelete.id } })
      setConfirmingRowDelete(null)
    } catch (err) {
      setRowDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const bulkActions = useMemo<BulkAction<IntegrationRow>[]>(
    () => [
      {
        id: 'disable',
        label: 'Disable',
        icon: PowerOffIcon,
        variant: 'outline',
        disabled: (rows) => rows.every((r) => !r.isActive),
        confirm: {
          title: (rows) => `Disable ${rows.length} integration${rows.length === 1 ? '' : 's'}?`,
          description:
            "Clients calling the API with these tokens will start getting 401 responses until you re-enable.",
          actionLabel: 'Disable',
        },
        perform: async (rows) => {
          await Promise.all(
            rows
              .filter((r) => r.isActive)
              .map((r) => setActive({ variables: { id: r.id, active: false } })),
          )
        },
      },
      {
        id: 'enable',
        label: 'Enable',
        icon: PowerIcon,
        variant: 'outline',
        disabled: (rows) => rows.every((r) => r.isActive),
        perform: async (rows) => {
          await Promise.all(
            rows
              .filter((r) => !r.isActive)
              .map((r) => setActive({ variables: { id: r.id, active: true } })),
          )
        },
      },
      {
        id: 'delete',
        label: 'Delete',
        icon: Trash2Icon,
        variant: 'destructive',
        confirm: {
          title: (rows) => `Delete ${rows.length} integration${rows.length === 1 ? '' : 's'}?`,
          description:
            'Clients calling the API with these tokens will receive 401 responses immediately. This cannot be undone.',
          actionLabel: 'Delete',
        },
        perform: async (rows) => {
          await Promise.all(rows.map((r) => deleteIntegration({ variables: { id: r.id } })))
        },
      },
    ],
    [setActive, deleteIntegration],
  )

  const columns = useMemo<ColumnDef<IntegrationRow>[]>(
    () => [
      selectColumn<IntegrationRow>(),
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link
            to={`/integrations/${row.original.id}`}
            className="font-medium hover:underline"
          >
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'description',
        header: 'Description',
        cell: ({ row }) => (
          <span className="block max-w-xs truncate text-muted-foreground">
            {row.original.description || <span className="italic">-</span>}
          </span>
        ),
      },
      {
        accessorKey: 'isActive',
        header: 'Active',
        size: 80,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const label = row.original.isActive ? 'active' : 'inactive'
          return value.includes(label)
        },
        cell: ({ row }) => (
          <Switch
            checked={row.original.isActive}
            disabled={toggling}
            onCheckedChange={(v) => onToggleActive(row.original, v)}
            aria-label={`Toggle ${row.original.name}`}
          />
        ),
      },
      {
        accessorKey: 'expiresAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Expires" />,
        sortingFn: (a: Row<IntegrationRow>, b: Row<IntegrationRow>) => {
          const av = a.original.expiresAt ? new Date(a.original.expiresAt).getTime() : Number.POSITIVE_INFINITY
          const bv = b.original.expiresAt ? new Date(b.original.expiresAt).getTime() : Number.POSITIVE_INFINITY
          return av - bv
        },
        cell: ({ row }) => {
          const e = formatExpiry(row.original.expiresAt)
          return (
            <span className={e.stale ? 'text-destructive' : 'text-muted-foreground'}>
              {e.label}
              {e.stale && ' (expired)'}
            </span>
          )
        },
      },
      {
        accessorKey: 'permissions',
        header: 'Permissions',
        enableSorting: false,
        enableGlobalFilter: false,
        cell: ({ row }) =>
          row.original.permissions.length === 0 ? (
            <span className="text-xs text-muted-foreground">none</span>
          ) : (
            <div className="flex flex-wrap gap-1">
              {row.original.permissions.map((p) => (
                <Badge key={p.id} variant="outline" className="font-mono text-xs" title={p.name}>
                  {p.slug}
                </Badge>
              ))}
            </div>
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
              <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${row.original.name}`}>
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/integrations/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onRegenerate(row.original)}>
                <RefreshCwIcon className="size-4" />
                Regenerate token
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
      },
    ],
    [toggling, onRegenerate, onToggleActive],
  )

  const rows = (data?.integrations ?? []) as IntegrationRow[]

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No integrations yet."
        searchPlaceholder="Search name or description…"
        filters={[
          {
            column: 'isActive',
            title: 'Status',
            options: [
              { label: 'Active', value: 'active', icon: CheckCircle2Icon },
              { label: 'Inactive', value: 'inactive', icon: CircleIcon },
            ],
          },
        ]}
        bulkActions={bulkActions}
        toolbarRightSlot={
          <IntegrationFormDialog
            trigger={
              <Button size="sm">
                <PlusIcon className="size-4" />
                New integration
              </Button>
            }
            onCreated={setNewToken}
          />
        }
      />

      <NewTokenDialog
        token={newToken?.token ?? null}
        integrationName={newToken?.name ?? null}
        open={!!newToken}
        onClose={() => setNewToken(null)}
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
            <AlertDialogTitle>Delete {confirmingRowDelete?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Any client calling the API with this integration's token will receive 401 responses
              immediately. This cannot be undone.
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
              {deletingRow ? 'Deleting…' : 'Delete integration'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
