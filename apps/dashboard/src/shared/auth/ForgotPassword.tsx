import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AuthHeader } from './AuthShell'
import { RequestAdminPasswordResetDocument } from './PasswordReset.gql'

/** Public page: request a password-reset link for an admin account - The
 *  backend is enumeration-safe (it never reveals whether the email exists),
 *  so on any successful call we show the same "check your email" confirmation */
export function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const [requestReset, { loading, error }] = useMutation(RequestAdminPasswordResetDocument)

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    try {
      const { data } = await requestReset({ variables: { email } })
      if (data) setSent(true)
    } catch {
      // rate-limit / transport errors surface via the `error` binding below
    }
  }

  return (
    <>
      <AuthHeader
        title="Reset your password"
        subtitle="Enter your account email and we’ll send you a reset link."
      />
      {sent ? (
        <div className="mt-8 flex flex-col gap-5">
          <Alert>
            <AlertTitle>Check your email</AlertTitle>
            <AlertDescription>
              If an account exists for {email || 'that address'}, a password-reset link is
              on its way. It expires in 30 minutes.
            </AlertDescription>
          </Alert>
          <p className="text-sm text-neutral-500">
            <Link
              to="/login"
              className="font-medium text-neutral-900 underline-offset-4 hover:underline"
            >
              Back to sign in
            </Link>
          </p>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="mt-8 flex flex-col gap-5">
          <div className="grid gap-2">
            <Label htmlFor="email" className="text-neutral-700">
              Email
            </Label>
            <Input
              id="email"
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
            />
          </div>
          {error && (
            <Alert variant="destructive">
              <AlertTitle>Something went wrong</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}
          <Button
            type="submit"
            disabled={loading}
            className="mt-1 h-11 bg-neutral-900 text-white hover:bg-neutral-800"
          >
            {loading ? 'Sending…' : 'Send reset link'}
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
