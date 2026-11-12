import { useId, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { useConfigScope, useUnsavedChangesGuard } from '@/lib/config-scope'
import { duringCommit } from '@/lib/nav-guard'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { EmailBlock, EmailStyles } from '@/features/email/blocks'
import { LibraryEmailDocument, LibraryEmailNamesDocument, RenderLibraryPreviewDocument } from '@/features/emails/Emails.gql'
import { PreviewDialog, type Rendered } from '@/features/emails/PreviewDialog'
import { CampaignAudienceEstimateDocument, CampaignGroupsDocument, CampaignLimitsDocument, CampaignSegmentsDocument, CampaignVariantStatsDocument, CancelEmailCampaignDocument, CreateEmailCampaignDocument, EmailCampaignDocument, EmailCampaignStatsDocument, RenderCampaignPreviewDocument, ScheduleEmailCampaignDocument, SendCampaignTestDocument, UpdateEmailCampaignDocument } from './Campaigns.gql'
import { AudiencePicker, EmailPicker, MeasurePicker, SharePicker, SubjectPicker, WhenPicker, WindowPicker } from './CampaignPickers'
import { Landing, Pick, Plan, Reach, Said, SendDate, Sentence, Stage } from './SendSheet'
import { abSplit, audiencePhrase, hasPassed, localInput, sendPlan, sendTime, whenPhrase, windowPhrase, type When } from './sentence'
import { PluginSections } from '@/plugins/detail-sections'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { LoadError } from '@/shared/errors/LoadError'
import { NotFound } from '@/shared/errors/NotFound'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'

type Draft = {
  id: string | null
  name: string
  emailId: string | null
  audienceKind: string
  audienceId: string | null
  status: string
  channelId: string
  scheduledFor: string | null
  sentAt: string | null
  // inline content, for a campaign that carries its own body instead of referencing a library email
  subject: string
  blocks: EmailBlock[]
  styles: EmailStyles
  // A/B subject test
  abEnabled: boolean
  abSubjectB: string
  abTestPercent: number
  abWinnerMetric: string
  abTestMinutes: number
  abWinner: string
}

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  scheduled: 'secondary',
  sending: 'default',
  sent: 'default',
  cancelled: 'outline',
}

function plannedWhen(scheduledFor: string | null): When {
  return scheduledFor ? { mode: 'later', at: localInput(new Date(scheduledFor)) } : { mode: 'now' }
}

const whenKey = (w: When) => (w.mode === 'later' ? w.at : '')

const listed = (items: string[]) => (items.length < 2 ? items.join('') : `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`)

export function CampaignComposer() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !id || id === 'new'

  const { data, loading, error, refetch } = useQuery(EmailCampaignDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New campaign" backTo="/campaigns" />
        <DetailPageBody>
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>Select a channel in the header before creating a campaign.</AlertDescription>
            </Alert>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )
  }
  // A failed load or a missing record used to leave the editor's header over an empty body
  if (!isNew && !data?.emailCampaign && error) {
    return <LoadError title="Couldn't load this campaign" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!isNew && !data?.emailCampaign && !loading) return <NotFound />
  if (!isNew && !data?.emailCampaign) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Campaign" backTo="/campaigns" />
        <DetailPageBody>{loading ? <LoadingState /> : null}</DetailPageBody>
      </DetailPageLayout>
    )
  }

  const initial: Draft = isNew
    ? { id: null, name: '', emailId: null, audienceKind: 'all', audienceId: null, status: 'draft', channelId: effectiveChannelId ?? '', scheduledFor: null, sentAt: null, subject: '', blocks: [], styles: {}, abEnabled: false, abSubjectB: '', abTestPercent: 20, abWinnerMetric: 'open', abTestMinutes: 240, abWinner: '' }
    : {
        id: data!.emailCampaign!.id,
        name: data!.emailCampaign!.name,
        emailId: data!.emailCampaign!.emailId ?? null,
        audienceKind: data!.emailCampaign!.audienceKind,
        audienceId: data!.emailCampaign!.audienceId ?? null,
        status: data!.emailCampaign!.status,
        channelId: data!.emailCampaign!.channelId,
        scheduledFor: data!.emailCampaign!.scheduledFor ?? null,
        sentAt: data!.emailCampaign!.sentAt ?? null,
        subject: data!.emailCampaign!.subject,
        blocks: (data!.emailCampaign!.blocks as EmailBlock[] | null) ?? [],
        styles: (data!.emailCampaign!.styles as EmailStyles | null) ?? {},
        abEnabled: data!.emailCampaign!.abEnabled,
        abSubjectB: data!.emailCampaign!.abSubjectB,
        abTestPercent: data!.emailCampaign!.abTestPercent,
        abWinnerMetric: data!.emailCampaign!.abWinnerMetric,
        abTestMinutes: data!.emailCampaign!.abTestMinutes,
        abWinner: data!.emailCampaign!.abWinner,
      }

  // Only a draft is edited, and only the editor needs the library, the audiences and the counts
  if (initial.status !== 'draft') return <ReadOnlyCampaign key={initial.id} initial={initial} />
  return <ComposerInner key={initial.id ?? 'new'} initial={initial} onCreated={(newId) => void duringCommit(() => navigate(`/campaigns/${newId}`))} />
}

