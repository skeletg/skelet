import { useMemo, useState } from 'react'
import { Link, useMatch, useNavigate, useParams } from 'react-router-dom'
import { gql } from '@apollo/client'
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
import { ShippingZoneCreatePage, ShippingZoneEditPage } from './ShippingZoneForm'

const SHIPPING_ZONES_QUERY = gql`
  query ShippingZones($channelId: ID!) {
    shippingZones(channelId: $channelId) {
      id
      channelId
      code
      name
      sortOrder
      isActive
      countries
      methods {
        id
        code
        name
        calcType
        flatCents
        freeAboveCents
        sortOrder
        isActive
      }
    }
  }
`

const DELETE_ZONE = gql`
  mutation DeleteShippingZone($id: ID!) {
    deleteShippingZone(id: $id)
  }
`

export type CalcType = 'FLAT' | 'FREE' | 'FREE_ABOVE'

export type ShippingMethod = {
  id: string
  code: string
  name: string
  calcType: CalcType
  flatCents: number
  freeAboveCents: number
  sortOrder: number
  isActive: boolean
  countryMode?: string
  allowedCountries?: string[]
}

export type ShippingZone = {
  id: string
  channelId: string
  code: string
  name: string
  sortOrder: number
  isActive: boolean
  countries: string[]
  methods: ShippingMethod[]
}

// Long country lists would blow out the column, so show the first few and count the rest
function countriesPreview(countries: string[]): string {
  if (countries.length === 0) return '-'
  if (countries.length <= 3) return countries.join(', ')
  return `${countries.slice(0, 3).join(', ')} +${countries.length - 3}`
}

// ShippingZonesList is the shell for /shipping, /shipping/new and /shipping/:id - Zones are channel-scoped, so GLOBAL
// scope gets a "pick a channel" page instead of the table
export function ShippingZonesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const rawId = params.id ?? null
  // /shipping/new is a literal route with no :id, so match the path rather than the param
  const isCreate = useMatch('/shipping/new') !== null || rawId === 'new'
  const activeID = !isCreate && rawId ? rawId : null

  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()

  const { data, loading, error } = useQuery<
    { shippingZones: ShippingZone[] },
    { channelId: string }
  >(SHIPPING_ZONES_QUERY, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
  })
  // Pre-sorted so equal sortOrder still tie-breaks on code - the table's own sort is stable, so it keeps that
  const zones = useMemo(
    () =>
      [...(data?.shippingZones ?? [])].sort(
        (a, b) => a.sortOrder - b.sortOrder || a.code.localeCompare(b.code),
      ),
    [data],
  )

  const [confirmingDelete, setConfirmingDelete] = useState<ShippingZone | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const refetchQueries = effectiveChannelId
    ? [{ query: SHIPPING_ZONES_QUERY, variables: { channelId: effectiveChannelId } }]
    : []
  const [deleteZone, { loading: deleting }] = useMutation<
    { deleteShippingZone: boolean },
    { id: string }
  >(DELETE_ZONE, { refetchQueries })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteZone({ variables: { id: confirmingDelete.id } })
      if (activeID === confirmingDelete.id) {
        navigate('/shipping', { replace: true })
      }
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const newHref = effectiveChannelId
    ? `/shipping/new?channelId=${effectiveChannelId}`
    : '/shipping/new'

  const columns = useMemo<ColumnDef<ShippingZone>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Zone" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Link to={`/shipping/${row.original.id}`} className="font-medium hover:underline">
              {row.original.name || row.original.code}
            </Link>
            {!row.original.isActive && (
              <Badge variant="outline" className="text-[10px]">
                Inactive
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'code',
        header: 'Code',
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{row.original.code}</code>
        ),
      },
      {
        id: 'countries',
        header: 'Countries',
        // accessorFn not just cell - the global filter only sees columns that resolve a value, and countries have to stay searchable
        accessorFn: (row) => row.countries.join(' '),
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {countriesPreview(row.original.countries)}
          </span>
        ),
      },
      {
        id: 'methods',
        header: 'Methods',
        size: 90,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.methods.length === 0 ? '-' : row.original.methods.length}
          </span>
        ),
      },
      {
        accessorKey: 'sortOrder',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Sort" />,
        size: 70,
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
                aria-label={`Actions for ${row.original.code}`}
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/shipping/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
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
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [],
  )

  // Create + edit render full-width (reached from the table) - these come before
  // the scope guard so a deep link still opens the form in GLOBAL scope
  if (isCreate) return <ShippingZoneCreatePage />
  if (activeID) return <ShippingZoneEditPage zoneId={activeID} />

  if (!isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader
          title="Shipping zones"
          meta="Pick a channel in the header switcher first - zones are scoped per channel."
        />
      </DetailPageLayout>
    )
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Shipping zones"
        meta="Country groups a channel ships to, each with its own methods. Zones are matched by the destination country; Sort decides which one wins when two cover the same country."
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable
            urlState={urlState}
            columns={columns}
            data={zones}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(row) => row.id}
            emptyState={`No zones for ${selectedChannel?.name ?? 'this channel'} yet.`}
            searchPlaceholder="Search code, name or country…"
            initialSorting={[{ id: 'sortOrder', desc: false }]}
            toolbarRightSlot={
              <Button size="sm" onClick={() => navigate(newHref)}>
                <PlusIcon className="size-4" />
                New zone
              </Button>
            }
          />
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmingDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {confirmingDelete?.name || confirmingDelete?.code}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Methods inside this zone are cascade-deleted. This action cannot be undone.
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
              {deleting ? 'Deleting…' : 'Delete zone'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}
