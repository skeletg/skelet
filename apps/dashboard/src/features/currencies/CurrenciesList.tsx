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
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CurrenciesAdminListDocument,
  DeleteCurrencyDocument,
} from './Currencies.gql'
import { parseReferenceCount } from '@/shared/errors/parseReferenceCount'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type CurrencyRow = {
  code: string
  name: string
  symbol: string
  decimals: number
  symbolPosition: string
  isActive: boolean
  sortOrder: number
  updatedAt: string
}

function friendlyDeleteError(message: string): string {
  if (message.includes('CURRENCY_IN_USE') || message.includes('FOREIGN KEY')) {
    const count = parseReferenceCount(message)
    return count != null
      ? `${count} reference${count === 1 ? '' : 's'}; delete blocked. Resolve them first (countries defaulting to this currency, listings using it, channels, etc.).`
      : 'This currency is still referenced. Resolve those references first, then retry.'
  }
  return message
}

export function CurrenciesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { isGlobal } = useConfigScope()
  // Creating and deleting need currencies.manage - a viewer's buttons only led to an access-restricted page or a refused delete
  const canManage = usePermissions().has('currencies.manage')

  const { data, loading, error } = useQuery(CurrenciesAdminListDocument)
  const rows = useMemo(() => (data?.currencies ?? []) as CurrencyRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<CurrencyRow | null>(null)

  const [deleteCurrency] = useMutation(DeleteCurrencyDocument, {
    refetchQueries: [{ query: CurrenciesAdminListDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    try {
      await deleteCurrency({ variables: { code: confirmingDelete.code } })
    } catch (err) {
      const raw = err instanceof Error ? err.message : 'Delete failed.'
      throw new Error(friendlyDeleteError(raw), { cause: err })
    }
  }

  const columns = useMemo<ColumnDef<CurrencyRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            {row.original.symbol && (
              <span className="font-mono text-sm leading-none text-muted-foreground" aria-hidden>
                {row.original.symbol}
              </span>
            )}
            <Link
              to={`/currencies/${row.original.code}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
          </div>
        ),
      },
      {
        accessorKey: 'code',
        header: 'Code',
        size: 90,
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{row.original.code}</code>
        ),
      },
      {
        accessorKey: 'decimals',
        header: 'Decimals',
        size: 90,
        cell: ({ row }) => (
          <span className="font-mono text-xs">{row.original.decimals}</span>
        ),
      },
      {
        accessorKey: 'symbolPosition',
        header: 'Symbol pos.',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.symbolPosition.toLowerCase()}
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
                <Link to={`/currencies/${row.original.code}`}>Edit</Link>
              </DropdownMenuItem>
              {isGlobal && canManage && (
                <>
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
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [isGlobal, canManage],
  )

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.code}
        emptyState="No currencies yet."
        searchPlaceholder="Search name or code…"
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
          canManage &&
          (isGlobal ? (
            <Button size="sm" asChild>
              <Link to="/currencies/new">
                <PlusIcon className="size-4" />
                New currency
              </Link>
            </Button>
          ) : (
            <Button size="sm" disabled title="Switch the header to GLOBAL to add a currency.">
              <PlusIcon className="size-4" />
              New currency
            </Button>
          ))
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.code}?</>}
        description="The backend refuses deletion if anything still references this currency (countries defaulting to it, product listings, channels, etc.)."
        confirmLabel="Delete currency"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
