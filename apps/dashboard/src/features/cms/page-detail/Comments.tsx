import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  CmsPageCommentsDocument,
  AddCmsPageCommentDocument,
  ResolveCmsPageCommentDocument,
  DeleteCmsPageCommentDocument,
} from '../Cms.gql'

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
