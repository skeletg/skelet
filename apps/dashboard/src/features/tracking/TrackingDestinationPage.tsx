import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckCircle2Icon, CircleIcon, SendIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { DestinationFields } from './DestinationFields'
import { PlatformIntro } from './PlatformIntro'
import { guideFor } from './platformGuides'
import { destinationStatus, type Destination, type Platform } from './status'
import { TrackingDeliveryLog } from './TrackingDeliveryLog'
import {
  DeleteTrackingDestinationDocument,
  SendTrackingTestDocument,
  SetTrackingDestinationEnabledDocument,
  TrackingDestinationDocument,
  TrackingPlatformsDocument,
  UpdateTrackingDestinationDocument,
} from './Tracking.gql'
import {
  toTrackingDestinationInput,
  toTrackingDestinationValues,
  trackingDestinationDiff,
  trackingDestinationSchema,
} from './schemas/trackingDestination'

export function TrackingDestinationPage() {
  const { id = '' } = useParams()
  const dest = useQuery(TrackingDestinationDocument, { variables: { id }, skip: !id })
  const destination = dest.data?.trackingDestination
  const platforms = useQuery(TrackingPlatformsDocument, {
    variables: { channelId: destination?.channelId ?? '' },
    skip: !destination,
  })
  const platform = platforms.data?.trackingPlatforms.find((p) => p.kind === destination?.kind)

  if ((dest.loading && !destination) || (destination && platforms.loading && !platform)) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/tracking" title="Tracking destination" meta="Loading…" />
      </DetailPageLayout>
    )
  }
  if (dest.error || !destination || !platform) {
    return (
      <DetailPageLayout>
        <DetailPageHeader
          backTo="/tracking"
          title="Tracking destination"
          meta={dest.error?.message ?? platforms.error?.message ?? (destination ? `The ${destination.kind} plugin isn't in this build` : 'Not found')}
        />
      </DetailPageLayout>
    )
  }
  return <DestinationForm key={destination.id} destination={destination} platform={platform} refetch={() => void dest.refetch()} />
}

