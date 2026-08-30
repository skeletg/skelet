import { useState } from 'react'
import { Trans } from '@lingui/react/macro'
import { CompassIcon, KeyRoundIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { ChangePasswordDialog } from '@/features/admins/ChangePasswordDialog'
import { useWelcome } from '@/features/onboarding/welcome-context'
import { ActiveSessionsCard } from '@/shared/auth/ActiveSessionsCard'
import { TwoFactorCard } from '@/shared/auth/TwoFactorCard'
import { AppearanceSection } from './AppearanceSection'
import { DashboardResetsSection } from './DashboardResetsSection'
import { LanguageSection } from './LanguageSection'
import { DefaultsSection } from './DefaultsSection'
import { NotificationsSection } from './NotificationsSection'
import { ShellSection } from './ShellSection'
import { StartupSection } from './StartupSection'

// Everything an admin can set for themselves - appearance, their own security, and control over
// what they've customized elsewhere in the dashboard
// No staged save here on purpose: every control writes immediately like the theme always has, and
// the destructive resets carry their own confirmation
export function PreferencesPage() {
  const [changingPassword, setChangingPassword] = useState(false)
  const welcome = useWelcome()

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={<Trans>Preferences</Trans>}
        meta={
          <span className="text-xs text-muted-foreground">
            <Trans>
              Yours alone, and they follow you to any device you sign in on - other open sessions
              pick up changes on their next reload
            </Trans>
          </span>
        }
      />
      <DetailPageBody>
        <AppearanceSection />
        <LanguageSection />
        <StartupSection />
        <ShellSection />
        <DefaultsSection />
        <NotificationsSection />

        {welcome && (
          <DetailPageSection
            title={<Trans>Getting started</Trans>}
            description={
              <Trans>
                The short tour of where things live, what the scope picker changes, and which parts
                of this panel you can rearrange.
              </Trans>
            }
          >
            <div>
              <Button variant="outline" onClick={welcome.open}>
                <CompassIcon className="size-4" />
                <Trans>Take the tour</Trans>
              </Button>
            </div>
          </DetailPageSection>
        )}

        <DetailPageSection
          title={<Trans>Password</Trans>}
          description={
            <Trans>Changing it needs your current password and doesn't sign you out.</Trans>
          }
        >
          <div>
            <Button variant="outline" onClick={() => setChangingPassword(true)}>
              <KeyRoundIcon className="size-4" />
              <Trans>Change password</Trans>
            </Button>
          </div>
        </DetailPageSection>

        <DetailPageSection title={<Trans>Two-factor authentication</Trans>}>
          <TwoFactorCard />
        </DetailPageSection>

        <DetailPageSection title={<Trans>Active sessions</Trans>}>
          <ActiveSessionsCard />
        </DetailPageSection>

        <DashboardResetsSection />
      </DetailPageBody>

      <ChangePasswordDialog open={changingPassword} onOpenChange={setChangingPassword} />
    </DetailPageLayout>
  )
}
