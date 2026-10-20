import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { BlogPostDetail } from './BlogPostDetail'
import {
  BlogCategoriesDocument,
  BlogPostDocument,
  BlogTagsDocument,
  DeleteBlogPostDocument,
  SetBlogPostTagsDocument,
  UpdateBlogPostDocument,
  UpsertBlogPostContentDocument,
} from './Blog.gql'

// What these pin: settings, the locale's content and the tags are one review, each write goes only
// when its part changed, and a content edit without a title is refused (the service requires one)

const post = {
  __typename: 'BlogPost' as const,
  id: 'p1',
  slug: 'hello',
  status: 'DRAFT',
  publishedAt: null,
  seoNoindex: false,
  readingTimeMinutes: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  author: null,
  content: { __typename: 'BlogPostContent' as const, locale: 'en', title: 'Hello', excerpt: null, metaTitle: null, metaDescription: null },
  allContent: [
    {
      __typename: 'BlogPostContent' as const,
      locale: 'en',
      title: 'Hello',
      excerpt: null,
      body: null,
      metaTitle: null,
      metaDescription: null,
      metaKeywords: null,
      updatedAt: '2026-01-01T00:00:00Z',
    },
  ],
  categories: [],
  tags: [],
  channelListings: [],
}

const tag = {
  __typename: 'BlogTag' as const,
  id: 't1',
  slug: 'news',
  isPublished: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  content: { __typename: 'BlogTagContent' as const, locale: 'en', name: 'News', description: null, metaTitle: null, metaDescription: null },
  allContent: [],
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderPost(extra: MockLink.MockedResponse[] = []) {
  const writes = { settings: vi.fn(), content: vi.fn(), tags: vi.fn() }
  renderPage({
    path: '/blog/posts/p1',
    adminId: 'admin-1',
    scope: { ...globalScope, effectiveLocale: 'en' },
    routes: { '/blog/posts/:id': <BlogPostDetail />, '/blog/posts': <p>Posts list</p> },
    mocks: [
      ...extra,
      meMock(),
      { request: { query: BlogPostDocument, variables: () => true }, result: { data: { blogPost: post } }, maxUsageCount: 5 },
      { request: { query: BlogCategoriesDocument, variables: () => true }, result: { data: { blogCategories: [] } }, maxUsageCount: 3 },
      { request: { query: BlogTagsDocument }, result: { data: { blogTags: [tag] } }, maxUsageCount: 3 },
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
      { request: { query: UpdateBlogPostDocument, variables: recorder(writes.settings) }, result: { data: { updateBlogPost: post } } },
      { request: { query: UpsertBlogPostContentDocument, variables: recorder(writes.content) }, result: { data: { upsertBlogPostContent: post } } },
      { request: { query: SetBlogPostTagsDocument, variables: recorder(writes.tags) }, result: { data: { setBlogPostTags: post } } },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('BlogPostDetail', () => {
  it('writes the content and tags it changed, and leaves the settings alone', async () => {
    const writes = renderPost()
    fireEvent.change(await screen.findByLabelText('Meta title', {}, SLOW), { target: { value: 'Hello, world' } })
    fireEvent.click(await screen.findByRole('button', { name: '#News' }))
    fireEvent.click(await screen.findByRole('button', { name: /Save changes/ }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.tags).toHaveBeenCalledWith({ postId: 'p1', tagIds: ['t1'] }), SLOW)
    expect(writes.content).toHaveBeenCalledWith({ input: expect.objectContaining({ postId: 'p1', locale: 'en', title: 'Hello', metaTitle: 'Hello, world' }) })
    expect(writes.settings).not.toHaveBeenCalled()
  })

  it('refuses a content edit without a title', async () => {
    const writes = renderPost()
    fireEvent.change(await screen.findByLabelText('Title', {}, SLOW), { target: { value: '' } })
    fireEvent.click(await screen.findByRole('button', { name: /Save changes/ }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/A translation needs a title/)).toBeDefined()
    expect(writes.content).not.toHaveBeenCalled()
  })
})

// Delete confirms through the shared ConfirmDialog without waiting on the server: it closes as soon as it is
// confirmed, a refused delete reports in the page feedback, and a successful one goes back to the list
describe('BlogPostDetail delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderPost([mock])
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  }

  it('closes at once and reports a refused delete on the page', async () => {
    await confirmDelete({ request: { query: DeleteBlogPostDocument, variables: { id: 'p1' } }, result: { errors: [{ message: 'post is pinned' }] }, delay: 500 })
    expect(screen.queryByText('post is pinned')).toBeNull() // closed before the server answered
    expect(await screen.findByText('post is pinned', {}, SLOW)).toBeDefined()
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteBlogPostDocument, variables: { id: 'p1' } }, result: { data: { deleteBlogPost: true } } })
    expect(await screen.findByText('Posts list', {}, SLOW)).toBeDefined()
  })
})
