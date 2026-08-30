import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AuthHeader } from './AuthShell'
import { ResetAdminPasswordDocument } from './PasswordReset.gql'

/** Public page reached from the emailed link (/reset-password?token=...)
 *  Sets a new password, then sends the admin to /login - Does NOT create a
 *  session (the mutation returns Boolean, and the reset revokes sessions) */
export function ResetPassword() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [resetPassword, { loading, error }] = useMutation(ResetAdminPasswordDocument)

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setFormError(null)
    // Mirror the server's floor (auth.ValidatePasswordStrength: >= 8 chars)
    // so the common mistakes fail fast without a round-trip
    if (password.length < 8) {
      setFormError('Password must be at least 8 characters.')
      return
    }
    if (password !== confirm) {
      setFormError('Passwords do not match.')
      return
    }
    try {
      const { data } = await resetPassword({ variables: { token, newPassword: password } })
      if (data) setDone(true)
    } catch {
      // invalid/expired token or a rejected password surface via `error` below
    }
  }

  return (
    <>
      <AuthHeader
        title="Set a new password"
        subtitle="Choose a new password for your admin account."
      />
      {!token ? (
        <div className="mt-8 flex flex-col gap-5">
          <Alert variant="destructive">
            <AlertTitle>Invalid reset link</AlertTitle>
            <AlertDescription>
              This link is missing its token. Request a fresh reset link and try again.
            </AlertDescription>
          </Alert>
          <p className="text-sm text-neutral-500">
            <Link
              to="/forgot-password"
              className="font-medium text-neutral-900 underline-offset-4 hover:underline"
            >
              Request a new link
            </Link>
          </p>
        </div>
      ) : done ? (
        <div className="mt-8 flex flex-col gap-5">
          <Alert>
            <AlertTitle>Password updated</AlertTitle>
            <AlertDescription>
              Your password has been reset and other sessions were signed out. Sign in with
              your new password.
            </AlertDescription>
          </Alert>
          <p className="text-sm text-neutral-500">
            <Link
              to="/login"
              className="font-medium text-neutral-900 underline-offset-4 hover:underline"
            >
              Go to sign in
            </Link>
          </p>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="mt-8 flex flex-col gap-5">
          <div className="grid gap-2">
            <Label htmlFor="password" className="text-neutral-700">
              New password
            </Label>
            <Input
              id="password"
              type="password"
              autoComplete="new-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="confirm" className="text-neutral-700">
              Confirm password
            </Label>
            <Input
              id="confirm"
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
            />
          </div>
          {(formError || error) && (
            <Alert variant="destructive">
              <AlertTitle>Could not reset password</AlertTitle>
              <AlertDescription>{formError ?? error?.message}</AlertDescription>
            </Alert>
          )}
          <Button
            type="submit"
            disabled={loading}
            className="mt-1 h-11 bg-neutral-900 text-white hover:bg-neutral-800"
          >
            {loading ? 'Saving…' : 'Reset password'}
          </Button>
          <p className="text-sm text-neutral-500">
            <Link
              to="/login"
              className="font-medium text-neutral-900 underline-offset-4 hover:underline"
            >
              Back to sign in
            </Link>
          </p>
        </form>
      )}
    </>
  )
}
