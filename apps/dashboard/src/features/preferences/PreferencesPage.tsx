import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Trans } from '@lingui/react/macro'
import { CompassIcon, KeyRoundIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
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

// Linkable through ?tab= so a "change your theme" pointer can land on the right tab
const TABS = ['appearance', 'workspace', 'notifications', 'account', 'customizations'] as const
type TabValue = (typeof TABS)[number]
const isTab = (v: string | null): v is TabValue => TABS.includes(v as TabValue)

// Everything an admin can set for themselves - appearance, their own security, and control over
// what they've customized elsewhere in the dashboard
// No staged save here on purpose: every control writes immediately like the theme always has, and
// the destructive resets carry their own confirmation
export function PreferencesPage() {
  const [changingPassword, setChangingPassword] = useState(false)
  const welcome = useWelcome()
  const [searchParams, setSearchParams] = useSearchParams()
  const raw = searchParams.get('tab')
  const tab: TabValue = isTab(raw) ? raw : 'appearance'

  function onTabChange(next: string) {
    const params = new URLSearchParams(searchParams)
    if (next === 'appearance') params.delete('tab')
    else params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

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
        <Tabs value={tab} onValueChange={onTabChange}>
          <div className="px-6 pt-6">
            <TabsList>
              <TabsTrigger value="appearance"><Trans>Appearance</Trans></TabsTrigger>
              <TabsTrigger value="workspace"><Trans>Workspace</Trans></TabsTrigger>
              <TabsTrigger value="notifications"><Trans>Notifications</Trans></TabsTrigger>
              <TabsTrigger value="account"><Trans>Account</Trans></TabsTrigger>
              <TabsTrigger value="customizations"><Trans>Customizations</Trans></TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="appearance" className="mt-0">
            <AppearanceSection />
          </TabsContent>

          <TabsContent value="workspace" className="mt-0">
            <LanguageSection />
            <StartupSection />
            <DefaultsSection />
            <ShellSection />
          </TabsContent>

          <TabsContent value="notifications" className="mt-0">
            <NotificationsSection />
          </TabsContent>

          <TabsContent value="account" className="mt-0">
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

            {welcome && (
              <DetailPageSection
                title={<Trans>Getting started</Trans>}
                description={
                  <Trans>
                    The short tour of where things live, what the scope picker changes, and which
                    parts of this panel you can rearrange.
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
          </TabsContent>

          <TabsContent value="customizations" className="mt-0">
            <DashboardResetsSection />
          </TabsContent>
        </Tabs>
      </DetailPageBody>

      <ChangePasswordDialog open={changingPassword} onOpenChange={setChangingPassword} />
    </DetailPageLayout>
  )
}
