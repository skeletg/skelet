import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Trans, useLingui } from '@lingui/react/macro'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useAuth } from '@/shared/auth/auth'
import { AuthHeader } from './AuthShell'
import { AdminLoginDocument } from './Login.gql'
import { markFirstLogin } from '@/features/onboarding/first-login'

export function Login() {
  const { t } = useLingui()
  const { auth, setAuth } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const redirectTo = (location.state as { from?: string } | null)?.from ?? '/'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  // Two-factor: shown after the server answers TOTP_REQUIRED (credentials
  // were valid); the form resubmits with the authenticator/recovery code
  const [totpNeeded, setTotpNeeded] = useState(false)
  const [totpCode, setTotpCode] = useState('')
  const [login, { loading, error }] = useMutation(AdminLoginDocument)

  if (auth) return <Navigate to={redirectTo} replace />

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    try {
      const { data } = await login({
        variables: { input: { email, password, totpCode: totpCode || null } },
      })
      if (!data) return
      // true only on the account's very first sign-in - the shell picks this up and opens its
      // introduction once
      if (data.adminLogin.firstLogin) markFirstLogin()
      setAuth(data.adminLogin)
      navigate(redirectTo, { replace: true })
    } catch (err) {
      // TOTP_REQUIRED = right password, prompt for the second factor
      const code = (err as { graphQLErrors?: { extensions?: { code?: string } }[] })
        ?.graphQLErrors?.[0]?.extensions?.code
      if (code === 'TOTP_REQUIRED') setTotpNeeded(true)
      // other errors surface via the `error` binding below
    }
  }

  return (
    <>
      <AuthHeader title={t`Sign in`} subtitle={t`Enter your admin email and password.`} />
      <form onSubmit={onSubmit} className="mt-8 flex flex-col gap-5">
        <div className="grid gap-2">
          <Label htmlFor="email" className="text-neutral-700">
            <Trans>Email</Trans>
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
        <div className="grid gap-2">
          <Label htmlFor="password" className="text-neutral-700">
            <Trans>Password</Trans>
          </Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
          />
        </div>
        {totpNeeded && (
          <div className="grid gap-2">
            <Label htmlFor="totp" className="text-neutral-700">
              <Trans>Authentication code</Trans>
            </Label>
            <Input
              id="totp"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              placeholder={t`6-digit code or recovery code`}
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
            />
            <p className="text-xs text-neutral-500">
              <Trans>Enter the code from your authenticator app, or a recovery code.</Trans>
            </p>
          </div>
        )}
        {error && !(totpNeeded && !totpCode) && (
          <Alert variant="destructive">
            <AlertTitle><Trans>Sign-in failed</Trans></AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        <Button
          type="submit"
          disabled={loading}
          className="mt-1 h-11 bg-neutral-900 text-white hover:bg-neutral-800"
        >
          {loading ? t`Signing in…` : t`Sign in`}
        </Button>
        <p className="text-sm text-neutral-500">
          <Link
            to="/forgot-password"
            className="font-medium text-neutral-900 underline-offset-4 hover:underline"
          >
            <Trans>Forgot your password?</Trans>
          </Link>
        </p>
      </form>
    </>
  )
}
