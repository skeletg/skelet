import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { useConfigScope, useUnsavedChangesGuard } from '@/lib/config-scope'
import type { EmailBlock, EmailStyles } from '@/features/email/blocks'
import { LibraryEmailsDocument, LibraryEmailDocument } from '@/features/emails/Emails.gql'
import { CampaignAudienceEstimateDocument, CampaignGroupsDocument, CampaignSegmentsDocument, CampaignVariantStatsDocument, CancelEmailCampaignDocument, CreateEmailCampaignDocument, EmailCampaignDocument, RenderCampaignPreviewDocument, ScheduleEmailCampaignDocument, SendCampaignTestDocument, UpdateEmailCampaignDocument } from './Campaigns.gql'
import { PluginSections } from '@/plugins/detail-sections'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { NotFound } from '@/shared/errors/NotFound'

type Draft = {
  id: string | null
  name: string
  emailId: string | null
  audienceKind: string
  audienceId: string | null
  status: string
  channelId: string
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
    ? { id: null, name: '', emailId: null, audienceKind: 'all', audienceId: null, status: 'draft', channelId: effectiveChannelId ?? '', subject: '', blocks: [], styles: {}, abEnabled: false, abSubjectB: '', abTestPercent: 20, abWinnerMetric: 'open', abTestMinutes: 240, abWinner: '' }
    : {
        id: data!.emailCampaign!.id,
        name: data!.emailCampaign!.name,
        emailId: data!.emailCampaign!.emailId ?? null,
        audienceKind: data!.emailCampaign!.audienceKind,
        audienceId: data!.emailCampaign!.audienceId ?? null,
        status: data!.emailCampaign!.status,
        channelId: data!.emailCampaign!.channelId,
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

  return <ComposerInner key={initial.id ?? 'new'} initial={initial} onCreated={(newId) => navigate(`/campaigns/${newId}`)} />
}

function ComposerInner({ initial, onCreated }: { initial: Draft; onCreated: (id: string) => void }) {
  // A new email needs emails.manage - editing a campaign or flow doesn't bring it
  const canCreateEmail = usePermissions().has('emails.manage')
  const editable = initial.status === 'draft'
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

  const baseline = useMemo(
    () =>
      JSON.stringify({
        n: initial.name, e: initial.emailId, ak: initial.audienceKind, ai: initial.audienceId,
        ab: initial.abEnabled, bs: initial.abSubjectB, ap: initial.abTestPercent, am: initial.abWinnerMetric, at: initial.abTestMinutes,
      }),
    [initial],
  )
  const current = JSON.stringify({
    n: name, e: emailId, ak: audienceKind, ai: audienceId,
    ab: abEnabled, bs: abSubjectB, ap: abTestPercent, am: abWinnerMetric, at: abTestMinutes,
  })
  const dirty = editable && current !== baseline
  useUnsavedChangesGuard(dirty, 'Unsaved campaign changes')

  const { data: emailsData } = useQuery(LibraryEmailsDocument, { variables: { channelId: initial.channelId }, skip: !initial.channelId })
  const { data: groupsData } = useQuery(CampaignGroupsDocument)
  const { data: segmentsData } = useQuery(CampaignSegmentsDocument)
  const libraryEmails = (emailsData?.libraryEmails ?? []).map((e) => ({ id: e.id, name: e.name }))
  const groups = (groupsData?.customerGroups ?? []).map((g) => ({ id: g.id, name: g.name, count: g.memberCount }))
  const segments = (segmentsData?.customerSegments.items ?? []).map((s) => ({ id: s.id, name: s.name, count: s.memberCount }))

  const audienceReady = audienceKind === 'all' || !!audienceId
  const { data: estData } = useQuery(CampaignAudienceEstimateDocument, {
    variables: { channelId: initial.channelId, audienceKind, audienceId },
    skip: !audienceReady || !initial.channelId,
    fetchPolicy: 'cache-and-network',
  })
  const estimate = estData?.campaignAudienceEstimate

  // selected email content (for the preview dialog)
  const { data: selEmail } = useQuery(LibraryEmailDocument, { variables: { id: emailId ?? '' }, skip: !emailId })

  const [createCampaign] = useMutation(CreateEmailCampaignDocument)
  const [updateCampaign] = useMutation(UpdateEmailCampaignDocument)
  const [scheduleCampaign] = useMutation(ScheduleEmailCampaignDocument)
  const [cancelCampaign] = useMutation(CancelEmailCampaignDocument)
  const [sendTest] = useMutation(SendCampaignTestDocument)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function inputVars() {
    return {
      channelId: initial.channelId,
      name: name.trim(),
      emailId,
      subject: '',
      blocks: [],
      styles: {},
      audienceKind,
      audienceId: audienceKind === 'all' ? null : audienceId,
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

  // schedule dialog
  const [scheduleOpen, setScheduleOpen] = useState(false)
  const [when, setWhen] = useState<'now' | 'later'>('now')
  const [whenAt, setWhenAt] = useState('')
  const [scheduling, setScheduling] = useState(false)
  async function onConfirmSchedule() {
    setScheduling(true)
    setError(null)
    try {
      const savedId = await save()
      if (!savedId) return
      const scheduledFor = when === 'later' && whenAt ? new Date(whenAt).toISOString() : null
      await scheduleCampaign({ variables: { id: savedId, scheduledFor } })
      setScheduleOpen(false)
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

  // preview dialog (renders the selected library email)
  const [previewOpen, setPreviewOpen] = useState(false)
  const { data: previewData, loading: previewLoading } = useQuery(RenderCampaignPreviewDocument, {
    variables: {
      channelId: initial.channelId,
      subject: selEmail?.libraryEmail?.subject ?? '',
      blocks: selEmail?.libraryEmail?.blocks ?? [],
      styles: selEmail?.libraryEmail?.styles ?? {},
    },
    skip: !previewOpen || !selEmail?.libraryEmail,
    fetchPolicy: 'no-cache',
  })

  // cancel
  const [confirmCancel, setConfirmCancel] = useState(false)
  async function onConfirmCancel() {
    if (!campaignId) return
    await cancelCampaign({ variables: { id: campaignId } })
    setConfirmCancel(false)
    onCreated(campaignId)
  }

  if (!editable) {
    return <ReadOnlyCampaign initial={initial} confirmCancel={confirmCancel} setConfirmCancel={setConfirmCancel} onConfirmCancel={onConfirmCancel} />
  }

  const title = name.trim() || (initial.id ? 'Campaign' : 'New campaign')

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={title}
        backTo="/campaigns"
        badges={<Badge variant={STATUS_VARIANT[initial.status] ?? 'outline'} className="capitalize">{initial.status}</Badge>}
        meta={
          <span className="text-xs text-muted-foreground">
            Pick the email, choose an audience, then schedule. Only customers opted in to marketing email receive it.
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" size="sm" disabled={!emailId} onClick={() => setPreviewOpen(true)}>
              Preview
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={!emailId || (!campaignId && !name.trim())} onClick={() => setTestOpen(true)}>
              Test
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={saving} onClick={onSaveClick}>
              {saving ? 'Saving…' : dirty ? 'Save draft' : 'Saved'}
            </Button>
            <Button type="button" size="sm" disabled={!name.trim() || !emailId} onClick={() => setScheduleOpen(true)}>
              Schedule / send
            </Button>
          </div>
        }
      />
      {error && (
        <Alert variant="destructive" className="mx-6 mt-4">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DetailPageBody>
        <DetailPageSection title="Campaign">
          <div className="grid max-w-md gap-1.5">
            <Label htmlFor="cname">Name</Label>
            <Input id="cname" value={name} onChange={(e) => setName(e.target.value)} placeholder="Summer sale" />
          </div>
        </DetailPageSection>

        <DetailPageSection title="Email" description="Pick a pre-built email from the library.">
          <div className="grid max-w-md gap-1.5">
            <Select value={emailId || undefined} onValueChange={setEmailId}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Pick an email…" />
              </SelectTrigger>
              <SelectContent>
                {libraryEmails.length === 0 ? (
                  <SelectItem value="__none" disabled>
                    No emails yet - create one in the library
                  </SelectItem>
                ) : (
                  libraryEmails.map((e) => (
                    <SelectItem key={e.id} value={e.id}>
                      {e.name}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
            <div className="flex gap-3 text-xs">
              {emailId && (
                <Link to={`/emails/${emailId}`} className="text-primary hover:underline" target="_blank">
                  Edit this email
                </Link>
              )}
              {canCreateEmail && (
                <Link to="/emails/new" className="text-primary hover:underline" target="_blank">
                  New email
                </Link>
              )}
            </div>
            {initial.emailId == null && initial.blocks.length > 0 && (
              <p className="text-xs text-muted-foreground">
                This campaign was built with an inline email - pick a library email above to switch.
              </p>
            )}
          </div>
        </DetailPageSection>

        <DetailPageSection title="Audience">
          <div className="grid max-w-md gap-2">
            <Select
              value={audienceKind}
              onValueChange={(k) => {
                setAudienceKind(k)
                setAudienceId(null)
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All customers</SelectItem>
                <SelectItem value="group">Customer group</SelectItem>
                <SelectItem value="segment">Segment</SelectItem>
              </SelectContent>
            </Select>
            {audienceKind === 'group' && (
              <Select value={audienceId ?? undefined} onValueChange={setAudienceId}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Pick a group…" />
                </SelectTrigger>
                <SelectContent>
                  {groups.map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.name} ({g.count})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            {audienceKind === 'segment' && (
              <Select value={audienceId ?? undefined} onValueChange={setAudienceId}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Pick a segment…" />
                </SelectTrigger>
                <SelectContent>
                  {segments.length === 0 ? (
                    <SelectItem value="__none" disabled>
                      No segments yet
                    </SelectItem>
                  ) : (
                    segments.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name} ({s.count})
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
            )}
            {estimate != null && (
              <p className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">{estimate.eligible}</span> of {estimate.total} will
                receive this - the rest aren't opted in to marketing email.
              </p>
            )}
          </div>
        </DetailPageSection>

        <DetailPageSection title="A/B subject test" description="Test two subject lines on a slice of the audience; the higher-performing subject is sent to the rest.">
          {editable ? (
            <div className="grid max-w-md gap-3">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={abEnabled} onChange={(e) => setAbEnabled(e.target.checked)} className="size-4" />
                Enable A/B subject test
              </label>
              {abEnabled && (
                <>
                  <div className="grid gap-1.5">
                    <Label htmlFor="absubb">Subject B</Label>
                    <Input id="absubb" value={abSubjectB} onChange={(e) => setAbSubjectB(e.target.value)} placeholder="Alternative subject line" />
                    <p className="text-xs text-muted-foreground">Subject A is the selected email&apos;s own subject; B replaces it for half the test slice.</p>
                  </div>
                  <div className="flex flex-wrap gap-3">
                    <div className="grid gap-1.5">
                      <Label htmlFor="abpct">Test audience %</Label>
                      <Input id="abpct" type="number" min={1} max={90} value={abTestPercent} onChange={(e) => setAbTestPercent(Number(e.target.value))} className="w-28" />
                    </div>
                    <div className="grid gap-1.5">
                      <Label htmlFor="abmin">Test window (min)</Label>
                      <Input id="abmin" type="number" min={1} value={abTestMinutes} onChange={(e) => setAbTestMinutes(Number(e.target.value))} className="w-32" />
                    </div>
                    <div className="grid gap-1.5">
                      <Label>Winner by</Label>
                      <Select value={abWinnerMetric} onValueChange={setAbWinnerMetric}>
                        <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="open">Open rate</SelectItem>
                          <SelectItem value="click">Click rate</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                </>
              )}
            </div>
          ) : initial.abEnabled ? (
            <ABResults campaignId={initial.id!} winner={initial.abWinner} metric={initial.abWinnerMetric} />
          ) : (
            <p className="text-sm text-muted-foreground">No A/B test on this campaign.</p>
          )}
        </DetailPageSection>
      </DetailPageBody>

      {/* Preview dialog */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Preview</DialogTitle>
            <DialogDescription>The selected email, rendered with sample data.</DialogDescription>
          </DialogHeader>
          <div className="overflow-hidden rounded-md border bg-white">
            <div className="border-b bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
              <span className="font-medium text-foreground">Subject:</span>{' '}
              {previewLoading ? '…' : previewData?.renderCampaignPreview.subject || '-'}
            </div>
            <iframe title="Email preview" sandbox="" className="h-[440px] w-full bg-white" srcDoc={previewData?.renderCampaignPreview.html ?? ''} />
          </div>
        </DialogContent>
      </Dialog>

      {/* Schedule dialog */}
      <Dialog open={scheduleOpen} onOpenChange={(o) => !scheduling && setScheduleOpen(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Schedule campaign</DialogTitle>
            <DialogDescription>
              {estimate
                ? `${estimate.eligible} of ${estimate.total} customers will receive this (the rest aren't opted in to marketing email).`
                : 'Choose when to send.'}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <label className="flex items-center gap-2 text-sm">
              <input type="radio" checked={when === 'now'} onChange={() => setWhen('now')} /> Send now
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="radio" checked={when === 'later'} onChange={() => setWhen('later')} /> Schedule for later
            </label>
            {when === 'later' && <Input type="datetime-local" value={whenAt} onChange={(e) => setWhenAt(e.target.value)} />}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setScheduleOpen(false)} disabled={scheduling}>
              Cancel
            </Button>
            <Button onClick={onConfirmSchedule} disabled={scheduling || (when === 'later' && !whenAt)}>
              {scheduling ? 'Scheduling…' : when === 'now' ? 'Send now' : 'Schedule'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Test-send dialog */}
      <Dialog open={testOpen} onOpenChange={setTestOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Send a test</DialogTitle>
            <DialogDescription>Saves the draft, then sends the selected email (sample data) to one address.</DialogDescription>
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

function ReadOnlyCampaign({
  initial,
  confirmCancel,
  setConfirmCancel,
  onConfirmCancel,
}: {
  initial: Draft
  confirmCancel: boolean
  setConfirmCancel: (o: boolean) => void
  onConfirmCancel: () => void
}) {
  // Prefer the referenced library email's content; fall back to the campaign's own inline body
  const { data: selEmail } = useQuery(LibraryEmailDocument, { variables: { id: initial.emailId ?? '' }, skip: !initial.emailId })
  const content = initial.emailId
    ? { subject: selEmail?.libraryEmail?.subject ?? '', blocks: selEmail?.libraryEmail?.blocks ?? [], styles: selEmail?.libraryEmail?.styles ?? {} }
    : { subject: initial.subject, blocks: initial.blocks, styles: initial.styles }
  const { data } = useQuery(RenderCampaignPreviewDocument, {
    variables: { channelId: initial.channelId, subject: content.subject, blocks: content.blocks, styles: content.styles },
    skip: initial.emailId != null && !selEmail?.libraryEmail,
    fetchPolicy: 'no-cache',
  })
  const canCancel = initial.status === 'scheduled' || initial.status === 'sending'
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
      <DetailPageBody>
        <DetailPageSection title="Delivery">
          <CampaignStats initial={initial} />
        </DetailPageSection>
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

      <AlertDialog open={confirmCancel} onOpenChange={setConfirmCancel}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel this send?</AlertDialogTitle>
            <AlertDialogDescription>
              The campaign stops sending. Recipients already emailed have been sent; the rest will not be.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep</AlertDialogCancel>
            <AlertDialogAction onClick={onConfirmCancel}>Cancel send</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

function CampaignStats({ initial }: { initial: Draft }) {
  const { data } = useQuery(EmailCampaignDocument, { variables: { id: initial.id ?? '' }, skip: !initial.id, pollInterval: 5000 })
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
