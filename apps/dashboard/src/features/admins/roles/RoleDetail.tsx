import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PencilIcon, PlusIcon, TrashIcon, UserMinusIcon } from 'lucide-react'
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
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { LoadingState } from '@/components/ui/loading-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { AddRoleMembersDialog } from '@/features/admins/roles/AddRoleMembersDialog'
import {
  AdminsDocument,
  UpdateAdminDocument,
} from '@/features/admins/Admins.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import {
  DeleteRoleDocument,
  RoleDocument,
  RolesDocument,
} from './Roles.gql'
import { buildRoleSummary, headlineForRole } from './roleSummary'
import { PluginSections } from '@/plugins/detail-sections'
import { usePageTitle } from '@/lib/page-title'

// RoleDetail is one flowing read-only page: overview, then the GRANTED permissions as a
// group-rail list (ungranted slugs live in the /admins/permissions catalog, repeating all 84
// here dimmed buried the actual grants), then members - no tabs, so nothing is hidden a click
// away and the layout's table of contents can index the sections
// The role id can come from props or from the URL (standalone /admins/roles/:id route)
export function RoleDetail({ roleId }: { roleId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = roleId ?? params.id ?? ''
  const navigate = useNavigate()

  const roleQuery = useQuery(RoleDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const adminsQuery = useQuery(AdminsDocument)
  const channelsQuery = useQuery(ChannelsDocument)
  const groupsQuery = useQuery(PermissionGroupsDocument)

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [addingMembers, setAddingMembers] = useState(false)
  const [removing, setRemoving] = useState<string | null>(null)
  const [removeError, setRemoveError] = useState<string | null>(null)

  const [deleteRole, { loading: deleting }] = useMutation(DeleteRoleDocument, {
    refetchQueries: [{ query: RolesDocument }],
  })
  const [updateAdmin] = useMutation(UpdateAdminDocument, {
    refetchQueries: [
      { query: AdminsDocument },
      { query: RoleDocument, variables: { id } },
    ],
  })

  const role = roleQuery.data?.role
  usePageTitle(role?.name ?? null) // browser + workspace tab show the role, not "Role detail"
  const groups = useMemo(
    () => groupsQuery.data?.permissionGroups ?? [],
    [groupsQuery.data],
  )
  const channels = useMemo(
    () => channelsQuery.data?.channels ?? [],
    [channelsQuery.data],
  )

  const channelNameByID = useMemo(() => {
    const map = new Map<string, string>()
    for (const c of channels) map.set(c.id, c.name)
    return (cid: string) => map.get(cid) ?? cid.slice(0, 8)
  }, [channels])

  const summary = useMemo(
    () => buildRoleSummary(role?.scopes ?? []),
    [role?.scopes],
  )

  // scopesByPermission groups the flat scope list into per-permission
  // bins for read-only display in the grants list
  const scopesByPermission = useMemo(() => {
    const map = new Map<string, Array<{ channelId: string | null; locale: string | null }>>()
    for (const s of role?.scopes ?? []) {
      const list = map.get(s.permission.id) ?? []
      list.push({ channelId: s.channelId ?? null, locale: s.locale ?? null })
      map.set(s.permission.id, list)
    }
    return map
  }, [role?.scopes])

  // Only groups where this role holds something - the full catalog lives at /admins/permissions
  const grantedGroups = useMemo(
    () =>
      groups
        .map((g) => ({
          group: g,
          granted: g.permissions.filter((p) => (scopesByPermission.get(p.id)?.length ?? 0) > 0),
        }))
        .filter(({ granted }) => granted.length > 0),
    [groups, scopesByPermission],
  )

  const members = useMemo(() => {
    if (!role) return []
    return (adminsQuery.data?.admins ?? []).filter((a) =>
      a.roles.some((r) => r.slug === role.slug),
    )
  }, [adminsQuery.data, role])

  async function onConfirmDelete() {
    setDeleteError(null)
    try {
      await deleteRole({ variables: { id } })
      navigate('/admins/roles', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  async function onRemoveMember(adminId: string) {
    if (!role) return
    setRemoving(adminId)
    setRemoveError(null)
    try {
      const admin = (adminsQuery.data?.admins ?? []).find((a) => a.id === adminId)
      if (!admin) return
      const nextSlugs = admin.roles.map((r) => r.slug).filter((s) => s !== role.slug)
      await updateAdmin({ variables: { id: adminId, input: { roleSlugs: nextSlugs } } })
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : 'Remove failed.')
    } finally {
      setRemoving(null)
    }
  }

  if (roleQuery.loading) return <LoadingState label="Loading role…" />
  if (roleQuery.error) {
    return (
      <LoadError
        title="Couldn't load role"
        message={roleQuery.error.message}
        onRetry={() => void roleQuery.refetch()}
        retrying={roleQuery.loading}
      />
    )
  }
  if (!role) return <p className="px-6 py-6 text-sm text-muted-foreground">Role not found.</p>

  const headline = headlineForRole(summary, channelNameByID)

  return (
    <>
      <DetailPageHeader
        backTo="/admins/roles"
        title={role.name}
        badges={summary.isFullyGlobal && (
          <Badge variant="secondary" className="font-normal">Global</Badge>
        )}
        meta={
          <>
            <code className="font-mono">{role.slug}</code>
            <span>·</span>
            <span>{summary.grantCount} grant{summary.grantCount === 1 ? '' : 's'}</span>
            <span>·</span>
            <span>{members.length} member{members.length === 1 ? '' : 's'}</span>
          </>
        }
        actions={
          <>
            <Button variant="outline" size="sm" asChild>
              <Link to={`/admins/roles/${role.id}/edit`}>
                <PencilIcon className="size-4" />
                Edit
              </Link>
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setDeleteError(null)
                setConfirmingDelete(true)
              }}
            >
              <TrashIcon className="size-4" />
              Delete
            </Button>
          </>
        }
      />

      <DetailPageBody>
        <DetailPageSection title="Overview">
          <p className="text-sm">{headline}</p>
          {role.description && (
            <p className="text-sm text-muted-foreground">{role.description}</p>
          )}
          <SummaryChipRow summary={summary} channelNameByID={channelNameByID} />
        </DetailPageSection>

        <DetailPageSection
          title="Permissions"
          description={
            summary.grantCount === 0
              ? 'Nothing granted yet.'
              : `${summary.grantCount} grant${summary.grantCount === 1 ? '' : 's'} across ${grantedGroups.length} group${grantedGroups.length === 1 ? '' : 's'} - ungranted permissions are in the catalog, not repeated here.`
          }
          trailing={
            <Button variant="outline" size="sm" asChild>
              <Link to={`/admins/roles/${role.id}/edit`}>
                <PencilIcon className="size-4" />
                Edit grants
              </Link>
            </Button>
          }
        >
          {summary.grantCount === 0 ? (
            <p className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
              This role has no permission grants.
            </p>
          ) : groupsQuery.loading ? (
            <p className="text-sm text-muted-foreground">Loading groups…</p>
          ) : (
            <div className="divide-y overflow-hidden rounded-md border">
              {grantedGroups.map(({ group, granted }) => (
                <GrantedGroupRow
                  key={group.id}
                  group={group}
                  granted={granted}
                  scopesByPermission={scopesByPermission}
                  channelNameByID={channelNameByID}
                />
              ))}
            </div>
          )}
        </DetailPageSection>

        <DetailPageSection
          title="Members"
          description={`${members.length} admin${members.length === 1 ? '' : 's'} hold${members.length === 1 ? 's' : ''} this role.`}
          trailing={
            <Button size="sm" onClick={() => setAddingMembers(true)}>
              <PlusIcon className="size-4" />
              Add members
            </Button>
          }
        >
          {removeError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn't remove member</AlertTitle>
              <AlertDescription>{removeError}</AlertDescription>
            </Alert>
          )}
          <div className="overflow-hidden rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Other roles</TableHead>
                  <TableHead className="w-20 text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4} className="text-center text-muted-foreground">
                      No members yet.
                    </TableCell>
                  </TableRow>
                ) : (
                  members.map((m) => {
                    const otherRoles = m.roles.filter((r) => r.slug !== role.slug)
                    return (
                      <TableRow key={m.id}>
                        <TableCell className="font-medium">
                          {m.firstName} {m.lastName}
                        </TableCell>
                        <TableCell className="text-muted-foreground">{m.email}</TableCell>
                        <TableCell>
                          {otherRoles.length === 0 ? (
                            <span className="text-xs text-muted-foreground">none</span>
                          ) : (
                            <div className="flex flex-wrap gap-1.5">
                              {otherRoles.map((r) => (
                                <Badge key={r.id} variant="secondary">{r.name}</Badge>
                              ))}
                            </div>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={removing === m.id}
                            onClick={() => onRemoveMember(m.id)}
                          >
                            <UserMinusIcon className="size-4" />
                            {removing === m.id ? 'Removing…' : 'Remove'}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </DetailPageSection>
        <PluginSections target="role" entity={role} mode="edit" />
      </DetailPageBody>

      <AddRoleMembersDialog
        roleId={role.id}
        roleSlug={role.slug}
        roleName={role.name}
        open={addingMembers}
        onOpenChange={setAddingMembers}
      />

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
            <AlertDialogTitle>Delete {role.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the role and its permission bindings. Admins currently in this role
              will lose the permissions it granted. Roles with members cannot be deleted -
              remove members first.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
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
            >
              {deleting ? 'Deleting…' : 'Delete role'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

// SummaryChipRow is the read-only twin of the editor's chip row - shows grant count, groups touched, channel reach,
// locale reach as muted badges - Mirrors the headline data in numeric form
function SummaryChipRow({
  summary,
  channelNameByID,
}: {
  summary: ReturnType<typeof buildRoleSummary>
  channelNameByID: (id: string) => string
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/20 px-3 py-2 text-xs">
      <Badge variant="secondary" className="font-normal">
        {summary.grantCount} grant{summary.grantCount === 1 ? '' : 's'}
      </Badge>
      <Badge variant="outline" className="font-normal">
        {summary.uniquePermissionCount} permission
        {summary.uniquePermissionCount === 1 ? '' : 's'}
      </Badge>
      {(summary.explicitChannelIds.length > 0 || summary.anyChannel) && (
        <Badge variant="outline" className="font-normal">
          Channels:{' '}
          {summary.anyChannel && summary.explicitChannelIds.length > 0
            ? `${summary.explicitChannelIds.length} explicit + any`
            : summary.anyChannel
              ? 'any'
              : summary.explicitChannelIds.map(channelNameByID).join(', ')}
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

// GrantedGroupRow is one group in the grants list: the muted rail names the group and counts
// granted/total, the right side lists ONLY the granted permissions with their scope badges
// A granted `.manage` whose `.view` sibling exists gets an "implies view" hint, matching the
// runtime behavior in usePermissions/admin.effectiveSlugs
function GrantedGroupRow({
  group,
  granted,
  scopesByPermission,
  channelNameByID,
}: {
  group: { id: string; name: string; permissions: { id: string; slug: string; name: string }[] }
  granted: { id: string; slug: string; name: string }[]
  scopesByPermission: Map<string, Array<{ channelId: string | null; locale: string | null }>>
  channelNameByID: (id: string) => string
}) {
  function impliesView(slug: string): boolean {
    if (!slug.endsWith('.manage')) return false
    const viewSlug = slug.slice(0, -'.manage'.length) + '.view'
    return group.permissions.some((p) => p.slug === viewSlug)
  }

  return (
    <div className="flex items-stretch">
      <div className="flex w-44 shrink-0 items-center gap-2 border-r bg-muted px-3 py-2">
        <span className="text-sm font-medium">{group.name}</span>
        <span className="ml-auto text-xs tabular-nums text-muted-foreground">
          {granted.length}/{group.permissions.length}
        </span>
      </div>
      <div className="flex flex-1 flex-col gap-1.5 px-3 py-2">
        {granted.map((permission) => {
          const scopes = scopesByPermission.get(permission.id) ?? []
          // a lone (null, null) tuple is the global default - badges only earn their space
          // when a grant is narrowed to a channel or locale
          const narrowed = scopes.filter((s) => s.channelId !== null || s.locale !== null)
          return (
            <div key={permission.id} className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span className="text-sm" title={permission.slug}>
                {permission.name}
              </span>
              {impliesView(permission.slug) && (
                <span className="text-xs text-muted-foreground italic">implies view</span>
              )}
              {narrowed.map((s, i) => (
                <Badge key={i} variant="outline" className="font-normal text-muted-foreground">
                  {s.channelId ? channelNameByID(s.channelId) : 'Any channel'}
                  {s.locale ? ` · ${s.locale}` : ''}
                </Badge>
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// RoleDetailPage is the standalone /admins/roles/:id route - RoleDetail reads the id from the
// URL itself, this just adds the standard detail-page chrome
export function RoleDetailPage() {
  return (
    <DetailPageLayout>
      <RoleDetail />
    </DetailPageLayout>
  )
}
