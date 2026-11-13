import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { BlogPostsList, BlogPostsNew } from './BlogPostsList'
import { BlogAuthorsDocument, BlogPostsDocument, CreateBlogPostDocument } from './Blog.gql'

const reads = () => [
  meMock(),
  { request: { query: BlogPostsDocument, variables: () => true }, result: { data: { blogPosts: { __typename: 'BlogPostList', items: [] } } }, maxUsageCount: 5 },
  { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
  { request: { query: BlogAuthorsDocument }, result: { data: { blogAuthors: [] } }, maxUsageCount: 3 },
  { request: { query: LocalesPickerDocument }, result: { data: { locales: [] } }, maxUsageCount: 3 },
]

const routes = { '/blog/posts': <BlogPostsList />, '/blog/posts/new': <BlogPostsNew />, '/blog/posts/:id': <p>writing page</p> }

describe('NewPostDialog', () => {
  it('opens from the list, follows the title into the slug and goes on to the new post', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/blog/posts',
      adminId: 'admin-1',
      routes,
      mocks: [...reads(), { request: { query: CreateBlogPostDocument, variables: captureInput(onCreate) }, result: { data: { createBlogPost: { __typename: 'BlogPost', id: 'bp1' } } } }],
    })

    fireEvent.click(await screen.findByRole('button', { name: 'New post' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Title'), { target: { value: 'Hello World' } })
    expect((within(dialog).getByLabelText('Slug') as HTMLInputElement).value).toBe('hello-world')
    fireEvent.change(within(dialog).getByLabelText('Excerpt (optional)'), { target: { value: 'First post.' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create post' }))

    expect(await screen.findByText('writing page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({
      slug: 'hello-world',
      locale: 'en',
      title: 'Hello World',
      excerpt: { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: 'First post.' }] }] } },
    })
  })

  it('keeps a slug typed by hand, and asks for a title before creating anything', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/blog/posts',
      adminId: 'admin-1',
      routes,
      mocks: [...reads(), { request: { query: CreateBlogPostDocument, variables: captureInput(onCreate) }, result: { data: { createBlogPost: { __typename: 'BlogPost', id: 'bp1' } } } }],
    })

    fireEvent.click(await screen.findByRole('button', { name: 'New post' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Slug'), { target: { value: 'my-own' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create post' }))
    expect(await within(dialog).findByText('Title is required')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(within(dialog).getByLabelText('Title'), { target: { value: 'Something else' } })
    expect((within(dialog).getByLabelText('Slug') as HTMLInputElement).value).toBe('my-own')
  })

  it("doesn't open for someone who may read posts but not write them", async () => {
    renderPage({ path: '/blog/posts/new', adminId: 'admin-1', routes, mocks: [meMock({ superuser: false, permissions: ['blog.view'] }), ...reads().slice(1)] })

    expect(await screen.findByText('No posts match your filters', {}, SLOW).catch(() => screen.findByRole('table', {}, SLOW))).toBeDefined()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByRole('button', { name: 'New post' })).toBeNull()
  })

  it('is already open at the address the old page had, and closing it leaves for the list', async () => {
    renderPage({ path: '/blog/posts/new', adminId: 'admin-1', routes, mocks: reads() })

    const dialog = await screen.findByRole('dialog', {}, SLOW)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), SLOW)
    // Back on the list, where the button opens it again
    fireEvent.click(await screen.findByRole('button', { name: 'New post' }, SLOW))
    expect(await screen.findByRole('dialog')).toBeDefined()
  })
})
