import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon } from 'lucide-react'

import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
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
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatCents } from '@/lib/format'
import type { SubscriptionStatus } from '@/graphql/schema.types'
import {
  CancelSubscriptionDocument,
  PauseSubscriptionDocument,
  ResumeSubscriptionDocument,
  SubscriptionsDocument,
  type SubscriptionsQuery,
} from './Subscriptions.gql'

type SubscriptionRow = SubscriptionsQuery['subscriptionsPaged']['items'][number]

const STATUS_FILTERS = ['ALL', 'ACTIVE', 'PAUSED', 'CANCELED', 'EXPIRED'] as const

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'ACTIVE':
      return 'default'
    case 'CANCELED':
    case 'EXPIRED':
      return 'destructive'
    default: // PAUSED
      return 'secondary'
  }
}

function intervalLabel(count: number, unit: string): string {
  const u = unit.toLowerCase()
  return count === 1 ? `every ${u}` : `every ${count} ${u}s`
}

export function SubscriptionsList() {
  const [statusFilter, setStatusFilter] = useState<(typeof STATUS_FILTERS)[number]>('ALL')
  // Server-paged - the page lives in the URL like the other lists
  const urlState = useListUrlState()
  const { pagination, setPagination } = urlState
  const { data, loading, error } = useQuery(SubscriptionsDocument, {
    variables: {
      status: statusFilter === 'ALL' ? undefined : (statusFilter as SubscriptionStatus),
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
  })
  const rows = useMemo(() => data?.subscriptionsPaged.items ?? [], [data])

  const refetchQueries = [SubscriptionsDocument]
  const [cancelSub] = useMutation(CancelSubscriptionDocument, { refetchQueries })
  const [pauseSub] = useMutation(PauseSubscriptionDocument, { refetchQueries })
  const [resumeSub] = useMutation(ResumeSubscriptionDocument, { refetchQueries })

  const [canceling, setCanceling] = useState<SubscriptionRow | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  async function run(fn: () => Promise<unknown>) {
    setActionError(null)
    try {
      await fn()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Action failed.')
    }
  }

  const columns = useMemo<ColumnDef<SubscriptionRow, unknown>[]>(
    () => [
      {
        accessorKey: 'title',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Product" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">{row.original.title || '-'}</p>
            {row.original.sku && (
              <p className="font-mono text-xs text-muted-foreground">{row.original.sku}</p>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        size: 110,
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)} className="font-normal">
            {row.original.status.toLowerCase()}
          </Badge>
        ),
      },
      {
        id: 'billing',
        header: 'Billing',
        size: 150,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {intervalLabel(row.original.intervalCount, row.original.intervalUnit)}
          </span>
        ),
      },
      {
        accessorKey: 'price',
        header: 'Price',
        size: 120,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {formatCents(row.original.price, row.original.currencyCode)}
          </span>
        ),
      },
      {
        accessorKey: 'nextBillingAt',
        header: 'Next billing',
        size: 140,
        cell: ({ row }) =>
          row.original.status === 'ACTIVE' ? (
            <span className="text-sm">
              {new Date(row.original.nextBillingAt).toLocaleDateString()}
            </span>
          ) : (
            <span className="text-sm text-muted-foreground">-</span>
          ),
      },
      {
        id: 'actions',
        size: 50,
        cell: ({ row }) => {
          const s = row.original
          const isCanceled = s.status === 'CANCELED' || s.status === 'EXPIRED'
          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="size-7">
                  <MoreHorizontalIcon className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {s.status === 'ACTIVE' && (
                  <DropdownMenuItem onClick={() => run(() => pauseSub({ variables: { id: s.id } }))}>
                    Pause
                  </DropdownMenuItem>
                )}
                {s.status === 'PAUSED' && (
                  <DropdownMenuItem onClick={() => run(() => resumeSub({ variables: { id: s.id } }))}>
                    Resume
                  </DropdownMenuItem>
                )}
                {!isCanceled && (
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => {
                      setActionError(null)
                      setCanceling(s)
                    }}
                  >
                    Cancel
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )
        },
      },
    ],
    [pauseSub, resumeSub],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Subscriptions"
        meta="Recurring customer subscriptions. Billing advances automatically; invoices stay due until payment capture is wired."
        actions={
          <Select
            value={statusFilter}
            onValueChange={(v) => {
              setStatusFilter(v as typeof statusFilter)
              setPagination({ ...pagination, pageIndex: 0 })
            }}
          >
            <SelectTrigger className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATUS_FILTERS.map((s) => (
                <SelectItem key={s} value={s}>
                  {s === 'ALL' ? 'All statuses' : s.charAt(0) + s.slice(1).toLowerCase()}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          {actionError && (
            <Alert variant="destructive">
              <AlertTitle>Action failed</AlertTitle>
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          )}

          {error ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't load subscriptions</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          ) : (
            <DataTable
              columns={columns}
              data={rows}
              loading={loading && rows.length === 0}
              manual={{
                rowCount: data?.subscriptionsPaged.totalCount ?? 0,
                pagination,
                onPaginationChange: setPagination,
                globalFilter: urlState.globalFilter,
                onGlobalFilterChange: urlState.setGlobalFilter,
                columnFilters: urlState.columnFilters,
                onColumnFiltersChange: urlState.setColumnFilters,
              }}
            />
          )}
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog open={Boolean(canceling)} onOpenChange={(v) => { if (!v) setCanceling(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel subscription?</AlertDialogTitle>
            <AlertDialogDescription>
              Stops future billing for{' '}
              <span className="font-medium">{canceling?.title}</span>. Already-issued invoices are
              kept. This can't be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep active</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                const s = canceling
                setCanceling(null)
                if (s) run(() => cancelSub({ variables: { id: s.id } }))
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Cancel subscription
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}
