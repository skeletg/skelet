import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
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
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { FieldError } from '@/components/form/fields'
import { LoadError } from '@/shared/errors/LoadError'
import { PermissionSelector } from '@/components/PermissionSelector'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { NewTokenDialog } from './NewTokenDialog'
import {
  DeleteIntegrationDocument,
  IntegrationsDocument,
  RegenerateIntegrationTokenDocument,
  SetIntegrationActiveDocument,
  UpdateIntegrationDocument,
  type IntegrationsQuery,
} from './Integrations.gql'
import { PluginSections } from '@/plugins/detail-sections'
import {
  INTEGRATION_DETAIL_LABELS,
  integrationDetailDiff,
  integrationDetailSchema,
  integrationProfileChanged,
  toIntegrationDetailInput,
  toIntegrationDetailValues,
} from './schemas/integration'

type Integration = IntegrationsQuery['integrations'][number]

// IntegrationDetail is a standalone PDP-style page reached from /integrations/:id - No admin `integration(id)` query exists on the backend,
// so we pull the full list and pick the one we need - Apollo's cache makes this efficient when the list was already loaded
export function IntegrationDetail() {
  const { id = '' } = useParams<{ id?: string }>()
  const { data, loading, error, refetch } = useQuery(IntegrationsDocument, {
    notifyOnNetworkStatusChange: true,
  })
  const integration = useMemo(() => (data?.integrations ?? []).find((i) => i.id === id), [data, id])

  if (loading && !integration) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading integration…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load integration"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!integration) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Integration not found.</p>
  }
  return <IntegrationForm integration={integration} />
}

// Status, profile and permissions are one form - the header Save reviews them together; the active
// flag goes through its own mutation, the rest through one update
function IntegrationForm({ integration }: { integration: Integration }) {
  const navigate = useNavigate()
  const refetchQueries = [{ query: IntegrationsDocument }]
  const [updateIntegration] = useMutation(UpdateIntegrationDocument, { refetchQueries })
  const [setActive] = useMutation(SetIntegrationActiveDocument, { refetchQueries })
  const [regenerate, regenState] = useMutation(RegenerateIntegrationTokenDocument, { refetchQueries })
  const [deleteIntegration, deleteState] = useMutation(DeleteIntegrationDocument, { refetchQueries })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [newToken, setNewToken] = useState<{ token: string; name: string } | null>(null)

  const baseline = useMemo(() => toIntegrationDetailValues(integration), [integration])
  const record = useRecordForm({
    id: 'integration',
    title: 'Integration',
    label: `Integration · ${integration.name}`,
    isEdit: true,
    baseline,
    schema: integrationDetailSchema,
    diff: integrationDetailDiff,
    persist: { routeKey: `integration:${integration.id}` },
    submit: async (v) => {
      if (v.isActive !== baseline.isActive) {
        await setActive({ variables: { id: integration.id, active: v.isActive } })
      }
      if (integrationProfileChanged(baseline, v)) {
        await updateIntegration({ variables: { input: toIntegrationDetailInput(v, integration.id) } })
      }
    },
  })
  const { form } = record
  const hasExpiry = useStore(form.store, (s) => s.values.hasExpiry)

  async function onRegenerate() {
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
    setDeleteError(null)
    try {
      await deleteIntegration({ variables: { id: integration.id } })
      navigate('/integrations', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
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
          {/* Staged like every other field - lands with the header Save */}
          <form.Field name="isActive">
            {(f) => (
              <label
                htmlFor="integration-active-header"
                className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm"
              >
                <Switch
                  id="integration-active-header"
                  checked={f.state.value}
                  onCheckedChange={f.handleChange}
                  aria-label="Toggle active"
                />
                <span className={f.state.value ? 'font-medium' : 'text-muted-foreground'}>
                  {f.state.value ? 'Active' : 'Inactive'}
                  {f.state.value !== baseline.isActive && <span className="ml-1 text-xs text-muted-foreground">(unsaved)</span>}
                </span>
              </label>
            )}
          </form.Field>
          <Button type="button" variant="outline" size="sm" onClick={onRegenerate} disabled={regenState.loading}>
            <RefreshCwIcon className="size-4" />
            {regenState.loading ? 'Regenerating…' : 'Regenerate token'}
          </Button>
        </>
      }
      createLabel="Save changes"
      labels={INTEGRATION_DETAIL_LABELS}
      after={
        <>
          <DetailPageSection
            title="Danger zone"
            description="Deleting the integration revokes its token immediately - any client calling the API with it will start getting 401 responses."
          >
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
          </DetailPageSection>
          <PluginSections target="integration" entity={integration} mode="edit" />
        </>
      }
    >
      <DetailPageSection title="Profile" description="Name, description, and expiry.">
        <div className="grid gap-4">
          <form.AppField name="name">{(f) => <f.TextField id="int-name" label="Name" />}</form.AppField>
          <form.AppField name="description">{(f) => <f.TextareaField id="int-desc" label="Description" />}</form.AppField>
          <div className="grid gap-2">
            <form.AppField name="hasExpiry">{(f) => <f.CheckboxField id="int-expiry" label="Set expiry date" />}</form.AppField>
            {hasExpiry && (
              <form.Field name="expiresAt">
                {(f) => (
                  <>
                    <Input
                      type="datetime-local"
                      aria-label="Expires at"
                      value={f.state.value}
                      onChange={(e) => f.handleChange(e.target.value)}
                      onBlur={f.handleBlur}
                    />
                    <FieldError field={f} />
                  </>
                )}
              </form.Field>
            )}
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Permissions"
        description="Which admin API operations this token can call - the same permission catalog roles use, granted globally across stores. Changes apply on the next request."
      >
        <form.Field name="permissionSlugs">
          {(f) => <PermissionSelector selected={f.state.value} onChange={f.handleChange} />}
        </form.Field>
      </DetailPageSection>

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
              This cannot be undone. Clients using this integration's token will be rejected immediately.
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
                void onDelete()
              }}
            >
              {deleteState.loading ? 'Deleting…' : 'Delete integration'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </RecordFormPage>
  )
}
