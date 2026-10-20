import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef, type PaginationState } from '@/components/data-table/tanstack'
import { StarIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { usePageTitle } from '@/lib/page-title'
import { LoadError } from '@/shared/errors/LoadError'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CustomersPagedDocument } from './Customers.gql'
import { useCustomerAccess } from './useCustomerAccess'
import {
  CustomerGroupDocument,
  CustomerGroupsDocument,
  DeleteCustomerGroupDocument,
  SetDefaultCustomerGroupDocument,
  UpdateCustomerGroupDocument,
  type CustomerGroupQuery,
} from './CustomerGroups.gql'
import { PluginSections } from '@/plugins/detail-sections'
import {
  CUSTOMER_GROUP_LABELS,
  customerGroupDiff,
  customerGroupSchema,
  toCustomerGroupInput,
  toCustomerGroupValues,
} from './schemas/customerGroup'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type MemberRow = {
  id: string
  firstName: string
  lastName: string
  email: string
  isActive: boolean
}

const memberColumns: ColumnDef<MemberRow>[] = [
  {
    id: 'name',
    accessorFn: (row) => `${row.firstName} ${row.lastName}`,
    header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
    cell: ({ row }) => (
      <Link to={`/customers/${row.original.id}`} className="font-medium hover:underline">
        {row.original.firstName} {row.original.lastName}
      </Link>
    ),
  },
  {
    accessorKey: 'email',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Email" />,
    cell: ({ row }) => <span className="text-muted-foreground">{row.original.email}</span>,
  },
  {
    id: 'status',
    accessorFn: (row) => (row.isActive ? 'active' : 'inactive'),
    header: 'Status',
    size: 110,
    cell: ({ row }) =>
      row.original.isActive ? (
        <Badge variant="secondary">active</Badge>
      ) : (
        <Badge variant="outline" className="text-muted-foreground">
          inactive
        </Badge>
      ),
  },
]

// The standalone /customers/groups/:id route - profile edits go through the header Save; members,
// default and delete act on their own
export function CustomerGroupDetailPage() {
  const { id = '' } = useParams<{ id?: string }>()
  const groupQuery = useQuery(CustomerGroupDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const group = groupQuery.data?.customerGroup
  usePageTitle(group?.name ?? null) // browser + workspace tab show the group, not "Customer group detail"

  if (groupQuery.loading && !group) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading group…</p>
  }
  if (groupQuery.error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load group"
          message={groupQuery.error.message}
          onRetry={() => void groupQuery.refetch()}
          retrying={groupQuery.loading}
        />
      </div>
    )
  }
  if (!group) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Group not found.</p>
  }
  return <CustomerGroupForm group={group} />
}

type Group = NonNullable<CustomerGroupQuery['customerGroup']>

