import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ChannelScopeOption, ConfigScopeContextValue } from '@/lib/config-scope'
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
  SetCategoryListingDocument,
  UpdateCategoryDocument,
  UpsertCategoryContentDocument,
} from './Categories.gql'
import { CategoryOptionsDocument } from './category-picker/CategoryOptions.gql'

const toastError = vi.hoisted(() => vi.fn())
vi.mock('sonner', () => ({ toast: { error: toastError } }))

// What these pin: a new category's slug follows its name and the create writes straight through,
// and an edit reviews the category's own fields and its translation, writing each only when it changed -
// product picks and removals stage like any other edit and only write once the Save is confirmed.
// The Stores picker: the header's store starts ticked (every active store in GLOBAL), each ticked store
// gets a listing on create, a failed listing is reported without undoing the create, and a long store
// list grows search and bulk select

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

const store = (id: string, name: string, isActive = true): ChannelScopeOption => ({
  id,
  name,
  defaultLocale: 'en',
  locales: [{ code: 'en' }],
  isActive,
})

const globalWith = (channels: ChannelScopeOption[]): ConfigScopeContextValue => ({ ...globalScope, channels })

const workingIn = (channels: ChannelScopeOption[], current: ChannelScopeOption): ConfigScopeContextValue => ({
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: current.id, locale: '' },
  channels,
  selectedChannel: current,
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: current.id,
})

const option = (id: string, name: string, parentId: string | null = null) => ({
  __typename: 'CategoryOption' as const,
  id,
  parentId,
  slug: name.toLowerCase(),
  name,
  position: 0,
  isPublished: true,
  productCount: 0,
})

// Shoes (cat1) has a child of its own - neither may become its parent
const options = [option('cat1', 'Shoes'), option('cat1-kid', 'Running', 'cat1'), option('cat3', 'Apparel')]

