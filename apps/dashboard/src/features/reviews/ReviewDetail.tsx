import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckIcon, RefreshCwIcon, SparklesIcon, XIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { LoadingState } from '@/components/ui/loading-state'
import { Textarea } from '@/components/ui/textarea'
import { LoadError } from '@/shared/errors/LoadError'
import { EntityChannelForbidden, EntityWrongChannel } from '@/shared/errors/ChannelScopeError'
import { isForbiddenError } from '@/shared/errors/isForbiddenError'
import { useConfigScope } from '@/lib/config-scope'
import { cn } from '@/lib/utils'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  ApproveReviewDocument,
  DraftReviewReplyDocument,
  GenerateProductReviewSummaryDocument,
  RecheckReviewModerationDocument,
  RejectReviewDocument,
  ReplyToReviewDocument,
  ReviewCategoriesDocument,
  ReviewDocument,
  ReviewModerationTagsDocument,
  ReviewsAiStatusDocument,
  ReviewsDocument,
  SetReviewCategoriesDocument,
  SetReviewModerationTagsDocument,
} from './Reviews.gql'

import { ReviewModerationBadge, ReviewStatusBadge } from './ReviewsList'
import { PluginSections } from '@/plugins/detail-sections'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'

// Per-review moderation page - Uses the shared DetailPage template: sticky header with title + status badges +
// Approve/Reject actions, body sections for the review text, answers, media, and moderator note

