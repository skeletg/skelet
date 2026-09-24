import { useCallback, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Badge } from '@/components/ui/badge'
import { FieldError } from '@/components/form/fields'
import { LoadError } from '@/shared/errors/LoadError'
import { LoadingState } from '@/components/ui/loading-state'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import {
  CreateRoleDocument,
  RoleDocument,
  RolesDocument,
  UpdateRoleDocument,
  type RoleQuery,
} from '@/features/admins/roles/Roles.gql'
import { useRoleDraft, type GroupRow, type RoleDraftState, type Scope } from './useRoleDraft'
import { GroupSidebar } from './GroupSidebar'
import { GroupPanel } from './GroupPanel'
import { ROLE_LABELS, roleDiff, roleSchema, toCreateRoleInput, toRoleValues, toUpdateRoleInput } from './schemas/role'

// RoleEditPage is the unified create/edit screen for admin roles
// Route: /admins/roles/new (create) or /admins/roles/:id/edit (edit)
// Layout: header with identity fields + Save, then a permission-groups sidebar + active-group panel
export function RoleEditPage() {
  const params = useParams<{ id?: string }>()
  const isEdit = !!params.id
  const roleQuery = useQuery(RoleDocument, {
    variables: { id: params.id ?? '' },
    skip: !isEdit,
  })

  if (isEdit && roleQuery.loading && !roleQuery.data) {
    return <LoadingState label="Loading role…" />
  }
  if (isEdit && roleQuery.error) {
    return (
      <LoadError
        title="Couldn't load role"
        message={roleQuery.error.message}
        onRetry={() => void roleQuery.refetch()}
      />
    )
  }
  const role = roleQuery.data?.role
  if (isEdit && !role) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Role not found.</p>
  }
  return <RoleForm role={role} />
}

