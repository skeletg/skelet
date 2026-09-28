import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput } from '@/test/page-harness'
import { ReviewCategoryFormDialog } from './ReviewCategoryFormDialog'
import { ReviewModerationTagFormDialog } from './ReviewModerationTagFormDialog'
import {
  CreateReviewCategoryDocument,
  ReviewCategoriesDocument,
  ReviewModerationTagsDocument,
  UpdateReviewModerationTagDocument,
} from './Reviews.gql'

// Radix's Switch measures itself inside a <form>, and jsdom has no ResizeObserver
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver

describe('ReviewCategoryFormDialog', () => {
  it('derives the slug from the name and creates the category', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <MockedProvider
        mocks={[
          {
            request: { query: CreateReviewCategoryDocument, variables: captureInput(onCreate) },
            result: { data: { createReviewCategory: { __typename: 'ReviewCategory', id: 'c1' } } },
          },
          { request: { query: ReviewCategoriesDocument }, result: { data: { reviewCategories: [] } }, maxUsageCount: 3 },
        ]}
      >
        <ReviewCategoryFormDialog open onOpenChange={onOpenChange} category={null} />
      </MockedProvider>,
    )
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Late shipment' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('late-shipment')
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith({ slug: 'late-shipment', name: 'Late shipment', description: null, isActive: true, sortOrder: 0 })
  })
})

describe('ReviewModerationTagFormDialog', () => {
  it('edits an existing tag without offering the slug', async () => {
    const onUpdate = vi.fn()
    const onOpenChange = vi.fn()
    const tag = {
      __typename: 'ReviewModerationTag' as const,
      id: 't1',
      slug: 'profanity',
      name: 'Profanity',
      description: 'Curse words',
      isActive: true,
      sortOrder: 2,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    render(
      <MockedProvider
        mocks={[
          {
            request: { query: UpdateReviewModerationTagDocument, variables: captureInput(onUpdate) },
            result: { data: { updateReviewModerationTag: { __typename: 'ReviewModerationTag', id: 't1' } } },
          },
          { request: { query: ReviewModerationTagsDocument }, result: { data: { reviewModerationTags: [] } }, maxUsageCount: 3 },
        ]}
      >
        <ReviewModerationTagFormDialog open onOpenChange={onOpenChange} tag={tag} />
      </MockedProvider>,
    )
    expect(screen.queryByLabelText('Slug')).toBeNull()
    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Curse words or slurs' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onUpdate).toHaveBeenCalledWith({ id: 't1', name: 'Profanity', description: 'Curse words or slurs', isActive: true, sortOrder: 2 })
  })
})
