import { useMutation, useQuery } from '@apollo/client/react'
import { Trans, useLingui } from '@lingui/react/macro'
import { DetailPageSection } from '@/components/layout/detail-page'
import { useState } from 'react'
import { LinkIcon } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { formatDateTime } from '@/lib/format'
import { AdminSsoLinkStartDocument, AdminSsoLinksDocument, AdminSsoUnlinkDocument } from './Login.gql'
import { goToProvider, ssoReturnUrl } from './sso'

// The signed-in admin's own provider accounts - connecting one is the only way it comes to sign them in, so a provider
// vouching for an address never reaches an account by itself
export function SsoLinksCard() {
  const { t } = useLingui()
  const { data } = useQuery(AdminSsoLinksDocument)
  const [start, starting] = useMutation(AdminSsoLinkStartDocument)
  const [unlink, unlinking] = useMutation(AdminSsoUnlinkDocument, { refetchQueries: [AdminSsoLinksDocument] })
  const [problem, setProblem] = useState<string | null>(null)
  const option = data?.adminSsoOption
  const links = data?.adminSsoLinks ?? []
  if (!option && links.length === 0) return null

  async function onConnect() {
    setProblem(null)
    try {
      const { data: started } = await start({ variables: { redirectUri: ssoReturnUrl() } })
      if (started && !goToProvider(started.adminSsoLinkStart, '/preferences?tab=account', 'link')) {
        setProblem(t`The provider's sign-in address isn't a safe one to open - ask an administrator to check the provider settings.`)
      }
    } catch (e) {
      setProblem(e instanceof Error ? e.message : t`Couldn't reach the provider.`)
    }
  }

  async function onDisconnect(id: string) {
    setProblem(null)
    await unlink({ variables: { id } }).catch((e: unknown) => setProblem(e instanceof Error ? e.message : t`Couldn't disconnect it.`))
  }

  return (
    <DetailPageSection
      title={<Trans>Sign-in provider</Trans>}
      description={<Trans>Connect your account at the store's sign-in provider to sign in with it. Your password and two-factor stay as they are.</Trans>}
    >
      <div className="grid gap-3">
        {problem && (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        )}
        {links.length > 0 && (
          <ul className="divide-y rounded-md border">
            {links.map((l) => (
              <li key={l.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                <div className="min-w-0">
                  <div className="truncate font-medium">{l.email || l.issuer}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {l.issuer} · {t`last used ${formatDateTime(l.lastUsedAt)}`}
                  </div>
                </div>
                <Button size="sm" variant="outline" disabled={unlinking.loading} onClick={() => void onDisconnect(l.id)}>
                  {t`Disconnect`}
                </Button>
              </li>
            ))}
          </ul>
        )}
        {option && (
          <Button size="sm" variant="outline" className="w-fit" disabled={starting.loading} onClick={() => void onConnect()}>
            <LinkIcon className="size-4" />
            {links.length > 0 ? t`Connect another account` : t`Connect your ${option.label} account`}
          </Button>
        )}
      </div>
    </DetailPageSection>
  )
}
