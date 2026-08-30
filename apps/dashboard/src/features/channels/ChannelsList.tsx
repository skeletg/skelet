import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  StarIcon,
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
import {
  ChannelsDocument,
  DeleteChannelDocument,
} from './Channels.gql'
import { NewChannelDialog } from './NewChannelDialog'

type LocaleSummary = {
  code: string
  name: string
  nativeName: string
  isActive: boolean
  isRTL: boolean
  sortOrder: number
}

type ChannelRow = {
  id: string
  slug: string
  name: string
  defaultLocale: string
  locales: LocaleSummary[]
  isActive: boolean
  isDefault: boolean
  createdAt: string
  currencyCode: string
  publicUrl?: string | null
}

export function ChannelsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canView = has('channels.view')
  const canManage = has('channels.manage')

  const { data, loading, error } = useQuery(ChannelsDocument, { skip: !canView })
  const rows = useMemo(() => (data?.channels ?? []) as ChannelRow[], [data])

  const [creating, setCreating] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState<ChannelRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteChannel, { loading: deleting }] = useMutation(DeleteChannelDocument, {
    refetchQueries: [{ query: ChannelsDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteChannel({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<ChannelRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Link
              to={`/channels/${row.original.id}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
            {row.original.isDefault && (
              <Badge variant="secondary" className="gap-1 font-normal">
                <StarIcon className="size-3" />
                default
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
        accessorKey: 'currencyCode',
        header: 'Currency',
        size: 90,
        cell: ({ row }) => (
          <Badge variant="outline" className="font-mono text-xs font-normal">
            {row.original.currencyCode}
          </Badge>
        ),
      },
      {
        accessorKey: 'defaultLocale',
        header: 'Default locale',
        cell: ({ row }) => (
          <span className="font-mono text-xs">{row.original.defaultLocale}</span>
        ),
      },
      {
        id: 'locales',
        header: 'Locales',
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.locales.length}
          </span>
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
            <Badge variant="secondary">Active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">Inactive</Badge>
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
                <Link to={`/channels/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              {canManage && !row.original.isDefault && (
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

  if (!canView) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Access denied</AlertTitle>
        <AlertDescription>
          You need the `channels.view` permission to view channels.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No channels yet."
        searchPlaceholder="Search name, slug, currency…"
        initialSorting={[{ id: 'name', desc: false }]}
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
        toolbarRightSlot={
          canManage ? (
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon className="size-4" />
              New channel
            </Button>
          ) : null
        }
      />

      <NewChannelDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(id) => navigate(`/channels/${id}`)}
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
              This permanently removes the channel and its locale configuration.
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
              {deleting ? 'Deleting…' : 'Delete channel'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
