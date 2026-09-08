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
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { DeleteExperimentDocument, ExperimentsDocument } from './Prism.signals.gql'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  running: 'default',
  stopped: 'secondary',
}

type Row = {
  id: string
  key: string
  name: string
  status: string
  coverage: number
  goalEvent: string
  variations: Array<{ key: string; name: string; weight: number; value: string }>
  createdAt: string
  updatedAt: string
}

export function PrismList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('experiments.manage')
  const { data, loading, error } = useQuery(ExperimentsDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const rows = useMemo(() => (data?.experiments ?? []) as Row[], [data?.experiments])

  const [confirmingDelete, setConfirmingDelete] = useState<Row | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteExp, { loading: deleting }] = useMutation(DeleteExperimentDocument, {
    refetchQueries: [{ query: ExperimentsDocument, variables: { channelId: effectiveChannelId ?? '' } }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteExp({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (r) => `${r.name} ${r.key}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/prism/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name || row.original.key}
          </Link>
        ),
      },
      {
        accessorKey: 'key',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Key" />,
        cell: ({ row }) => <code className="font-mono text-xs text-muted-foreground">{row.original.key}</code>,
      },
      {
        id: 'status',
        accessorKey: 'status',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (row, _colId, value: string[]) => !value?.length || value.includes(row.original.status),
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANT[row.original.status] ?? 'outline'} className="capitalize">
            {row.original.status}
          </Badge>
        ),
      },
      {
        id: 'variations',
        accessorFn: (r) => r.variations.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Variations" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.variations.length}</span>,
      },
      {
        accessorKey: 'coverage',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Coverage" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.coverage}%</span>,
      },
      {
        accessorKey: 'goalEvent',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Goal event" />,
        cell: ({ row }) =>
          row.original.goalEvent ? (
            <code className="font-mono text-xs">{row.original.goalEvent}</code>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => new Date(row.original.updatedAt).toLocaleDateString(),
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${row.original.name || row.original.key}`}>
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/prism/${row.original.id}`}>Open</Link>
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
    <DetailPageLayout>
      <DetailPageHeader
        title="Experiments"
        backTo="/prism"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            All Prism experiments for this channel - drafts, running, and stopped.
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Experiments are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection>
            <DataTable
              urlState={urlState}
              columns={columns}
              data={rows}
              loading={loading && !data}
              error={error ? { message: error.message } : null}
              getRowId={(r) => r.id}
              emptyState="No experiments yet."
              searchPlaceholder="Search name or key…"
              initialSorting={[{ id: 'updatedAt', desc: true }]}
              filters={[
                {
                  column: 'status',
                  title: 'Status',
                  options: [
                    { label: 'Running', value: 'running' },
                    { label: 'Draft', value: 'draft' },
                    { label: 'Stopped', value: 'stopped' },
                  ],
                },
              ]}
              toolbarRightSlot={
                canManage && (
                  <Button size="sm" asChild>
                    <Link to="/prism/new">
                      <PlusIcon className="size-4" />
                      New experiment
                    </Link>
                  </Button>
                )
              }
            />
          </DetailPageSection>
        )}
      </DetailPageBody>

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
            <AlertDialogTitle>Delete {confirmingDelete?.name || confirmingDelete?.key || 'experiment'}?</AlertDialogTitle>
            <AlertDialogDescription>
              Permanently removes the experiment and its configuration. Recorded exposures and conversions stay on the
              analytics spine but will no longer be attributed to this experiment.
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
              {deleting ? 'Deleting…' : 'Delete experiment'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}
