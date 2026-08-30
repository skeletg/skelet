import { useMemo } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import { RolesDocument } from './Roles.gql'
import { buildRoleSummary, type ScopeLike } from './roleSummary'

type RoleRow = {
  id: string
  slug: string
  name: string
  description: string
  memberCount: number
  scopes: ScopeLike[]
}

type Row = RoleRow & {
  grantCount: number
  scopeLabel: string
  isFullyGlobal: boolean
}

// RolesList: the roles as ONE flat DataTable, a click deep-linking into the standalone RoleDetail page - a master/detail shell would park a
// second sidebar next to the app nav and greeted the operator with an empty "select a role" panel
export function RolesList() {
  const navigate = useNavigate()
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { data, loading, error } = useQuery(RolesDocument)
  const permissions = usePermissions()

  const rows: Row[] = useMemo(
    () =>
      ((data?.roles ?? []) as RoleRow[]).map((r) => {
        const summary = buildRoleSummary(r.scopes)
        return {
          ...r,
          grantCount: summary.grantCount,
          isFullyGlobal: summary.isFullyGlobal,
          scopeLabel: summary.isFullyGlobal
            ? 'Global'
            : summary.explicitChannelIds.length > 0
              ? `${summary.explicitChannelIds.length} store${summary.explicitChannelIds.length === 1 ? '' : 's'}`
              : 'Scoped',
        }
      }),
    [data],
  )

  const columns = useMemo<ColumnDef<Row, unknown>[]>(
    () => [
      {
        id: 'role',
        // slug + description in the accessor so the toolbar search matches them too
        accessorFn: (r) => `${r.name} ${r.slug} ${r.description}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Role" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate text-sm font-medium leading-tight">{row.original.name}</div>
            <code className="block truncate font-mono text-xs text-muted-foreground">
              {row.original.slug}
            </code>
          </div>
        ),
      },
      {
        accessorKey: 'description',
        header: 'Description',
        cell: ({ row }) => (
          <span className="line-clamp-2 text-sm text-muted-foreground" title={row.original.description}>
            {row.original.description || '–'}
          </span>
        ),
      },
      {
        id: 'scope',
        accessorFn: (r) => r.scopeLabel,
        header: 'Scope',
        size: 110,
        cell: ({ row }) =>
          row.original.isFullyGlobal ? (
            <Badge variant="secondary" className="font-normal">
              Global
            </Badge>
          ) : (
            <span className="text-sm text-muted-foreground">{row.original.scopeLabel}</span>
          ),
      },
      {
        accessorKey: 'grantCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Grants" />,
        size: 90,
        cell: ({ row }) => <span className="text-sm tabular-nums">{row.original.grantCount}</span>,
      },
      {
        accessorKey: 'memberCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Members" />,
        size: 100,
        cell: ({ row }) => <span className="text-sm tabular-nums">{row.original.memberCount}</span>,
      },
    ],
    [],
  )

  return (
    <DataTable
      urlState={urlState}
      columns={columns}
      data={rows}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState="No roles defined yet."
      searchPlaceholder="Search name, slug or description…"
      initialSorting={[{ id: 'role', desc: false }]}
      onRowClick={(row) => navigate(`/admins/roles/${row.id}`)}
      toolbarRightSlot={
        permissions.has('admins.manage') ? (
          <Button size="sm" asChild>
            <Link to="/admins/roles/new">
              <PlusIcon className="size-4" />
              New role
            </Link>
          </Button>
        ) : null
      }
    />
  )
}
