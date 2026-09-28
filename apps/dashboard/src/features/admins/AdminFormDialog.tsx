import type { ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Label } from '@/components/ui/label'
import { RoleSelector } from '@/components/RoleSelector'
import { PASSWORD_HINT } from '@/shared/schemas/fields'
import { AdminsDocument, CreateAdminDocument, UpdateAdminDocument } from './Admins.gql'
import { createAdminSchema, editAdminSchema, toAdminValues, type ExistingAdmin } from './schemas/admin'

export type { ExistingAdmin }

export function AdminFormDialog({
  admin,
  trigger,
  open,
  onOpenChange,
}: {
  admin?: ExistingAdmin
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} trigger={trigger} className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
      {(close) => <AdminForm admin={admin} onClose={close} />}
    </FormDialog>
  )
}

function AdminForm({ admin, onClose }: { admin?: ExistingAdmin; onClose: () => void }) {
  const refetchQueries = [{ query: AdminsDocument }]
  const [createAdmin] = useMutation(CreateAdminDocument, { refetchQueries })
  const [updateAdmin] = useMutation(UpdateAdminDocument, { refetchQueries })

  const dialog = useDialogForm({
    baseline: toAdminValues(admin),
    schema: admin ? editAdminSchema : createAdminSchema,
    submit: async (v) => {
      const profile = { firstName: v.firstName.trim(), lastName: v.lastName.trim(), roleSlugs: v.roleSlugs }
      if (admin) await updateAdmin({ variables: { id: admin.id, input: { ...profile, isActive: v.isActive } } })
      else await createAdmin({ variables: { input: { ...profile, email: v.email.trim(), password: v.password } } })
    },
    onSaved: onClose,
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title={admin ? `Edit ${admin.firstName} ${admin.lastName}`.trim() : 'New admin'}
      description={
        admin
          ? "Update the admin's profile, roles, and active status. Email and password can't be changed here."
          : 'The admin will be able to sign in immediately with the password you set here. Share credentials over a secure channel.'
      }
      submitLabel={admin ? 'Save changes' : 'Create admin'}
      onCancel={onClose}
    >
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
        <form.AppField name="firstName">
          {(f) => <f.TextField id="admin-first" label="First name" autoComplete="given-name" />}
        </form.AppField>
        <form.AppField name="lastName">
          {(f) => <f.TextField id="admin-last" label="Last name" autoComplete="family-name" />}
        </form.AppField>
      </div>

      <form.AppField name="email">
        {(f) => (
          <f.TextField
            id="admin-email"
            label="Email"
            type="email"
            autoComplete="off"
            disabled={!!admin}
            hint={admin ? 'Email is immutable.' : undefined}
          />
        )}
      </form.AppField>

      {!admin && (
        <form.AppField name="password">
          {(f) => (
            <f.TextField id="admin-password" label="Password" type="password" autoComplete="new-password" hint={`${PASSWORD_HINT}.`} />
          )}
        </form.AppField>
      )}

      {admin && (
        <form.AppField name="isActive">
          {(f) => <f.CheckboxField id="admin-active" label="Active - inactive admins cannot sign in" />}
        </form.AppField>
      )}

      <div className="grid gap-2">
        <Label>Roles</Label>
        {admin?.isSuperuser ? (
          <p className="rounded border bg-muted p-3 text-sm text-muted-foreground">
            This admin is a superuser and bypasses role-based permissions. Role assignments have no effect.
          </p>
        ) : (
          <form.Field name="roleSlugs">{(f) => <RoleSelector selected={f.state.value} onChange={f.handleChange} />}</form.Field>
        )}
      </div>
    </FormDialogBody>
  )
}
