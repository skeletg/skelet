import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { RefreshCwIcon, Trash2Icon } from 'lucide-react'
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
import { LoadError } from '@/shared/errors/LoadError'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { PermissionSelector } from '@/components/PermissionSelector'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { NewTokenDialog } from './NewTokenDialog'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { added, changed, removed, display } from '@/shared/saving/diff'
import {
  DeleteIntegrationDocument,
  IntegrationsDocument,
  RegenerateIntegrationTokenDocument,
  SetIntegrationActiveDocument,
  UpdateIntegrationDocument,
} from './Integrations.gql'
import { PluginSections } from '@/plugins/detail-sections'

// IntegrationDetail is a standalone PDP-style page reached from /integrations/:id - No admin `integration(id)` query exists on the backend,
// so we pull the full list and pick the one we need - Apollo's cache makes this efficient when the list was already loaded
export function IntegrationDetail() {
  const params = useParams<{ id?: string }>()
  const id = params.id ?? ''
  const navigate = useNavigate()

  const { data, loading, error, refetch: refetchIntegrations } = useQuery(IntegrationsDocument, {
    notifyOnNetworkStatusChange: true,
  })
  const integration = useMemo(
    () => (data?.integrations ?? []).find((i) => i.id === id),
    [data, id],
  )

  const refetch = [{ query: IntegrationsDocument }]
  const [updateIntegration] = useMutation(UpdateIntegrationDocument, {
    refetchQueries: refetch,
  })
  const [setActive] = useMutation(SetIntegrationActiveDocument, {
    refetchQueries: refetch,
  })
  const [regenerate, regenState] = useMutation(RegenerateIntegrationTokenDocument, {
    refetchQueries: refetch,
  })
  const [deleteIntegration, deleteState] = useMutation(DeleteIntegrationDocument, {
    refetchQueries: refetch,
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [newToken, setNewToken] = useState<{ token: string; name: string } | null>(null)

  // Page save registry: profile, permissions, and the active switch all stage their edits and the header Save applies
  // them through the SaveConfirmDialog diff modal - the dashboard-wide convention
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

  async function onRegenerate() {
    if (!integration) return
    try {
      const { data } = await regenerate({ variables: { id: integration.id } })
      if (data) {
        setNewToken({
          token: data.regenerateIntegrationToken.rawToken,
          name: data.regenerateIntegrationToken.integration.name,
        })
      }
    } catch {
      /* noop */
    }
  }

  async function onDelete() {
    if (!integration) return
    setDeleteError(null)
    try {
      await deleteIntegration({ variables: { id: integration.id } })
      navigate('/integrations', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  if (loading && !integration) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading integration…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load integration"
          message={error.message}
          onRetry={() => void refetchIntegrations()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!integration) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Integration not found.</p>
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/integrations"
        backLabel="Back to integrations"
        title={integration.name}
        badges={
          <Badge variant="outline" className="font-normal">
            {integration.permissions.length} permission
            {integration.permissions.length === 1 ? '' : 's'}
          </Badge>
        }
        meta={
          <>
            <span>{integration.description || 'No description'}</span>
            <span>·</span>
            <span>Created {new Date(integration.createdAt).toLocaleDateString()}</span>
          </>
        }
        actions={
          <>
            <ActiveToggle
              isActive={integration.isActive}
              save={async (active) => {
                await setActive({ variables: { id: integration.id, active } })
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onRegenerate}
              disabled={regenState.loading}
            >
              <RefreshCwIcon className="size-4" />
              {regenState.loading ? 'Regenerating…' : 'Regenerate token'}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!isDirty || savingAll}
              onClick={() => { setSaveError(null); setConfirmingSave(true) }}
            >
              {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
            </Button>
          </>
        }
      />

      <DetailPageBody>
        <ProfileSection
          integration={integration}
          save={async (input) => {
            await updateIntegration({ variables: { input: { id: integration.id, ...input } } })
          }}
        />

        <PermissionsSection
          currentSlugs={integration.permissions.map((p) => p.slug)}
          save={async (permissionSlugs) => {
            await updateIntegration({
              variables: { input: { id: integration.id, permissionSlugs } },
            })
          }}
        />

        <section className="grid gap-4 px-6 py-6">
          <div className="grid gap-1">
            <h3 className="text-lg font-semibold tracking-tight">
              <span className="mr-1.5 text-primary">#</span>
              Danger zone
            </h3>
            <p className="text-sm text-muted-foreground">
              Deleting the integration revokes its token immediately - any client calling
              the API with it will start getting 401 responses.
            </p>
          </div>
          <div>
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                setDeleteError(null)
                setConfirmingDelete(true)
              }}
            >
              <Trash2Icon className="size-4" />
              Delete integration
            </Button>
          </div>
        </section>
        <PluginSections target="integration" entity={integration} mode="edit" />
      </DetailPageBody>

      <NewTokenDialog
        token={newToken?.token ?? null}
        integrationName={newToken?.name ?? null}
        open={!!newToken}
        onClose={() => setNewToken(null)}
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
            <AlertDialogTitle>Delete {integration.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This cannot be undone. Clients using this integration's token will be rejected
              immediately.
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
              {deleteState.loading ? 'Deleting…' : 'Delete integration'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists every staged change across the
          page's sections (status, profile, permissions) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Integration · ${integration.name}`}
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

// Header active switch - staged like every other field edit - The switch only flips local state; the change lands via
// the page Save through the SetIntegrationActive mutation
function ActiveToggle({
  isActive,
  save,
}: {
  isActive: boolean
  save: (active: boolean) => Promise<void>
}) {
  const [active, setActive] = useState(isActive)

  // Reset the draft when the saved flag changes (adjust state during render)
  const [prevIsActive, setPrevIsActive] = useState(isActive)
  if (prevIsActive !== isActive) {
    setPrevIsActive(isActive)
    setActive(isActive)
  }

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      active === isActive
        ? []
        : [{ label: 'active', before: display(isActive), after: display(active) }],
    [active, isActive],
  )

  useSaveSection({
    id: 'status',
    title: 'Status',
    diff,
    save: () => save(active),
  })

  return (
    <label
      htmlFor="integration-active-header"
      className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm"
    >
      <Switch
        id="integration-active-header"
        checked={active}
        onCheckedChange={setActive}
        aria-label="Toggle active"
      />
      <span className={active ? 'font-medium' : 'text-muted-foreground'}>
        {active ? 'Active' : 'Inactive'}
        {active !== isActive && <span className="ml-1 text-xs text-muted-foreground">(unsaved)</span>}
      </span>
    </label>
  )
}

type IntegrationView = {
  name: string
  description: string
  expiresAt?: string | null
}

function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function ProfileSection({
  integration,
  save,
}: {
  integration: IntegrationView
  save: (input: {
    name?: string
    description?: string
    expiresAt?: string | null
    clearExpiresAt?: boolean
  }) => Promise<void>
}) {
  const [name, setName] = useState(integration.name)
  const [description, setDescription] = useState(integration.description)
  const [hasExpiry, setHasExpiry] = useState(!!integration.expiresAt)
  const [expiresAt, setExpiresAt] = useState(toLocalInput(integration.expiresAt))

  // Re-hydrate the draft when the server payload changes (post-save
  // refetch) - adjust state during render instead of an effect
  const [prevIntegration, setPrevIntegration] = useState(integration)
  if (prevIntegration !== integration) {
    setPrevIntegration(integration)
    setName(integration.name)
    setDescription(integration.description)
    setHasExpiry(!!integration.expiresAt)
    setExpiresAt(toLocalInput(integration.expiresAt))
  }

  const currentExpiryIso = integration.expiresAt ?? null
  const newExpiryIso = hasExpiry && expiresAt ? new Date(expiresAt).toISOString() : null
  // Compare at the minute precision the <input type=datetime-local> actually edits - comparing a round-tripped ISO
  // against the stored one flags stored seconds/millis as a phantom change
  const currentExpiryLocal = toLocalInput(currentExpiryIso)
  const newExpiryLocal = hasExpiry ? expiresAt : ''

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    const nameEntry = changed('name', integration.name, name)
    if (nameEntry) out.push(nameEntry)
    const descEntry = changed('description', integration.description, description)
    if (descEntry) out.push(descEntry)
    const expiryEntry = changed('expires at', currentExpiryLocal, newExpiryLocal)
    if (expiryEntry) out.push(expiryEntry)
    return out
  }, [integration.name, integration.description, name, description, currentExpiryLocal, newExpiryLocal])

  useSaveSection({
    id: 'profile',
    title: 'Profile',
    diff,
    save: async () => {
      if (name.trim() === '') throw new Error('Name is required.')
      if (hasExpiry && !expiresAt) throw new Error('Pick an expiry date or disable the expiry.')
      await save({
        name,
        description,
        ...(hasExpiry ? { expiresAt: newExpiryIso } : { clearExpiresAt: true }),
      })
    },
  })

  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="grid gap-1">
        <h3 className="text-lg font-semibold tracking-tight">
          <span className="mr-1.5 text-primary">#</span>
          Profile
        </h3>
        <p className="text-sm text-muted-foreground">
          Name, description, and expiry.
        </p>
      </div>
      <div className="grid gap-4">
        <div className="grid gap-2">
          <Label htmlFor="int-name">Name</Label>
          <Input
            id="int-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="int-desc">Description</Label>
          <Textarea
            id="int-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="grid gap-2">
          <div className="flex items-center gap-2">
            <Checkbox
              id="int-expiry"
              checked={hasExpiry}
              onCheckedChange={(v) => setHasExpiry(!!v)}
            />
            <Label htmlFor="int-expiry" className="cursor-pointer">
              Set expiry date
            </Label>
          </div>
          {hasExpiry && (
            <Input
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              required
            />
          )}
        </div>
      </div>
    </section>
  )
}

function PermissionsSection({
  currentSlugs,
  save,
}: {
  currentSlugs: string[]
  save: (slugs: string[]) => Promise<void>
}) {
  const [selected, setSelected] = useState<string[]>(currentSlugs)

  // The parent rebuilds the slug array every render, so change detection keys off the joined value; stableSlugs carries
  // the same values with a render-stable identity for the hooks below (slugs never contain commas)
  const slugsKey = currentSlugs.join(',')
  const stableSlugs = useMemo(
    () => (slugsKey === '' ? [] : slugsKey.split(',')),
    [slugsKey],
  )

  // Reset the draft when the saved slugs change (adjust state during render)
  const [prevSlugsKey, setPrevSlugsKey] = useState(slugsKey)
  if (prevSlugsKey !== slugsKey) {
    setPrevSlugsKey(slugsKey)
    setSelected(stableSlugs)
  }

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    for (const s of [...selected].sort()) {
      if (!stableSlugs.includes(s)) out.push(added(`permission · ${s}`))
    }
    for (const s of [...stableSlugs].sort()) {
      if (!selected.includes(s)) out.push(removed(`permission · ${s}`))
    }
    return out
  }, [selected, stableSlugs])

  useSaveSection({
    id: 'permissions',
    title: 'Permissions',
    diff,
    save: () => save(selected),
  })

  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="grid gap-1">
        <h3 className="text-lg font-semibold tracking-tight">
          <span className="mr-1.5 text-primary">#</span>
          Permissions
        </h3>
        <p className="text-sm text-muted-foreground">
          Which admin API operations this token can call - the same permission catalog roles use,
          granted globally across stores. Changes apply on the next request.
        </p>
      </div>
      <PermissionSelector selected={selected} onChange={setSelected} />
    </section>
  )
}
