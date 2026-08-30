import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckCircle2Icon, RotateCcwIcon, XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  CmsPageDocument,
  CmsPageCommentsDocument,
  AddCmsPageCommentDocument,
  ResolveCmsPageCommentDocument,
  DeleteCmsPageCommentDocument,
  SubmitCmsPageForReviewDocument,
  ApproveCmsPageDocument,
  RejectCmsPageDocument,
  ReopenCmsPageDocument,
  ClaimCmsPageDocument,
  UnassignCmsPageDocument,
} from '../Cms.gql'
import { workflowLabel } from './shared'

// CmsPageWorkflowSection drives the editorial lifecycle: submit → approve →
// (publish), with reject/reopen and claim/unassign - Approve is gated on
// cms.approve so an editor can't approve their own work - Actions apply
// immediately (they're lifecycle transitions, not staged field edits)
export function CmsPageWorkflowSection({
  pageId,
  state,
  assignedTo,
  canManage,
  canApprove,
  refetchVariables,
}: {
  pageId: string
  state: string
  assignedTo: string | null
  canManage: boolean
  canApprove: boolean
  refetchVariables: { id: string }
}) {
  const refetch = [{ query: CmsPageDocument, variables: refetchVariables }]
  const [submit] = useMutation(SubmitCmsPageForReviewDocument, { refetchQueries: refetch })
  const [approve] = useMutation(ApproveCmsPageDocument, { refetchQueries: refetch })
  const [reject] = useMutation(RejectCmsPageDocument, { refetchQueries: refetch })
  const [reopen] = useMutation(ReopenCmsPageDocument, { refetchQueries: refetch })
  const [claim] = useMutation(ClaimCmsPageDocument, { refetchQueries: refetch })
  const [unassign] = useMutation(UnassignCmsPageDocument, { refetchQueries: refetch })
  const [busy, setBusy] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [note, setNote] = useState('')

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await fn()
    } finally {
      setBusy(false)
    }
  }

  if (!canManage) {
    return (
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">State:</span>
        <Badge variant="secondary">{workflowLabel(state)}</Badge>
      </div>
    )
  }

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="text-muted-foreground">State:</span>
        <Badge variant="secondary">{workflowLabel(state)}</Badge>
        {assignedTo && <span className="text-xs text-muted-foreground">· assigned for review</span>}
      </div>
      <div className="flex flex-wrap gap-2">
        {state === 'DRAFT' && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => run(() => submit({ variables: { id: pageId } }))}>
            Submit for review
          </Button>
        )}
        {state === 'IN_REVIEW' &&
          (canApprove ? (
            <Button size="sm" disabled={busy} onClick={() => run(() => approve({ variables: { id: pageId } }))}>
              <CheckCircle2Icon className="size-4" /> Approve
            </Button>
          ) : (
            <span className="inline-flex items-center text-xs text-muted-foreground">Awaiting an approver</span>
          ))}
        {(state === 'IN_REVIEW' || state === 'APPROVED') && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => setRejecting((v) => !v)}>
            Reject
          </Button>
        )}
        {state !== 'DRAFT' && (
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => run(() => reopen({ variables: { id: pageId } }))}>
            <RotateCcwIcon className="size-4" /> Reopen to draft
          </Button>
        )}
        {assignedTo ? (
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => run(() => unassign({ variables: { id: pageId } }))}>
            Unassign
          </Button>
        ) : (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => run(() => claim({ variables: { id: pageId } }))}>
            Claim for review
          </Button>
        )}
      </div>
      {rejecting && (
        <div className="grid gap-2 rounded-md border p-3">
          <Textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Reason for rejection (saved as a comment, optional)…"
            rows={2}
          />
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="destructive"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  await reject({ variables: { id: pageId, note: note.trim() || null } })
                  setNote('')
                  setRejecting(false)
                })
              }
            >
              Reject &amp; return to draft
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setRejecting(false)}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

// CmsPageCommentsSection lists + posts collaboration comments - Comments are not
// part of the staged page save - they write immediately
export function CmsPageCommentsSection({ pageId, canManage }: { pageId: string; canManage: boolean }) {
  const refetch = [{ query: CmsPageCommentsDocument, variables: { pageId } }]
  const { data, loading } = useQuery(CmsPageCommentsDocument, { variables: { pageId } })
  const [add, { loading: adding }] = useMutation(AddCmsPageCommentDocument, { refetchQueries: refetch })
  const [resolve] = useMutation(ResolveCmsPageCommentDocument, { refetchQueries: refetch })
  const [del] = useMutation(DeleteCmsPageCommentDocument, { refetchQueries: refetch })
  const [body, setBody] = useState('')
  const comments = data?.cmsPageComments ?? []

  return (
    <div className="grid gap-3">
      {loading && comments.length === 0 ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : comments.length === 0 ? (
        <p className="text-sm text-muted-foreground">No comments yet.</p>
      ) : (
        <ul className="grid gap-2">
          {comments.map((c) => (
            <li key={c.id} className="rounded-md border p-2.5">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className={`whitespace-pre-wrap text-sm ${c.resolved ? 'text-muted-foreground line-through' : ''}`}>
                    {c.body}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {c.blockId ? `block ${c.blockId} · ` : ''}
                    {new Date(c.createdAt as string).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}
                  </p>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => void resolve({ variables: { id: c.id, resolved: !c.resolved } })}
                  >
                    {c.resolved ? 'Reopen' : 'Resolve'}
                  </Button>
                  {canManage && (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-muted-foreground hover:text-destructive"
                      onClick={() => void del({ variables: { id: c.id } })}
                      aria-label="Delete comment"
                    >
                      <XIcon className="size-4" />
                    </Button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="flex items-end gap-2">
        <Textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Add a comment…"
          rows={2}
          className="flex-1"
        />
        <Button
          size="sm"
          disabled={!body.trim() || adding}
          onClick={async () => {
            await add({ variables: { input: { pageId, body: body.trim() } } })
            setBody('')
          }}
        >
          Comment
        </Button>
      </div>
    </div>
  )
}
