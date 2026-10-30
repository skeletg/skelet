import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { AddCmsPageCommentDocument, CmsPageCommentsDocument } from '../Cms.gql'
import { CmsPageComments } from './Comments'

// What these pin: resolved comments fold away under their own toggle, Ctrl+Enter posts the trimmed text, and a
// refused post says why and keeps what was typed

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const comment = (id: string, body: string, resolved: boolean) => ({
  __typename: 'CMSPageComment' as const,
  id,
  pageId: 'pg1',
  locale: 'en',
  blockId: null,
  authorId: 'admin-1',
  body,
  resolved,
  resolvedBy: null,
  resolvedAt: null,
  createdAt: '2026-10-01T10:00:00Z',
})

describe('CmsPageComments', () => {
  it('folds resolved comments away and posts with Ctrl+Enter', async () => {
    const onAdd = vi.fn()
    renderDialog(<CmsPageComments pageId="pg1" canManage />, [
      {
        request: { query: CmsPageCommentsDocument, variables: { pageId: 'pg1' } },
        result: { data: { cmsPageComments: [comment('c1', 'Tighten the hero copy', false), comment('c2', 'Fix the typo', true)] } },
        maxUsageCount: 3,
      },
      { request: { query: AddCmsPageCommentDocument, variables: captureInput(onAdd) }, result: { data: { addCMSPageComment: null } } },
    ])
    expect(await screen.findByText('Tighten the hero copy', {}, SLOW)).toBeDefined()
    expect(screen.queryByText('Fix the typo')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /Resolved \(1\)/ }))
    expect(screen.getByText('Fix the typo')).toBeDefined()

    const box = screen.getByLabelText('New comment')
    fireEvent.change(box, { target: { value: '  Looks good now  ' } })
    fireEvent.keyDown(box, { key: 'Enter', ctrlKey: true })
    await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith({ pageId: 'pg1', body: 'Looks good now' }), SLOW)
  })

  it('keeps the text and says why when a post is refused', async () => {
    renderDialog(<CmsPageComments pageId="pg1" canManage />, [
      { request: { query: CmsPageCommentsDocument, variables: { pageId: 'pg1' } }, result: { data: { cmsPageComments: [] } }, maxUsageCount: 3 },
      { request: { query: AddCmsPageCommentDocument, variables: () => true }, result: { errors: [{ message: 'comments are closed on this page' }] } },
    ])
    const box = await screen.findByLabelText('New comment', {}, SLOW)
    fireEvent.change(box, { target: { value: 'Looks good now' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comment' }))
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith('comments are closed on this page'), SLOW)
    expect(box).toHaveProperty('value', 'Looks good now')
  })
})
