import { useMemo, useState } from 'react'
import { Link, useMatch, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon } from 'lucide-react'

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
import { usePermissions } from '@/shared/auth/usePermissions'
import { DeleteShippingZoneDocument, ShippingZonesDocument, type ShippingZonesQuery } from './Shipping.gql'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type ShippingZone = ShippingZonesQuery['shippingZones'][number]

// Long country lists would blow out the column, so show the first few and count the rest
function countriesPreview(countries: string[]): string {
  if (countries.length === 0) return '-'
  if (countries.length <= 3) return countries.join(', ')
  return `${countries.slice(0, 3).join(', ')} +${countries.length - 3}`
}

// ShippingZonesList is the shell for /shipping, /shipping/new and /shipping/:id - Zones are channel-scoped, so GLOBAL
// scope gets a "pick a channel" page instead of the table
export function ShippingZonesList() {
  // Creating and deleting zones need settings.manage - without it the buttons only led to an access-restricted page or a refused save
  const canManage = usePermissions().has('settings.manage')
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const rawId = params.id ?? null
  // /shipping/new is a literal route with no :id, so match the path rather than the param
  const isCreate = useMatch('/shipping/new') !== null || rawId === 'new'
  const activeID = !isCreate && rawId ? rawId : null

  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()

  const { data, loading, error } = useQuery(ShippingZonesDocument, {
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

  const refetchQueries = effectiveChannelId
    ? [{ query: ShippingZonesDocument, variables: { channelId: effectiveChannelId } }]
    : []
  const [deleteZone] = useMutation(DeleteShippingZoneDocument, { refetchQueries })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteZone({ variables: { id: confirmingDelete.id } })
    if (activeID === confirmingDelete.id) {
      navigate('/shipping', { replace: true })
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
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
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
              canManage && (
                <Button size="sm" onClick={() => navigate(newHref)}>
                  <PlusIcon className="size-4" />
                  New zone
                </Button>
              )
            }
          />
        </DetailPageSection>
      </DetailPageBody>

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.name || confirmingDelete?.code}?</>}
        description="Methods inside this zone are cascade-deleted. This action cannot be undone."
        confirmLabel="Delete zone"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </DetailPageLayout>
  )
}
