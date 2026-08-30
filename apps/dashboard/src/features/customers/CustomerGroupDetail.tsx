import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef, type PaginationState } from '@/components/data-table/tanstack'
import { StarIcon, Trash2Icon } from 'lucide-react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePageTitle } from '@/lib/page-title'
import { LoadError } from '@/shared/errors/LoadError'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { CustomersPagedDocument } from './Customers.gql'
import { useCustomerAccess } from './useCustomerAccess'
import {
  CustomerGroupDocument,
  CustomerGroupsDocument,
  DeleteCustomerGroupDocument,
  SetDefaultCustomerGroupDocument,
  UpdateCustomerGroupDocument,
} from './CustomerGroups.gql'
import { PluginSections } from '@/plugins/detail-sections'

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

// Rendered inside CustomerGroupsList's DetailPageLayout (sidebar + detail split), so this component supplies
// header/body/sections but not the layout wrapper itself
export function CustomerGroupDetail({ groupId }: { groupId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = groupId ?? params.id ?? ''
  const navigate = useNavigate()

  const { canManageGroups } = useCustomerAccess()

  const groupQuery = useQuery(CustomerGroupDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })

  const refetch = [
    { query: CustomerGroupDocument, variables: { id } },
    { query: CustomerGroupsDocument },
  ]
  const [updateGroup] = useMutation(UpdateCustomerGroupDocument, {
    refetchQueries: refetch,
  })
  const [setDefault, setDefaultState] = useMutation(SetDefaultCustomerGroupDocument, {
    refetchQueries: refetch,
  })
  const [deleteGroup, deleteState] = useMutation(DeleteCustomerGroupDocument, {
    refetchQueries: [{ query: CustomerGroupsDocument }],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  // Page save registry: the profile form stages its edits and the header Save applies them through the
  // SaveConfirmDialog diff modal - the dashboard-wide convention (see docs/conventions.md)
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  const group = groupQuery.data?.customerGroup
  usePageTitle(group?.name ?? null) // browser + workspace tab show the group, not "Customer group detail"

  // Members come back a page at a time - customerGroup(id:) reports memberCount 0 (only the LIST
  // query populates it), so this query's totalCount is the count the page trusts
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 20 })
  const membersQuery = useQuery(CustomersPagedDocument, {
    variables: {
      groupSlug: group?.slug ?? null,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
    skip: !group,
    fetchPolicy: 'cache-and-network',
    notifyOnNetworkStatusChange: true,
  })
  const members = membersQuery.data?.customersPaged.items ?? []
  const memberCount = membersQuery.data?.customersPaged.totalCount
  const memberCountKnown = memberCount !== undefined

  async function onSetDefault() {
    if (!group || group.isDefault) return
    setActionError(null)
    try {
      await setDefault({ variables: { id } })
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Set default failed.')
    }
  }

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteGroup({ variables: { id } })
      navigate('/customers/groups', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  if (groupQuery.loading) {
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

  // Guard and displayed count read the same number, so Delete is never offered on a populated group
  const canDelete = canManageGroups && !group.isDefault && memberCountKnown && memberCount === 0

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <div className="flex flex-col">
      <DetailPageHeader
        backTo="/customers/groups"
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
          <>
            {!group.isDefault && canManageGroups && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onSetDefault}
                disabled={setDefaultState.loading}
              >
                <StarIcon className="size-4" />
                {setDefaultState.loading ? 'Setting…' : 'Set as default'}
              </Button>
            )}
            {canManageGroups && (
              <Button
                type="button"
                size="sm"
                disabled={!isDirty || savingAll}
                onClick={() => { setSaveError(null); setConfirmingSave(true) }}
              >
                {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
              </Button>
            )}
          </>
        }
      />
      {actionError && (
        <div className="border-b bg-destructive/10 px-6 py-2 text-xs text-destructive">
          {actionError}
        </div>
      )}

      <DetailPageBody>
        <DetailPageSection
          title="Profile"
          description="Slug is immutable - create a new group if you need a different slug."
        >
          {/* Group CRUD is global-only server-side - display:contents keeps the section layout */}
          <fieldset disabled={!canManageGroups} className="contents">
            <ProfileSection
              group={group}
              save={(input) => updateGroup({ variables: { id, input } })}
            />
          </fieldset>
        </DetailPageSection>

        <DetailPageSection
          title="Members"
          description="Customers currently assigned to this group."
        >
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
              onClick={() => {
                setDeleteError(null)
                setConfirmingDelete(true)
              }}
            >
              <Trash2Icon className="size-4" />
              Delete group
            </Button>
          </div>
        </DetailPageSection>
        <PluginSections target="customer-group" entity={group} mode="edit" />
      </DetailPageBody>

      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(false)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {group.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              The group definition will be removed. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteState.loading}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteState.loading}
              onClick={(e) => {
                e.preventDefault()
                onDelete()
              }}
            >
              {deleteState.loading ? 'Deleting…' : 'Delete group'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists the staged profile changes */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Customer group · ${group.name}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </div>
    </SaveRegistryContext.Provider>
  )
}

type GroupView = {
  name: string
  description: string
}

function ProfileSection({
  group,
  save,
}: {
  group: GroupView
  save: (input: { name?: string; description?: string | null }) => Promise<unknown>
}) {
  const [name, setName] = useState(group.name)
  const [description, setDescription] = useState(group.description)

  // Re-hydrate the draft when the server payload changes (post-save
  // refetch) - adjust state during render instead of an effect
  const [prevGroup, setPrevGroup] = useState(group)
  if (prevGroup !== group) {
    setPrevGroup(group)
    setName(group.name)
    setDescription(group.description)
  }

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      fieldDiff(group, { name, description }, [
        { label: 'name', value: (g) => g.name },
        { label: 'description', value: (g) => g.description },
      ]),
    [group, name, description],
  )

  useSaveSection({
    id: 'profile',
    title: 'Profile',
    diff,
    save: async () => {
      if (!name.trim()) throw new Error('Name is required.')
      await save({ name: name.trim(), description: description.trim() || null })
    },
  })

  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="cg-name">Name</Label>
        <Input
          id="cg-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cg-desc">Description</Label>
        <Textarea
          id="cg-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>
      {diff.length > 0 && (
        <p className="text-xs text-muted-foreground">
          Pending changes - applied with the page Save button.
        </p>
      )}
    </div>
  )
}

// CustomerGroupDetailPage is the standalone /customers/groups/:id route - CustomerGroupDetail
// reads the id from the URL itself, this just adds the standard detail-page chrome
export function CustomerGroupDetailPage() {
  return (
    <DetailPageLayout>
      <CustomerGroupDetail />
    </DetailPageLayout>
  )
}
