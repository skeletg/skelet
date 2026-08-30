import { useQuery } from '@apollo/client/react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ReviewCountsDocument } from '@/features/reviews/Reviews.gql'
import { ProductQuestionsDocument } from '@/features/qna/Qna.gql'
import { CmsPagesDocument } from '@/features/cms/Cms.gql'
import { Empty, QueueRow, RowList, WidgetSection } from './primitives'

const PROBE = 50 // enough to count a queue worth clearing; past that the exact number stops mattering

// Three moderation queues in one section, each row gated on its own permission and simply absent
// without it - a reviewer without CMS access sees two rows, not an empty third
export function ModerationWidget() {
  const { has } = usePermissions()
  const canReviews = has('reviews.view')
  const canQna = has('qna.view')
  const canCms = has('cms.view')

  const reviews = useQuery(ReviewCountsDocument, { skip: !canReviews, fetchPolicy: 'cache-and-network' })
  const questions = useQuery(ProductQuestionsDocument, {
    variables: { status: 'pending', limit: PROBE },
    skip: !canQna,
    fetchPolicy: 'cache-and-network',
  })
  const pages = useQuery(CmsPagesDocument, {
    variables: { filter: { workflowState: 'IN_REVIEW' }, limit: PROBE },
    skip: !canCms,
    fetchPolicy: 'cache-and-network',
  })

  const rows = [
    canReviews && {
      key: 'reviews',
      label: 'Reviews to moderate',
      hint: 'Waiting on approve or reject',
      count: reviews.data?.reviewCounts.pending ?? 0,
      to: '/reviews',
    },
    canQna && {
      key: 'questions',
      label: 'Product questions',
      hint: 'Asked, not yet answered',
      count: questions.data?.productQuestions.length ?? 0,
      to: '/qna',
    },
    canCms && {
      key: 'pages',
      label: 'Pages in review',
      hint: 'Submitted for editorial approval',
      count: pages.data?.cmsPages.length ?? 0,
      to: '/cms',
    },
  ].filter((r): r is { key: string; label: string; hint: string; count: number; to: string } => !!r)

  const error = reviews.error ?? questions.error ?? pages.error ?? null

  return (
    <WidgetSection
      title="Waiting on you"
      description="Queues that only clear when a person looks at them."
      error={error}
    >
      {rows.length === 0 ? (
        <Empty>No moderation queues for this account.</Empty>
      ) : (
        <RowList>
          {rows.map((r) => (
            <QueueRow
              key={r.key}
              label={r.label}
              hint={r.hint}
              count={r.count}
              to={r.to}
              tone={r.count > 0 ? 'warn' : 'default'}
            />
          ))}
        </RowList>
      )}
    </WidgetSection>
  )
}
