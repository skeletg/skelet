import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { MailCheckIcon, Trash2Icon } from 'lucide-react'
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
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { RoleSelector } from '@/components/RoleSelector'
import { useAuth } from '@/shared/auth/auth'
import { LoadError } from '@/shared/errors/LoadError'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { added, removed, display } from '@/shared/saving/diff'
import {
  AdminDocument,
  AdminsDocument,
  DeleteAdminDocument,
  UpdateAdminDocument,
} from './Admins.gql'
import { PluginSections } from '@/plugins/detail-sections'

export function AdminDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { auth } = useAuth()
  const isSelf = auth?.admin.id === id

  const { data, loading, error, refetch: refetchAdmin } = useQuery(AdminDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const admin = data?.admin

  const refetch = [
    { query: AdminDocument, variables: { id } },
    { query: AdminsDocument },
  ]
  const [updateAdmin] = useMutation(UpdateAdminDocument, { refetchQueries: refetch })
  const [deleteAdmin, deleteState] = useMutation(DeleteAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Page save registry: profile and roles stage their edits locally and the header Save applies them through the
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

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteAdmin({ variables: { id } })
      navigate('/admins', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  if (loading) return <p className="text-sm text-muted-foreground">Loading admin…</p>
  if (error) {
    return (
      <LoadError
        title="Couldn't load admin"
        message={error.message}
        onRetry={() => void refetchAdmin()}
        retrying={loading}
      />
    )
  }
  if (!admin) return <p className="text-sm text-muted-foreground">Admin not found.</p>

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/admins"
        backLabel="All admins"
        title={
          <>
            {admin.firstName} {admin.lastName}
            {isSelf && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">(you)</span>
            )}
          </>
        }
        badges={
          admin.isSuperuser ? (
            <Badge>superuser</Badge>
          ) : admin.isActive ? (
            <Badge variant="secondary">active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">inactive</Badge>
          )
        }
        meta={
          <>
            <span className="flex items-center gap-1.5">
              <span>{admin.email}</span>
              <MailCheckIcon className="size-3.5" aria-label="Immutable" />
            </span>
            <span>·</span>
            <span>Joined {new Date(admin.createdAt).toLocaleString()}</span>
          </>
        }
        actions={
          <Button
            type="button"
            size="sm"
            disabled={!isDirty || savingAll}
            onClick={() => { setSaveError(null); setConfirmingSave(true) }}
          >
            {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
          </Button>
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Profile"
          description={
            isSelf ? (
              <>
                Email can't be changed. Your password, two-factor and sessions live in{' '}
                <Link to="/preferences" className="text-primary hover:underline">
                  Preferences
                </Link>
                .
              </>
            ) : (
              "Email and password can't be changed here - those are self-service."
            )
          }
        >
          <ProfileCard
            admin={admin}
            save={(input) => updateAdmin({ variables: { id, input } })}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Roles"
          description={
            admin.isSuperuser
              ? 'Superusers bypass all role-based permission checks. Role assignments have no effect.'
              : 'Which roles this admin belongs to - grants the union of their permissions.'
          }
        >
          {!admin.isSuperuser && (
            <RolesCard
              currentSlugs={admin.roles.map((r) => r.slug)}
              save={(roleSlugs) => updateAdmin({ variables: { id, input: { roleSlugs } } })}
            />
          )}
        </DetailPageSection>

        {!admin.isSuperuser && admin.permissions.length > 0 && (
          <DetailPageSection
            title="Effective permissions"
            description="Union of permissions granted by the assigned roles."
          >
            <ul className="flex flex-wrap gap-1.5">
              {admin.permissions.map((p) => (
                <li
                  key={p.id}
                  className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs"
                  title={p.name}
                >
                  {p.slug}
                </li>
              ))}
            </ul>
          </DetailPageSection>
        )}

        <DetailPageSection
          title={<span className="text-destructive">Danger zone</span>}
          description={
            isSelf
              ? "You can't delete your own account."
              : 'Deleting an admin revokes access immediately and invalidates any refresh tokens.'
          }
        >
          <div>
            <Button
              variant="destructive"
              disabled={isSelf}
              onClick={() => { setDeleteError(null); setConfirmingDelete(true) }}
            >
              <Trash2Icon className="size-4" />
              Delete admin
            </Button>
          </div>
        </DetailPageSection>
        <PluginSections target="admin" entity={admin ?? null} mode="edit" />
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
            <AlertDialogTitle>
              Delete {admin.firstName} {admin.lastName}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Access is revoked immediately and any refresh tokens become invalid. This cannot
              be undone.
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
              onClick={(e) => { e.preventDefault(); onDelete() }}
            >
              {deleteState.loading ? 'Deleting…' : 'Delete admin'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>


      {/* Review-before-save modal listing the staged profile/role changes */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Admin · ${admin.firstName} ${admin.lastName}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// ---------- Section cards ----------

type AdminView = {
  firstName: string
  lastName: string
  isActive: boolean
}

function ProfileCard({
  admin,
  save,
}: {
  admin: AdminView
  save: (input: { firstName?: string; lastName?: string; isActive?: boolean }) => Promise<unknown>
}) {
  const [firstName, setFirstName] = useState(admin.firstName)
  const [lastName, setLastName] = useState(admin.lastName)
  const [isActive, setIsActive] = useState(admin.isActive)

  // Re-hydrate when the loaded admin changes, e.g. after a save +
  // refetch (adjust-state-during-render)
  const [prevAdmin, setPrevAdmin] = useState(admin)
  if (prevAdmin !== admin) {
    setPrevAdmin(admin)
    setFirstName(admin.firstName)
    setLastName(admin.lastName)
    setIsActive(admin.isActive)
  }

  // Staged save: edits register with the page save registry; the header
  // Save + confirm modal applies them
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    const field = (label: string, before: string, after: string) => {
      if (before !== after) out.push({ label, before, after })
    }
    field('first name', admin.firstName, firstName)
    field('last name', admin.lastName, lastName)
    field('active', display(admin.isActive), display(isActive))
    return out
  }, [admin, firstName, lastName, isActive])

  useSaveSection({
    id: 'profile',
    title: 'Profile',
    diff,
    save: async () => {
      if (firstName.trim() === '') throw new Error('First name is required.')
      if (lastName.trim() === '') throw new Error('Last name is required.')
      await save({ firstName, lastName, isActive })
    },
  })

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="ad-first">First name</Label>
          <Input id="ad-first" value={firstName} onChange={(e) => setFirstName(e.target.value)} required />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="ad-last">Last name</Label>
          <Input id="ad-last" value={lastName} onChange={(e) => setLastName(e.target.value)} required />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Checkbox id="ad-active" checked={isActive} onCheckedChange={(v) => setIsActive(!!v)} />
        <Label htmlFor="ad-active" className="cursor-pointer">Active - inactive admins can't sign in</Label>
      </div>
    </div>
  )
}

function RolesCard({
  currentSlugs,
  save,
}: {
  currentSlugs: string[]
  save: (slugs: string[]) => Promise<unknown>
}) {
  const [selected, setSelected] = useState<string[]>(currentSlugs)

  // Reset the draft when the saved slugs change by CONTENT (the parent rebuilds the array every render, so compare
  // joined strings, not identity) - Adjust-state-during-render
  const slugsLabel = currentSlugs.join(', ')
  const [prevSlugsLabel, setPrevSlugsLabel] = useState(slugsLabel)
  if (prevSlugsLabel !== slugsLabel) {
    setPrevSlugsLabel(slugsLabel)
    setSelected(currentSlugs)
  }

  const dirty =
    selected.length !== currentSlugs.length ||
    selected.some((s) => !currentSlugs.includes(s))

  const diff = useMemo<SectionDiffEntry[]>(() => {
    if (!dirty) return []
    const out: SectionDiffEntry[] = []
    for (const s of [...selected].sort()) {
      if (!currentSlugs.includes(s)) out.push(added(`role · ${s}`))
    }
    for (const s of [...currentSlugs].sort()) {
      if (!selected.includes(s)) out.push(removed(`role · ${s}`))
    }
    return out
  }, [dirty, selected, currentSlugs])

  useSaveSection({
    id: 'roles',
    title: 'Roles',
    diff,
    save: async () => {
      await save(selected)
    },
  })

  return <RoleSelector selected={selected} onChange={setSelected} />
}
