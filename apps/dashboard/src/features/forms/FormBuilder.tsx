import { useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { useFeatures } from '@/shared/features/useFeatures'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { useConfigScope } from '@/lib/config-scope'
import { CreateSignupFormDocument, DeleteSignupFormDocument, FormSegmentsDocument, SignupFormDocument, SignupFormSubmissionsDocument, UpdateSignupFormDocument, type SignupFormFieldsFragment } from './Forms.gql'
import { usePermissions } from '@/shared/auth/usePermissions'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  active: 'default',
}

const SUBMISSIONS_LIMIT = 25

// slugify - lowercase, hyphenate, strip non-url-safe chars
function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

type Loaded = {
  id: string | null
  channelId: string
  slug: string
  name: string
  status: string
  type: string
  heading: string
  body: string
  buttonLabel: string
  successMessage: string
  collectName: boolean
  consentKind: string
  targetSegmentId: string | null
  triggerFlow: boolean
  trigger: string
  delaySeconds: number
  scrollPercent: number
  frequencyDays: number
}

function fromForm(f: SignupFormFieldsFragment): Loaded {
  return {
    id: f.id,
    channelId: f.channelId,
    slug: f.slug,
    name: f.name,
    status: f.status,
    type: f.type,
    heading: f.heading,
    body: f.body,
    buttonLabel: f.buttonLabel,
    successMessage: f.successMessage,
    collectName: f.collectName,
    consentKind: f.consentKind,
    targetSegmentId: f.targetSegmentId ?? null,
    triggerFlow: f.triggerFlow,
    trigger: f.display.trigger,
    delaySeconds: f.display.delaySeconds,
    scrollPercent: f.display.scrollPercent,
    frequencyDays: f.display.frequencyDays,
  }
}

export function FormBuilder() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !id || id === 'new'

  const { data, loading } = useQuery(SignupFormDocument, {
    variables: { id: id ?? '' },
    skip: isNew,
    fetchPolicy: 'cache-and-network',
  })

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New form" backTo="/forms" />
        <DetailPageBody>
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>Select a channel in the header before building a form.</AlertDescription>
            </Alert>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )
  }
  if (!isNew && !data?.signupForm) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Form" backTo="/forms" />
        <DetailPageBody>{loading ? <LoadingState /> : null}</DetailPageBody>
      </DetailPageLayout>
    )
  }

  const initial: Loaded = isNew
    ? {
        id: null,
        channelId: effectiveChannelId ?? '',
        slug: '',
        name: '',
        status: 'draft',
        type: 'popup',
        heading: '',
        body: '',
        buttonLabel: 'Subscribe',
        successMessage: 'Thanks for signing up!',
        collectName: false,
        consentKind: 'marketing_email',
        targetSegmentId: null,
        triggerFlow: false,
        trigger: 'immediate',
        delaySeconds: 5,
        scrollPercent: 50,
        frequencyDays: 7,
      }
    : fromForm(data!.signupForm!)

  return <Builder key={initial.id ?? 'new'} initial={initial} onCreated={() => void duringCommit(() => navigate('/forms'))} onDeleted={() => void duringCommit(() => navigate('/forms'))} />
}

