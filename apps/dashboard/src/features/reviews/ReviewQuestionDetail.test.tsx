import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import { ReviewQuestionDetailPage } from './ReviewQuestionDetail'
import { ReviewQuestionDocument, UpsertReviewQuestionContentDocument } from './Reviews.gql'

// What these pin: every translation row goes through the one header Save, only changed rows are
// written, and a stored translation can't be saved with its label emptied

const question = {
  __typename: 'ReviewQuestion' as const,
  id: 'q1',
  slug: 'fit',
  inputType: 'RATING_1_5' as const,
  isRequired: false,
  allowComment: true,
  sortOrder: 0,
  content: { __typename: 'ReviewQuestionContent' as const, locale: 'en', label: 'How does it fit?', helpText: '', updatedAt: '2026-01-01T00:00:00Z' },
  allContent: [
    { __typename: 'ReviewQuestionContent' as const, locale: 'en', label: 'How does it fit?', helpText: '', updatedAt: '2026-01-01T00:00:00Z' },
  ],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function renderQuestion(onUpsert = vi.fn()) {
  renderPage({
    path: '/reviews/questions/q1',
    scope: { ...globalScope, availableLocales: ['en', 'de'] },
    routes: { '/reviews/questions/:id': <ReviewQuestionDetailPage /> },
    mocks: [
      meMock(),
      { request: { query: ReviewQuestionDocument, variables: { id: 'q1' } }, result: { data: { reviewQuestion: question } }, maxUsageCount: 5 },
      {
        request: { query: UpsertReviewQuestionContentDocument, variables: captureInput(onUpsert) },
        result: { data: { upsertReviewQuestionContent: question } },
        maxUsageCount: 2,
      },
    ],
  })
  return onUpsert
}

describe('ReviewQuestionDetail', () => {
  it('writes the edited and the new translation, and leaves the rest alone', async () => {
    const onUpsert = renderQuestion()
    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Help text (en)'), { target: { value: 'Runs small or large?' } })
    fireEvent.change(screen.getByLabelText('Label (de)'), { target: { value: 'Wie ist die Passform?' } })
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(onUpsert).toHaveBeenCalledTimes(2), SLOW)
    expect(onUpsert).toHaveBeenCalledWith({ questionId: 'q1', locale: 'en', label: 'How does it fit?', helpText: 'Runs small or large?' })
    expect(onUpsert).toHaveBeenCalledWith({ questionId: 'q1', locale: 'de', label: 'Wie ist die Passform?', helpText: null })
  })

  it('refuses to empty a stored label', async () => {
    const onUpsert = renderQuestion()
    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Label (en)'), { target: { value: '' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Label is required/)).toBeDefined()
    expect(onUpsert).not.toHaveBeenCalled()
  })
})
