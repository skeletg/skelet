import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Trans, useLingui } from '@lingui/react/macro'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useAuth } from '@/shared/auth/auth'
import { markFirstLogin } from '@/features/onboarding/first-login'
import { AuthHeader } from './AuthShell'
import { AdminSsoCompleteDocument } from './Login.gql'
import { takePendingSso } from './sso'

// Where the provider sends the browser back - the code is single-use, so it's completed exactly once
export function SsoReturn() {
  const { t } = useLingui()
  const { auth, setAuth } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [complete] = useMutation(AdminSsoCompleteDocument)
  const [failure, setFailure] = useState<string | null>(null)
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    const pending = takePendingSso()
    const code = params.get('code')
    const state = params.get('state')
    if (params.get('error')) {
      setFailure(params.get('error_description') || t`The provider didn't sign you in.`)
      return
    }
    if (!pending || !code || !state || state !== pending.state) {
      setFailure(t`This sign-in wasn't started in this browser tab - start again from the sign-in page.`)
      return
    }
    complete({ variables: { state, code, binding: pending.binding } })
      .then(({ data }) => {
        if (!data) return
        if (data.adminSsoComplete.firstLogin) markFirstLogin()
        setAuth(data.adminSsoComplete)
        navigate(pending.redirectTo || '/', { replace: true })
      })
      .catch((e: unknown) => setFailure(e instanceof Error ? e.message : t`Sign-in failed.`))
  }, [complete, navigate, params, setAuth, t])

  if (auth && !failure) return <Navigate to="/" replace />
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
