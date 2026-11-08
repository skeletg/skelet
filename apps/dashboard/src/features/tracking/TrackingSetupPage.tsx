import { useMemo, useRef } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { LoadError } from '@/shared/errors/LoadError'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { DestinationFields } from './DestinationFields'
import { PlatformIntro } from './PlatformIntro'
import { guideFor } from './platformGuides'
import { installCommand, type Platform } from './status'
import { CreateTrackingDestinationDocument, TrackingPlatformsDocument } from './Tracking.gql'
import {
  toTrackingDestinationInput,
  toTrackingDestinationValues,
  trackingDestinationDiff,
  trackingDestinationSchema,
} from './schemas/trackingDestination'

// Setting a platform up, one step at a time: what it needs and where to find it, the consent it needs, its settings.
// It is saved switched off, in the platform's test mode where it has one - the destination page then sends a test
// and turns it on
export function TrackingSetupPage() {
  const { kind = '' } = useParams()
  const { effectiveChannelId, selectedChannel } = useConfigScope()
  const { data, loading, error, refetch } = useQuery(TrackingPlatformsDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
  })
  const platform = data?.trackingPlatforms.find((p) => p.kind === kind)

  const header = (meta: string) => (
    <DetailPageLayout>
      <DetailPageHeader backTo="/tracking" title="Set up tracking" meta={meta} />
    </DetailPageLayout>
  )
  if (!effectiveChannelId) return header('Pick a channel in the header - every destination belongs to one')
  if (error) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/tracking" title="Set up tracking" />
        <LoadError title="Could not load the platforms" message={error.message} onRetry={() => void refetch()} retrying={loading} />
      </DetailPageLayout>
    )
  }
  if (loading && !data) return header('Loading…')
  if (!platform) return header(`No platform called ${kind}`)
  if (!platform.installed || !platform.availableOnChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/tracking" title={`Set up ${platform.label}`} />
        <div className="px-6 py-6">
          <Alert variant="warning">
            <AlertTitle>{platform.installed ? `${platform.label} is switched off on this channel` : `${platform.label} isn't in this build`}</AlertTitle>
            <AlertDescription>
              {platform.installed ? (
                <>Turn its plugin on for {selectedChannel?.name ?? 'this channel'} under Settings → Plugins, then come back.</>
              ) : (
                <>
                  Install its plugin and rebuild the backend: <code className="font-mono">{installCommand(platform.pluginSlug)}</code>
                </>
              )}
            </AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }
  return <SetupForm platform={platform} channelId={effectiveChannelId} channelName={selectedChannel?.name} />
}

function SetupForm({ platform, channelId, channelName }: { platform: Platform; channelId: string; channelName?: string }) {
  const navigate = useNavigate()
  const [create] = useMutation(CreateTrackingDestinationDocument)
  const created = useRef<string | null>(null)
  const guide = guideFor(platform.kind)
  const testKey = platform.fields.find((f) => f.testMode)?.key
  const preset = testKey && guide.testMode?.preset ? { [testKey]: guide.testMode.preset } : {}
  const baseline = useMemo(() => ({ ...toTrackingDestinationValues(platform.fields, null, preset), name: platform.label }), [platform]) // eslint-disable-line react-hooks/exhaustive-deps
  const record = useRecordForm({
    id: 'tracking-destination',
    title: platform.label,
    label: `Set up ${platform.label}`,
    isEdit: false,
    baseline,
    schema: trackingDestinationSchema(platform.fields, { isEdit: false }),
    diff: trackingDestinationDiff(platform.fields, false),
    persist: { routeKey: `tracking:new:${platform.kind}`, exclude: ['secret'] },
    submit: async (v) => {
      const res = await create({ variables: { channelId, kind: platform.kind, input: toTrackingDestinationInput(platform.fields, v, false) } })
      created.current = res.data?.createTrackingDestination.id ?? null
    },
    onSaved: () => {
      if (created.current) navigate(`/tracking/${created.current}?setup=1`)
    },
  })

  return (
    <RecordFormPage
      record={record}
      backTo="/tracking"
      backLabel="Back to tracking"
      title={`Set up ${platform.label}`}
      meta={channelName ? `For ${channelName}` : undefined}
      createLabel="Save and continue"
      actions={<Link to="/tracking" className="text-sm text-muted-foreground underline-offset-4 hover:underline">Cancel</Link>}
    >
      <PlatformIntro platform={platform} numbered />
      <DestinationFields form={record.form} fields={platform.fields} defaultEvents={platform.defaultEvents} secretSet={false} idPrefix="trk-new" />
      <DetailPageSection title="Next: test, then turn it on">
        <p className="max-w-2xl text-sm text-muted-foreground">
          It is saved switched off.
          {!testKey
            ? ` ${platform.label} has no test mode, so a test send is recorded there as a real purchase of 10 EUR.`
            : guide.testMode
              ? ` ${guide.testMode.note}${guide.testMode.preset ? ' - that is set for you, and cleared when it goes live.' : '.'}`
              : ''}
        </p>
      </DetailPageSection>
    </RecordFormPage>
  )
}
