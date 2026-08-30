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
import { useConfigScope } from '@/lib/config-scope'
import {
  CountriesAdminListDocument,
  DeleteCountryDocument,
} from './Countries.gql'

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

function parseReferenceCount(message: string): number | null {
  const match = message.match(/(\d+)\s+references?/i)
  if (!match) return null
  const n = Number(match[1])
  return Number.isFinite(n) ? n : null
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

  const { data, loading, error } = useQuery(CountriesAdminListDocument)
  const rows = useMemo(() => (data?.countries ?? []) as CountryRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<CountryRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteCountry, { loading: deleting }] = useMutation(DeleteCountryDocument, {
    refetchQueries: [{ query: CountriesAdminListDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteCountry({ variables: { code: confirmingDelete.code } })
      setConfirmingDelete(null)
    } catch (err) {
      const raw = err instanceof Error ? err.message : 'Delete failed.'
      setDeleteError(friendlyDeleteError(raw))
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
              {isGlobal && (
                <>
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
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [isGlobal],
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
          isGlobal ? (
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
          )
        }
      />

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
            <AlertDialogTitle>Delete {confirmingDelete?.code}?</AlertDialogTitle>
            <AlertDialogDescription>
              The backend refuses deletion if anything still references this country
              (locales, addresses, tax rules, etc.).
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete blocked</AlertTitle>
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
              {deleting ? 'Deleting…' : 'Delete country'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