function ComposerInner({ initial, onCreated }: { initial: Draft; onCreated: (id: string) => void }) {
  const fid = useId()
  // A new email needs emails.manage - editing a campaign or flow doesn't bring it
  const canCreateEmail = usePermissions().has('emails.manage')
  const { channels } = useConfigScope()
  const channelName = channels.find((c) => c.id === initial.channelId)?.name ?? ''
  const [now] = useState(() => new Date())
  const [name, setName] = useState(initial.name)
  const [emailId, setEmailId] = useState<string | null>(initial.emailId)
  const [audienceKind, setAudienceKind] = useState(initial.audienceKind)
  const [audienceId, setAudienceId] = useState<string | null>(initial.audienceId)
  const [campaignId, setCampaignId] = useState<string | null>(initial.id)
  const [abEnabled, setAbEnabled] = useState(initial.abEnabled)
  const [abSubjectB, setAbSubjectB] = useState(initial.abSubjectB)
  const [abTestPercent, setAbTestPercent] = useState(initial.abTestPercent)
  const [abWinnerMetric, setAbWinnerMetric] = useState(initial.abWinnerMetric)
  const [abTestMinutes, setAbTestMinutes] = useState(initial.abTestMinutes)
  const [when, setWhen] = useState<When>(() => plannedWhen(initial.scheduledFor))
  const at = sendTime(when)
  const past = at != null && at.getTime() <= now.getTime()

  const baseline = useMemo(
    () =>
      JSON.stringify({
        n: initial.name, e: initial.emailId, ak: initial.audienceKind, ai: initial.audienceId,
        ab: initial.abEnabled, bs: initial.abSubjectB, ap: initial.abTestPercent, am: initial.abWinnerMetric, at: initial.abTestMinutes,
        w: whenKey(plannedWhen(initial.scheduledFor)),
      }),
    [initial],
  )
  const current = JSON.stringify({
    n: name, e: emailId, ak: audienceKind, ai: audienceId,
    ab: abEnabled, bs: abSubjectB, ap: abTestPercent, am: abWinnerMetric, at: abTestMinutes,
    w: whenKey(when),
  })
  const dirty = current !== baseline
  useUnsavedChangesGuard(dirty, 'Unsaved campaign changes')

  const { data: emailsData, refetch: refetchEmails } = useQuery(LibraryEmailNamesDocument, { variables: { channelId: initial.channelId }, skip: !initial.channelId })
  const { data: limitsData } = useQuery(CampaignLimitsDocument)
  const { data: groupsData } = useQuery(CampaignGroupsDocument)
  const { data: segmentsData } = useQuery(CampaignSegmentsDocument)
  const emails = emailsData?.libraryEmails ?? []
  const groups = (groupsData?.customerGroups ?? []).map((g) => ({ id: g.id, name: g.name, count: g.memberCount }))
  const segments = (segmentsData?.customerSegments.items ?? []).map((s) => ({ id: s.id, name: s.name, count: s.memberCount }))

  const audienceReady = audienceKind === 'all' || !!audienceId
  const { data: estData, error: estError } = useQuery(CampaignAudienceEstimateDocument, {
    variables: { channelId: initial.channelId, audienceKind, audienceId },
    skip: !audienceReady || !initial.channelId,
    fetchPolicy: 'cache-and-network',
  })
  const estimate = audienceReady ? estData?.campaignAudienceEstimate : undefined

  const { data: selEmail, loading: emailLoading } = useQuery(LibraryEmailDocument, { variables: { id: emailId ?? '' }, skip: !emailId })
  const email = emailId ? (selEmail?.libraryEmail ?? null) : null
  const emailName = emails.find((e) => e.id === emailId)?.name ?? email?.name ?? null
  const content = email
    ? { subject: email.subject, preheader: email.preheader, blocks: email.blocks as EmailBlock[], styles: (email.styles ?? {}) as EmailStyles }
    : !emailId && initial.blocks.length > 0
      ? { subject: initial.subject, preheader: '', blocks: initial.blocks, styles: initial.styles }
      : null
  const emailSet = !!content || (!!emailId && emailLoading)

  const { data: renderData, previousData: renderBefore, error: renderError } = useQuery(RenderLibraryPreviewDocument, {
    variables: { channelId: initial.channelId, customerId: null, subject: content?.subject ?? '', preheader: content?.preheader ?? '', blocks: content?.blocks ?? [], styles: content?.styles ?? {} },
    skip: !content,
    fetchPolicy: 'no-cache',
  })
  const shown = content ? (renderData ?? renderBefore)?.renderLibraryPreview : null
  const rendered: Rendered | null = shown
    ? { subject: shown.subject, html: shown.html, text: shown.text ?? '', preheader: shown.preheader ?? '', fromName: shown.fromName ?? '', fromAddress: shown.fromAddress ?? '' }
    : null

  const [createCampaign] = useMutation(CreateEmailCampaignDocument)
  const [updateCampaign] = useMutation(UpdateEmailCampaignDocument)
  const [scheduleCampaign] = useMutation(ScheduleEmailCampaignDocument)
  const [sendTest] = useMutation(SendCampaignTestDocument)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function inputVars() {
    return {
      channelId: initial.channelId,
      name: name.trim(),
      emailId,
      // A campaign that carries its own email keeps it through a save
      subject: initial.subject,
      blocks: initial.blocks,
      styles: initial.styles,
      audienceKind,
      audienceId: audienceKind === 'all' ? null : audienceId,
      scheduledFor: at ? at.toISOString() : null,
      abEnabled,
      abSubjectB: abSubjectB.trim(),
      abTestPercent,
      abWinnerMetric,
      abTestMinutes,
    }
  }

  async function save(): Promise<string | null> {
    setError(null)
    setSaving(true)
    try {
      if (campaignId) {
        await updateCampaign({ variables: { id: campaignId, input: inputVars() } })
        return campaignId
      }
      const res = await createCampaign({ variables: { input: inputVars() } })
      const newId = res.data?.createEmailCampaign?.id ?? null
      if (newId) setCampaignId(newId)
      return newId
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
      return null
    } finally {
      setSaving(false)
    }
  }

  async function onSaveClick() {
    const savedId = await save()
    if (savedId && !initial.id) onCreated(savedId)
  }

  const [confirmOpen, setConfirmOpen] = useState(false)
  const [scheduling, setScheduling] = useState(false)
  async function onConfirmSend() {
    if (at && hasPassed(at)) {
      setError('That send time has passed - pick a later one.')
      return
    }
    setScheduling(true)
    setError(null)
    try {
      const savedId = await save()
      if (!savedId) return
      await scheduleCampaign({ variables: { id: savedId, scheduledFor: at ? at.toISOString() : null } })
      setConfirmOpen(false)
      onCreated(savedId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Schedule failed.')
    } finally {
      setScheduling(false)
    }
  }

  // test send
  const [testOpen, setTestOpen] = useState(false)
  const [testTo, setTestTo] = useState('')
  const [sendingTest, setSendingTest] = useState(false)
  const [testMsg, setTestMsg] = useState<string | null>(null)
  async function onSendTest() {
    if (!testTo.trim()) return
    setSendingTest(true)
    setTestMsg(null)
    try {
      const savedId = await save()
      if (!savedId) return
      await sendTest({ variables: { id: savedId, to: testTo.trim() } })
      setTestMsg(`Sent to ${testTo.trim()}.`)
    } catch (err) {
      setTestMsg(err instanceof Error ? err.message : 'Send failed.')
    } finally {
      setSendingTest(false)
    }
  }

  const [previewOpen, setPreviewOpen] = useState(false)

  const title = name.trim() || (initial.id ? 'Campaign' : 'New campaign')
  const emailText = emailName ?? (content ? 'its own email' : 'an email')
  const audienceName = (audienceKind === 'group' ? groups : segments).find((o) => o.id === audienceId)?.name ?? null
  const audienceText = audiencePhrase(audienceKind, audienceKind === 'all' ? null : audienceName, channelName)
  const whenText = when.mode === 'later' && !at ? 'at a set time' : whenPhrase(at, now)
  const subjectB = abSubjectB.trim()
  const split = abEnabled && estimate ? abSplit(estimate.total, estimate.eligible, abTestPercent) : null
  const plan = sendPlan({ at, eligible: estimate?.eligible ?? null, ab: abEnabled ? { minutes: abTestMinutes, metric: abWinnerMetric, split } : null, now })
  const missing = [
    !name.trim() && 'a name',
    !emailSet && 'an email',
    !audienceReady && (audienceKind === 'group' ? 'a customer group' : 'a segment'),
    abEnabled && !subjectB && 'a second subject',
    when.mode === 'later' && !at && 'a send time',
    past && 'a send time that has not passed',
  ].filter((m): m is string => !!m)
  const sendLabel = when.mode === 'now' ? 'Send now' : 'Schedule'

  return (
    <DetailPageLayout toc={false}>
      <DetailPageHeader
        title={title}
        backTo="/campaigns"
        badges={<Badge variant={STATUS_VARIANT[initial.status] ?? 'outline'} className="capitalize">{initial.status}</Badge>}
        actions={
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" size="sm" disabled={!content} onClick={() => setPreviewOpen(true)}>
              Preview
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={!content || (!campaignId && !name.trim())} onClick={() => setTestOpen(true)}>
              Send a test
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={saving || !name.trim()} title={name.trim() ? undefined : 'Name the campaign to save it'} onClick={onSaveClick}>
              {saving ? 'Saving…' : dirty || !initial.id ? 'Save draft' : 'Saved'}
            </Button>
            <Button type="button" size="sm" disabled={missing.length > 0} title={missing.length ? `Still needs ${listed(missing)}` : undefined} onClick={() => setConfirmOpen(true)}>
              {sendLabel}
            </Button>
          </div>
        }
      />
      {error && !confirmOpen && (
        <Alert variant="destructive" className="mx-6 mt-4">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DetailPageBody className="divide-y-0">
        <div className="px-6 pt-6">
          <Stage
            aside={
              at ? (
                <SendDate at={at} lead={past ? 'Passed' : 'Goes out'} />
              ) : when.mode === 'later' ? (
                <SendDate word="Not set" beside={['Pick a time', 'in the sentence']} />
              ) : (
                <SendDate word="Now" beside={['As soon as', 'you send it']} note="Starts going out within about a minute" />
              )
            }
            head={
              <div className="grid gap-1">
                <Label htmlFor={`${fid}-name`} className="sr-only">
                  Campaign name
                </Label>
                <input
                  id={`${fid}-name`}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Name this campaign"
                  autoComplete="off"
                  className={cn(
                    'w-full max-w-xl border-b bg-transparent pb-0.5 text-lg font-semibold tracking-tight text-foreground outline-none transition-colors placeholder:font-normal placeholder:text-muted-foreground focus:border-foreground/50',
                    name.trim() ? 'border-transparent hover:border-foreground/20' : 'border-dashed border-foreground/45',
                  )}
                />
                <p className="truncate text-xs text-muted-foreground">
                  {rendered ? `From ${rendered.fromName} <${rendered.fromAddress}>` : channelName ? `From ${channelName}` : 'Only you see the name'}
                </p>
              </div>
            }
          >
            <Sentence>
              Send{' '}
              <Pick what="Email" value={emailText} filled={emailSet} className="w-96" onOpen={() => void refetchEmails()}>
                {(close) => (
                  <EmailPicker
                    emails={emails}
                    value={emailId}
                    canCreate={canCreateEmail}
                    onPick={(v) => {
                      setEmailId(v)
                      close()
                    }}
                  />
                )}
              </Pick>{' '}
              to{' '}
              <Pick what="Audience" value={audienceText} filled={audienceReady}>
                {(close) => (
                  <AudiencePicker
                    kind={audienceKind}
                    id={audienceId}
                    channel={channelName}
                    groups={groups}
                    segments={segments}
                    close={close}
                    onChange={(k, v) => {
                      setAudienceKind(k)
                      setAudienceId(v)
                    }}
                  />
                )}
              </Pick>{' '}
              <Pick what="When" value={whenText} filled={!(when.mode === 'later' && (!at || past))}>
                {() => <WhenPicker when={when} min={localInput(now)} past={past} onChange={setWhen} />}
              </Pick>
              .
            </Sentence>

            {abEnabled ? (
              <Sentence minor>
                First, test the subject{' '}
                <Pick what="Subject B" value={subjectB ? `"${subjectB}"` : 'a second subject'} filled={!!subjectB}>
                  {(close) => <SubjectPicker value={abSubjectB} own={content?.subject ?? ''} onChange={setAbSubjectB} close={close} />}
                </Pick>{' '}
                on{' '}
                <Pick what="Share of the audience in the test" value={`${abTestPercent}%`}>
                  {() => <SharePicker value={abTestPercent} limits={limitsData?.campaignLimits} onChange={setAbTestPercent} />}
                </Pick>{' '}
                of them for{' '}
                <Pick what="How long the test runs" value={windowPhrase(abTestMinutes)}>
                  {() => <WindowPicker minutes={abTestMinutes} onChange={setAbTestMinutes} />}
                </Pick>
                . The one with more{' '}
                <Pick what="What decides the winner" value={abWinnerMetric === 'click' ? 'clicks' : 'opens'} className="w-64">
                  {(close) => <MeasurePicker value={abWinnerMetric} onChange={setAbWinnerMetric} close={close} />}
                </Pick>{' '}
                goes to the rest.
              </Sentence>
            ) : null}

            <div className="flex flex-wrap items-center gap-x-8 gap-y-2 text-sm">
              <label className="flex items-center gap-2.5 text-foreground">
                <Switch size="sm" checked={abEnabled} onCheckedChange={setAbEnabled} />
                Test two subject lines first
              </label>
              {missing.length > 0 ? <p className="text-muted-foreground">Still needs {listed(missing)}.</p> : null}
            </div>
          </Stage>
        </div>

        <div className="@container px-6 py-8">
          <div className="grid grid-cols-1 gap-x-12 gap-y-10 @4xl:grid-cols-[minmax(0,1.6fr)_minmax(17rem,1fr)]">
            <Landing
              r={rendered}
              subjectB={abEnabled ? subjectB || 'Subject B' : null}
              empty={renderError ? `The email didn't render: ${renderError.message}` : emailSet ? 'Rendering the email…' : emails.length === 0 ? 'There are no emails in the library yet. Build one, then pick it in the sentence above.' : 'Pick an email in the sentence above, and it shows here the way it lands.'}
              action={
                <Button type="button" variant="ghost" size="sm" onClick={() => setPreviewOpen(true)}>
                  Open full preview
                </Button>
              }
            />
            <div className="grid content-start gap-10">
              <Reach
                total={estimate?.total ?? null}
                eligible={estimate?.eligible ?? null}
                split={split}
                pending={estError ? `The audience couldn't be counted: ${estError.message}` : audienceReady ? 'Counting…' : `Pick ${audienceKind === 'group' ? 'a customer group' : 'a segment'} to see who gets it.`}
              />
              <Plan steps={plan} />
            </div>
          </div>
        </div>
      </DetailPageBody>

      {content ? (
        <PreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} channelId={initial.channelId} subject={content.subject} preheader={content.preheader} blocks={content.blocks} styles={content.styles} />
      ) : null}

      <Dialog open={confirmOpen} onOpenChange={(o) => !scheduling && setConfirmOpen(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{when.mode === 'now' ? 'Send this campaign now?' : 'Schedule this campaign?'}</DialogTitle>
            <DialogDescription>
              {emailText.charAt(0).toUpperCase() + emailText.slice(1)} goes to {audienceText} {whenText}.
              {estimate ? ` ${formatNumber(estimate.eligible)} of ${formatNumber(estimate.total)} are opted in to marketing email and will get it.` : ''}
            </DialogDescription>
          </DialogHeader>
          {abEnabled ? <Plan steps={plan} /> : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={scheduling}>
              Cancel
            </Button>
            <Button onClick={onConfirmSend} disabled={scheduling}>
              {scheduling ? (when.mode === 'now' ? 'Sending…' : 'Scheduling…') : sendLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={testOpen} onOpenChange={setTestOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Send a test</DialogTitle>
            <DialogDescription>Saves the draft, then sends the email to one address with sample data filled in.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="ctest">Recipient</Label>
            <Input id="ctest" type="email" value={testTo} onChange={(e) => setTestTo(e.target.value)} placeholder="you@example.com" />
          </div>
          {testMsg && (
            <Alert className="mt-1">
              <AlertDescription>{testMsg}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setTestOpen(false)}>
              Close
            </Button>
            <Button onClick={onSendTest} disabled={!testTo.trim() || sendingTest}>
              {sendingTest ? 'Sending…' : 'Send test'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DetailPageLayout>
  )
}

function ReadOnlyCampaign({ initial }: { initial: Draft }) {
  const [cancelCampaign] = useMutation(CancelEmailCampaignDocument)
  const [error, setError] = useState<string | null>(null)
  const [confirmCancel, setConfirmCancel] = useState(false)
  // The cancelled campaign comes back from the mutation, and the page redraws from it
  async function onConfirmCancel() {
    if (!initial.id) return
    setError(null)
    try {
      await cancelCampaign({ variables: { id: initial.id } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'The send could not be cancelled.')
    }
  }
  const { channels } = useConfigScope()
  const channelName = channels.find((c) => c.id === initial.channelId)?.name ?? ''
  const [now] = useState(() => new Date())
  // Prefer the referenced library email's content; fall back to the campaign's own inline body
  const { data: selEmail } = useQuery(LibraryEmailDocument, { variables: { id: initial.emailId ?? '' }, skip: !initial.emailId })
  const { data: groupsData } = useQuery(CampaignGroupsDocument, { skip: initial.audienceKind !== 'group' })
  const { data: segmentsData } = useQuery(CampaignSegmentsDocument, { skip: initial.audienceKind !== 'segment' })
  const content = initial.emailId
    ? { subject: selEmail?.libraryEmail?.subject ?? '', blocks: selEmail?.libraryEmail?.blocks ?? [], styles: selEmail?.libraryEmail?.styles ?? {} }
    : { subject: initial.subject, blocks: initial.blocks, styles: initial.styles }
  const { data } = useQuery(RenderCampaignPreviewDocument, {
    variables: { channelId: initial.channelId, subject: content.subject, blocks: content.blocks, styles: content.styles },
    skip: initial.emailId != null && !selEmail?.libraryEmail,
    fetchPolicy: 'no-cache',
  })
  const canCancel = initial.status === 'scheduled' || initial.status === 'sending'

  const audienceName =
    initial.audienceKind === 'group'
      ? groupsData?.customerGroups.find((g) => g.id === initial.audienceId)?.name
      : segmentsData?.customerSegments.items.find((s) => s.id === initial.audienceId)?.name
  const audience = audiencePhrase(initial.audienceKind, audienceName ?? null, channelName)
  const emailName = initial.emailId ? selEmail?.libraryEmail?.name : undefined
  const unnamed = initial.emailId ? 'an email' : 'its own email'
  const email = emailName ?? unnamed.charAt(0).toUpperCase() + unnamed.slice(1)
  const stamp = initial.sentAt ?? initial.scheduledFor
  const at = stamp ? new Date(stamp) : null
  const began = initial.status === 'sent' ? 'Sent' : initial.status === 'scheduled' ? 'Goes out' : 'Started'
  const said = at ? whenPhrase(at, now) : ''
  const lead = said.charAt(0).toUpperCase() + said.slice(1)

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={initial.name || 'Campaign'}
        backTo="/campaigns"
        badges={<Badge variant={STATUS_VARIANT[initial.status] ?? 'outline'} className="capitalize">{initial.status}</Badge>}
        actions={
          canCancel ? (
            <Button type="button" variant="outline" size="sm" className="text-destructive" onClick={() => setConfirmCancel(true)}>
              Cancel send
            </Button>
          ) : undefined
        }
      />
      {error && (
        <Alert variant="destructive" className="mx-6 mt-4">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DetailPageBody>
        <div className="px-6 py-6">
          <Stage
            aside={
              initial.status === 'cancelled' || !at ? (
                <SendDate word="Not sent" beside={initial.status === 'cancelled' ? ['Cancelled', 'before it went out'] : ['No send time', 'on record']} />
              ) : (
                <SendDate at={at} lead={began} />
              )
            }
            head={
              <div className="grid gap-1">
                <p className="truncate text-lg font-semibold tracking-tight text-foreground">{initial.name || 'Campaign'}</p>
                {channelName ? <p className="truncate text-xs text-muted-foreground">From {channelName}</p> : null}
              </div>
            }
          >
            <Sentence>
              {initial.status === 'cancelled' ? (
                <>
                  The send of <Said>{emailName ?? unnamed}</Said> to <Said>{audience}</Said> was cancelled.
                </>
              ) : lead && (initial.status === 'sent' || initial.status === 'scheduled') ? (
                <>
                  <Said>{lead}</Said>, <Said>{emailName ?? unnamed}</Said> {initial.status === 'sent' ? 'went' : 'goes'} to <Said>{audience}</Said>.
                </>
              ) : initial.status === 'sent' ? (
                <>
                  <Said>{email}</Said> went to <Said>{audience}</Said>.
                </>
              ) : (
                <>
                  <Said>{email}</Said> is going out to <Said>{audience}</Said>.
                </>
              )}
            </Sentence>
            {initial.abEnabled ? (
              <Sentence minor>
                It tests <Said>"{initial.abSubjectB}"</Said> against the email's own subject on <Said>{initial.abTestPercent}%</Said> for{' '}
                <Said>{windowPhrase(initial.abTestMinutes)}</Said>, and the one with more <Said>{initial.abWinnerMetric === 'click' ? 'clicks' : 'opens'}</Said> goes to the rest.
              </Sentence>
            ) : null}
          </Stage>
        </div>
        <DetailPageSection title="Delivery">
          <CampaignStats initial={initial} />
        </DetailPageSection>
        {initial.abEnabled && initial.id ? (
          <DetailPageSection title="Subject test">
            <ABResults campaignId={initial.id} winner={initial.abWinner} metric={initial.abWinnerMetric} />
          </DetailPageSection>
        ) : null}
        <DetailPageSection title="Preview" description="Rendered with sample data.">
          <div className="overflow-hidden rounded-md border bg-white">
            <div className="border-b bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
              <span className="font-medium text-foreground">Subject:</span> {data?.renderCampaignPreview.subject || '-'}
            </div>
            <iframe title="Campaign preview" sandbox="" className="h-[480px] w-full bg-white" srcDoc={data?.renderCampaignPreview.html ?? ''} />
          </div>
        </DetailPageSection>
        <PluginSections target="campaign" entity={initial} mode={initial.id ? 'edit' : 'create'} />
      </DetailPageBody>

      <ConfirmDialog
        open={confirmCancel}
        onOpenChange={setConfirmCancel}
        title="Cancel this send?"
        description="The campaign stops sending. Recipients already emailed have been sent; the rest will not be."
        confirmLabel="Cancel send"
        cancelLabel="Keep"
        // Not awaited: the dialog closes at once, as it always has
        onConfirm={() => void onConfirmCancel()}
      />
    </DetailPageLayout>
  )
}

// Counters only move while a send is in flight, so only then does this poll - and it polls the counters alone, not
// the full campaign with its email design. The polled status lands in the same cached campaign the page builds
// `initial` from, so a finished send ends the polling by itself
const IN_FLIGHT = new Set(['scheduled', 'sending', 'testing'])

export function CampaignStats({ initial }: { initial: Draft }) {
  const pollInterval = useVisiblePollInterval(IN_FLIGHT.has(initial.status) ? 5000 : 0)
  const { data, refetch } = useQuery(EmailCampaignStatsDocument, { variables: { id: initial.id ?? '' }, skip: !initial.id, pollInterval })
  useRefetchWhenShown(refetch)
  const c = data?.emailCampaign
  const cells: Array<[string, string | number]> = [
    ['Total', c?.totalRecipients ?? 0],
    ['Sent', c?.sentCount ?? 0],
    ['Failed', c?.failedCount ?? 0],
    ['Skipped', c?.skippedCount ?? 0],
  ]
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {cells.map(([label, val]) => (
        <div key={label} className="rounded-lg border p-3">
          <div className="text-xs text-muted-foreground">{label}</div>
          <div className="text-lg font-semibold tabular-nums">{val}</div>
        </div>
      ))}
    </div>
  )
}

// ABResults shows the per-variant tally + winner for a sent/testing A/B campaign
function ABResults({ campaignId, winner, metric }: { campaignId: string; winner: string; metric: string }) {
  const { data } = useQuery(CampaignVariantStatsDocument, { variables: { id: campaignId }, fetchPolicy: 'cache-and-network' })
  const stats = data?.campaignVariantStats ?? []
  const row = (v: string) => stats.find((s) => s.variant === v)
  const rate = (s?: { sent: number; opened: number; clicked: number }) => {
    if (!s || s.sent === 0) return '-'
    const n = metric === 'click' ? s.clicked : s.opened
    return `${Math.round((n / s.sent) * 100)}%`
  }
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
            <th className="px-3 py-2">Variant</th>
            <th className="px-3 py-2 text-right">Sent</th>
            <th className="px-3 py-2 text-right">Opens</th>
            <th className="px-3 py-2 text-right">Clicks</th>
            <th className="px-3 py-2 text-right">{metric === 'click' ? 'Click' : 'Open'} rate</th>
          </tr>
        </thead>
        <tbody>
          {['a', 'b'].map((v) => {
            const s = row(v)
            return (
              <tr key={v} className="border-b last:border-0">
                <td className="px-3 py-2 font-medium">
                  Subject {v.toUpperCase()} {winner === v && <Badge className="ml-1">Winner</Badge>}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">{s?.sent ?? 0}</td>
                <td className="px-3 py-2 text-right tabular-nums">{s?.opened ?? 0}</td>
                <td className="px-3 py-2 text-right tabular-nums">{s?.clicked ?? 0}</td>
                <td className="px-3 py-2 text-right tabular-nums">{rate(s)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {winner === '' && <p className="p-2 text-xs text-muted-foreground">Test in progress - winner pending.</p>}
    </div>
  )
}
