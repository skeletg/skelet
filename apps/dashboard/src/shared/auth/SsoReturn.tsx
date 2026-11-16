import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Trans, useLingui } from '@lingui/react/macro'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '@/shared/auth/auth'
import { markFirstLogin } from '@/features/onboarding/first-login'
import { AuthHeader } from './AuthShell'
import { AdminSsoCompleteDocument, AdminSsoLinkCompleteDocument, AdminSsoLinksDocument, AdminSsoTotpDocument, type AdminSessionFragment } from './Login.gql'
import { takePendingSso } from './sso'

// Where the provider sends the browser back - the code is single-use, so it's completed exactly once
export function SsoReturn() {
  const { t } = useLingui()
  const { setAuth } = useAuth()
  const [params] = useSearchParams()
  const [complete] = useMutation(AdminSsoCompleteDocument)
  const [link] = useMutation(AdminSsoLinkCompleteDocument, { refetchQueries: [AdminSsoLinksDocument] })
  const [finish, finishing] = useMutation(AdminSsoTotpDocument)
  const [failure, setFailure] = useState<string | null>(null)
  const [ticket, setTicket] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const target = useRef('/')
  const [landed, setLanded] = useState<string | null>(null)
  const started = useRef(false)

  const signedIn = useCallback(
    (session: AdminSessionFragment) => {
      if (session.firstLogin) markFirstLogin()
      setAuth(session)
      setLanded(target.current)
    },
    [setAuth],
  )

  useEffect(() => {
    if (started.current) return
    started.current = true
    const pending = takePendingSso()
    const state = params.get('state')
    if (!pending || !state || state !== pending.state) {
      setFailure(t`This sign-in wasn't started in this browser tab - start again from the sign-in page.`)
      return
    }
    target.current = pending.redirectTo
    const error = params.get('error')
    const providerCode = params.get('code')
    if (error || !providerCode) {
      setFailure(error === 'access_denied' ? t`You cancelled the sign-in at the provider.` : t`The provider didn't sign you in.`)
      return
    }
    const variables = { state, code: providerCode, binding: pending.binding }
    const failed = (e: unknown) => setFailure(e instanceof Error ? e.message : t`Sign-in failed.`)
    if (pending.mode === 'link') {
      link({ variables })
        .then(({ data }) => data && setLanded(target.current))
        .catch(failed)
      return
    }
    complete({ variables })
      .then(({ data }) => {
        const result = data?.adminSsoComplete
        if (result?.totpTicket) setTicket(result.totpTicket)
        else if (result?.session) signedIn(result.session)
      })
      .catch(failed)
  }, [complete, link, params, signedIn, t])

  async function onTotp(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!ticket) return
    const { data } = await finish({ variables: { ticket, code } }).catch(() => ({ data: undefined }))
    if (data) signedIn(data.adminSsoTotp)
  }

  if (landed) return <Navigate to={landed} replace />
  if (ticket && !failure) {
    return (
      <>
        <AuthHeader title={t`Two-factor check`} subtitle={t`Enter the code from your authenticator app, or a recovery code.`} />
        <form onSubmit={onTotp} className="mt-8 flex flex-col gap-5">
          <div className="grid gap-2">
            <Label htmlFor="sso-totp" className="text-neutral-700">
              <Trans>Authentication code</Trans>
            </Label>
            <Input
              id="sso-totp"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
              placeholder={t`6-digit code or recovery code`}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              className="h-11 border-neutral-300 bg-white text-neutral-900 placeholder:text-neutral-400"
            />
          </div>
          {finishing.error && (
            <Alert variant="destructive">
              <AlertTitle><Trans>Sign-in failed</Trans></AlertTitle>
              <AlertDescription>{finishing.error.message}</AlertDescription>
            </Alert>
          )}
          <Button type="submit" disabled={finishing.loading} className="h-11 bg-neutral-900 text-white hover:bg-neutral-800">
            {finishing.loading ? t`Signing in…` : t`Sign in`}
          </Button>
        </form>
      </>
    )
  }
  return (
    <>
      <AuthHeader title={t`Signing in`} subtitle={failure ? t`That didn't work.` : t`Finishing your sign-in…`} />
      {failure && (
        <div className="mt-8 flex flex-col gap-5">
          <Alert variant="destructive">
            <AlertTitle><Trans>Sign-in failed</Trans></AlertTitle>
            <AlertDescription>{failure}</AlertDescription>
          </Alert>
          <Link to="/login" className="text-sm font-medium text-neutral-900 underline-offset-4 hover:underline">
            <Trans>Back to sign in</Trans>
          </Link>
        </div>
      )}
    </>
  )
}
