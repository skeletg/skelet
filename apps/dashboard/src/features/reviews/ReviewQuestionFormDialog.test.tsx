import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog, stubResizeObserver } from '@/test/page-harness'
import { ReviewQuestionFormDialog } from './ReviewQuestionFormDialog'
import { CreateReviewQuestionDocument, ReviewQuestionsDocument, UpdateReviewQuestionDocument } from './Reviews.gql'

stubResizeObserver()

const list = { request: { query: ReviewQuestionsDocument }, result: { data: { reviewQuestions: [] } }, maxUsageCount: 3 }

describe('ReviewQuestionFormDialog', () => {
  it('seeds the label for the default locale on create', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(<ReviewQuestionFormDialog open onOpenChange={onOpenChange} attribute={null} />, [
      {
        request: { query: CreateReviewQuestionDocument, variables: captureInput(onCreate) },
        result: { data: { createReviewQuestion: { __typename: 'ReviewQuestion', id: 'q1' } } },
      },
      list,
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    expect(await screen.findByText('Label is required')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Label (en)'), { target: { value: 'Grip on wet roads' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'grip-on-wet-roads', label: 'Grip on wet roads', locale: 'en', inputType: 'RATING_1_5', helpText: null }),
    )
  })

  it('edits a question with no default-locale label, which create would reject', async () => {
    const onUpdate = vi.fn()
    const onOpenChange = vi.fn()
    const question = {
      __typename: 'ReviewQuestion' as const,
      id: 'q1',
      slug: 'grip',
      inputType: 'RATING_1_5' as const,
      isRequired: false,
      allowComment: true,
      sortOrder: 0,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
      content: null,
      allContent: [],
    }
    renderDialog(
      <ReviewQuestionFormDialog open onOpenChange={onOpenChange} attribute={question as never} />,
      [
        {
          request: { query: UpdateReviewQuestionDocument, variables: captureInput(onUpdate) },
          result: { data: { updateReviewQuestion: { __typename: 'ReviewQuestion', id: 'q1' } } },
        },
        list,
      ],
    )
    fireEvent.click(screen.getByLabelText('Required'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onUpdate).toHaveBeenCalledWith({ id: 'q1', isRequired: true, allowComment: true, sortOrder: 0 })
  })
})
