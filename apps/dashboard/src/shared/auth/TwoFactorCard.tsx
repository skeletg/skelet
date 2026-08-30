import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { ShieldCheckIcon, ShieldOffIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  AdminTotpDisableDocument,
  AdminTotpEnableDocument,
  AdminTotpSetupDocument,
  MeDocument,
} from './Me.gql'

// Two-factor (TOTP) self-service for the CURRENT admin: stage a secret, confirm the first code (which returns one-use
// recovery codes, shown exactly once), or disable with a current/recovery code
export function TwoFactorCard() {
  const meQuery = useQuery(MeDocument)
  const enabled = meQuery.data?.me?.totpEnabled ?? false

  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string } | null>(null)
  const [code, setCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  const refetch = [{ query: MeDocument }]
  const [totpSetup, setupState] = useMutation(AdminTotpSetupDocument)
  const [totpEnable, enableState] = useMutation(AdminTotpEnableDocument, { refetchQueries: refetch })
  const [totpDisable, disableState] = useMutation(AdminTotpDisableDocument, { refetchQueries: refetch })
  const busy = setupState.loading || enableState.loading || disableState.loading

  async function onSetup() {
    setMessage(null)
    try {
      const { data } = await totpSetup()
      if (data) setSetup(data.adminTotpSetup)
    } catch (e) {
      setMessage(e instanceof Error ? e.message : 'Setup failed.')
    }
  }

  async function onEnable() {
    setMessage(null)
    try {
      const { data } = await totpEnable({ variables: { code } })
      if (data) {
        setRecoveryCodes(data.adminTotpEnable)
        setSetup(null)
        setCode('')
      }
    } catch (e) {
      setMessage(e instanceof Error ? e.message : 'Could not verify the code.')
    }
  }

  async function onDisable() {
    setMessage(null)
    try {
      await totpDisable({ variables: { code } })
      setCode('')
      setRecoveryCodes(null)
    } catch (e) {
      setMessage(e instanceof Error ? e.message : 'Could not verify the code.')
    }
  }

  return (
    <div className="grid gap-4 rounded-xl border bg-card p-5">
      <div className="flex items-center gap-2.5">
        <span className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          {enabled ? <ShieldCheckIcon className="size-4" /> : <ShieldOffIcon className="size-4" />}
        </span>
        <div>
          <div className="text-sm font-semibold">Two-factor authentication</div>
          <div className="text-xs text-muted-foreground">
            {enabled
              ? 'Enabled - sign-in requires your authenticator app.'
              : 'Add a second factor to protect this account.'}
          </div>
        </div>
      </div>

      {message && (
        <Alert variant="destructive">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {recoveryCodes && (
        <Alert>
          <AlertTitle>Save your recovery codes now</AlertTitle>
          <AlertDescription>
            <p className="mb-2">
              Each works once if you lose your authenticator. They will not be shown again.
            </p>
            <div className="grid grid-cols-2 gap-1 font-mono text-xs">
              {recoveryCodes.map((c) => (
                <span key={c}>{c}</span>
              ))}
            </div>
          </AlertDescription>
        </Alert>
      )}

      {!enabled && !setup && (
        <Button size="sm" className="w-fit" onClick={onSetup} disabled={busy}>
          Set up two-factor
        </Button>
      )}

      {!enabled && setup && (
        <div className="grid gap-3">
          <p className="text-sm">
            Add this key to your authenticator app (manual entry), then confirm with a code:
          </p>
          <code className="w-fit rounded bg-muted px-2 py-1 font-mono text-sm">{setup.secret}</code>
          <p className="break-all text-xs text-muted-foreground">{setup.otpauthUrl}</p>
          <div className="flex items-end gap-2">
            <div className="grid gap-1.5">
              <Label htmlFor="totp-confirm">Authentication code</Label>
              <Input
                id="totp-confirm"
                inputMode="numeric"
                className="w-40"
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </div>
            <Button size="sm" onClick={onEnable} disabled={busy || !code}>
              {enableState.loading ? 'Verifying…' : 'Enable'}
            </Button>
          </div>
        </div>
      )}

      {enabled && (
        <div className="flex items-end gap-2">
          <div className="grid gap-1.5">
            <Label htmlFor="totp-disable">Current or recovery code</Label>
            <Input
              id="totp-disable"
              inputMode="numeric"
              className="w-44"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </div>
          <Button size="sm" variant="outline" onClick={onDisable} disabled={busy || !code}>
            {disableState.loading ? 'Verifying…' : 'Disable two-factor'}
          </Button>
        </div>
      )}
    </div>
  )
}
