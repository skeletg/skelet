import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon, Trash2Icon } from 'lucide-react'
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
import {
  DeleteShippingBoxDocument,
  ShippingBoxesDocument,
} from './Shipping.gql'
import { ShippingBoxFormDialog, type ShippingBoxRow } from './ShippingBoxFormDialog'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

const kg = (grams: number) => (grams > 0 ? `${(grams / 1000).toLocaleString()} kg` : '-')

export function ShippingBoxesPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, channels } = useConfigScope()
  // Boxes are per channel - fall back to the first active one on global scope
  const channelId = effectiveChannelId ?? channels[0]?.id ?? ''

  const { data, loading, error, refetch } = useQuery(ShippingBoxesDocument, {
    variables: { channelId },
    skip: !channelId,
  })
  const rows = useMemo(() => (data?.shippingBoxes ?? []) as ShippingBoxRow[], [data])

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ShippingBoxRow | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState<ShippingBoxRow | null>(null)

  const [deleteBox] = useMutation(DeleteShippingBoxDocument)

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteBox({ variables: { id: confirmingDelete.id } })
    void refetch()
  }

  function openEdit(row: ShippingBoxRow) {
    setEditing(row)
    setFormOpen(true)
  }

  const columns = useMemo<ColumnDef<ShippingBoxRow>[]>(
    () => [
      {
        accessorKey: 'code',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Code" />,
        cell: ({ row }) => (
          <button
            type="button"
            className="font-mono text-xs font-medium hover:underline"
            onClick={() => openEdit(row.original)}
          >
            {row.original.code}
          </button>
        ),
      },
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => row.original.name || <span className="text-muted-foreground">-</span>,
      },
      {
        id: 'inner',
        header: 'Inner size (mm)',
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.innerLengthMm} x {row.original.innerWidthMm} x {row.original.innerHeightMm}
          </span>
        ),
      },
      {
        accessorKey: 'maxWeightGrams',
        header: 'Max load',
        cell: ({ row }) =>
          row.original.maxWeightGrams > 0 ? (
            kg(row.original.maxWeightGrams)
          ) : (
            <span className="text-xs text-muted-foreground">unlimited</span>
          ),
      },
      {
        accessorKey: 'boxWeightGrams',
        header: 'Box weight',
        cell: ({ row }) => kg(row.original.boxWeightGrams),
      },
      {
        accessorKey: 'isActive',
        header: 'Status',
        size: 100,
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
              <DropdownMenuItem onSelect={() => openEdit(row.original)}>Edit</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => {
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
        <DetailPageHeader title="Shipping boxes" meta="Pick a channel in the header switcher first." />
      </DetailPageLayout>
    )
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Shipping boxes"
        meta="The real cartons your warehouse packs into. With at least one active box, rate quotes and labels run cartonization (first-fit-decreasing) and send per-parcel dimensions to carriers. No boxes = cartonization off."
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
            emptyState="No boxes yet - cartonization is off and carriers quote without parcel dimensions."
            searchPlaceholder="Search code or name…"
            initialSorting={[{ id: 'sortOrder', desc: false }]}
            toolbarRightSlot={
              <Button
                size="sm"
                onClick={() => {
                  setEditing(null)
                  setFormOpen(true)
                }}
              >
                <PlusIcon className="size-4" />
                New box
              </Button>
            }
          />
        </DetailPageSection>
      </DetailPageBody>

      <ShippingBoxFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditing(null)
        }}
        channelId={channelId}
        existing={editing}
        onSaved={() => void refetch()}
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete box {confirmingDelete?.code}?</>}
        description="Future quotes and labels stop packing into it. If this was the last active box, cartonization turns off entirely."
        confirmLabel="Delete box"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </DetailPageLayout>
  )
}
