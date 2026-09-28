import { act, fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { BlogCategoryEditor } from './BlogCategoryEditor'
import { BlogTagEditor } from './BlogTagEditor'
import {
  BlogCategoriesDocument,
  BlogTagDocument,
  BlogTagsDocument,
  CreateBlogCategoryDocument,
  UpdateBlogTagDocument,
  UpsertBlogTagContentDocument,
} from './Blog.gql'

// Categories and tags share one editor. What these pin through the real pages: an edit lights the
// header Save (it never did - the section registered into a context that wasn't there yet), a
// renamed tag writes its name for the locale being edited, admins without blog.manage get a
// read-only page, and a category can't be created without a name

// Radix's Switch measures itself inside a <form>, and jsdom has no ResizeObserver
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver

const content = (name: string) => ({
  __typename: 'BlogTagContent' as const,
  locale: 'en',
  name,
  description: null,
  metaTitle: null,
  metaDescription: null,
})

const tag = {
  __typename: 'BlogTag' as const,
  id: 't1',
  slug: 'release-notes',
  isPublished: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  content: content('Release notes'),
  allContent: [{ ...content('Release notes'), metaKeywords: null, updatedAt: '2026-01-01T00:00:00Z' }],
}

function renderTag(opts: { superuser?: boolean; permissions?: string[] } = {}) {
  const writes = { update: vi.fn(), upsert: vi.fn() }
  renderPage({
    path: '/blog/tags/t1',
    routes: {
      '/blog/tags/:id': <BlogTagEditor />,
      '/blog/taxonomy': <p>taxonomy</p>,
    },
    mocks: [
      meMock(opts),
      { request: { query: BlogTagDocument, variables: { id: 't1' } }, result: { data: { blogTag: tag } }, maxUsageCount: 3 },
      { request: { query: UpdateBlogTagDocument, variables: captureInput(writes.update) }, result: { data: { updateBlogTag: tag } } },
      {
        request: { query: UpsertBlogTagContentDocument, variables: captureInput(writes.upsert) },
        result: { data: { upsertBlogTagContent: tag } },
      },
      { request: { query: BlogTagsDocument }, result: { data: { blogTags: [] } }, maxUsageCount: 5 },
    ],
  })
  return writes
}

describe('BlogTagEditor', () => {
  it('reviews a rename and writes it for the locale being edited', async () => {
    const writes = renderTag()
    fireEvent.change(await screen.findByLabelText('Name (en)'), { target: { value: 'Changelog' } })
    const save = (await screen.findByRole('button', { name: 'Save changes' })) as HTMLButtonElement
    expect(save.disabled).toBe(false)
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Changelog')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('taxonomy', {}, SLOW)
    // Slug untouched, so it isn't re-sent
    expect(writes.update).toHaveBeenCalledWith({ id: 't1', slug: null, isPublished: true })
    expect(writes.upsert).toHaveBeenCalledWith({ tagId: 't1', locale: 'en', name: 'Changelog' })
  })

  it('is read-only without blog.manage', async () => {
    renderTag({ superuser: false, permissions: ['blog.view'] })
    const name = (await screen.findByLabelText('Name (en)')) as HTMLInputElement
    // Let the mocked me query land - before it does every page is read-only, which would pass this vacuously
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)))
    expect(screen.getByRole('link', { name: 'Cancel' })).toBeDefined()
    expect(name.closest('fieldset')?.disabled).toBe(true)
    expect(screen.queryByRole('button', { name: 'Save changes' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
  })
})

describe('BlogCategoryEditor', () => {
  it('needs a name to create a category', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/blog/categories/new',
      routes: { '/blog/categories/:id': <BlogCategoryEditor />, '/blog/taxonomy': <p>taxonomy</p> },
      mocks: [
        meMock(),
        {
          request: { query: CreateBlogCategoryDocument, variables: captureInput(onCreate) },
          result: { data: { createBlogCategory: { __typename: 'BlogCategory', id: 'c1' } } },
        },
        { request: { query: BlogCategoriesDocument }, result: { data: { blogCategories: [] } }, maxUsageCount: 5 },
      ],
    })
    fireEvent.change(await screen.findByLabelText('Slug'), { target: { value: 'news' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Create category' }))
    expect(await screen.findByText(/Fix the highlighted field: Name is required/)).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Name (en)'), { target: { value: 'News' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create category' }))
    await screen.findByText('taxonomy', {}, SLOW)
    expect(onCreate).toHaveBeenCalledWith({ slug: 'news', name: 'News', isPublished: false, locale: 'en' })
  })
})
