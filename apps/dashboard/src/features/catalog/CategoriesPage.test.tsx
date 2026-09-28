import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CategoriesPage } from './CategoriesPage'
import {
  AddProductToCategoryDocument,
  AttributesDocument,
  ProductPickerPagedDocument,
  ProductTypesDocument,
  RemoveProductFromCategoryDocument,
} from './Catalog.gql'
import {
  CategoryDocument,
  CreateCategoryDocument,
  RootCategoriesDocument,
  UpdateCategoryDocument,
  UpsertCategoryContentDocument,
} from './Categories.gql'

// What these pin: a new category's slug follows its name and the create writes straight through,
// and an edit reviews the category's own fields and its translation, writing each only when it changed -
// product picks and removals stage like any other edit and only write once the Save is confirmed

const product = (id: string, name: string, sku: string) => ({
  __typename: 'Product' as const,
  id,
  name,
  sku,
  kind: 'SIMPLE' as const,
  thumbnail: null,
  displayImage: null,
})

const category = (id: string, slug: string, name: string, products: ReturnType<typeof product>[] = []) => ({
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
  products,
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
  const writes = { create: vi.fn(), update: vi.fn(), content: vi.fn(), add: vi.fn(), remove: vi.fn() }
  const membership = (productId: string) => ({
    __typename: 'Product' as const,
    id: productId,
    categories: [],
    primaryCategory: null,
  })
  renderPage({
    path,
    adminId: 'admin-1',
    routes: { '/categories/:id': <CategoriesPage /> },
    mocks: [
      meMock(),
      { request: { query: RootCategoriesDocument }, result: { data: { rootCategories: [] } }, maxUsageCount: 10 },
      { request: { query: CategoryDocument, variables: { id: 'cat1' } }, result: { data: { category: category('cat1', 'shoes', 'Shoes') } }, maxUsageCount: 5 },
      { request: { query: CategoryDocument, variables: { id: 'cat9' } }, result: { data: { category: category('cat9', 'womens-shoes', 'Womens Shoes') } }, maxUsageCount: 5 },
      {
        request: { query: CategoryDocument, variables: { id: 'cat2' } },
        result: { data: { category: category('cat2', 'boots', 'Boots', [product('p1', 'Air Runner', 'SKU-1')]) } },
        maxUsageCount: 5,
      },
      {
        request: { query: ProductPickerPagedDocument, variables: () => true },
        result: {
          data: {
            productsPaged: {
              __typename: 'ProductPage' as const,
              totalCount: 2,
              items: ['p1', 'p2'].map((id, i) => ({
                ...product(id, i === 0 ? 'Air Runner' : 'Trail Max', `SKU-${i + 1}`),
                defaultPrice: 1000,
                defaultCurrencyCode: 'USD',
                productType: { __typename: 'ProductType' as const, id: 'pt1', slug: 'shoe', name: 'Shoe' },
              })),
            },
          },
        },
        maxUsageCount: 5,
      },
      { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 5 },
      {
        request: { query: AddProductToCategoryDocument, variables: recorder(writes.add) },
        result: (vars: Record<string, unknown>) => ({ data: { addProductToCategory: membership(String(vars.productId)) } }),
        maxUsageCount: 5,
      },
      {
        request: { query: RemoveProductFromCategoryDocument, variables: recorder(writes.remove) },
        result: (vars: Record<string, unknown>) => ({ data: { removeProductFromCategory: membership(String(vars.productId)) } }),
        maxUsageCount: 5,
      },
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

  it('stages products picked in the dialog and adds them only on save', async () => {
    const writes = renderCategories('/categories/cat1')
    fireEvent.click(await screen.findByRole('button', { name: 'Add products' }, SLOW))
    const picker = await screen.findByRole('dialog', {}, SLOW)
    fireEvent.click(await within(picker).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW))
    fireEvent.click(within(picker).getByRole('checkbox', { name: 'Select Trail Max' }))
    fireEvent.click(within(picker).getByRole('button', { name: 'Add 2 products' }))

    expect(await screen.findAllByText('Will add', {}, SLOW)).toHaveLength(2)
    expect(writes.add).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const confirm = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(confirm).getByText('Air Runner')).toBeTruthy()
    expect(within(confirm).getByText('Trail Max')).toBeTruthy()
    fireEvent.click(within(confirm).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.add).toHaveBeenCalledTimes(2), SLOW)
    expect(writes.add).toHaveBeenCalledWith(expect.objectContaining({ productId: 'p1', categoryId: 'cat1' }))
    expect(writes.add).toHaveBeenCalledWith(expect.objectContaining({ productId: 'p2', categoryId: 'cat1' }))
  })

  it('locks products already in the category inside the picker', async () => {
    renderCategories('/categories/cat2')
    fireEvent.click(await screen.findByRole('button', { name: 'Add products' }, SLOW))
    const picker = await screen.findByRole('dialog', {}, SLOW)
    const taken = await within(picker).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW)
    expect(taken.hasAttribute('disabled')).toBe(true)
    expect(within(picker).getByText('In category')).toBeTruthy()
  })

  it('stages a removal, undoes it, and removes on save', async () => {
    const writes = renderCategories('/categories/cat2')
    const openActions = async () => {
      const trigger = await screen.findByRole('button', { name: 'Actions for Air Runner' }, SLOW)
      fireEvent.keyDown(trigger, { key: 'Enter' })
    }
    await openActions()
    fireEvent.click(await screen.findByRole('menuitem', { name: /Remove from category/ }, SLOW))
    expect(await screen.findByText('Will remove', {}, SLOW)).toBeTruthy()

    await openActions()
    fireEvent.click(await screen.findByRole('menuitem', { name: /Undo/ }, SLOW))
    await vi.waitFor(() => expect(screen.queryByText('Will remove')).toBeNull(), SLOW)

    await openActions()
    fireEvent.click(await screen.findByRole('menuitem', { name: /Remove from category/ }, SLOW))
    await screen.findByText('Will remove', {}, SLOW)
    expect(writes.remove).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const confirm = await screen.findByRole('alertdialog', {}, SLOW)
    fireEvent.click(within(confirm).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.remove).toHaveBeenCalledTimes(1), SLOW)
    expect(writes.remove).toHaveBeenCalledWith(expect.objectContaining({ productId: 'p1', categoryId: 'cat2' }))
  })
})
