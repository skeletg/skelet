import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { ExternalLinkIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { PRIVACY_DOCS_URL, PURPOSE_LABELS } from '@/features/tracking/platformGuides'
import { TrackingOverviewDocument } from '@/features/tracking/Tracking.gql'
import { SettingsContent } from './SettingsContent'
import { ConfigSectionsDocument } from './Settings.gql'
import { useConfigDrafts } from './useConfigDrafts'

const CATEGORIES = [
  { key: 'necessary', label: 'Necessary', about: 'Always on - the consent cookie itself, carts and sign-in.' },
  { key: 'analytics', label: PURPOSE_LABELS.analytics, about: 'The visitor id, Signals profiles and analytics platforms.' },
  { key: 'ads', label: PURPOSE_LABELS.ads, about: 'Ad click ids and ad platforms.' },
  { key: 'personalization', label: PURPOSE_LABELS.personalization, about: 'Personalised marketing platforms and Customer Match.' },
]

// The cookie banner and what each choice allows, in one place: the consent mode and exceptions for the channel, and
// which of its tracking destinations each category feeds
export function PrivacyPage() {
  const { effectiveChannelId, scopeInput, selectedChannel } = useConfigScope()
  const registry = useSaveRegistry('Privacy')
  const [confirm, setConfirm] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          title="Privacy"
          meta="How the cookie banner asks, and what each choice lets the shop send."
          actions={
            <>
              <Button asChild variant="outline" size="sm">
                <a href={PRIVACY_DOCS_URL} target="_blank" rel="noreferrer">
                  Notice text <ExternalLinkIcon className="size-3" />
                </a>
              </Button>
              <Button disabled={!registry.isDirty || saving} onClick={() => setConfirm(true)}>Save</Button>
            </>
          }
        />
        <DetailPageBody>
          <fieldset disabled={saving} className="min-w-0">
            {!effectiveChannelId ? (
              <p className="p-6 text-sm text-muted-foreground">Pick a channel in the header - consent is set per channel.</p>
            ) : (
              <>
                <PrivacySettings key={effectiveChannelId} scopeInput={scopeInput} />
                <ConsentMap channelId={effectiveChannelId} />
              </>
            )}
          </fieldset>
        </DetailPageBody>
      </DetailPageLayout>
      <SaveConfirmDialog
        open={confirm}
        scopeLabel={selectedChannel?.name ?? 'Selected channel'}
        sections={registry.dirtySections}
        saving={saving}
        error={error}
        onCancel={() => setConfirm(false)}
        onConfirm={async () => {
          setSaving(true)
          setError(null)
          try {
            await registry.saveAll()
            setConfirm(false)
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Could not save privacy settings')
          } finally {
            setSaving(false)
          }
        }}
      />
    </SaveRegistryContext.Provider>
  )
}

function PrivacySettings({ scopeInput }: { scopeInput: ReturnType<typeof useConfigScope>['scopeInput'] }) {
  const { data } = useQuery(ConfigSectionsDocument, { variables: { scope: scopeInput } })
  const drafts = useConfigDrafts({ sections: data?.configSections, scopeInput })
  return (
    <DetailPageSection title="Cookie consent" description="Ask first is the EU and UK model; no banner is the US one.">
      <div className="rounded-md border">
        <SettingsContent activeSectionSlug="general" activeGroupSlug="privacy" drafts={drafts} />
      </div>
    </DetailPageSection>
  )
}

// Which destinations each banner category feeds, so a choice reads as what it actually does
function ConsentMap({ channelId }: { channelId: string }) {
  const { has } = usePermissions()
  const { data } = useQuery(TrackingOverviewDocument, { variables: { channelId }, skip: !has('tracking.view') })
  if (!has('tracking.view')) return null
  const platforms = data?.trackingPlatforms ?? []
  const destinations = (data?.trackingDestinations ?? []).filter((d) => d.enabled)
  const feeds = (category: string) =>
    destinations.filter((d) => {
      const purposes = platforms.find((p) => p.kind === d.kind)?.purposes ?? []
      return category === 'necessary' ? purposes.length === 0 : purposes.includes(category)
    })
  return (
    <DetailPageSection
      title="What each choice sends"
      description="Destinations that are on, by the category a visitor has to allow. Refusing still counts them in Signals as nobody."
      trailing={<Button asChild variant="outline" size="sm"><Link to="/tracking">Tracking</Link></Button>}
    >
      <ul className="divide-y rounded-md border">
        {CATEGORIES.map((c) => (
          <li key={c.key} className="grid gap-1 p-3 text-sm sm:grid-cols-[12rem_1fr]">
            <div>
              <p className="font-medium">{c.label}</p>
              <p className="text-xs text-muted-foreground">{c.about}</p>
            </div>
            <div className="flex flex-wrap items-center gap-1">
              {feeds(c.key).length === 0 ? (
                <span className="text-muted-foreground">No destination</span>
              ) : (
                feeds(c.key).map((d) => (
                  <Badge key={d.id} variant="outline">{d.name}</Badge>
                ))
              )}
            </div>
          </li>
        ))}
      </ul>
    </DetailPageSection>
  )
}
