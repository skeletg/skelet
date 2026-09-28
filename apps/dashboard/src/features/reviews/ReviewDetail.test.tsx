import { fireEvent, screen, within } from '@testing-library/react'
import { useQuery } from '@apollo/client/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { ReviewDetail } from './ReviewDetail'
import {
  ApproveReviewDocument,
  ReviewCategoriesDocument,
  ReviewDocument,
  ReviewModerationTagsDocument,
  ReviewsAiStatusDocument,
  ReviewsDocument,
  type ReviewsQueryVariables,
} from './Reviews.gql'

// What these pin: approving answers with the whole review, so the page updates from the response
// without fetching the review again, and the reviews list is refetched by its generated document -
// whichever copy is mounted, with that copy's own filter and page

const review = {
  __typename: 'Review' as const,
  id: 'rv1',
  productId: 'p1',
  customerId: null,
  channelId: 'c1',
  locale: 'en',
  title: 'Great tee',
  body: 'Soft and fits well',
  isVerifiedPurchase: true,
  status: 'PENDING' as const,
  submittedAt: '2026-01-01T00:00:00Z',
  moderatedAt: null,
  moderatedBy: null,
  moderatorNote: '',
  moderationLabel: '',
  moderationReason: '',
  moderationTags: [],
  categorySlugs: [],
  merchantReply: '',
  merchantReplyAt: null,
  answers: [],
  media: [],
}

// The pending queue, page two - variables no mutation site could know
const listVariables: ReviewsQueryVariables = { filter: { status: ['PENDING'] }, limit: 20, offset: 20 }

// Stands in for ReviewsList: watches the list under its own variables
function ReviewsListProbe() {
  useQuery(ReviewsDocument, { variables: listVariables })
  return null
}

const recorder = (calls: Record<string, unknown>[]) => (vars: Record<string, unknown>) => {
  calls.push(vars)
  return true
}

function renderReview() {
  const detailRequests: Record<string, unknown>[] = []
  const listRequests: Record<string, unknown>[] = []
  renderPage({
    path: '/reviews/rv1',
    routes: {
      '/reviews/:id': (
        <>
          <ReviewsListProbe />
          <ReviewDetail />
        </>
      ),
    },
    mocks: [
      meMock(),
      {
        request: { query: ReviewDocument, variables: recorder(detailRequests) },
        result: { data: { review } },
        maxUsageCount: 5,
      },
      {
        request: { query: ReviewsDocument, variables: recorder(listRequests) },
        result: { data: { reviews: { __typename: 'ReviewListPage', totalCount: 0, items: [] } } },
        maxUsageCount: 5,
      },
      {
        request: { query: ApproveReviewDocument, variables: { id: 'rv1', note: null } },
        result: {
          data: {
            approveReview: { ...review, status: 'APPROVED', moderatedAt: '2026-01-02T00:00:00Z', moderatedBy: 'admin-1' },
          },
        },
      },
      { request: { query: ReviewsAiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 3 },
      { request: { query: ReviewCategoriesDocument }, result: { data: { reviewCategories: [] } }, maxUsageCount: 3 },
      { request: { query: ReviewModerationTagsDocument }, result: { data: { reviewModerationTags: [] } }, maxUsageCount: 3 },
    ],
  })
  return { detailRequests, listRequests }
}

describe('ReviewDetail', () => {
  it('approves from the response and refetches only the mounted list', async () => {
    const { detailRequests, listRequests } = renderReview()
    await vi.waitFor(() => expect(listRequests).toHaveLength(1), SLOW)

    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Approve' }))

    await vi.waitFor(() => expect(listRequests).toHaveLength(2), SLOW)
    expect(listRequests[1]).toEqual(listVariables)
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), SLOW)
    expect(detailRequests).toEqual([{ id: 'rv1' }])
  })
})