function DestinationForm({ destination, platform, refetch }: { destination: Destination; platform: Platform; refetch: () => void }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('tracking.manage')
  const guide = guideFor(platform.kind)
  const [update] = useMutation(UpdateTrackingDestinationDocument)
  const [setEnabled, { loading: toggling }] = useMutation(SetTrackingDestinationEnabledDocument)
  const [sendTest, { loading: testing }] = useMutation(SendTrackingTestDocument)
  const [remove] = useMutation(DeleteTrackingDestinationDocument)
  const [test, setTest] = useState<{ ok: boolean; message: string } | null>(null)
  const [opError, setOpError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [confirmTest, setConfirmTest] = useState(false)

  const baseline = useMemo(() => toTrackingDestinationValues(platform.fields, destination), [destination, platform])
  const record = useRecordForm({
    id: 'tracking-destination',
    title: platform.label,
    label: `Tracking · ${destination.name}`,
    isEdit: true,
    baseline,
    schema: trackingDestinationSchema(platform.fields, { isEdit: true }),
    diff: trackingDestinationDiff(platform.fields, destination.secretSet),
    persist: { routeKey: `tracking:${destination.id}`, exclude: ['secret'] },
    submit: async (v) => {
      await update({ variables: { id: destination.id, input: toTrackingDestinationInput(platform.fields, v) } })
      refetch()
    },
  })

  const testKey = platform.fields.find((f) => f.testMode)?.key
  const { inTestMode } = destination
  const status = destinationStatus(destination)

  const act = async (fn: () => Promise<unknown>) => {
    setOpError(null)
    try {
      await fn()
      refetch()
    } catch (e) {
      setOpError(e instanceof Error ? e.message : 'Something went wrong')
    }
  }
  const runTest = () =>
    act(async () => {
      const res = await sendTest({ variables: { id: destination.id } })
      setTest(res.data?.sendTrackingTest ?? null)
    })
  // Outside test mode the platform records the sample purchase as real revenue
  const askTest = () => (inTestMode ? void runTest() : setConfirmTest(true))
  // Going live drops the test setting and switches it on in one save, so no real event is sent in test mode
  const goLive = () =>
    act(async () => {
      const values = toTrackingDestinationValues(platform.fields, destination)
      if (testKey) values.settings[testKey] = ''
      await update({ variables: { id: destination.id, input: toTrackingDestinationInput(platform.fields, values, true) } })
    })

  const steps = [
    { done: !destination.needsSecret, label: destination.needsSecret ? `Enter the ${platform.fields.find((f) => f.secret)?.label ?? 'secret'} below and save` : 'Settings saved' },
    { done: test?.ok === true, label: test?.ok ? 'Test accepted' : 'Send a test' },
    { done: destination.enabled, label: 'Turn it on' },
  ]

  return (
    <>
      <RecordFormPage
        record={record}
        backTo="/tracking"
        backLabel="Back to tracking"
        title={destination.name}
        readOnly={!canManage}
        badges={
          <>
            <Badge variant="outline">{platform.label}</Badge>
            <Badge variant={status.variant}>{status.label}</Badge>
            {inTestMode && <Badge variant="info">Test mode</Badge>}
          </>
        }
        meta={
          destination.health.lastSentAt
            ? `Last sent ${new Date(destination.health.lastSentAt).toLocaleString()}`
            : 'Nothing sent yet'
        }
        createLabel="Save changes"
        actions={
          canManage && (
            <>
              <label htmlFor="trk-enabled" className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm">
                <Switch
                  id="trk-enabled"
                  checked={destination.enabled}
                  disabled={toggling || destination.needsSecret}
                  onCheckedChange={(on) => act(() => setEnabled({ variables: { id: destination.id, enabled: on } }))}
                />
                <span className={destination.enabled ? 'font-medium' : 'text-muted-foreground'}>{destination.enabled ? 'On' : 'Off'}</span>
              </label>
              <Button type="button" variant="outline" size="sm" onClick={askTest} disabled={testing || destination.needsSecret}>
                <SendIcon className="size-4" />
                {testing ? 'Sending…' : 'Send test'}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2Icon className="size-4" />
                Delete
              </Button>
            </>
          )
        }
        after={
          <DetailPageSection title="Delivery log" description="Events still waiting to be sent or that failed, with the platform's answer. Sent events leave the log.">
            <TrackingDeliveryLog destinationId={destination.id} canManage={canManage} />
          </DetailPageSection>
        }
      >
        {(opError || test) && (
          <div className="px-6 pt-6">
            {opError ? (
              <Alert variant="destructive">
                <AlertTitle>That didn't work</AlertTitle>
                <AlertDescription>{opError}</AlertDescription>
              </Alert>
            ) : test?.ok ? (
              <Alert variant="success">
                <AlertTitle>{platform.label} accepted the test</AlertTitle>
                <AlertDescription>
                  {inTestMode ? 'It ran in test mode, so nothing was recorded.' : !testKey ? 'It is recorded there as a real purchase of 10 EUR.' : 'Check it arrived in ' + platform.label + '.'}
                </AlertDescription>
              </Alert>
            ) : (
              <Alert variant="destructive">
                <AlertTitle>{platform.label} refused the test</AlertTitle>
                <AlertDescription className="font-mono text-xs">{test?.message}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        {destination.needsSecret && (
          <div className="px-6 pt-6">
            <Alert variant="warning">
              <AlertTitle>Needs its {platform.fields.find((f) => f.secret)?.label ?? 'secret'}</AlertTitle>
              <AlertDescription>
                {destination.importedFrom
                  ? 'Imported from Signals, which never hands secrets over. Enter it below and save, then send a test.'
                  : 'Enter it below and save before turning this destination on.'}
              </AlertDescription>
            </Alert>
          </div>
        )}

        {destination.importedFrom && platform.kind === 'webhook' && (
          <div className="px-6 pt-6">
            <Alert variant="info">
              <AlertTitle>The webhook format changed</AlertTitle>
              <AlertDescription>
                Signals posted batches signed with X-Destination-Signature. This sends one event per POST, signed with X-Webhook-Signature-256 - update the receiver before turning it on.
              </AlertDescription>
            </Alert>
          </div>
        )}

        {!destination.enabled && canManage && (
          <DetailPageSection title="Finish setting up">
            <div className="grid max-w-2xl gap-3">
              <ol className="grid gap-2 text-sm">
                {steps.map((s, i) => (
                  <li key={i} className="flex items-center gap-2">
                    {s.done ? <CheckCircle2Icon className="size-4 text-success" /> : <CircleIcon className="size-4 text-muted-foreground" />}
                    <span className={s.done ? 'text-muted-foreground' : ''}>{s.label}</span>
                  </li>
                ))}
              </ol>
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" size="sm" onClick={askTest} disabled={testing || destination.needsSecret}>
                  <SendIcon className="size-4" />
                  Send a test
                </Button>
                <Button type="button" size="sm" onClick={goLive} disabled={destination.needsSecret || record.registry.isDirty}>
                  {inTestMode ? 'Leave test mode and turn on' : 'Turn on'}
                </Button>
              </div>
              {record.registry.isDirty && <p className="text-xs text-muted-foreground">Save your changes first.</p>}
              {!testKey && <p className="text-xs text-muted-foreground">{platform.label} has no test mode - a test send is recorded there as a real purchase of 10 EUR.</p>}
            </div>
          </DetailPageSection>
        )}

        {destination.enabled && inTestMode && (
          <div className="px-6 pt-6">
            <Alert variant="warning">
              <AlertTitle>Still in test mode</AlertTitle>
              <AlertDescription>
                {guide.testMode?.note ?? 'Its test-mode setting is on'}. Events reach {platform.label} but aren't recorded.{' '}
                {canManage && (
                  <Button type="button" variant="link" className="h-auto p-0" onClick={goLive} disabled={record.registry.isDirty}>
                    Leave test mode
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          </div>
        )}

        <DestinationFields form={record.form} fields={platform.fields} defaultEvents={platform.defaultEvents} secretSet={destination.secretSet} idPrefix="trk" />
        <PlatformIntro platform={platform} />
      </RecordFormPage>

      <ConfirmDialog
        open={confirmTest}
        onOpenChange={setConfirmTest}
        title="Send a real test purchase?"
        description={`${destination.name} isn't in test mode, so ${platform.label} records the sample 10 EUR purchase as a real conversion in its reports.`}
        confirmLabel="Send test"
        onConfirm={runTest}
      />
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`Delete ${destination.name}?`}
        description="It stops receiving events at once, and its delivery log goes with it."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          await remove({ variables: { id: destination.id } })
          navigate('/tracking')
        }}
      />
    </>
  )
}
