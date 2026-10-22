import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef, type Row } from '@/components/data-table/tanstack'
import { PlusIcon } from 'lucide-react'

import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import type { CartKind, CartOfferState } from '@/graphql/schema.types'
import { CartsDocument } from './Carts.gql'
import { NewCartDialog } from './NewCartDialog'
import { OfferStateBadge } from './offer/OfferStateBadge'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useDebounced } from '@/shared/hooks/useDebounced'

type CartRow = {
  id: string
  customerId?: string | null
  customerEmail?: string | null
  customerName?: string | null
  status: string
  kind: CartKind
  email: string
  offerState?: CartOfferState | null
  offerExpiresAt?: string | null
  itemCount: number
  subtotal: number
  currencyCode: string
  updatedAt: string
}

// The Order Manager's two lists - shoppers' own carts, and the offers admins build and send as a link. One table,
// shaped per kind: a cart has a status and goes abandoned, an offer has a state and an expiry
export function CartsList() {
  return <CartListPage kind="SHOPPER" />
}

export function OffersList() {
  return <CartListPage kind="OFFER" />
}

type CartStatus = 'active' | 'abandoned' | 'converted' | 'all'
type OfferFilter = 'open' | 'all' | CartOfferState

const OFFER_FILTERS: { value: OfferFilter; label: string }[] = [
  { value: 'open', label: 'Open' },
  { value: 'DRAFT', label: 'Draft' },
  { value: 'SENT', label: 'Sent' },
  { value: 'OPENED', label: 'Opened' },
  { value: 'COMPLETED', label: 'Completed' },
  { value: 'EXPIRED', label: 'Expired' },
  { value: 'REVOKED', label: 'Withdrawn' },
  { value: 'all', label: 'All' },
]
const OPEN_STATES: CartOfferState[] = ['DRAFT', 'SENT', 'OPENED']

