import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CategoriesPage } from './CategoriesPage'
import { AttributesDocument, ProductsDocument } from './Catalog.gql'
import { CategoryDocument, CreateCategoryDocument, RootCategoriesDocument, UpdateCategoryDocument, UpsertCategoryContentDocument } from './Categories.gql'

// What these pin: a new category's slug follows its name and the create writes straight through,
// and an edit reviews the category's own fields and its translation, writing each only when it changed

const category = (id: string, slug: string, name: string) => ({
  __typename: 'Category' as const,
  id,
  slug,
  urlPath: slug,
  position: 0,
  level: 0,
  isPublished: true,
  seoNoindex: false,
  name,
  shortDescription: null,
  description: null,
  metaTitle: null,
  metaDescription: null,
  contentLocale: 'en',
  content: [
    {
      __typename: 'CategoryContent' as const,
      locale: 'en',
      name,
      shortDescription: null,
      description: null,
      metaTitle: null,
      metaDescription: null,
      metaKeywords: null,
      updatedAt: '2026-01-01T00:00:00Z',
    },
  ],
  coverImage: null,
  parent: null,
  ancestors: [],
  products: [],
  listings: [],
  facetOverrides: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderCategories(path: string) {
  const writes = { create: vi.fn(), update: vi.fn(), content: vi.fn() }
  renderPage({
    path,
    adminId: 'admin-1',
    routes: { '/categories/:id': <CategoriesPage /> },
    mocks: [
      meMock(),
      { request: { query: RootCategoriesDocument }, result: { data: { rootCategories: [] } }, maxUsageCount: 10 },
      { request: { query: CategoryDocument, variables: { id: 'cat1' } }, result: { data: { category: category('cat1', 'shoes', 'Shoes') } }, maxUsageCount: 5 },
      { request: { query: CategoryDocument, variables: { id: 'cat9' } }, result: { data: { category: category('cat9', 'womens-shoes', 'Womens Shoes') } }, maxUsageCount: 5 },
      { request: { query: ProductsDocument, variables: () => true }, result: { data: { products: [] } }, maxUsageCount: 5 },
      { request: { query: AttributesDocument }, result: { data: { attributes: [] } }, maxUsageCount: 5 },
      {
        request: { query: CreateCategoryDocument, variables: recorder(writes.create) },
        result: { data: { createCategory: category('cat9', 'womens-shoes', 'Womens Shoes') } },
      },
      {
        request: { query: UpdateCategoryDocument, variables: recorder(writes.update) },
        result: { data: { updateCategory: category('cat1', 'shoes', 'Shoes') } },
      },
      {
        request: { query: UpsertCategoryContentDocument, variables: recorder(writes.content) },
        result: { data: { upsertCategoryContent: category('cat1', 'shoes', 'Shoes') } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('CategoriesPage', () => {
  it('fills the slug from the name and creates the category', async () => {
    const writes = renderCategories('/categories/new')
    fireEvent.change(await screen.findByLabelText('Name', {}, SLOW), { target: { value: 'Womens Shoes' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('womens-shoes')
    fireEvent.click(await screen.findByRole('button', { name: 'Create' }))
    await vi.waitFor(() => expect(writes.create).toHaveBeenCalled(), SLOW)
    expect(writes.create).toHaveBeenCalledWith({
      input: expect.objectContaining({ parentId: null, slug: 'womens-shoes', name: 'Womens Shoes', locale: 'en', position: 0 }),
    })
  })

  it('reviews a structural and a translation edit, and writes both', async () => {
    const writes = renderCategories('/categories/cat1')
    fireEvent.change(await screen.findByLabelText('Position', {}, SLOW), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText('Meta title'), { target: { value: 'All shoes' } })
    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.content).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: expect.objectContaining({ id: 'cat1', position: 3, clearParent: true }) })
    expect(writes.content).toHaveBeenCalledWith({ categoryId: 'cat1', input: expect.objectContaining({ locale: 'en', name: 'Shoes', metaTitle: 'All shoes' }) })
  })
})
