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
  CountriesAdminListDocument,
  DeleteCountryDocument,
} from './Countries.gql'
import { parseReferenceCount } from '@/shared/errors/parseReferenceCount'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type CountryRow = {
  code: string
  name: string
  nativeName: string
  emojiFlag: string
  phoneCode: string
  defaultCurrencyCode: string
  isActive: boolean
  sortOrder: number
  updatedAt: string
}

function friendlyDeleteError(message: string): string {
  if (message.includes('COUNTRY_IN_USE') || message.includes('FOREIGN KEY')) {
    const count = parseReferenceCount(message)
    return count != null
      ? `${count} reference${count === 1 ? '' : 's'}; delete blocked. Resolve them first (locales defaulting to this country, addresses, tax rules, etc.).`
      : 'This country is still referenced. Resolve those references first, then retry.'
  }
  return message
}

export function CountriesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { isGlobal } = useConfigScope()
  // Creating and deleting need countries.manage - a viewer's buttons only led to an access-restricted page or a refused delete
  const canManage = usePermissions().has('countries.manage')

  const { data, loading, error } = useQuery(CountriesAdminListDocument)
  const rows = useMemo(() => (data?.countries ?? []) as CountryRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<CountryRow | null>(null)

  const [deleteCountry] = useMutation(DeleteCountryDocument, {
    refetchQueries: [{ query: CountriesAdminListDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    try {
      await deleteCountry({ variables: { code: confirmingDelete.code } })
    } catch (err) {
      const raw = err instanceof Error ? err.message : 'Delete failed.'
      throw new Error(friendlyDeleteError(raw), { cause: err })
    }
  }

  const columns = useMemo<ColumnDef<CountryRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            {row.original.emojiFlag && (
              <span className="text-lg leading-none" aria-hidden>
                {row.original.emojiFlag}
              </span>
            )}
            <Link
              to={`/countries/${row.original.code}`}
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
        accessorKey: 'nativeName',
        header: 'Native name',
        cell: ({ row }) => (
          <span className="text-sm">
            {row.original.nativeName || <span className="italic text-muted-foreground">-</span>}
          </span>
        ),
      },
      {
        accessorKey: 'phoneCode',
        header: 'Phone',
        size: 80,
        cell: ({ row }) =>
          row.original.phoneCode ? (
            <span className="font-mono text-xs">+{row.original.phoneCode}</span>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        accessorKey: 'defaultCurrencyCode',
        header: 'Currency',
        size: 90,
        cell: ({ row }) =>
          row.original.defaultCurrencyCode ? (
            <Badge variant="outline" className="font-mono text-xs font-normal">
              {row.original.defaultCurrencyCode}
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
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
                <Link to={`/countries/${row.original.code}`}>Edit</Link>
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
        emptyState="No countries yet."
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
              <Link to="/countries/new">
                <PlusIcon className="size-4" />
                New country
              </Link>
            </Button>
          ) : (
            <Button size="sm" disabled title="Switch the header to GLOBAL to add a country.">
              <PlusIcon className="size-4" />
              New country
            </Button>
          ))
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.code}?</>}
        description="The backend refuses deletion if anything still references this country (locales, addresses, tax rules, etc.)."
        confirmLabel="Delete country"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
