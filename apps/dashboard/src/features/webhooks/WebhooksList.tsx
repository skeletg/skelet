import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
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
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import type { WebhookKind } from '@/graphql/schema.types'
import { WebhookKindBadge } from './WebhookKindBadge'
import {
  DeleteWebhookDocument,
  WebhooksDocument,
} from './Webhooks.gql'

type WebhookRow = {
  id: string
  name: string
  targetUrl: string
  kind: WebhookKind
  isActive: boolean
  eventTypes: string[]
  createdAt: string
  channel?: { id: string; slug: string; name: string } | null
}

export function WebhooksList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('webhooks.manage')

  const { data, loading, error } = useQuery(WebhooksDocument)
  const rows = useMemo(() => (data?.webhooks ?? []) as WebhookRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<WebhookRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteWebhook, { loading: deleting }] = useMutation(DeleteWebhookDocument, {
    refetchQueries: [{ query: WebhooksDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteWebhook({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<WebhookRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link
            to={`/webhooks/${row.original.id}`}
            className="font-medium hover:underline"
          >
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'targetUrl',
        header: 'Target URL',
        cell: ({ row }) => (
          <code className="block max-w-md truncate font-mono text-xs text-muted-foreground">
            {row.original.targetUrl}
          </code>
        ),
      },
      {
        accessorKey: 'kind',
        header: 'Kind',
        size: 90,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.kind)
        },
        cell: ({ row }) => <WebhookKindBadge kind={row.original.kind} />,
      },
      {
        id: 'events',
        header: 'Events',
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.eventTypes.length} subscribed
          </span>
        ),
      },
      {
        id: 'store',
        header: 'Store',
        enableSorting: false,
        cell: ({ row }) =>
          row.original.channel ? (
            <Badge variant="secondary">{row.original.channel.name}</Badge>
          ) : (
            <span className="text-xs text-muted-foreground">All stores</span>
          ),
      },
      {
        accessorKey: 'isActive',
        header: 'Status',
        size: 100,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const label = row.original.isActive ? 'active' : 'inactive'
          return value.includes(label)
        },
        cell: ({ row }) =>
          row.original.isActive ? (
            <span className="inline-flex items-center gap-1.5 text-xs">
              <span className="size-1.5 rounded-full bg-emerald-500" /> Active
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="size-1.5 rounded-full bg-muted-foreground/40" /> Inactive
            </span>
          ),
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Created" />,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleDateString()}
          </span>
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
                <Link to={`/webhooks/${row.original.id}`}>View</Link>
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
                    <Trash2Icon className="size-4" />
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
        emptyState="No webhooks yet."
        searchPlaceholder="Search name or URL…"
        initialSorting={[{ id: 'createdAt', desc: true }]}
        filters={[
          {
            column: 'kind',
            title: 'Kind',
            options: [
              { label: 'Async', value: 'ASYNC' },
              { label: 'Sync', value: 'SYNC' },
            ],
          },
          {
            column: 'isActive',
            title: 'Status',
            options: [
              { label: 'Active', value: 'active', icon: CheckCircle2Icon },
              { label: 'Inactive', value: 'inactive', icon: CircleIcon },
            ],
          },
        ]}
        toolbarRightSlot={
          canManage ? (
            <Button size="sm" asChild>
              <Link to="/webhooks/new">
                <PlusIcon className="size-4" />
                New webhook
              </Link>
            </Button>
          ) : null
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
              This will permanently delete the webhook and all its delivery history.
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
              {deleting ? 'Deleting…' : 'Delete webhook'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
