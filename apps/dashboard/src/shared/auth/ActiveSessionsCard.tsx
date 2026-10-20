import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { LogOutIcon } from 'lucide-react'
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
import { useAuth } from '@/shared/auth/auth'
import { LogoutOtherDevicesDocument, MeDocument } from '@/shared/auth/Me.gql'

export function ActiveSessionsCard() {
  const { auth } = useAuth()
  const { data, loading } = useQuery(MeDocument)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [logoutOtherDevices, logoutState] = useMutation(LogoutOtherDevicesDocument, {
    refetchQueries: [MeDocument],
  })

  const sessions = data?.me.sessions ?? []
  const otherSessionCount = sessions.filter((session) => !session.isCurrent).length

  async function onConfirmLogoutOtherDevices() {
    if (!auth?.refreshToken) return
    try {
      await logoutOtherDevices({ variables: { token: auth.refreshToken } })
      setConfirmOpen(false)
    } catch {
      // The mutation error is rendered from `logoutState.error`
    }
  }

  return (
    <>
      {/* flat bordered block rather than a Card, matching TwoFactorCard next to it */}
      <div className="grid gap-4 rounded-xl border bg-card p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="grid gap-1.5">
            <div className="text-sm font-semibold">Active sessions</div>
            <p className="text-sm text-muted-foreground">
              Devices currently signed in to this admin account
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => setConfirmOpen(true)}
            disabled={loading || otherSessionCount === 0 || logoutState.loading}
          >
            <LogOutIcon className="size-4" />
            {logoutState.loading ? 'Logging out…' : 'Log out other devices'}
          </Button>
        </div>
        <div className="text-sm">
          {loading && <p className="text-muted-foreground">Loading…</p>}
          {logoutState.error && (
            <p className="mb-3 text-destructive">Error: {logoutState.error.message}</p>
          )}
          {data && sessions.length === 0 && (
            <p className="text-muted-foreground">No active sessions found.</p>
          )}
          {data && sessions.length > 0 && (
            <div className="divide-y rounded-md border">
              {sessions.map((session) => (
                <div
                  key={session.id}
                  className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_auto]"
                >
                  <div className="min-w-0 space-y-2">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <p className="font-medium">{session.ipAddress || 'Unknown IP'}</p>
                      {session.isCurrent && <Badge>Current session</Badge>}
                    </div>
                    <p className="break-words text-xs text-muted-foreground">
                      {session.userAgent || 'Unknown user agent'}
                    </p>
                  </div>
                  <dl className="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-[auto_1fr] lg:min-w-72">
                    <dt className="text-muted-foreground">Created</dt>
                    <dd>{formatDateTime(session.createdAt)}</dd>
                    <dt className="text-muted-foreground">Last active</dt>
                    <dd>{formatDateTime(session.lastActiveAt)}</dd>
                  </dl>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <AlertDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!logoutState.loading) setConfirmOpen(open)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Log out other devices?</AlertDialogTitle>
            <AlertDialogDescription>
              This will revoke {otherSessionCount} other active admin session{otherSessionCount === 1 ? '' : 's'}.
              Your current session will stay signed in.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={logoutState.loading}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={logoutState.loading || !auth?.refreshToken}
              onClick={(e) => {
                e.preventDefault()
                void onConfirmLogoutOtherDevices()
              }}
            >
              {logoutState.loading ? 'Logging out…' : 'Log out other devices'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function formatDateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
