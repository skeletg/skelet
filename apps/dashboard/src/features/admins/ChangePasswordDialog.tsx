import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { CheckCircle2Icon } from 'lucide-react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { PASSWORD_HINT } from '@/shared/schemas/fields'
import { ChangePasswordDocument } from './Admins.gql'
import { changePasswordSchema, emptyChangePassword } from './schemas/changePassword'

const TITLE = 'Change password'
const DESCRIPTION = 'Updates the password for your own account. You stay signed in everywhere.'

// Self-service password change for the SIGNED-IN admin - the backend only swaps the hash, it does
// NOT revoke sessions, so the admin stays logged in and gets a confirmation in place
export function ChangePasswordDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="sm:max-w-md">
      {(close) => <ChangePasswordForm onClose={close} />}
    </FormDialog>
  )
}

function ChangePasswordForm({ onClose }: { onClose: () => void }) {
  const [done, setDone] = useState(false)
  const [changePassword] = useMutation(ChangePasswordDocument)
  const dialog = useDialogForm({
    baseline: emptyChangePassword,
    schema: changePasswordSchema,
    submit: async ({ currentPassword, newPassword }) => {
      await changePassword({ variables: { currentPassword, newPassword } })
    },
    onSaved: () => setDone(true),
  })
  const { form } = dialog

  if (done) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>{TITLE}</DialogTitle>
          <DialogDescription>{DESCRIPTION}</DialogDescription>
        </DialogHeader>
        <Alert>
          <CheckCircle2Icon className="size-4" />
          <AlertTitle>Password changed</AlertTitle>
          <AlertDescription>
            Use the new password next time you sign in. Existing sessions are untouched - revoke them from Active sessions
            if you want them gone.
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button type="button" onClick={onClose}>
            Done
          </Button>
        </DialogFooter>
      </>
    )
  }

  return (
    <FormDialogBody dialog={dialog} title={TITLE} description={DESCRIPTION} submitLabel="Change password" onCancel={onClose}>
      <form.AppField name="currentPassword">
        {(f) => <f.TextField id="cp-current" label="Current password" type="password" autoComplete="current-password" />}
      </form.AppField>
      <form.AppField name="newPassword">
        {(f) => (
          <f.TextField
            id="cp-new"
            label="New password"
            type="password"
            autoComplete="new-password"
            hint={`${PASSWORD_HINT}.`}
          />
        )}
      </form.AppField>
      <form.AppField name="confirmPassword">
        {(f) => <f.TextField id="cp-confirm" label="Confirm new password" type="password" autoComplete="new-password" />}
      </form.AppField>
    </FormDialogBody>
  )
}