function Builder({
  initial,
  onCreated,
  onDeleted,
}: {
  initial: Loaded
  onCreated: (id: string) => void
  onDeleted: () => void
}) {
  const isNew = initial.id == null
  // Saving and deleting a form need forms.manage - a viewer gets the form read-only, no Save or Delete
  const canEdit = usePermissions().has('forms.manage')
  const draft = useDraft({
    name: initial.name,
    slug: initial.slug,
    status: initial.status,
    type: initial.type,
    heading: initial.heading,
    body: initial.body,
    buttonLabel: initial.buttonLabel,
    successMessage: initial.successMessage,
    collectName: initial.collectName,
    triggerFlow: initial.triggerFlow,
    targetSegmentId: initial.targetSegmentId as string | null,
    trigger: initial.trigger,
    delaySeconds: initial.delaySeconds,
    scrollPercent: initial.scrollPercent,
    frequencyDays: initial.frequencyDays,
  })
  const { name, slug, status, type, heading, body, buttonLabel, successMessage, collectName, triggerFlow, targetSegmentId, trigger, delaySeconds, scrollPercent, frequencyDays } = draft.values
  const { setName, setSlug, setStatus, setType, setHeading, setBody, setButtonLabel, setSuccessMessage, setCollectName, setTriggerFlow, setTargetSegmentId, setTrigger, setDelaySeconds, setScrollPercent, setFrequencyDays } = draft.setters

  const [formId, setFormId] = useState<string | null>(initial.id)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  // On create, auto-suggest the slug from the name until the operator edits it
  const slugTouched = useRef(!isNew)

  const { enabled: featureEnabled, known: featuresKnown } = useFeatures()
  const signalsOn = featureEnabled('signals') // segments are the suite's
  const { data: segData } = useQuery(FormSegmentsDocument, { skip: !signalsOn })
  const segments = (segData?.customerSegments.items ?? []).map((s) => ({ id: s.id, name: s.name }))

  const { data: subsData } = useQuery(SignupFormSubmissionsDocument, {
    variables: { formId: formId ?? '', limit: SUBMISSIONS_LIMIT },
    skip: !formId,
    fetchPolicy: 'cache-and-network',
  })
  const submissions = subsData?.signupFormSubmissions ?? []

  const [createForm] = useMutation(CreateSignupFormDocument)
  const [updateForm] = useMutation(UpdateSignupFormDocument)
  const [deleteForm] = useMutation(DeleteSignupFormDocument)

  function onNameChange(v: string) {
    setName(v)
    if (!slugTouched.current) setSlug(slugify(v))
  }

  function onSlugChange(v: string) {
    slugTouched.current = true
    setSlug(v)
  }

  function inputVars() {
    return {
      channelId: initial.channelId,
      slug: slug.trim() || slugify(name) || 'form',
      name: name.trim() || 'Untitled form',
      status,
      type,
      heading: heading.trim(),
      body: body.trim(),
      buttonLabel: buttonLabel.trim(),
      successMessage: successMessage.trim(),
      collectName,
      consentKind: initial.consentKind,
      targetSegmentId,
      triggerFlow,
      display: {
        trigger,
        delaySeconds,
        scrollPercent,
        frequencyDays,
      },
    }
  }

  async function save(): Promise<string | null> {
    setError(null)
    setSaving(true)
    try {
      if (formId) {
        await updateForm({ variables: { id: formId, input: inputVars() } })
        draft.saved()
        return formId
      }
      const res = await createForm({ variables: { input: inputVars() } })
      const nid = res.data?.createSignupForm?.id ?? null
      if (nid) setFormId(nid)
      return nid
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
      return null
    } finally {
      setSaving(false)
    }
  }

  async function onSaveClick() {
    const nid = await save()
    if (nid && isNew) onCreated(nid)
  }

  async function onConfirmDelete() {
    if (!formId) return
    try {
      await deleteForm({ variables: { id: formId } })
      setConfirmDelete(false)
      onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed.')
      setConfirmDelete(false)
    }
  }

  const title = name.trim() || (isNew ? 'New form' : 'Form')

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={title}
        backTo="/forms"
        badges={<Badge variant={STATUS_VARIANT[status] ?? 'outline'} className="capitalize">{status}</Badge>}
        meta={
          <span className="text-xs text-muted-foreground">
            Capture a visitor&apos;s email with a popup, flyout or embedded form - then optionally kick off a welcome flow.
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            {!isNew && canEdit && (
              <Button type="button" variant="outline" size="sm" className="text-destructive" onClick={() => setConfirmDelete(true)}>
                Delete
              </Button>
            )}
            {canEdit && (
              <Button type="button" size="sm" disabled={saving} onClick={onSaveClick}>
                {saving ? 'Saving…' : 'Save'}
              </Button>
            )}
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
        <fieldset disabled={!canEdit} className="contents">
          <DetailPageSection title="Basics">
            <div className="grid max-w-md gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="fname">Name</Label>
                <Input id="fname" value={name} onChange={(e) => onNameChange(e.target.value)} placeholder="Newsletter popup" />
                <p className="text-xs text-muted-foreground">Internal name - not shown to visitors.</p>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="fslug">Slug</Label>
                <Input id="fslug" value={slug} onChange={(e) => onSlugChange(e.target.value)} placeholder="newsletter-popup" />
                <p className="text-xs text-muted-foreground">Used to reference this form on the storefront.</p>
              </div>
              <div className="grid gap-1.5">
                <Label>Status</Label>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="draft">Draft</SelectItem>
                    <SelectItem value="active">Active</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>Type</Label>
                <Select value={type} onValueChange={setType}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="popup">Popup</SelectItem>
                    <SelectItem value="inline">Inline (embedded)</SelectItem>
                    <SelectItem value="flyout">Flyout</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection title="Content" description="What visitors see.">
            <div className="grid max-w-md gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="fheading">Heading</Label>
                <Input id="fheading" value={heading} onChange={(e) => setHeading(e.target.value)} placeholder="Join our newsletter" />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="fbody">Body</Label>
                <Textarea id="fbody" value={body} onChange={(e) => setBody(e.target.value)} rows={3} placeholder="Get 10% off your first order and the latest updates." />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="fbtn">Button label</Label>
                <Input id="fbtn" value={buttonLabel} onChange={(e) => setButtonLabel(e.target.value)} placeholder="Subscribe" />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="fsuccess">Success message</Label>
                <Input id="fsuccess" value={successMessage} onChange={(e) => setSuccessMessage(e.target.value)} placeholder="Thanks for signing up!" />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <Switch checked={collectName} onCheckedChange={setCollectName} />
                Also collect the visitor&apos;s name
              </label>
            </div>
          </DetailPageSection>

          <DetailPageSection title="After signup" description="What happens once someone subscribes.">
            <div className="grid max-w-md gap-4">
              <div className="grid gap-1.5">
                <Label>Add to segment</Label>
                <Select value={targetSegmentId ?? '__none'} onValueChange={(v) => setTargetSegmentId(v === '__none' ? null : v)}>
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="No segment" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none">No segment</SelectItem>
                    {segments.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {signalsOn || !featuresKnown ? 'Optionally drop new subscribers into a segment.' : 'Segments need Signals, which is switched off.'}
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <Switch checked={triggerFlow} onCheckedChange={setTriggerFlow} />
                Trigger a welcome flow
              </label>
            </div>
          </DetailPageSection>

          <DetailPageSection title="Display rules" description="When and how often the form appears.">
            <div className="grid max-w-md gap-4">
              <div className="grid gap-1.5">
                <Label>Trigger</Label>
                <Select value={trigger} onValueChange={setTrigger}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="immediate">Immediately</SelectItem>
                    <SelectItem value="delay">After a delay</SelectItem>
                    <SelectItem value="scroll">On scroll</SelectItem>
                    <SelectItem value="exit_intent">On exit intent</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {trigger === 'delay' && (
                <div className="grid gap-1.5">
                  <Label htmlFor="fdelay">Delay (seconds)</Label>
                  <Input
                    id="fdelay"
                    type="number"
                    min={0}
                    value={delaySeconds}
                    onChange={(e) => setDelaySeconds(Math.max(0, Number(e.target.value) || 0))}
                    className="w-32"
                  />
                </div>
              )}
              {trigger === 'scroll' && (
                <div className="grid gap-1.5">
                  <Label htmlFor="fscroll">Scroll depth (%)</Label>
                  <Input
                    id="fscroll"
                    type="number"
                    min={0}
                    max={100}
                    value={scrollPercent}
                    onChange={(e) => setScrollPercent(Math.min(100, Math.max(0, Number(e.target.value) || 0)))}
                    className="w-32"
                  />
                </div>
              )}
              <div className="grid gap-1.5">
                <Label htmlFor="ffreq">Don&apos;t re-show for (days)</Label>
                <Input
                  id="ffreq"
                  type="number"
                  min={0}
                  value={frequencyDays}
                  onChange={(e) => setFrequencyDays(Math.max(0, Number(e.target.value) || 0))}
                  className="w-32"
                />
                <p className="text-xs text-muted-foreground">After a visitor dismisses or submits, wait this many days before showing it again.</p>
              </div>
            </div>
          </DetailPageSection>

          {!isNew && (
            <DetailPageSection title="Recent signups" description={`Latest ${SUBMISSIONS_LIMIT} - newest first.`}>
              {submissions.length === 0 ? (
                <p className="text-sm text-muted-foreground">No signups yet.</p>
              ) : (
                <div className="overflow-x-auto rounded-lg border">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                        <th className="px-3 py-2">Email</th>
                        <th className="px-3 py-2 text-right">Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      {submissions.map((s, i) => (
                        <tr key={`${s.email}-${i}`} className="border-b last:border-0">
                          <td className="px-3 py-2">{s.email}</td>
                          <td className="px-3 py-2 text-right text-muted-foreground">{fmtDate(s.createdAt)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </DetailPageSection>
          )}
        </fieldset>
      </DetailPageBody>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this form?</AlertDialogTitle>
            <AlertDialogDescription>
              The form stops appearing on the storefront. Existing signups are kept. This can&apos;t be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep</AlertDialogCancel>
            <AlertDialogAction onClick={onConfirmDelete}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

function fmtDate(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}
