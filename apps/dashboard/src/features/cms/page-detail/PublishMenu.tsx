import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { toast } from 'sonner'
import {
  CalendarClockIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleOffIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  XIcon,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import {
  ApproveCmsPageDocument,
  ClaimCmsPageDocument,
  CmsPageDocument,
  CmsPagesDocument,
  PublishCmsPageDocument,
  RebuildCmsPageDocument,
  RejectCmsPageDocument,
  ReopenCmsPageDocument,
  ScheduleCmsPageDocument,
  SubmitCmsPageForReviewDocument,
  UnassignCmsPageDocument,
  UnpublishCmsPageDocument,
} from '../Cms.gql'
import { publishStatus, workflowLabel, type PublishPage } from './shared'

export function PublishStatus({ page }: { page: PublishPage }) {
  const s = publishStatus(page)
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span className={cn('size-2 rounded-full', s.dot)} aria-hidden />
      {s.label}
    </span>
  )
}

// Every lifecycle action in one menu - publishing, the schedule, review and the cache rebuild are transitions
// rather than field edits, so they apply at once instead of waiting for the page Save
export function PublishMenu({
  page,
  canApprove,
  refetchVariables,
}: {
  page: PublishPage
  canApprove: boolean
  refetchVariables: { id: string }
}) {
  const refetchQueries = [{ query: CmsPageDocument, variables: refetchVariables }, CmsPagesDocument]
  const [publish] = useMutation(PublishCmsPageDocument, { refetchQueries })
  const [unpublish] = useMutation(UnpublishCmsPageDocument, { refetchQueries })
  const [schedule] = useMutation(ScheduleCmsPageDocument, { refetchQueries })
  const [rebuild] = useMutation(RebuildCmsPageDocument)
  const [submit] = useMutation(SubmitCmsPageForReviewDocument, { refetchQueries })
  const [approve] = useMutation(ApproveCmsPageDocument, { refetchQueries })
  const [reject] = useMutation(RejectCmsPageDocument, { refetchQueries })
  const [reopen] = useMutation(ReopenCmsPageDocument, { refetchQueries })
  const [claim] = useMutation(ClaimCmsPageDocument, { refetchQueries })
  const [unassign] = useMutation(UnassignCmsPageDocument, { refetchQueries })
  const [busy, setBusy] = useState(false)
  const [dialog, setDialog] = useState<'schedule' | 'reject' | null>(null)
  const [when, setWhen] = useState('')
  const [note, setNote] = useState('')

  const id = page.id
  const state = page.workflowState
  const scheduled = !page.isPublished && !!page.scheduledAt

  // Resolves false on failure so a dialog can stay open with its input
  async function run(fn: () => Promise<unknown>, done?: string): Promise<boolean> {
    setBusy(true)
    try {
      await fn()
      if (done) toast.success(done)
      return true
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Something went wrong.')
      return false
    } finally {
      setBusy(false)
    }
  }

  const status = publishStatus(page)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" size="sm" disabled={busy} aria-label={`Publishing: ${status.label}`}>
            <span className={cn('size-2 rounded-full', status.dot)} aria-hidden />
            {status.label}
            <ChevronDownIcon className="size-4 text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          {page.isPublished ? (
            <DropdownMenuItem onSelect={() => void run(() => unpublish({ variables: { id } }), 'Page unpublished.')}>
              <CircleOffIcon /> Unpublish
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onSelect={() => void run(() => publish({ variables: { id } }), 'Page published.')}>
              <CheckCircle2Icon /> Publish now
            </DropdownMenuItem>
          )}
          {scheduled ? (
            <DropdownMenuItem onSelect={() => void run(() => schedule({ variables: { id, scheduledAt: null } }), 'Schedule cancelled.')}>
              <XIcon /> Cancel the scheduled publish
            </DropdownMenuItem>
          ) : !page.isPublished ? (
            <DropdownMenuItem onSelect={() => setDialog('schedule')}>
              <CalendarClockIcon /> Schedule publish…
            </DropdownMenuItem>
          ) : null}

          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
            Review · {workflowLabel(state)}
            {page.assignedTo ? ', claimed' : ''}
          </DropdownMenuLabel>
          {state === 'DRAFT' && (
            <DropdownMenuItem onSelect={() => void run(() => submit({ variables: { id } }), 'Sent for review.')}>Submit for review</DropdownMenuItem>
          )}
          {state === 'IN_REVIEW' &&
            (canApprove ? (
              <DropdownMenuItem onSelect={() => void run(() => approve({ variables: { id } }), 'Page approved.')}>Approve</DropdownMenuItem>
            ) : (
              <DropdownMenuItem disabled>Awaiting an approver</DropdownMenuItem>
            ))}
          {(state === 'IN_REVIEW' || state === 'APPROVED') && (
            <DropdownMenuItem onSelect={() => setDialog('reject')}>Reject…</DropdownMenuItem>
          )}
          {state !== 'DRAFT' && (
            <DropdownMenuItem onSelect={() => void run(() => reopen({ variables: { id } }), 'Back to draft.')}>
              <RotateCcwIcon /> Reopen to draft
            </DropdownMenuItem>
          )}
          {page.assignedTo ? (
            <DropdownMenuItem onSelect={() => void run(() => unassign({ variables: { id } }))}>Unassign</DropdownMenuItem>
          ) : (
            <DropdownMenuItem onSelect={() => void run(() => claim({ variables: { id } }))}>Claim for review</DropdownMenuItem>
          )}

          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => void run(() => rebuild({ variables: { id } }), 'Page rebuilt in the storefront cache.')}>
            <RefreshCwIcon /> Rebuild storefront cache
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={dialog === 'schedule'} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Schedule publish</DialogTitle>
            <DialogDescription>The page publishes itself at this time.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="cms-schedule-at">Publish at</Label>
            <Input id="cms-schedule-at" type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialog(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              disabled={!when || busy}
              onClick={async () => {
                if (await run(() => schedule({ variables: { id, scheduledAt: new Date(when).toISOString() } }), 'Publish scheduled.')) {
                  setDialog(null)
                  setWhen('')
                }
              }}
            >
              Schedule
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={dialog === 'reject'} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Reject and return to draft</DialogTitle>
            <DialogDescription>The reason is saved as a comment on the page.</DialogDescription>
          </DialogHeader>
          <Textarea value={note} onChange={(e) => setNote(e.target.value)} placeholder="Reason (optional)" rows={3} aria-label="Reason" />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialog(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={busy}
              onClick={async () => {
                if (await run(() => reject({ variables: { id, note: note.trim() || null } }), 'Returned to draft.')) {
                  setDialog(null)
                  setNote('')
                }
              }}
            >
              Reject
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
