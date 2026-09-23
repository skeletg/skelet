import { useMemo, useState } from 'react'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { LoadingState } from '@/components/ui/loading-state'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import {
  CreateRoleDocument,
  RoleDocument,
  RolesDocument,
  UpdateRoleDocument,
} from '@/features/admins/roles/Roles.gql'
import { useRoleDraft, type GroupRow, type Scope } from './useRoleDraft'
import { GroupSidebar } from './GroupSidebar'
import { GroupPanel } from './GroupPanel'

// RoleEditPage is the unified create/edit screen for admin roles
// Route: /admins/roles/new (create) or /admins/roles/:id/edit (edit)
// Layout: header with identity fields + Save/Discard, then a
// permission-groups sidebar + active-group panel
//
// The local draft (scopes array + identity fields) is owned here and
// reset on Save (refetch hydrates the seed) - Discard is a single
// click on the page header
export function RoleEditPage() {
  const params = useParams<{ id?: string }>()
  const navigate = useNavigate()
  const isEdit = !!params.id

  const roleQuery = useQuery(RoleDocument, {
    variables: { id: params.id ?? '' },
    skip: !isEdit,
  })
  const groupsQuery = useQuery(PermissionGroupsDocument)
  const channelsQuery = useQuery(ChannelsDocument)

  const role = roleQuery.data?.role
  const groups: GroupRow[] = useMemo(
    () => groupsQuery.data?.permissionGroups ?? [],
    [groupsQuery.data],
  )
  const channels = channelsQuery.data?.channels ?? []

  // Identity fields - On Edit they hydrate from the loaded role; on Create they start blank - Slug is editable on
  // Create only - changing it post-create would break every audit trail that mentions the role by slug
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Hydrate when the loaded role changes (adjust-state-during-render)
  const [prevRole, setPrevRole] = useState<typeof role>(undefined)
  if (prevRole !== role) {
    setPrevRole(role)
    if (role) {
      setSlug(role.slug)
      setName(role.name)
      setDescription(role.description)
    }
  }

  // Seed the draft from the loaded scopes (or [] on Create) - The hook tracks its own initial snapshot for Discard; we
  // just feed it the GraphQL shape projected into Scope
  const seedScopes: Scope[] = useMemo(() => {
    if (!role) return []
    return role.scopes.map((s) => ({
      permissionId: s.permission.id,
      permissionSlug: s.permission.slug,
      channelId: s.channelId ?? null,
      locale: s.locale ?? null,
    }))
  }, [role])

  const draft = useRoleDraft({ scopes: seedScopes }, groups)

  // Default the active group to the first one with grants on Edit, or the first registered group on Create - Falls
  // through to whatever's first if nothing's granted yet
  const [activeGroupID, setActiveGroupID] = useState<string | null>(null)
  if (!activeGroupID && groups.length > 0) {
    const firstGranted = groups.find((g) => draft.grantedCountByGroup[g.id] > 0)
    setActiveGroupID((firstGranted ?? groups[0]).id)
  }

  const activeGroup = useMemo(
    () => groups.find((g) => g.id === activeGroupID) ?? null,
    [groups, activeGroupID],
  )

  const refetchQueries = isEdit && role
    ? [{ query: RolesDocument }, { query: RoleDocument, variables: { id: role.id } }]
    : [{ query: RolesDocument }]
  const [createRole, { loading: creating }] = useMutation(CreateRoleDocument, { refetchQueries })
  const [updateRole, { loading: updating }] = useMutation(UpdateRoleDocument, { refetchQueries })
  const saving = creating || updating

  const dirty = draft.dirty
    || (isEdit && role
      ? name !== role.name || description !== role.description
      : Boolean(slug || name || description))

  useUnsavedChangesGuard(dirty)

  async function onSave() {
    setSubmitError(null)
    if (!isEdit && !slug.trim()) {
      setSubmitError('Slug is required.')
      return
    }
    if (!name.trim()) {
      setSubmitError('Name is required.')
      return
    }
    if (draft.scopes.length === 0) {
      setSubmitError('A role with zero grants does nothing - grant at least one permission before saving.')
      return
    }

    const scopesInput = draft.scopes.map((s) => ({
      permissionSlug: s.permissionSlug,
      channelId: s.channelId,
      locale: s.locale,
    }))
    try {
      if (isEdit && role) {
        await updateRole({
          variables: {
            id: role.id,
            input: {
              name,
              description: description || null,
              scopes: scopesInput,
            },
          },
        })
        // Refetch hydrates seedScopes; useRoleDraft picks up the new
        // baseline - No manual reset needed
      } else {
        const result = await createRole({
          variables: {
            input: {
              slug,
              name,
              description: description || null,
              scopes: scopesInput,
            },
          },
        })
        const newID = result.data?.createRole.id
        // The role IS saved - the redirect onto its edit page must not ask about discarding it
        if (newID) await duringCommit(() => navigate(`/admins/roles/${newID}/edit`, { replace: true }))
      }
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  function onDiscard() {
    if (role) {
      setName(role.name)
      setDescription(role.description)
    } else {
      setSlug('')
      setName('')
      setDescription('')
    }
    draft.reset()
    setSubmitError(null)
  }

  if (isEdit && roleQuery.loading) {
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
  if (isEdit && !role) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Role not found.</p>
  }

  const headerTitle = isEdit ? role?.name ?? 'Role' : 'New role'
  const grantSummary = draft.summary

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo={isEdit && role ? `/admins/roles/${role.id}` : '/admins/roles'}
        backLabel={isEdit ? 'Back to role' : 'Back to roles'}
        title={headerTitle}
        meta={
          <>
            <code className="font-mono">{slug || '-'}</code>
            <span>·</span>
            <span>{grantSummary.grantCount} grant{grantSummary.grantCount === 1 ? '' : 's'}</span>
            {grantSummary.groupsWithGrants.length > 0 && (
              <>
                <span>·</span>
                <span>{grantSummary.groupsWithGrants.join(', ')}</span>
              </>
            )}
          </>
        }
        actions={
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onDiscard}
              disabled={!dirty || saving}
            >
              Discard
            </Button>
            <Button type="button" size="sm" onClick={onSave} disabled={!dirty || saving}>
              {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create role'}
            </Button>
          </>
        }
      />

      <div className="grid gap-6 px-6 py-6">
        <section className="grid gap-3 rounded-md border p-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 items-start">
            <div className="grid gap-1.5">
              <Label htmlFor="role-slug">Slug</Label>
              <Input
                id="role-slug"
                disabled={isEdit}
                required
                placeholder="warehouse_manager"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {isEdit ? 'Slug is immutable.' : 'Short identifier referenced in audits.'}
              </p>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="role-name">Name</Label>
              <Input
                id="role-name"
                required
                placeholder="Warehouse Manager"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="role-description">Description</Label>
            <Textarea
              id="role-description"
              placeholder="What does this role do?"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </section>

        <SummaryChipRow draft={draft} />

        {submitError && (
          <Alert variant="destructive">
            <AlertTitle>Save failed</AlertTitle>
            <AlertDescription>{submitError}</AlertDescription>
          </Alert>
        )}

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
              <GroupPanel
                group={activeGroup}
                draft={draft}
                channels={channels}
                scopes={draft.scopes}
              />
            ) : (
              <p className="text-sm text-muted-foreground">No permission groups registered.</p>
            )}
          </div>
        </div>
      </div>
    </DetailPageLayout>
  )
}

// SummaryChipRow renders the live, at-a-glance summary of the role's shape: grant count, groups touched, channels
// covered, locales covered - Updates as the operator edits
function SummaryChipRow({ draft }: { draft: ReturnType<typeof useRoleDraft> }) {
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
