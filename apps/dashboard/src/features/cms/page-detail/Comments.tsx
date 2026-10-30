import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { toast } from 'sonner'
import { CheckIcon, ChevronRightIcon, MessageSquareIcon, RotateCcwIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  CmsPageCommentsDocument,
  AddCmsPageCommentDocument,
  ResolveCmsPageCommentDocument,
  DeleteCmsPageCommentDocument,
  type CmsPageCommentRowFragment,
} from '../Cms.gql'

// Lists and posts collaboration comments - not part of the staged page save, they write immediately
export function CmsPageComments({ pageId, canManage }: { pageId: string; canManage: boolean }) {
  const refetch = [{ query: CmsPageCommentsDocument, variables: { pageId } }]
  const { data, loading } = useQuery(CmsPageCommentsDocument, { variables: { pageId } })
  const [add, { loading: adding }] = useMutation(AddCmsPageCommentDocument, { refetchQueries: refetch })
  const [resolve] = useMutation(ResolveCmsPageCommentDocument, { refetchQueries: refetch })
  const [del] = useMutation(DeleteCmsPageCommentDocument, { refetchQueries: refetch })
  const [body, setBody] = useState('')
  const [showResolved, setShowResolved] = useState(false)
  const comments = data?.cmsPageComments ?? []
  const open = comments.filter((c) => !c.resolved)
  const resolved = comments.filter((c) => c.resolved)

  // A failed write keeps what was typed and says why, instead of an unhandled rejection nobody sees
  async function attempt(fn: () => Promise<unknown>, failure: string): Promise<boolean> {
    try {
      await fn()
      return true
    } catch (e) {
      toast.error(e instanceof Error ? e.message : failure)
      return false
    }
  }

  async function post() {
    if (!body.trim() || adding) return
    if (await attempt(() => add({ variables: { input: { pageId, body: body.trim() } } }), "Couldn't post the comment.")) setBody('')
  }

  const row = (c: CmsPageCommentRowFragment) => (
    <li key={c.id} className="group flex items-start gap-3 px-5 py-3">
      <div className="min-w-0 flex-1">
        <p className={cn('text-sm whitespace-pre-wrap', c.resolved && 'text-muted-foreground')}>{c.body}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          {formatDateTime(c.createdAt)}
          {c.blockId ? <span title={`Block ${c.blockId}`}> · on a block</span> : null}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-0.5">
        <Button
          size="sm"
          variant="ghost"
          className="h-7 px-2 text-xs"
          onClick={() => void attempt(() => resolve({ variables: { id: c.id, resolved: !c.resolved } }), "Couldn't update the comment.")}
        >
          {c.resolved ? <RotateCcwIcon className="size-3.5" /> : <CheckIcon className="size-3.5" />}
          {c.resolved ? 'Reopen' : 'Resolve'}
        </Button>
        {canManage && (
          <Button
            size="icon"
            variant="ghost"
            className="size-7 text-muted-foreground opacity-60 transition-opacity group-hover:opacity-100 hover:text-destructive"
            onClick={() => void attempt(() => del({ variables: { id: c.id } }), "Couldn't delete the comment.")}
            aria-label="Delete comment"
          >
            <Trash2Icon className="size-3.5" />
          </Button>
        )}
      </div>
    </li>
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading && comments.length === 0 ? (
          <p className="px-5 py-4 text-sm text-muted-foreground">Loading…</p>
        ) : open.length === 0 && resolved.length === 0 ? (
          <div className="grid place-items-center px-8 py-12 text-center">
            <div className="grid justify-items-center gap-2">
              <MessageSquareIcon className="size-8 text-muted-foreground/60" />
              <p className="text-sm font-medium">No comments yet</p>
              <p className="text-sm text-muted-foreground">Start the thread below.</p>
            </div>
          </div>
        ) : (
          <>
            {open.length > 0 ? (
              <ul className="divide-y">{open.map(row)}</ul>
            ) : (
              <p className="px-5 py-4 text-sm text-muted-foreground">Nothing open - every comment is resolved.</p>
            )}
            {resolved.length > 0 && (
              <div className="border-t">
                <button
                  type="button"
                  aria-expanded={showResolved}
                  onClick={() => setShowResolved((v) => !v)}
                  className="flex w-full items-center gap-1.5 px-5 py-2.5 text-left text-xs font-medium text-muted-foreground hover:text-foreground"
                >
                  <ChevronRightIcon className={cn('size-3.5 transition-transform duration-200', showResolved && 'rotate-90')} />
                  Resolved ({resolved.length})
                </button>
                {showResolved && <ul className="divide-y border-t bg-muted/30">{resolved.map(row)}</ul>}
              </div>
            )}
          </>
        )}
      </div>
      <div className="grid gap-2 border-t p-3">
        <Textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              void post()
            }
          }}
          placeholder="Add a comment…"
          aria-label="New comment"
          rows={2}
          className="min-h-16 resize-none"
        />
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">Ctrl/⌘ + Enter to send</span>
          <Button size="sm" disabled={!body.trim() || adding} onClick={() => void post()}>
            {adding ? 'Posting…' : 'Comment'}
          </Button>
        </div>
      </div>
    </div>
  )
}
