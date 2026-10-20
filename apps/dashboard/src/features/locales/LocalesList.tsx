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
  DeleteLocaleDocument,
  LocalesAdminListDocument,
} from './Locales.gql'
import { parseReferenceCount } from '@/shared/errors/parseReferenceCount'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type LocaleRow = {
  code: string
  name: string
  nativeName: string
  isActive: boolean
  isRTL: boolean
  sortOrder: number
  flagEmoji: string
  defaultCountryCode: string
  updatedAt: string
}

function friendlyDeleteError(message: string): string {
  if (message.includes('LOCALE_IN_USE')) {
    const count = parseReferenceCount(message)
    return count != null
      ? `${count} reference${count === 1 ? '' : 's'}; delete blocked. Resolve them first (translations, channels, role scopes, or config values).`
      : 'This locale is still referenced. Resolve those references first, then retry.'
  }
  return message
}

export function LocalesList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { isGlobal } = useConfigScope()
  // Creating and deleting need locales.manage - a viewer's buttons only led to an access-restricted page or a refused delete
  const canManage = usePermissions().has('locales.manage')

  const { data, loading, error } = useQuery(LocalesAdminListDocument)
  const rows = useMemo(() => (data?.locales ?? []) as LocaleRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<LocaleRow | null>(null)

  const [deleteLocale] = useMutation(DeleteLocaleDocument, {
    refetchQueries: [LocalesAdminListDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    try {
      await deleteLocale({ variables: { code: confirmingDelete.code } })
    } catch (err) {
      const raw = err instanceof Error ? err.message : 'Delete failed.'
      throw new Error(friendlyDeleteError(raw), { cause: err })
    }
  }

  const columns = useMemo<ColumnDef<LocaleRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            {row.original.flagEmoji && (
              <span className="text-lg leading-none" aria-hidden>
                {row.original.flagEmoji}
              </span>
            )}
            <Link
              to={`/locales/${row.original.code}`}
              className="font-medium hover:underline"
            >
              {row.original.name}
            </Link>
            {row.original.isRTL && (
              <Badge variant="outline" className="text-[10px] font-normal">
                RTL
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
        accessorKey: 'nativeName',
        header: 'Native name',
        cell: ({ row }) => (
          <span className="text-sm">
            {row.original.nativeName || <span className="italic text-muted-foreground">-</span>}
          </span>
        ),
      },
      {
        accessorKey: 'defaultCountryCode',
        header: 'Country',
        size: 90,
        cell: ({ row }) =>
          row.original.defaultCountryCode ? (
            <code className="font-mono text-xs">{row.original.defaultCountryCode}</code>
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
                <Link to={`/locales/${row.original.code}`}>Edit</Link>
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
        emptyState="No languages yet."
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
              <Link to="/locales/new">
                <PlusIcon className="size-4" />
                New language
              </Link>
            </Button>
          ) : (
            <Button size="sm" disabled title="Switch the header to GLOBAL to add a language.">
              <PlusIcon className="size-4" />
              New language
            </Button>
          ))
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete?.code}?</>}
        description="The backend refuses to delete locales that are still referenced anywhere (translations, channel assignments, default locale, role scopes, config values). The error message below will tell you the reference count."
        confirmLabel="Delete language"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
