import { useId, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import type { Node } from '@xyflow/react'
import { CircleAlertIcon, TriangleAlertIcon } from 'lucide-react'
import { PanelTitle } from '@/components/layout/workbench'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { EmailBlock, EmailStyles } from '@/features/email/blocks'
import { LibraryEmailDocument } from '@/features/emails/Emails.gql'
import { PreviewDialog } from '@/features/emails/PreviewDialog'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConditionEditor } from './ConditionEditor'
import { StepIcon } from './FlowNodes'
import type { Check } from './flowGraph'
import { NODE_HINTS, NODE_LABELS, TRIGGER_HELP, TRIGGER_LABELS, defaultCondition, type FlowCondition, type FlowNodeData } from './flowTypes'

export function CheckMark({ level }: { level: 'error' | 'warning' }) {
  return level === 'error' ? <CircleAlertIcon className="mt-px size-3.5 shrink-0 text-destructive" /> : <TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-warning" />
}

export function StepSettings({
  selected,
  found,
  channelId,
  triggerType,
  onTrigger,
  triggers,
  patch,
  onDelete,
  onDuplicate,
  segments,
  libraryEmails,
  enrollmentFilter,
  onEnrollmentFilter,
  transactional,
  onTransactional,
}: {
  selected: Node
  found: Check[]
  channelId: string
  triggerType: string
  onTrigger: (t: string) => void
  triggers: Array<{ type: string; transactional: boolean }>
  patch: (p: Partial<FlowNodeData>) => void
  onDelete: () => void
  onDuplicate: () => void
  segments: Array<{ id: string; name: string }>
  libraryEmails: Array<{ id: string; name: string; subject: string; preheader: string }>
  enrollmentFilter: FlowCondition | null
  onEnrollmentFilter: (c: FlowCondition | null) => void
  transactional: boolean
  onTransactional: (v: boolean) => void
}) {
  const fid = useId()
  // A new email needs emails.manage - editing a campaign or flow doesn't bring it
  const canCreateEmail = usePermissions().has('emails.manage')
  const [previewOpen, setPreviewOpen] = useState(false)
  const type = selected.type ?? ''
  const d = selected.data as FlowNodeData
  const email = libraryEmails.find((e) => e.id === d.emailId)
  const canBeTransactional = triggers.find((t) => t.type === triggerType)?.transactional ?? false
  const { data: opened, error: openError } = useQuery(LibraryEmailDocument, { variables: { id: d.emailId ?? '' }, skip: !previewOpen || !d.emailId })
  const title = (
    <>
      <PanelTitle icon={<StepIcon type={type} className="size-4" />} title={NODE_LABELS[type] ?? 'Step'} hint={NODE_HINTS[type]} />
      {found.length > 0 ? (
        <ul className="grid gap-1">
          {found.map((c, i) => (
            <li key={i} className="flex items-start gap-2 text-xs">
              <CheckMark level={c.level} />
              <span className={c.level === 'error' ? 'text-destructive' : 'text-warning'}>{c.text}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </>
  )

  if (type === 'trigger') {
    return (
      <>
        {title}
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-when-this-happens`}>When this happens</Label>
          <Select value={triggerType} onValueChange={onTrigger}>
            <SelectTrigger id={`${fid}-when-this-happens`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {triggers.map((t) => (
                <SelectItem key={t.type} value={t.type}>
                  {TRIGGER_LABELS[t.type] ?? t.type}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{TRIGGER_HELP[triggerType] ?? 'The customer enters the flow when this event fires.'}</p>
        </div>
        <div className="grid gap-2">
          <label className="flex items-center gap-2 text-sm font-medium">
            <Switch
              checked={enrollmentFilter != null}
              onCheckedChange={(on) => onEnrollmentFilter(on ? defaultCondition('lifecycle') : null)}
            />
            Only enroll when…
          </label>
          {enrollmentFilter != null && (
            <>
              <ConditionEditor value={enrollmentFilter} onChange={onEnrollmentFilter} segments={segments} />
              <p className="text-xs text-muted-foreground">
                Customers who don’t match are skipped at enrollment (e.g. only marketing-opted-in, or only active buyers).
              </p>
            </>
          )}
        </div>
        <div className="grid gap-2">
          <label className="flex items-center gap-2 text-sm font-medium">
            <Switch checked={transactional} disabled={!canBeTransactional} onCheckedChange={onTransactional} />
            Transactional
          </label>
          <p className="text-xs text-muted-foreground">
            Transactional flows (order receipts, account emails) send regardless of marketing consent, and their links aren’t click-wrapped.
            {canBeTransactional ? ' Recommended for this trigger.' : ' Only a flow that starts on an order or account moment can be one.'}
          </p>
        </div>
      </>
    )
  }

  return (
    <>
      {title}
      {type === 'send_email' && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-email-to-send`}>Email to send</Label>
          <Select
            value={d.emailId || undefined}
            onValueChange={(v) => {
              setPreviewOpen(false)
              patch({ emailId: v })
            }}
          >
            <SelectTrigger id={`${fid}-email-to-send`} className="w-full">
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
          {email ? (
            <div className="grid gap-0.5 rounded-md border bg-muted/30 px-2.5 py-2 text-xs">
              <p className="truncate font-medium text-foreground">{email.subject || '(no subject)'}</p>
              {email.preheader ? <p className="truncate text-muted-foreground">{email.preheader}</p> : null}
            </div>
          ) : null}
          <div className="flex flex-wrap items-center gap-3 text-xs">
            {email && (
              <button type="button" className="text-primary hover:underline" onClick={() => setPreviewOpen(true)}>
                Preview
              </button>
            )}
            {d.emailId && (
              <Link to={`/emails/${d.emailId}`} className="text-primary hover:underline" target="_blank">
                Edit this email
              </Link>
            )}
            {canCreateEmail && (
              <Link to="/emails/new" className="text-primary hover:underline" target="_blank">
                New email
              </Link>
            )}
          </div>
          {previewOpen && openError ? <p className="text-xs text-destructive">The email couldn't be opened: {openError.message}</p> : null}
          <p className="text-xs text-muted-foreground">Build emails in Marketing → Emails, then pick one here.</p>
          {opened?.libraryEmail ? (
            <PreviewDialog
              open={previewOpen}
              onOpenChange={setPreviewOpen}
              channelId={channelId}
              subject={opened.libraryEmail.subject}
              preheader={opened.libraryEmail.preheader}
              blocks={(opened.libraryEmail.blocks as EmailBlock[] | null) ?? []}
              styles={(opened.libraryEmail.styles as EmailStyles | null) ?? {}}
            />
          ) : null}
        </div>
      )}

      {type === 'send_sms' && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-message`}>Message</Label>
          <textarea id={`${fid}-message`}
            value={d.smsBody ?? ''}
            onChange={(e) => patch({ smsBody: e.target.value })}
            rows={4}
            maxLength={480}
            placeholder="Hi {{customer.firstName}}! …"
            className="w-full rounded-md border bg-background px-2 py-1.5 text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Plain text. Merge tags: <code className="text-[11px]">{'{{customer.firstName}}'}</code>,{' '}
            <code className="text-[11px]">{'{{cart.recoveryUrl}}'}</code>,{' '}
            <code className="text-[11px]">{'{{product.url}}'}</code>. Sent only to customers opted in to marketing
            SMS with a phone on file.
          </p>
        </div>
      )}

      {type === 'wait' && <WaitConfig seconds={d.delaySeconds ?? 86400} onChange={(s) => patch({ delaySeconds: s })} />}

      {type === 'split' && (
        <div className="grid gap-1.5">
          <Label>Branch when…</Label>
          <ConditionEditor
            value={d.condition ?? { kind: 'segment', segmentId: '' }}
            onChange={(c) => patch({ condition: c })}
            segments={segments}
          />
          <p className="text-xs text-muted-foreground">Customers who match take the “yes” branch, everyone else “no”.</p>
        </div>
      )}

      {type === 'add_segment' && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-add-to-segment`}>Add to segment</Label>
          <Select value={d.segmentId || undefined} onValueChange={(v) => patch({ segmentId: v })}>
            <SelectTrigger id={`${fid}-add-to-segment`} className="w-full">
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
                    {s.name}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
        </div>
      )}

      {type === 'webhook' && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-post-url`}>POST URL</Label>
          <Input id={`${fid}-post-url`} value={d.url ?? ''} onChange={(e) => patch({ url: e.target.value })} placeholder="https://example.com/hook" />
          <p className="text-xs text-muted-foreground">Fire-and-forget. Receives {`{ flowId, customerId, context }`}. Private hosts are blocked.</p>
        </div>
      )}

      <div className="flex gap-2">
        <Button type="button" variant="outline" size="sm" className="flex-1" onClick={onDuplicate}>
          Duplicate
        </Button>
        <Button type="button" variant="outline" size="sm" className="flex-1 text-destructive" onClick={onDelete}>
          Delete step
        </Button>
      </div>
    </>
  )
}

function WaitConfig({ seconds, onChange }: { seconds: number; onChange: (s: number) => void }) {
  const fid = useId()
  const unit = seconds % 86400 === 0 ? 'days' : seconds % 3600 === 0 ? 'hours' : 'minutes'
  const amount = unit === 'days' ? seconds / 86400 : unit === 'hours' ? seconds / 3600 : Math.round(seconds / 60)
  const toSeconds = (amt: number, u: string) => (u === 'days' ? amt * 86400 : u === 'hours' ? amt * 3600 : amt * 60)
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={`${fid}-wait-for`} className="text-xs">Wait for</Label>
      <div className="flex gap-2">
        <Input id={`${fid}-wait-for`}
          type="number"
          min={1}
          value={amount}
          onChange={(e) => onChange(toSeconds(Math.max(1, Number(e.target.value || 1)), unit))}
          className="w-24"
        />
        <Select value={unit} onValueChange={(u) => onChange(toSeconds(amount, u))}>
          <SelectTrigger className="flex-1">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="minutes">minutes</SelectItem>
            <SelectItem value="hours">hours</SelectItem>
            <SelectItem value="days">days</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
