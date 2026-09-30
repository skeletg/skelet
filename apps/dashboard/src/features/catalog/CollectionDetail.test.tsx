import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CollectionDetail } from './CollectionDetail'
import { ProductPickerPagedDocument, ProductTypesDocument } from './Catalog.gql'
import {
  AddProductToCollectionDocument,
  CollectionDocument,
  RemoveProductFromCollectionDocument,
  ReorderCollectionProductsDocument,
  UpdateCollectionDocument,
  UpsertCollectionContentDocument,
} from './Collections.gql'

// What these pin: the translation and the record's own fields are one review, a duplicate metadata
// key and a nameless translation are refused, and each write goes only when its part changed -
// product adds, removals and moves stage, and a reorder is sent only when the order really changed

const product = (id: string, name: string) => ({
  __typename: 'Product' as const,
  id,
  name,
  sku: id.toUpperCase(),
  defaultPrice: 1000,
  defaultCurrencyCode: 'USD',
  thumbnail: null,
})
const members = [product('p1', 'Air Runner'), product('p2', 'Trail Max')]

const collection = {
  __typename: 'Collection' as const,
  id: 'col1',
  slug: 'summer',
  isPublished: false,
  seoNoindex: false,
  name: 'Summer',
  description: null,
  metaTitle: null,
  metaDescription: null,
  metaKeywords: null,
  contentLocale: 'en',
  content: [
    {
      __typename: 'CollectionContent' as const,
      locale: 'en',
      name: 'Summer',
      description: null,
      metaTitle: null,
      metaDescription: null,
      metaKeywords: null,
      updatedAt: '2026-01-01T00:00:00Z',
    },
  ],
  coverImage: null,
  products: [],
  metadata: [{ __typename: 'MetadataItem' as const, key: 'season', value: 'ss26' }],
  privateMetadata: [],
  facetOverrides: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderCollection(products: ReturnType<typeof product>[] = []) {
  const writes = { update: vi.fn(), content: vi.fn(), add: vi.fn(), remove: vi.fn(), reorder: vi.fn() }
  const membership = { __typename: 'Collection' as const, id: 'col1', updatedAt: '2026-01-02T00:00:00Z', products }
  renderPage({
    path: '/collections/col1',
    adminId: 'admin-1',
    routes: { '/collections/:id': <CollectionDetail /> },
    mocks: [
      meMock(),
      { request: { query: CollectionDocument, variables: { id: 'col1' } }, result: { data: { collection: { ...collection, products } } }, maxUsageCount: 5 },
      {
        request: { query: ProductPickerPagedDocument, variables: () => true },
        result: {
          data: {
            productsPaged: {
              __typename: 'ProductPage' as const,
              totalCount: 1,
              items: [
                {
                  ...product('p3', 'City Loafer'),
                  kind: 'SIMPLE' as const,
                  displayImage: null,
                  productType: { __typename: 'ProductType' as const, id: 'pt1', slug: 'shoe', name: 'Shoe' },
                },
              ],
            },
          },
        },
        maxUsageCount: 5,
      },
      { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 5 },
      {
        request: { query: AddProductToCollectionDocument, variables: recorder(writes.add) },
        result: { data: { addProductToCollection: membership } },
        maxUsageCount: 5,
      },
      {
        request: { query: RemoveProductFromCollectionDocument, variables: recorder(writes.remove) },
        result: { data: { removeProductFromCollection: membership } },
        maxUsageCount: 5,
      },
      {
        request: { query: ReorderCollectionProductsDocument, variables: recorder(writes.reorder) },
        result: { data: { reorderCollectionProducts: membership } },
        maxUsageCount: 5,
      },
      {
        request: { query: UpdateCollectionDocument, variables: recorder(writes.update) },
        result: { data: { updateCollection: { __typename: 'Collection', id: 'col1', slug: 'summer' } } },
      },
      {
        request: { query: UpsertCollectionContentDocument, variables: recorder(writes.content) },
        result: { data: { upsertCollectionContent: { __typename: 'Collection', id: 'col1' } } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('CollectionDetail', () => {
  it('refuses a duplicate metadata key, then writes the record and the translation', async () => {
    const writes = renderCollection()
    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Published' }))
    fireEvent.change(screen.getByLabelText('Meta title'), { target: { value: 'Summer edit' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Add row' })[0])
    fireEvent.change(screen.getByLabelText('Metadata 2 key'), { target: { value: 'season' } })
    fireEvent.click(save)
    let dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Duplicate metadata key/)).toBeDefined()
    expect(writes.update).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getByRole('button', { name: 'Remove metadata 2' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.content).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: { id: 'col1', slug: 'summer', isPublished: true, seoNoindex: false } })
    expect(writes.content).toHaveBeenCalledWith(
      expect.objectContaining({ collectionId: 'col1', locale: 'en', input: expect.objectContaining({ name: 'Summer', metaTitle: 'Summer edit' }) }),
    )
  })

  it('refuses a translation edit without a name', async () => {
    const writes = renderCollection()
    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/A translation needs a name/)).toBeDefined()
    expect(writes.content).not.toHaveBeenCalled()
  })

  it('stages a move and sends the new order on save', async () => {
    const writes = renderCollection(members)
    fireEvent.click(await screen.findByRole('button', { name: 'Move Trail Max up' }, SLOW))
    expect(writes.reorder).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const confirm = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(confirm).getByText('Product order')).toBeTruthy()
    fireEvent.click(within(confirm).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.reorder).toHaveBeenCalledTimes(1), SLOW)
    expect(writes.reorder).toHaveBeenCalledWith({ collectionId: 'col1', productIds: ['p2', 'p1'] })
  })

  it('stages a pick and a removal, and writes them without a reorder', async () => {
    const writes = renderCollection(members)
    fireEvent.click(await screen.findByRole('button', { name: 'Add products' }, SLOW))
    const picker = await screen.findByRole('dialog', {}, SLOW)
    fireEvent.click(await within(picker).findByRole('checkbox', { name: 'Select City Loafer' }, SLOW))
    fireEvent.click(within(picker).getByRole('button', { name: 'Add 1 product' }))
    await screen.findByText('Will add', {}, SLOW)

    fireEvent.keyDown(screen.getByRole('button', { name: 'Actions for Air Runner' }), { key: 'Enter' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Remove from collection/ }, SLOW))
    await screen.findByText('Will remove', {}, SLOW)
    expect(writes.add).not.toHaveBeenCalled()
    expect(writes.remove).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }))
    const confirm = await screen.findByRole('alertdialog', {}, SLOW)
    fireEvent.click(within(confirm).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.add).toHaveBeenCalledTimes(1), SLOW)
    expect(writes.add).toHaveBeenCalledWith(expect.objectContaining({ collectionId: 'col1', productId: 'p3' }))
    expect(writes.remove).toHaveBeenCalledWith({ collectionId: 'col1', productId: 'p1' })
    expect(writes.reorder).not.toHaveBeenCalled()
  })
})
