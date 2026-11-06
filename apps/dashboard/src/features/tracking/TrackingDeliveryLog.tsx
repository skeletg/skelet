import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { RotateCcwIcon } from 'lucide-react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'
import type { TrackingDeliveryStatus } from '@/graphql/schema.types'
import { eventLabel } from './platformGuides'
import { RedeliverTrackingDeliveryDocument, TrackingDeliveriesDocument, type TrackingDeliveriesQuery } from './Tracking.gql'

type Delivery = TrackingDeliveriesQuery['trackingDeliveries'][number]

const FILTERS: { value: TrackingDeliveryStatus | null; label: string }[] = [
  { value: null, label: 'All' },
  { value: 'DEAD', label: 'Failed' },
  { value: 'PENDING', label: 'Waiting' },
]

const STATUS: Record<TrackingDeliveryStatus, { label: string; variant: 'warning' | 'destructive' }> = {
  PENDING: { label: 'Waiting', variant: 'warning' },
  DEAD: { label: 'Failed', variant: 'destructive' },
}

// What is still waiting or retrying and what gave up - a sent delivery is deleted, so a healthy log is empty. The event itself isn't shown - it holds the shopper's
// email, IP and click ids - only its name, id and the platform's last answer
export function TrackingDeliveryLog({ destinationId, canManage }: { destinationId: string; canManage: boolean }) {
  const [status, setStatus] = useState<TrackingDeliveryStatus | null>(null)
  const { data, loading, error, refetch } = useQuery(TrackingDeliveriesDocument, {
    variables: { destinationId, status, limit: 100 },
    pollInterval: useVisiblePollInterval(15000),
  })
  useRefetchWhenShown(refetch)
  const [redeliver] = useMutation(RedeliverTrackingDeliveryDocument)
  const [retrying, setRetrying] = useState<string | null>(null)
  const [retryError, setRetryError] = useState<string | null>(null)

  const columns = useMemo<ColumnDef<Delivery>[]>(
    () => [
      { id: 'event', header: 'Event', cell: ({ row }) => <span>{eventLabel(row.original.eventName)}</span> },
      {
        id: 'status',
        header: 'Status',
        cell: ({ row }) => {
          const s = STATUS[row.original.status]
          return <Badge variant={s.variant}>{s.label}</Badge>
        },
      },
      { accessorKey: 'attempts', header: 'Attempts' },
      {
        id: 'error',
        header: 'Last answer',
        cell: ({ row }) => <span className="line-clamp-2 max-w-md font-mono text-xs text-muted-foreground">{row.original.lastError || '-'}</span>,
      },
      { id: 'time', header: 'Time', cell: ({ row }) => new Date(row.original.createdAt).toLocaleString() },
      {
        id: 'retry',
        header: '',
        cell: ({ row }) =>
          canManage && row.original.status === 'DEAD' ? (
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1.5 text-xs"
              disabled={retrying === row.original.id}
              onClick={async (e) => {
                e.stopPropagation()
                setRetrying(row.original.id)
                setRetryError(null)
                try {
                  await redeliver({ variables: { id: row.original.id } })
                  await refetch()
                } catch (err) {
                  setRetryError(err instanceof Error ? err.message : 'Could not retry')
                } finally {
                  setRetrying(null)
                }
              }}
            >
              <RotateCcwIcon className="size-3" />
              Retry
            </Button>
          ) : null,
      },
    ],
    [canManage, redeliver, refetch, retrying],
  )

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap gap-2" role="group" aria-label="Filter deliveries">
        {FILTERS.map((f) => (
          <Button key={f.label} size="sm" variant={status === f.value ? 'secondary' : 'ghost'} aria-pressed={status === f.value} onClick={() => setStatus(f.value)}>
            {f.label}
          </Button>
        ))}
      </div>
      {retryError && <p className="text-sm text-destructive">{retryError}</p>}
      <DataTable
        columns={columns}
        data={data?.trackingDeliveries ?? []}
        loading={loading && !data}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState={status === 'DEAD' ? 'Nothing has failed.' : 'Nothing waiting or failed.'}
      />
      <p className="text-xs text-muted-foreground">Failed sends retry after 10s, 1m, 5m, 30m and 2h before giving up. Entries are kept for 30 days.</p>
    </div>
  )
}
