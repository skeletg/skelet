import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  AlertTriangleIcon,
  DownloadIcon,
  MoreHorizontalIcon,
  PlusIcon,
  SendIcon,
  Trash2Icon,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
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
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { CustomerSegmentsListDocument } from '@/features/promotions/Promotions.gql'
import { DestinationKindBadge } from './DestinationKindBadge'
import {
  DestinationFormDialog,
  type ExistingDestination,
} from './DestinationFormDialog'
import {
  DeleteEventDestinationDocument,
  EventDestinationsDocument,
  SyncAudienceDocument,
} from './Destinations.gql'

type DestinationRow = ExistingDestination & {
  lastError: string
  lastForwardedAt?: string | null
  createdAt: string
}

export function DestinationsPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, channels } = useConfigScope()
  // Destinations are strictly channel-scoped: fall back to the first
  // active channel when the header scope is global
  const channelId = effectiveChannelId ?? channels[0]?.id ?? ''

  const { data, loading, error, refetch } = useQuery(EventDestinationsDocument, {
    variables: { channelId },
    skip: !channelId,
  })
  const rows = useMemo(() => (data?.eventDestinations ?? []) as DestinationRow[], [data])

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<DestinationRow | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState<DestinationRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteDestination, { loading: deleting }] = useMutation(DeleteEventDestinationDocument)

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteDestination({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
      void refetch()
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  function openEdit(row: DestinationRow) {
    setEditing(row)
    setFormOpen(true)
  }

  const columns = useMemo<ColumnDef<DestinationRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <button
            type="button"
            className="font-medium hover:underline"
            onClick={() => openEdit(row.original)}
          >
            {row.original.name}
          </button>
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
        cell: ({ row }) => <DestinationKindBadge kind={row.original.kind} />,
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
        id: 'filters',
        header: 'Filters',
        enableSorting: false,
        cell: ({ row }) => {
          const parts: string[] = []
          const { eventSources, eventNames } = row.original
          if (eventSources.length > 0) {
            parts.push(`${eventSources.length} source${eventSources.length === 1 ? '' : 's'}`)
          }
          if (eventNames.length > 0) {
            parts.push(`${eventNames.length} event${eventNames.length === 1 ? '' : 's'}`)
          }
          return (
            <span className="text-xs text-muted-foreground">
              {parts.length > 0 ? parts.join(' · ') : 'All events'}
            </span>
          )
        },
      },
      {
        accessorKey: 'isActive',
        header: 'Status',
        size: 100,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.isActive ? 'active' : 'inactive')
        },
        cell: ({ row }) =>
          row.original.isActive ? (
            <span className="inline-flex items-center gap-1.5 text-xs">
              <span className="size-1.5 rounded-full bg-success" /> Active
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="size-1.5 rounded-full bg-muted-foreground/40" /> Inactive
            </span>
          ),
      },
      {
        id: 'delivery',
        header: 'Delivery',
        enableSorting: false,
        cell: ({ row }) => {
          const { lastError, lastForwardedAt } = row.original
          if (lastError) {
            return (
              <span
                className="inline-flex max-w-xs items-center gap-1.5 text-xs text-destructive"
                title={lastError}
              >
                <AlertTriangleIcon className="size-3.5 shrink-0" />
                <span className="truncate">{lastError}</span>
              </span>
            )
          }
          if (lastForwardedAt) {
            return (
              <span className="text-xs text-muted-foreground">
                Last forwarded {new Date(lastForwardedAt).toLocaleString()}
              </span>
            )
          }
          return <span className="text-xs text-muted-foreground">Nothing forwarded yet</span>
        },
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
              <DropdownMenuItem onSelect={() => openEdit(row.original)}>Edit</DropdownMenuItem>
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
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [],
  )

  if (!channelId) {
    return (
      <DetailPageLayout>
        <DetailPageHeader
          title="Event destinations"
          meta="Pick a channel in the header switcher first."
        />
      </DetailPageLayout>
    )
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Event destinations"
        meta="Forward analytics events to external systems - generic signed webhooks, GA4 Measurement Protocol, or Meta Conversions API. A sweep runs every minute and delivers at-least-once in ingest order; delivery problems surface per destination until a sweep succeeds again."
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable
            urlState={urlState}
            columns={columns}
            data={rows}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(row) => row.id}
            emptyState="No destinations yet. Create one to start forwarding events - forwarding begins at creation, history is not replayed."
            searchPlaceholder="Search name or URL…"
            initialSorting={[{ id: 'name', desc: false }]}
            filters={[
              {
                column: 'kind',
                title: 'Kind',
                options: [
                  { label: 'Webhook', value: 'webhook' },
                  { label: 'GA4', value: 'ga4_mp' },
                  { label: 'Meta CAPI', value: 'meta_capi' },
                ],
              },
              {
                column: 'isActive',
                title: 'Status',
                options: [
                  { label: 'Active', value: 'active' },
                  { label: 'Inactive', value: 'inactive' },
                ],
              },
            ]}
            toolbarRightSlot={
              <Button
                size="sm"
                onClick={() => {
                  setEditing(null)
                  setFormOpen(true)
                }}
              >
                <PlusIcon className="size-4" />
                New destination
              </Button>
            }
          />
        </DetailPageSection>

        <AudienceSyncSection destinations={rows} />
      </DetailPageBody>

      <DestinationFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditing(null)
        }}
        channelId={channelId}
        existing={editing}
        onSaved={() => void refetch()}
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
              Forwarding stops immediately and the delivery cursor is lost - re-creating the
              destination later starts from that moment, not from where this one stopped.
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
              {deleting ? 'Deleting…' : 'Delete destination'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

// Push a customer segment's membership to a destination as SHA-256-hashed emails (the normalization Meta and Google
// Customer Match specify), or download the same hashes as a CSV for a manual upload
function AudienceSyncSection({ destinations }: { destinations: DestinationRow[] }) {
  const { data: segData } = useQuery(CustomerSegmentsListDocument, {
    variables: { limit: 200 },
  })
  const segments = segData?.customerSegments.items ?? []

  const [segmentId, setSegmentId] = useState('')
  const [destinationId, setDestinationId] = useState('')

  const [syncAudience, { loading: syncing }] = useMutation(SyncAudienceDocument)

  async function onSync() {
    if (!segmentId || !destinationId) return
    try {
      const { data } = await syncAudience({ variables: { segmentId, destinationId } })
      const destination = destinations.find((d) => d.id === destinationId)
      toast.success(
        `Sent ${data?.syncAudience ?? 0} hashed identifiers to ${destination?.name ?? 'destination'}.`,
      )
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Audience sync failed.')
    }
  }

  return (
    <DetailPageSection
      title="Audience sync"
      description="Push a customer segment's membership to a destination as SHA-256-hashed emails. For a manual Customer Match upload, download the hashes from Import & export. Raw emails never leave the store."
    >
      <div className="flex flex-wrap items-center gap-2">
        <Select value={segmentId} onValueChange={setSegmentId}>
          <SelectTrigger className="w-64">
            <SelectValue placeholder="Pick a segment" />
          </SelectTrigger>
          <SelectContent>
            {segments.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name} ({s.memberCount})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={destinationId} onValueChange={setDestinationId}>
          <SelectTrigger className="w-64">
            <SelectValue placeholder="Pick a destination" />
          </SelectTrigger>
          <SelectContent>
            {destinations.map((d) => (
              <SelectItem key={d.id} value={d.id}>
                {d.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          className="h-10"
          variant="outline"
          size="sm"
          disabled={!segmentId || !destinationId || syncing}
          onClick={() => void onSync()}
        >
          <SendIcon className="size-4" />
          {syncing ? 'Syncing…' : 'Sync now'}
        </Button>
        <Button variant="outline" size="sm" asChild>
          <Link to="/data?resource=audience">
            <DownloadIcon className="size-4" />
            Download hashes
          </Link>
        </Button>
      </div>
    </DetailPageSection>
  )
}
