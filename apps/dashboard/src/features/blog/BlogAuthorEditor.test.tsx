import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { BlogAuthorEditor } from './BlogAuthorEditor'
import {
  BlogAuthorDocument,
  BlogAuthorsDocument,
  UpdateBlogAuthorDocument,
  UpsertBlogAuthorContentDocument,
} from './Blog.gql'
import { bioToText } from './schemas/author'

// Through the real page: an edit lights the header Save (it never did - the section registered
// into a context that wasn't there yet), a cleared email is cleared rather than left alone, and
// the bio is written for the locale being edited

const bio = { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: 'Writes about Go.' }] }] } }
const row = { __typename: 'BlogAuthorContent' as const, locale: 'en', name: 'Jane Doe', bio, updatedAt: '2026-01-01T00:00:00Z' }
const author = {
  __typename: 'BlogAuthor' as const,
  id: 'a1',
  slug: 'jane-doe',
  displayName: 'Jane Doe',
  email: 'jane@example.com',
  emailPublic: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  content: row,
  allContent: [row],
}

describe('BlogAuthorEditor', () => {
  it('reviews and saves an edit', async () => {
    const writes = { update: vi.fn(), upsert: vi.fn() }
    renderPage({
      path: '/blog/authors/a1',
      routes: { '/blog/authors/:id': <BlogAuthorEditor />, '/blog/taxonomy': <p>taxonomy</p> },
      mocks: [
        meMock(),
        { request: { query: BlogAuthorDocument, variables: { id: 'a1' } }, result: { data: { blogAuthor: author } }, maxUsageCount: 3 },
        { request: { query: UpdateBlogAuthorDocument, variables: captureInput(writes.update) }, result: { data: { updateBlogAuthor: author } } },
        {
          request: { query: UpsertBlogAuthorContentDocument, variables: captureInput(writes.upsert) },
          result: { data: { upsertBlogAuthorContent: author } },
        },
        { request: { query: BlogAuthorsDocument }, result: { data: { blogAuthors: [] } }, maxUsageCount: 5 },
      ],
    })

    expect(await screen.findByDisplayValue('Writes about Go.')).toBeDefined()
    fireEvent.change(screen.getByLabelText('Email (optional)'), { target: { value: '' } })
    fireEvent.change(screen.getByLabelText('Bio (en, plain text)'), { target: { value: 'Writes about Rust.' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Email')).toBeDefined()
    expect(within(dialog).getByText('Bio')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))

    await screen.findByText('taxonomy', {}, SLOW)
    expect(writes.update).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1', slug: null, email: null, clearEmail: true, emailPublic: false }))
    const upsert = writes.upsert.mock.calls[0][0]
    expect(upsert).toMatchObject({ authorId: 'a1', locale: 'en', name: 'Jane Doe' })
    expect(bioToText(upsert.bio)).toBe('Writes about Rust.')
  })

  it('publishes the email only when the switch is turned on', async () => {
    const writes = { update: vi.fn(), upsert: vi.fn() }
    renderPage({
      path: '/blog/authors/a1',
      routes: { '/blog/authors/:id': <BlogAuthorEditor />, '/blog/taxonomy': <p>taxonomy</p> },
      mocks: [
        meMock(),
        { request: { query: BlogAuthorDocument, variables: { id: 'a1' } }, result: { data: { blogAuthor: author } }, maxUsageCount: 3 },
        { request: { query: UpdateBlogAuthorDocument, variables: captureInput(writes.update) }, result: { data: { updateBlogAuthor: author } } },
        {
          request: { query: UpsertBlogAuthorContentDocument, variables: captureInput(writes.upsert) },
          result: { data: { upsertBlogAuthorContent: author } },
        },
        { request: { query: BlogAuthorsDocument }, result: { data: { blogAuthors: [] } }, maxUsageCount: 5 },
      ],
    })

    fireEvent.click(await screen.findByRole('switch', { name: 'Show email publicly' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Email shown publicly')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('taxonomy', {}, SLOW)
    expect(writes.update).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1', email: 'jane@example.com', emailPublic: true }))
  })

  it('sends an untouched bio back as stored, formatting included', async () => {
    // Bold text - the plain-text editor can't write it, so only the stored document keeps it
    const bold = { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: 'Writes about Go.', format: 1 }] }] } }
    const formatted = { ...author, content: { ...row, bio: bold }, allContent: [{ ...row, bio: bold }] }
    const writes = { update: vi.fn(), upsert: vi.fn() }
    renderPage({
      path: '/blog/authors/a1',
      routes: { '/blog/authors/:id': <BlogAuthorEditor />, '/blog/taxonomy': <p>taxonomy</p> },
      mocks: [
        meMock(),
        { request: { query: BlogAuthorDocument, variables: { id: 'a1' } }, result: { data: { blogAuthor: formatted } }, maxUsageCount: 3 },
        { request: { query: UpdateBlogAuthorDocument, variables: captureInput(writes.update) }, result: { data: { updateBlogAuthor: formatted } } },
        {
          request: { query: UpsertBlogAuthorContentDocument, variables: captureInput(writes.upsert) },
          result: { data: { upsertBlogAuthorContent: formatted } },
        },
        { request: { query: BlogAuthorsDocument }, result: { data: { blogAuthors: [] } }, maxUsageCount: 5 },
      ],
    })

    fireEvent.change(await screen.findByLabelText('Display name'), { target: { value: 'Jane Q. Doe' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('taxonomy', {}, SLOW)
    expect(writes.upsert.mock.calls[0][0]).toMatchObject({ name: 'Jane Q. Doe', bio: bold })
  })
})
