import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { BlogTaxonomyPage } from './BlogTaxonomyPage'
import { BlogTagsDocument, DeleteBlogTagDocument } from './Blog.gql'

// What this pins: a taxonomy row delete runs through the shared ConfirmDialog - a refused delete keeps it open
// with the server's message, a successful one closes it

const content = { __typename: 'BlogTagContent', locale: 'en', name: 'News', description: '', metaTitle: '', metaDescription: '' }
const news = {
  __typename: 'BlogTag', id: 't1', slug: 'news', isPublished: true, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  content, allContent: [{ ...content, metaKeywords: '', updatedAt: '2026-10-01T00:00:00Z' }],
}

async function confirmDelete(mock: MockLink.MockedResponse) {
  renderPage({
    path: '/blog/taxonomy?tab=tags',
    adminId: 'admin-1',
    routes: { '/blog/taxonomy': <BlogTaxonomyPage /> },
    mocks: [meMock(), { request: { query: BlogTagsDocument }, result: { data: { blogTags: [news] } }, maxUsageCount: 5 }, mock],
  })
  // The delete button sits behind the manage grant - wait for New tag, which shares it
  await screen.findByText(/New tag/, {}, SLOW)
  const row = (await screen.findByText('#News', {}, SLOW)).closest('tr') as HTMLElement
  fireEvent.click(within(row).getAllByRole('button').at(-1)!)
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('Delete #News?')).toBeDefined()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
  return dialog
}

describe('BlogTaxonomyPage delete', () => {
  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteBlogTagDocument, variables: { id: 't1' } }, result: { errors: [{ message: 'tag in use' }] } })
    expect(await within(dialog).findByText('tag in use')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteBlogTagDocument, variables: { id: 't1' } }, result: { data: { deleteBlogTag: true } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
