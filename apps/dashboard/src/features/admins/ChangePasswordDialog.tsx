import { useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import { CheckCircle2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ChangePasswordDocument } from './Admins.gql'

// Backend minimum (auth.MinPasswordLen) - mirrored so we can say it up front
const MIN_LENGTH = 8

// Self-service password change for the SIGNED-IN admin - The backend only swaps the hash - it does NOT revoke sessions
// - so we keep the operator logged in and just confirm in place
export function ChangePasswordDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)

  const [changePassword, { loading }] = useMutation(ChangePasswordDocument)

  function reset() {
    setCurrentPassword('')
    setNewPassword('')
    setConfirmPassword('')
    setError(null)
    setDone(false)
  }

  function handleOpenChange(next: boolean) {
    if (loading) return
    if (!next) reset()
    onOpenChange(next)
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    if (newPassword !== confirmPassword) {
      setError("The new passwords don't match.")
      return
    }
    if (newPassword.length < MIN_LENGTH) {
      setError(`New password must be at least ${MIN_LENGTH} characters.`)
      return
    }
    if (newPassword === currentPassword) {
      setError('The new password has to be different from the current one.')
      return
    }
    try {
      await changePassword({ variables: { currentPassword, newPassword } })
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setDone(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not change the password.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Change password</DialogTitle>
          <DialogDescription>
            Updates the password for your own account. You stay signed in everywhere.
          </DialogDescription>
        </DialogHeader>

        {done ? (
          <>
            <Alert>
              <CheckCircle2Icon className="size-4" />
              <AlertTitle>Password changed</AlertTitle>
              <AlertDescription>
                Use the new password next time you sign in. Existing sessions are untouched -
                revoke them from Active sessions if you want them gone.
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button type="button" onClick={() => handleOpenChange(false)}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <form id="change-password-form" onSubmit={onSubmit} className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="cp-current">Current password</Label>
                <Input
                  id="cp-current"
                  type="password"
                  required
                  autoComplete="current-password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  disabled={loading}
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="cp-new">New password</Label>
                <Input
                  id="cp-new"
                  type="password"
                  required
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">
                  At least {MIN_LENGTH} characters.
                </p>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="cp-confirm">Confirm new password</Label>
                <Input
                  id="cp-confirm"
                  type="password"
                  required
                  autoComplete="new-password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  disabled={loading}
                  aria-invalid={confirmPassword !== '' && confirmPassword !== newPassword}
                />
                {confirmPassword !== '' && confirmPassword !== newPassword && (
                  <p className="text-xs text-destructive">Passwords don't match.</p>
                )}
              </div>

              {/* Inline and sticky on purpose - a wrong current password is worth reading */}
              {error && (
                <Alert variant="destructive">
                  <AlertTitle>Couldn't change the password</AlertTitle>
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
            </form>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
                disabled={loading}
              >
                Cancel
              </Button>
              <Button type="submit" form="change-password-form" disabled={loading}>
                {loading ? 'Changing…' : 'Change password'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