function CustomerGroupForm({ group }: { group: Group }) {
  const navigate = useNavigate()
  const { canManageGroups } = useCustomerAccess()

  const refetch = [
    { query: CustomerGroupDocument, variables: { id: group.id } },
    { query: CustomerGroupsDocument },
  ]
  const [updateGroup] = useMutation(UpdateCustomerGroupDocument, { refetchQueries: refetch })
  const [setDefault, setDefaultState] = useMutation(SetDefaultCustomerGroupDocument, { refetchQueries: refetch })
  const [deleteGroup] = useMutation(DeleteCustomerGroupDocument, {
    refetchQueries: [CustomerGroupsDocument],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const baseline = useMemo(() => toCustomerGroupValues(group), [group])
  const record = useRecordForm({
    id: 'profile',
    title: 'Profile',
    label: `Customer group · ${group.name}`,
    isEdit: true,
    baseline,
    schema: customerGroupSchema,
    diff: customerGroupDiff,
    persist: { routeKey: `customer-group:${group.id}` },
    submit: async (v) => {
      await updateGroup({ variables: { id: group.id, input: toCustomerGroupInput(v) } })
    },
  })
  const { form } = record

  // Members come back a page at a time - customerGroup(id:) reports memberCount 0 (only the LIST
  // query populates it), so this query's totalCount is the count the page trusts
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 20 })
  const membersQuery = useQuery(CustomersPagedDocument, {
    variables: {
      groupSlug: group.slug,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
    fetchPolicy: 'cache-and-network',
    notifyOnNetworkStatusChange: true,
  })
  const members = membersQuery.data?.customersPaged.items ?? []
  const memberCount = membersQuery.data?.customersPaged.totalCount
  const memberCountKnown = memberCount !== undefined

  async function onSetDefault() {
    if (group.isDefault) return
    setActionError(null)
    try {
      await setDefault({ variables: { id: group.id } })
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Set default failed.')
    }
  }

  async function onDelete() {
    await deleteGroup({ variables: { id: group.id } })
    navigate('/customers/groups', { replace: true })
  }

  // Guard and displayed count read the same number, so Delete is never offered on a populated group
  const canDelete = canManageGroups && !group.isDefault && memberCountKnown && memberCount === 0

  return (
    <RecordFormPage
      record={record}
      backTo="/customers/groups"
      backLabel="Back to customer groups"
      title={group.name}
      badges={
        group.isDefault ? (
          <Badge variant="secondary" className="gap-1 font-normal">
            <StarIcon className="size-3" />
            default
          </Badge>
        ) : null
      }
      meta={
        <span className="font-mono">
          {group.slug} ·{' '}
          {memberCountKnown ? `${memberCount} member${memberCount === 1 ? '' : 's'}` : 'counting members…'}
          {' '}· created {new Date(group.createdAt).toLocaleDateString()}
        </span>
      }
      actions={
        !group.isDefault &&
        canManageGroups && (
          <Button type="button" variant="outline" size="sm" onClick={onSetDefault} disabled={setDefaultState.loading}>
            <StarIcon className="size-4" />
            {setDefaultState.loading ? 'Setting…' : 'Set as default'}
          </Button>
        )
      }
      createLabel="Save changes"
      labels={CUSTOMER_GROUP_LABELS}
      // Group CRUD is global-only server-side
      readOnly={!canManageGroups}
      after={
        <>
          <DetailPageSection title="Members" description="Customers currently assigned to this group.">
            <DataTable
              columns={memberColumns}
              data={members}
              loading={membersQuery.loading && members.length === 0}
              error={membersQuery.error ? { message: membersQuery.error.message } : null}
              getRowId={(row) => row.id}
              emptyState="No customers in this group."
              manual={{
                rowCount: memberCount ?? 0,
                pagination,
                onPaginationChange: setPagination,
                globalFilter: '',
                onGlobalFilterChange: () => {},
                columnFilters: [],
                onColumnFiltersChange: () => {},
              }}
            />
          </DetailPageSection>

          <DetailPageSection
            title="Danger zone"
            description={
              !canManageGroups
                ? 'Deleting a customer group needs a global customers.manage grant.'
                : group.isDefault
                  ? 'The default group cannot be deleted. Mark another group as default first.'
                  : !memberCountKnown
                    ? 'Counting members…'
                    : memberCount > 0
                      ? `Move the ${memberCount} customer${memberCount === 1 ? '' : 's'} in this group to another group before deleting.`
                      : 'Delete the group. This only removes the group definition - there are no members to reassign.'
            }
          >
            <div>
              <Button
                type="button"
                variant="destructive"
                disabled={!canDelete}
                onClick={() => setConfirmingDelete(true)}
              >
                <Trash2Icon className="size-4" />
                Delete group
              </Button>
            </div>
          </DetailPageSection>
          <PluginSections target="customer-group" entity={group} mode="edit" />
        </>
      }
    >
      {actionError && <div className="border-b bg-destructive/10 px-6 py-2 text-xs text-destructive">{actionError}</div>}

      <DetailPageSection title="Profile" description="Slug is immutable - create a new group if you need a different slug.">
        <div className="grid gap-4">
          <form.AppField name="name">{(f) => <f.TextField id="cg-name" label="Name" />}</form.AppField>
          <form.AppField name="description">{(f) => <f.TextareaField id="cg-desc" label="Description" />}</form.AppField>
        </div>
      </DetailPageSection>

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={<>Delete {group.name}?</>}
        description="The group definition will be removed. This cannot be undone."
        confirmLabel="Delete group"
        busyLabel="Deleting…"
        onConfirm={onDelete}
      />
    </RecordFormPage>
  )
}
