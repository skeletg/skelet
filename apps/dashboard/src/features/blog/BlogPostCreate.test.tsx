import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { BlogPostCreate } from './BlogPostCreate'
import { CreateBlogPostDocument } from './Blog.gql'

describe('BlogPostCreate', () => {
  it('derives the slug from the title and wraps the excerpt as rich text', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/blog/posts/new',
      routes: { '/blog/posts/new': <BlogPostCreate />, '/blog/posts/:id': <p>post editor</p> },
      mocks: [
        { request: { query: LocalesPickerDocument }, result: { data: { locales: [] } } },
        {
          request: { query: CreateBlogPostDocument, variables: captureInput(onCreate) },
          result: { data: { createBlogPost: { __typename: 'BlogPost', id: 'bp1' } } },
        },
      ],
    })
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Hello World' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('hello-world')
    fireEvent.change(screen.getByLabelText('Excerpt (optional)'), { target: { value: 'First post.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create post' }))
    expect(await screen.findByText('post editor', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({
      slug: 'hello-world',
      locale: 'en',
      title: 'Hello World',
      excerpt: { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: 'First post.' }] }] } },
    })
  })
})