function RoleForm({ role }: { role?: RoleQuery['role'] }) {
  const navigate = useNavigate()
  const isEdit = !!role

  const groupsQuery = useQuery(PermissionGroupsDocument)
  const channelsQuery = useQuery(ChannelsDocument)
  const groups: GroupRow[] = useMemo(() => groupsQuery.data?.permissionGroups ?? [], [groupsQuery.data])
  const channels = useMemo(() => channelsQuery.data?.channels ?? [], [channelsQuery.data])

  const refetchQueries = role
    ? [{ query: RolesDocument }, { query: RoleDocument, variables: { id: role.id } }]
    : [{ query: RolesDocument }]
  const [createRole] = useMutation(CreateRoleDocument, { refetchQueries })
  const [updateRole] = useMutation(UpdateRoleDocument, { refetchQueries })
  const createdId = useRef('')

  // Keyed on the record - it's the form's defaultValues and what the diff and stored draft compare to
  const baseline = useMemo(() => toRoleValues(role), [role])
  const diff = useMemo(() => roleDiff((id) => channels.find((c) => c.id === id)?.name ?? id), [channels])

  const record = useRecordForm({
    id: 'role',
    title: 'Role',
    label: role ? `Role · ${role.name}` : 'New role',
    isEdit,
    baseline,
    schema: roleSchema,
    diff,
    persist: { routeKey: `role:${role?.id ?? 'new'}` },
    submit: async (v) => {
      if (role) {
        await updateRole({ variables: { id: role.id, input: toUpdateRoleInput(v) } })
        return
      }
      const res = await createRole({ variables: { input: toCreateRoleInput(v) } })
      const id = res.data?.createRole.id
      if (!id) throw new Error('Role creation returned no id.')
      createdId.current = id
    },
    // The role IS saved - the redirect onto its edit page must not ask about discarding it
    onSaved: isEdit ? undefined : () => navigate(`/admins/roles/${createdId.current}/edit`, { replace: true }),
  })
  const { form } = record

  // The grant list lives in the form - the draft hook is the operations and derivations over it
  const scopes = useStore(form.store, (s) => s.values.scopes)
  const slug = useStore(form.store, (s) => s.values.slug)
  const setScopes = useCallback((update: (prev: Scope[]) => Scope[]) => form.setFieldValue('scopes', update), [form])
  const draft = useRoleDraft(scopes, setScopes, groups)

  // Default the active group to the first one with grants on Edit, or the first registered group on Create
  const [activeGroupID, setActiveGroupID] = useState<string | null>(null)
  if (!activeGroupID && groups.length > 0) {
    const firstGranted = groups.find((g) => draft.grantedCountByGroup[g.id] > 0)
    setActiveGroupID((firstGranted ?? groups[0]).id)
  }
  const activeGroup = groups.find((g) => g.id === activeGroupID) ?? null
  const { summary } = draft

  return (
    <RecordFormPage
      record={record}
      backTo={role ? `/admins/roles/${role.id}` : '/admins/roles'}
      backLabel={isEdit ? 'Back to role' : 'Back to roles'}
      title={role?.name ?? 'New role'}
      meta={
        <>
          <code className="font-mono">{slug || '-'}</code>
          <span>·</span>
          <span>{summary.grantCount} grant{summary.grantCount === 1 ? '' : 's'}</span>
          {summary.groupsWithGrants.length > 0 && (
            <>
              <span>·</span>
              <span>{summary.groupsWithGrants.join(', ')}</span>
            </>
          )}
        </>
      }
      createLabel="Create role"
      labels={ROLE_LABELS}
    >
      <div className="grid gap-6 px-6 py-6">
        <section className="grid gap-3 rounded-md border p-4">
          <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
            <form.AppField name="slug">
              {(f) => (
                <f.TextField
                  id="role-slug"
                  label="Slug"
                  disabled={isEdit}
                  placeholder="warehouse_manager"
                  hint={isEdit ? 'Slug is immutable.' : 'Short identifier referenced in audits.'}
                />
              )}
            </form.AppField>
            <form.AppField name="name">
              {(f) => <f.TextField id="role-name" label="Name" placeholder="Warehouse Manager" />}
            </form.AppField>
          </div>
          <form.AppField name="description">
            {(f) => <f.TextareaField id="role-description" label="Description" placeholder="What does this role do?" />}
          </form.AppField>
        </section>

        <SummaryChipRow draft={draft} />
        <form.Field name="scopes">{(f) => <FieldError field={f} />}</form.Field>

        <div className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)]">
          <aside className="lg:sticky lg:top-4 lg:self-start">
            {groupsQuery.loading ? (
              <p className="text-sm text-muted-foreground">Loading groups…</p>
            ) : (
              <GroupSidebar
                groups={groups}
                activeGroupID={activeGroupID}
                grantedCount={draft.grantedCountByGroup}
                onSelect={setActiveGroupID}
              />
            )}
          </aside>
          <div>
            {activeGroup ? (
              <GroupPanel group={activeGroup} draft={draft} channels={channels} scopes={scopes} />
            ) : (
              <p className="text-sm text-muted-foreground">No permission groups registered.</p>
            )}
          </div>
        </div>
      </div>
    </RecordFormPage>
  )
}

// SummaryChipRow renders the live, at-a-glance summary of the role's shape: grant count, groups touched, channels
// covered, locales covered - Updates as the operator edits
function SummaryChipRow({ draft }: { draft: RoleDraftState }) {
  const { summary } = draft
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/20 px-3 py-2 text-xs">
      <Badge variant="secondary" className="font-normal">
        {summary.grantCount} grant{summary.grantCount === 1 ? '' : 's'}
      </Badge>
      {summary.groupsWithGrants.length > 0 ? (
        <Badge variant="outline" className="font-normal">
          {summary.groupsWithGrants.length} group{summary.groupsWithGrants.length === 1 ? '' : 's'}: {summary.groupsWithGrants.join(' · ')}
        </Badge>
      ) : (
        <span className="text-muted-foreground">No groups touched yet.</span>
      )}
      {(summary.explicitChannels.length > 0 || summary.anyChannel) && (
        <Badge variant="outline" className="font-normal">
          Channels:{' '}
          {summary.anyChannel && summary.explicitChannels.length > 0
            ? `${summary.explicitChannels.length} explicit + any`
            : summary.anyChannel
              ? 'any'
              : `${summary.explicitChannels.length}`}
        </Badge>
      )}
      {(summary.explicitLocales.length > 0 || summary.anyLocale) && (
        <Badge variant="outline" className="font-normal">
          Locales:{' '}
          {summary.anyLocale && summary.explicitLocales.length > 0
            ? `${summary.explicitLocales.length} explicit + any`
            : summary.anyLocale
              ? 'any'
              : summary.explicitLocales.join(', ')}
        </Badge>
      )}
    </div>
  )
}
