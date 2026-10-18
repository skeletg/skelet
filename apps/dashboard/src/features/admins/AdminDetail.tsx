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
import { DetailPageSection } from '@/components/layout/detail-page'
import { RoleSelector } from '@/components/RoleSelector'
import { useAuth } from '@/shared/auth/auth'
import { LoadError } from '@/shared/errors/LoadError'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import {
  AdminDocument,
  AdminsDocument,
  DeleteAdminDocument,
  UpdateAdminDocument,
  type AdminQuery,
} from './Admins.gql'
import { PluginSections } from '@/plugins/detail-sections'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  ADMIN_DETAIL_LABELS,
  adminDetailDiff,
  adminDetailSchema,
  toAdminDetailValues,
  toAdminUpdateInput,
} from './schemas/admin'

export function AdminDetail() {
  const { id = '' } = useParams()
  const { data, loading, error, refetch } = useQuery(AdminDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const admin = data?.admin

  if (loading && !admin) return <p className="text-sm text-muted-foreground">Loading admin…</p>
  if (error) {
    return (
      <LoadError
        title="Couldn't load admin"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  }
  if (!admin) return <p className="text-sm text-muted-foreground">Admin not found.</p>
  return <AdminForm admin={admin} />
}

// Profile and roles are one form - the header Save reviews both through the confirm dialog and
// writes them in one update (docs/conventions.md)
function AdminForm({ admin }: { admin: AdminQuery['admin'] }) {
  const navigate = useNavigate()
  // Editing and deleting an admin need admins.manage - a viewer gets the page read-only
  const canManage = usePermissions().has('admins.manage')
  const { auth } = useAuth()
  const isSelf = auth?.admin.id === admin.id

  const [updateAdmin] = useMutation(UpdateAdminDocument, {
    refetchQueries: [{ query: AdminDocument, variables: { id: admin.id } }, { query: AdminsDocument }],
  })
  const [deleteAdmin, deleteState] = useMutation(DeleteAdminDocument, {
    refetchQueries: [{ query: AdminsDocument }],
  })
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const baseline = useMemo(() => toAdminDetailValues(admin), [admin])
  const record = useRecordForm({
    id: 'admin',
    title: 'Admin',
    label: `Admin · ${admin.firstName} ${admin.lastName}`,
    isEdit: true,
    baseline,
    schema: adminDetailSchema,
    diff: adminDetailDiff,
    persist: { routeKey: `admin:${admin.id}` },
    submit: async (v) => {
      await updateAdmin({ variables: { id: admin.id, input: toAdminUpdateInput(v, baseline) } })
    },
  })
  const { form } = record

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteAdmin({ variables: { id: admin.id } })
      navigate('/admins', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
      backTo="/admins"
      backLabel="All admins"
      title={
        <>
          {admin.firstName} {admin.lastName}
          {isSelf && <span className="ml-2 text-sm font-normal text-muted-foreground">(you)</span>}
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
      createLabel="Save changes"
      labels={ADMIN_DETAIL_LABELS}
      readOnly={!canManage}
      after={
        <>
          {canManage && (
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
                  onClick={() => {
                    setDeleteError(null)
                    setConfirmingDelete(true)
                  }}
                >
                  <Trash2Icon className="size-4" />
                  Delete admin
                </Button>
              </div>
            </DetailPageSection>
          )}
          <PluginSections target="admin" entity={admin} mode="edit" />
        </>
      }
    >
      <DetailPageSection
        title="Profile"
        description={
          isSelf ? (
            <>
              Email can't be changed. Your password, two-factor and sessions live in{' '}
              <Link to="/preferences?tab=account" className="text-primary hover:underline">
                Preferences
              </Link>
              .
            </>
          ) : (
            "Email and password can't be changed here - those are self-service."
          )
        }
      >
        <div className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <form.AppField name="firstName">{(f) => <f.TextField id="ad-first" label="First name" />}</form.AppField>
            <form.AppField name="lastName">{(f) => <f.TextField id="ad-last" label="Last name" />}</form.AppField>
          </div>
          <form.AppField name="isActive">
            {(f) => <f.CheckboxField id="ad-active" label="Active - inactive admins can't sign in" />}
          </form.AppField>
        </div>
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
          <form.Field name="roleSlugs">{(f) => <RoleSelector selected={f.state.value} onChange={f.handleChange} />}</form.Field>
        )}
      </DetailPageSection>

      {!admin.isSuperuser && admin.permissions.length > 0 && (
        <DetailPageSection title="Effective permissions" description="Union of permissions granted by the assigned roles.">
          <ul className="flex flex-wrap gap-1.5">
            {admin.permissions.map((p) => (
              <li key={p.id} className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs" title={p.name}>
                {p.slug}
              </li>
            ))}
          </ul>
        </DetailPageSection>
      )}

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
              Access is revoked immediately and any refresh tokens become invalid. This cannot be undone.
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
              {deleteState.loading ? 'Deleting…' : 'Delete admin'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </RecordFormPage>
  )
}