function CartListPage({ kind }: { kind: CartKind }) {
  const isOffer = kind === 'OFFER'
  const base = isOffer ? '/offers' : '/carts'
  // Creating needs orders.update - without it the button only led to a refused save
  const canCreate = usePermissions().has('orders.update')
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, scope } = useConfigScope()
  const [status, setStatus] = useState<CartStatus>('active')
  const [offerFilter, setOfferFilter] = useState<OfferFilter>('open')
  const [newOpen, setNewOpen] = useState(false)

  const { pagination, setPagination, globalFilter: search } = urlState
  const debouncedSearch = useDebounced(search.trim(), 300)

  // Shopper carts grow forever, so the server pages and searches them. An offer's state reads off several
  // columns the server can't filter on, so the offers list loads every offer and narrows by state here
  // shortcut: offers cap at 200, upgrade with a server offerState filter once a channel sends that many
  const variables = useMemo(
    () => ({
      channelId: effectiveChannelId ?? '',
      kind,
      status: isOffer || status === 'all' || status === 'abandoned' ? null : status,
      abandoned: !isOffer && status === 'abandoned' ? true : null,
      search: isOffer ? null : debouncedSearch || null,
      limit: isOffer ? 200 : pagination.pageSize,
      offset: isOffer ? 0 : pagination.pageIndex * pagination.pageSize,
    }),
    [effectiveChannelId, kind, isOffer, status, debouncedSearch, pagination],
  )

  const { data, loading, error } = useQuery(CartsDocument, {
    variables,
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const rows = useMemo(() => {
    const all = (data?.carts.items ?? []) as CartRow[]
    if (!isOffer || offerFilter === 'all') return all
    return all.filter((r) => (offerFilter === 'open' ? !!r.offerState && OPEN_STATES.includes(r.offerState) : r.offerState === offerFilter))
  }, [data, isOffer, offerFilter])

  const columns = useMemo<ColumnDef<CartRow>[]>(() => {
    const who: ColumnDef<CartRow> = {
      id: 'customer',
      accessorFn: (r) => r.customerName ?? r.customerEmail ?? (r.email || 'Guest'),
      header: ({ column }) => <DataTableColumnHeader column={column} title={isOffer ? 'Recipient' : 'Customer'} />,
      cell: ({ row }) => (
        <Link to={`${base}/${row.original.id}`} className="font-medium hover:underline">
          {row.original.customerName ?? row.original.customerEmail ?? (row.original.email || <span className="text-muted-foreground">Guest</span>)}
        </Link>
      ),
    }
    const items: ColumnDef<CartRow> = {
      accessorKey: 'itemCount',
      header: 'Items',
      size: 90,
      cell: ({ row }) => <span className="tabular-nums">{row.original.itemCount}</span>,
    }
    const subtotal: ColumnDef<CartRow> = {
      accessorKey: 'subtotal',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Subtotal" />,
      size: 130,
      cell: ({ row }) => <span className="tabular-nums">{formatCents(row.original.subtotal, row.original.currencyCode)}</span>,
    }
    const updated: ColumnDef<CartRow> = {
      accessorKey: 'updatedAt',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
      size: 160,
      sortFn: (a: Row<CartRow>, b: Row<CartRow>) => new Date(a.original.updatedAt).getTime() - new Date(b.original.updatedAt).getTime(),
      cell: ({ row }) => <span className="text-muted-foreground">{new Date(row.original.updatedAt).toLocaleString()}</span>,
    }
    if (!isOffer) {
      return [
        who,
        items,
        subtotal,
        {
          accessorKey: 'status',
          header: 'Status',
          size: 110,
          cell: ({ row }) => (
            <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
              {row.original.status}
            </Badge>
          ),
        },
        updated,
      ]
    }
    return [
      who,
      items,
      subtotal,
      {
        id: 'state',
        accessorFn: (r) => r.offerState ?? '',
        header: 'State',
        size: 120,
        cell: ({ row }) => (row.original.offerState ? <OfferStateBadge state={row.original.offerState} /> : null),
      },
      {
        id: 'expires',
        accessorFn: (r) => r.offerExpiresAt ?? '',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Expires" />,
        size: 160,
        cell: ({ row }) =>
          row.original.offerExpiresAt ? (
            <span className="text-muted-foreground">{new Date(row.original.offerExpiresAt).toLocaleString()}</span>
          ) : null,
      },
      updated,
    ]
  }, [isOffer, base])

  if (scope.mode === 'GLOBAL' || !effectiveChannelId) {
    return (
      <div className="rounded-md border py-10 text-center text-sm text-muted-foreground">
        Pick a channel in the header scope picker to browse its {isOffer ? 'offers' : 'carts'}.
      </div>
    )
  }

  return (
    <>
      <DataTable
        urlState={isOffer ? urlState : undefined}
        manual={
          isOffer
            ? undefined
            : {
                rowCount: data?.carts.totalCount ?? 0,
                pagination,
                onPaginationChange: setPagination,
                globalFilter: search,
                onGlobalFilterChange: urlState.setGlobalFilter,
                columnFilters: urlState.columnFilters,
                onColumnFiltersChange: urlState.setColumnFilters,
              }
        }
        prefsKey={isOffer ? 'offers.list' : 'carts.list'}
        enableColumnControls
        enableDensityToggle
        key={isOffer ? offerFilter : status}
        columns={columns}
        data={rows}
        loading={loading && rows.length === 0}
        error={error ? { message: error.message } : null}
        getRowId={(r) => r.id}
        emptyState={isOffer ? 'No offers for this filter.' : 'No carts for this filter.'}
        searchPlaceholder={isOffer ? 'Search by recipient…' : 'Search by customer…'}
        toolbarRightSlot={
          <div className="flex items-center gap-2">
            {isOffer ? (
              <Select value={offerFilter} onValueChange={(v) => setOfferFilter(v as OfferFilter)}>
                <SelectTrigger size="sm" className="w-36" aria-label="State">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {OFFER_FILTERS.map((f) => (
                    <SelectItem key={f.value} value={f.value}>
                      {f.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Select
                value={status}
                onValueChange={(v) => {
                  setStatus(v as CartStatus)
                  setPagination({ ...pagination, pageIndex: 0 })
                }}
              >
                <SelectTrigger size="sm" className="w-36" aria-label="Status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="abandoned">Abandoned</SelectItem>
                  <SelectItem value="converted">Converted</SelectItem>
                  <SelectItem value="all">All</SelectItem>
                </SelectContent>
              </Select>
            )}
            {canCreate && (
              <Button size="sm" className="h-8" onClick={() => setNewOpen(true)}>
                <PlusIcon className="size-4" />
                {isOffer ? 'New offer' : 'New cart'}
              </Button>
            )}
          </div>
        }
      />
      <NewCartDialog open={newOpen} onOpenChange={setNewOpen} channelId={effectiveChannelId} offer={isOffer} />
    </>
  )
}