function renderCategories(path: string, opts: { scope?: ConfigScopeContextValue; failListingOn?: string } = {}) {
  const writes = { create: vi.fn(), update: vi.fn(), content: vi.fn(), listing: vi.fn(), add: vi.fn(), remove: vi.fn() }
  const membership = (productId: string) => ({
    __typename: 'Product' as const,
    id: productId,
    categories: [],
    primaryCategory: null,
  })
  renderPage({
    path,
    adminId: 'admin-1',
    scope: opts.scope,
    routes: { '/categories/:id': <CategoriesPage /> },
    mocks: [
      meMock(),
      { request: { query: RootCategoriesDocument }, result: { data: { rootCategories: [] } }, maxUsageCount: 50 },
      { request: { query: CategoryOptionsDocument }, result: { data: { categoryOptions: options } }, maxUsageCount: 50 },
      {
        request: {
          query: SetCategoryListingDocument,
          variables: (v: Record<string, unknown>) => (v.input as { channelId: string }).channelId === opts.failListingOn,
        },
        error: new Error('listing refused'),
        maxUsageCount: 5,
      },
      {
        request: { query: SetCategoryListingDocument, variables: recorder(writes.listing) },
        result: { data: { setCategoryListing: category('cat9', 'womens-shoes', 'Womens Shoes') } },
        maxUsageCount: 20,
      },
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

afterEach(() => {
  localStorage.clear()
  toastError.mockReset()
})

// The Create button renders once the admin's permissions load - the controls are disabled until then
const ready = () => screen.findByRole('button', { name: 'Create' }, SLOW)

async function createNamed(name: string) {
  fireEvent.change(await screen.findByLabelText('Name', {}, SLOW), { target: { value: name } })
  fireEvent.click(await screen.findByRole('button', { name: 'Create' }))
}

// Radix checkboxes are buttons - their state rides aria-checked
const ticked = (el: HTMLElement) => el.getAttribute('aria-checked') === 'true'

const listedOn = (listing: ReturnType<typeof vi.fn>) =>
  listing.mock.calls.map(([v]) => (v as { input: { channelId: string } }).input.channelId).sort()

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

  it('offers neither the category nor its subtree as its parent, and moves it under the one picked', async () => {
    const writes = renderCategories('/categories/cat1')
    await screen.findByLabelText('Position', {}, SLOW)
    const trigger = document.getElementById('cat-parent')!
    expect(trigger.textContent).toContain('None (root)')
    fireEvent.click(trigger)
    const tree = await screen.findByRole('tree', { name: 'Categories' }, SLOW)
    await within(tree).findByText('Apparel', {}, SLOW)
    expect(within(tree).queryByText('Shoes')).toBeNull()
    fireEvent.change(screen.getByRole('combobox', { name: 'Search categories' }), { target: { value: 'run' } })
    expect(await screen.findByText(/No categories match/)).toBeTruthy()
    fireEvent.change(screen.getByRole('combobox', { name: 'Search categories' }), { target: { value: '' } })

    fireEvent.click(await screen.findByText('Apparel'))
    await vi.waitFor(() => expect(document.getElementById('cat-parent')!.textContent).toContain('Apparel'), SLOW)
    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: expect.objectContaining({ id: 'cat1', parentId: 'cat3' }) })
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

  describe('Stores', () => {
    const us = store('ch-us', 'US store')
    const eu = store('ch-eu', 'EU store')

    it('ticks the store the admin works in and lists the new category there', async () => {
      const writes = renderCategories('/categories/new', { scope: workingIn([us, eu], eu) })
      const current = await screen.findByRole('checkbox', { name: /EU store/ }, SLOW)
      expect(ticked(current)).toBe(true)
      expect(ticked(screen.getByRole('checkbox', { name: /US store/ }))).toBe(false)
      expect(screen.getByText(/working here/)).toBeTruthy()
      expect(screen.getByText('Will be added to EU store, but hidden until you switch it to Published.')).toBeTruthy()

      await createNamed('Womens Shoes')
      await vi.waitFor(() => expect(writes.listing).toHaveBeenCalled(), SLOW)
      expect(listedOn(writes.listing)).toEqual(['ch-eu'])
      expect(writes.listing).toHaveBeenCalledWith({
        input: { categoryId: 'cat9', channelId: 'ch-eu', isPublished: true, sortOrderOverride: null },
      })
    })

    it('warns when the current store is unticked, and then lists nowhere', async () => {
      const writes = renderCategories('/categories/new', { scope: workingIn([us, eu], eu) })
      await ready()
      fireEvent.click(screen.getByRole('checkbox', { name: /EU store/ }))
      expect(screen.getByText(/won.t appear in this store.s category list/)).toBeTruthy()
      expect(screen.getByText('Not added to any store - it will only show under Global default.')).toBeTruthy()

      await createNamed('Womens Shoes')
      await vi.waitFor(() => expect(writes.create).toHaveBeenCalled(), SLOW)
      expect(writes.listing).not.toHaveBeenCalled()
    })

    it('starts every active store ticked in GLOBAL, leaving inactive ones out', async () => {
      const outlet = store('ch-outlet', 'Outlet', false)
      const writes = renderCategories('/categories/new', { scope: globalWith([us, eu, outlet]) })
      expect(ticked(await screen.findByRole('checkbox', { name: /Outlet/ }, SLOW))).toBe(false)
      expect(screen.getByText('Inactive')).toBeTruthy()

      await createNamed('Womens Shoes')
      await vi.waitFor(() => expect(writes.listing).toHaveBeenCalledTimes(2), SLOW)
      expect(listedOn(writes.listing)).toEqual(['ch-eu', 'ch-us'])
    })

    it('keeps the create when one listing fails, and names that store', async () => {
      const writes = renderCategories('/categories/new', { scope: globalWith([us, eu]), failListingOn: 'ch-eu' })
      await createNamed('Womens Shoes')
      await vi.waitFor(() => expect(toastError).toHaveBeenCalled(), SLOW)
      expect(toastError.mock.calls[0][0]).toMatch(/couldn't be added to EU store/)
      expect(listedOn(writes.listing)).toEqual(['ch-us'])
    })

    it('grows search and bulk select for a long store list', async () => {
      const many = [
        ...Array.from({ length: 7 }, (_, i) => store(`ch-${i}`, `Store ${i + 1}`)),
        store('ch-eu1', 'EU North'),
        store('ch-eu2', 'EU South'),
      ]
      renderCategories('/categories/new', { scope: globalWith(many) })
      await ready()
      expect(screen.getByText('9 of 9 selected')).toBeTruthy()

      fireEvent.click(screen.getByRole('button', { name: 'Select none' }))
      expect(screen.getByText('0 of 9 selected')).toBeTruthy()

      fireEvent.change(screen.getByLabelText('Search stores'), { target: { value: 'eu' } })
      expect(screen.getAllByRole('checkbox')).toHaveLength(2)
      fireEvent.click(screen.getByRole('button', { name: 'Select shown' }))
      expect(screen.getByText('2 of 9 selected')).toBeTruthy()
      expect(screen.getByText('Will be added to EU North, EU South, but hidden until you switch it to Published.')).toBeTruthy()

      fireEvent.change(screen.getByLabelText('Search stores'), { target: { value: 'nowhere' } })
      expect(screen.getByText(/No stores match/)).toBeTruthy()
    })

    it('keeps the plain list for a handful of stores', async () => {
      renderCategories('/categories/new', { scope: globalWith([us, eu]) })
      await screen.findByRole('checkbox', { name: /US store/ }, SLOW)
      expect(screen.queryByLabelText('Search stores')).toBeNull()
    })
  })
})