export function ReviewDetail() {
  const { id = '' } = useParams()
  const { has } = usePermissions()
  const { scope, selectedChannel } = useConfigScope()
  const canModerate = has('reviews.moderate')

  const { data, loading, error, refetch } = useQuery(ReviewDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const review = data?.review

  const [action, setAction] = useState<'approve' | 'reject' | null>(null)
  const [note, setNote] = useState('')
  const [mutationError, setMutationError] = useState<string | null>(null)

  // Every review mutation returns ReviewFields through the same hydration as the Review query, so
  // the cache updates this page from the response - only the list needs a refetch, for status
  // filtering, and by name so it keeps its own filter and page
  const [approve, { loading: approving }] = useMutation(ApproveReviewDocument, { refetchQueries: [ReviewsDocument] })
  const [reject, { loading: rejecting }] = useMutation(RejectReviewDocument, { refetchQueries: [ReviewsDocument] })

  // --- AI tools ---
  const { data: aiData } = useQuery(ReviewsAiStatusDocument)
  const aiEnabled = aiData?.aiStatus.enabled ?? false
  const [recheck, { loading: rechecking }] = useMutation(RecheckReviewModerationDocument)
  const { data: catData } = useQuery(ReviewCategoriesDocument, { fetchPolicy: 'cache-and-network' })
  const categories = catData?.reviewCategories ?? []
  const categoryNameOf = new Map(categories.map((c) => [c.slug, c.name]))
  const [setCategories, { loading: settingCategory }] = useMutation(SetReviewCategoriesDocument)

  // Toggle one category on/off and persist the new set (multi-category)
  function toggleCategory(slug: string) {
    if (!review) return
    const next = review.categorySlugs.includes(slug)
      ? review.categorySlugs.filter((s) => s !== slug)
      : [...review.categorySlugs, slug]
    void runAI(() => setCategories({ variables: { id: review.id, categorySlugs: next } }))
  }

  const { data: tagData } = useQuery(ReviewModerationTagsDocument, { fetchPolicy: 'cache-and-network' })
  const modTags = tagData?.reviewModerationTags ?? []
  const tagNameOf = new Map(modTags.map((t) => [t.slug, t.name]))
  const [setModTags, { loading: settingTags }] = useMutation(SetReviewModerationTagsDocument)

  // Toggle one moderation flag on/off and persist the new set
  function toggleTag(slug: string) {
    if (!review) return
    const next = review.moderationTags.includes(slug)
      ? review.moderationTags.filter((s) => s !== slug)
      : [...review.moderationTags, slug]
    void runAI(() => setModTags({ variables: { id: review.id, tagSlugs: next } }))
  }
  const [replyMut, { loading: replying }] = useMutation(ReplyToReviewDocument)
  const [draftReply, { loading: drafting }] = useMutation(DraftReviewReplyDocument)
  const [genSummary, { loading: genningSummary }] = useMutation(GenerateProductReviewSummaryDocument)
  const [reply, setReply] = useState('')
  const [summary, setSummary] = useState<string | null>(null)
  const [aiError, setAiError] = useState<string | null>(null)
  // The reply box is re-seeded from the saved reply after load, so dirt is computed against it rather than a mount snapshot
  useUnsavedChangesGuard(!!review && reply.trim() !== (review.merchantReply ?? ''), 'Review reply')

  // Seed the reply box from the saved merchant reply when the review loads
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional: sync the local draft from the server prop when a review loads/changes.
    setReply(review?.merchantReply ?? '')
  }, [review?.id, review?.merchantReply])

  async function runAI(fn: () => Promise<unknown>) {
    setAiError(null)
    try {
      await fn()
    } catch (err) {
      setAiError(err instanceof Error ? err.message : 'AI request failed.')
    }
  }

  async function onConfirm() {
    if (!action || !review) return
    setMutationError(null)
    try {
      const variables = { id: review.id, note: note.trim() || null }
      if (action === 'approve') await approve({ variables })
      else await reject({ variables })
      setAction(null)
      setNote('')
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Failed.')
    }
  }

  if (loading) return <LoadingState />
  if (error) {
    // Channel-scoped roles get `forbidden` when the review belongs to a
    // channel outside their scopes
    if (isForbiddenError(error)) {
      return (
        <EntityChannelForbidden
          entityLabel="review"
          permission="reviews.view"
          backTo="/reviews"
          backLabel="Back to reviews"
        />
      )
    }
    return (
      <LoadError
        title="Couldn't load review"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  }
  if (!review) {
    return (
      <Alert>
        <AlertTitle>Review not found</AlertTitle>
      </Alert>
    )
  }

  // Header channel scope gate - reviews belong to one channel
  if (scope.mode === 'CHANNEL' && selectedChannel && review.channelId !== selectedChannel.id) {
    return (
      <EntityWrongChannel
        entityLabel="review"
        entityChannelId={review.channelId}
        permission="reviews.view"
        backTo="/reviews"
        backLabel="Back to reviews"
      />
    )
  }

  const inFlight = approving || rejecting
  const title = review.title || '(no title)'

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/reviews/pending"
        backLabel="Back to moderation queue"
        title={title}
        badges={
          <>
            <ReviewStatusBadge status={review.status} />
            {review.isVerifiedPurchase ? (
              <Badge variant="secondary">Verified purchase</Badge>
            ) : (
              <Badge variant="outline">Unverified</Badge>
            )}
            <ReviewModerationBadge label={review.moderationTags.length > 0 ? 'flagged' : review.moderationLabel} />
            {review.moderationTags.map((slug) => (
              <Badge key={slug} variant="destructive">
                {tagNameOf.get(slug) ?? slug}
              </Badge>
            ))}
            {review.categorySlugs.map((slug) => (
              <Badge key={slug} variant="outline">
                {categoryNameOf.get(slug) ?? slug}
              </Badge>
            ))}
          </>
        }
        meta={
          <>
            <span>Submitted {new Date(review.submittedAt).toLocaleString()}</span>
            <span>·</span>
            <Link
              to={`/products/${review.productId}`}
              className="font-mono text-foreground hover:underline"
            >
              Product {review.productId.slice(0, 8)}…
            </Link>
          </>
        }
        actions={
          canModerate ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => {
                  setAction('reject')
                  setNote(review.moderatorNote ?? '')
                }}
                disabled={inFlight}
              >
                <XIcon className="size-4" />
                Reject
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  setAction('approve')
                  setNote(review.moderatorNote ?? '')
                }}
                disabled={inFlight}
              >
                <CheckIcon className="size-4" />
                Approve
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        <DetailPageSection title="Review">
          <p className="whitespace-pre-wrap text-sm">
            {review.body || <em className="text-muted-foreground">(no body)</em>}
          </p>
        </DetailPageSection>

        {review.answers.length > 0 ? (
          <DetailPageSection title="Answers">
            <div className="grid gap-3">
              {review.answers.map((a, i) => (
                <div key={i} className="rounded-md border p-3 text-sm">
                  <div className="flex items-center justify-between gap-2">
                    <p className="font-medium">
                      {a.question.content?.label || a.question.slug}
                    </p>
                    {a.rating != null ? (
                      <Badge variant="secondary">{a.rating} / 5</Badge>
                    ) : a.boolValue != null ? (
                      <Badge variant="secondary">{a.boolValue ? 'Yes' : 'No'}</Badge>
                    ) : null}
                  </div>
                  {a.text ? (
                    <p className="mt-1 text-sm text-muted-foreground">{a.text}</p>
                  ) : null}
                </div>
              ))}
            </div>
          </DetailPageSection>
        ) : null}

        {review.media.length > 0 ? (
          <DetailPageSection title="Photos & videos">
            <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
              {review.media.map((m) => (
                <li key={m.id} className="overflow-hidden rounded-md border">
                  {m.mimeType.startsWith('video/') ? (
                    <video controls src={m.url} className="aspect-square w-full bg-black object-contain" />
                  ) : (
                    <img
                      src={m.url}
                      alt=""
                      className="aspect-square w-full object-cover"
                    />
                  )}
                  <p className="px-2 py-1 text-[10px] text-muted-foreground">
                    {m.mimeType}
                  </p>
                </li>
              ))}
            </ul>
          </DetailPageSection>
        ) : null}

        {review.moderatorNote ? (
          <DetailPageSection
            title="Moderator note"
            description={
              review.moderatedAt
                ? `Set ${new Date(review.moderatedAt).toLocaleString()}`
                : undefined
            }
          >
            <p className="whitespace-pre-wrap text-sm">{review.moderatorNote}</p>
          </DetailPageSection>
        ) : null}

        {canModerate ? (
          <DetailPageSection
            title="AI moderation"
            description="A hint only - you still approve or reject. Runs automatically on submission."
          >
            <div className="flex flex-wrap items-center gap-3">
              {review.moderationLabel ? (
                <ReviewModerationBadge label={review.moderationLabel} />
              ) : (
                <span className="text-sm text-muted-foreground">Not checked yet.</span>
              )}
              {review.moderationReason ? (
                <span className="text-sm text-muted-foreground">{review.moderationReason}</span>
              ) : null}
              {aiEnabled ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="ml-auto h-8"
                  disabled={rechecking}
                  onClick={() => void runAI(() => recheck({ variables: { id: review.id } }))}
                >
                  <RefreshCwIcon className="size-4" />
                  {rechecking ? 'Checking…' : 'Re-check'}
                </Button>
              ) : null}
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
              <span className="text-sm text-muted-foreground">Flags</span>
              {modTags.length === 0 ? (
                <span className="text-xs text-muted-foreground">No moderation tags defined.</span>
              ) : (
                modTags.map((t) => {
                  const on = review.moderationTags.includes(t.slug)
                  return (
                    <button
                      key={t.id}
                      type="button"
                      disabled={settingTags}
                      onClick={() => toggleTag(t.slug)}
                      className={cn(
                        'rounded-full border px-2.5 py-1 text-xs transition-colors disabled:opacity-50',
                        on
                          ? 'border-destructive bg-destructive/10 text-destructive'
                          : 'text-muted-foreground hover:bg-muted',
                      )}
                    >
                      {t.name}
                    </button>
                  )
                })
              )}
              <span className="text-xs text-muted-foreground">
                AI-assigned - toggle to override.
              </span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
              <span className="text-sm text-muted-foreground">Categories</span>
              {categories.length === 0 ? (
                <span className="text-xs text-muted-foreground">No categories defined.</span>
              ) : (
                categories.map((c) => {
                  const on = review.categorySlugs.includes(c.slug)
                  return (
                    <button
                      key={c.id}
                      type="button"
                      disabled={settingCategory}
                      onClick={() => toggleCategory(c.slug)}
                      className={cn(
                        'rounded-full border px-2.5 py-1 text-xs transition-colors disabled:opacity-50',
                        on
                          ? 'border-primary bg-primary/10 text-foreground'
                          : 'text-muted-foreground hover:bg-muted',
                      )}
                    >
                      {c.name}
                    </button>
                  )
                })
              )}
              <span className="text-xs text-muted-foreground">
                AI-assigned - a review can be in several; toggle to override.
              </span>
            </div>
          </DetailPageSection>
        ) : null}

        {canModerate ? (
          <DetailPageSection
            title="Reply to customer"
            description="Shown publicly under the review on the storefront."
          >
            <div className="grid gap-2">
              <Textarea
                value={reply}
                onChange={(e) => setReply(e.target.value)}
                placeholder="Write a reply…"
                rows={3}
              />
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  disabled={replying || reply.trim() === (review.merchantReply ?? '')}
                  onClick={() => void runAI(() => replyMut({ variables: { id: review.id, body: reply.trim() } }))}
                >
                  {replying ? 'Saving…' : 'Save reply'}
                </Button>
                {review.merchantReply ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={replying}
                    onClick={() =>
                      void runAI(async () => {
                        await replyMut({ variables: { id: review.id, body: '' } })
                        setReply('')
                      })
                    }
                  >
                    Clear
                  </Button>
                ) : null}
                {aiEnabled ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="ml-auto"
                    disabled={drafting}
                    onClick={() =>
                      void runAI(async () => {
                        const res = await draftReply({ variables: { id: review.id } })
                        if (res.data?.draftReviewReply) setReply(res.data.draftReviewReply)
                      })
                    }
                  >
                    <SparklesIcon className="size-4" />
                    {drafting ? 'Drafting…' : 'Draft with AI'}
                  </Button>
                ) : null}
              </div>
            </div>
          </DetailPageSection>
        ) : null}

        {canModerate && aiEnabled ? (
          <DetailPageSection
            title="Product review summary"
            description="The cached 'what customers say' summary shown on this product's page. Auto-refreshes when reviews change; regenerate it now below."
          >
            <div className="grid gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                disabled={genningSummary}
                onClick={() =>
                  void runAI(async () => {
                    const res = await genSummary({ variables: { productId: review.productId } })
                    setSummary(res.data?.generateProductReviewSummary || '(not enough approved reviews yet - needs 3+)')
                  })
                }
              >
                <SparklesIcon className="size-4" />
                {genningSummary ? 'Generating…' : 'Generate summary'}
              </Button>
              {summary ? <p className="whitespace-pre-wrap rounded-md border bg-muted/30 p-3 text-sm">{summary}</p> : null}
            </div>
          </DetailPageSection>
        ) : null}

        {aiError ? (
          <Alert variant="destructive">
            <AlertTitle>AI</AlertTitle>
            <AlertDescription>{aiError}</AlertDescription>
          </Alert>
        ) : null}
        <PluginSections target="review" entity={review ?? null} mode="edit" />
      </DetailPageBody>

      <Dialog open={action !== null} onOpenChange={(o) => !o && setAction(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {action === 'approve' ? 'Approve this review?' : 'Reject this review?'}
            </DialogTitle>
            <DialogDescription>
              {action === 'approve'
                ? 'The review becomes publicly visible on the channel.'
                : 'The review is hidden from the channel. The customer can update it to resubmit.'}
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Optional note (visible to other moderators)"
            rows={3}
          />
          {mutationError ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't save</AlertTitle>
              <AlertDescription>{mutationError}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setAction(null)} disabled={inFlight}>
              Cancel
            </Button>
            <Button onClick={() => void onConfirm()} disabled={inFlight}>
              {inFlight ? 'Saving…' : action === 'approve' ? 'Approve' : 'Reject'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DetailPageLayout>
  )
}
