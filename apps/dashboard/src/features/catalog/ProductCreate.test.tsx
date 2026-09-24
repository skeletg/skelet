import { fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { draftKey, writeDraft } from '@/lib/form-drafts'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { ProductCreate } from './ProductCreate'
import { CreateProductDocument, ProductTypeDocument, ProductTypesDocument, SetProductAttributesDocument } from './Catalog.gql'

// What these pin: a type's required attribute is asked for before anything is created, the type
// survives a refresh (it lives in the form, not beside it), and the attribute values follow the create

const attribute = {
  __typename: 'Attribute' as const,
  id: 'attr-material',
  slug: 'material',
  name: 'Material',
  inputType: 'PLAIN_TEXT' as const,
  unit: '',
  valueRequired: true,
  filterable: false,
  searchable: false,
  isPublic: true,
  facetCollapsed: false,
  isIndexable: false,
  h1Position: 'BEFORE' as const,
  synapseTrack: false,
  autoSegment: false,
  values: [],
}

const productType = {
  __typename: 'ProductType' as const,
  id: 'pt-tee',
  slug: 'tee',
  name: 'T-shirt',
  isDigital: false,
  isShippingRequired: true,
  urlTemplate: '',
  tracksInventory: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

afterEach(() => localStorage.clear())

describe('ProductCreate', () => {
  it('asks for a required attribute, then creates the product and writes its values', async () => {
    // A refreshed page: name and type come back from the stored draft
    writeDraft(draftKey('admin-1', null, 'product:new'), {
      name: { base: '', value: 'Classic tee' },
      productTypeId: { base: '', value: 'pt-tee' },
    })
    const writes = { create: vi.fn(), attrs: vi.fn() }
    renderPage({
      path: '/products/new',
      adminId: 'admin-1',
      routes: { '/products/new': <ProductCreate />, '/products/:id': <p>product page</p> },
      mocks: [
        meMock(),
        { request: { query: ProductTypesDocument }, result: { data: { productTypes: [{ ...productType, attributes: [] }] } } },
        {
          request: { query: ProductTypeDocument, variables: { id: 'pt-tee' } },
          result: { data: { productType: { ...productType, attributes: [{ __typename: 'ProductTypeAttribute', sortOrder: 0, groupId: null, attribute }], groups: [] } } },
          maxUsageCount: 3,
        },
        { request: { query: ChannelsDocument }, result: { data: { channels: [] } } },
        { request: { query: CurrenciesPickerDocument }, result: { data: { currencies: [] } } },
        {
          request: { query: CreateProductDocument, variables: captureInput(writes.create) },
          result: { data: { createProduct: { __typename: 'Product', id: 'p1' } } },
        },
        {
          request: {
            query: SetProductAttributesDocument,
            variables: (vars: Record<string, unknown>) => {
              writes.attrs(vars)
              return true
            },
          },
          result: { data: { setProductAttributes: { __typename: 'Product', id: 'p1' } } },
        },
      ],
    })

    expect(await screen.findByText(/Restored unsaved changes/)).toBeDefined()
    fireEvent.click(await screen.findByRole('button', { name: 'Create product' }))
    expect(await screen.findByText('Material is required')).toBeDefined()
    expect(writes.create).not.toHaveBeenCalled()

    fireEvent.change(screen.getByPlaceholderText('Enter a value'), { target: { value: 'Cotton' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create product' }))
    expect(await screen.findByText('product page', {}, SLOW)).toBeDefined()
    expect(writes.create).toHaveBeenCalledWith(
      expect.objectContaining({ productTypeId: 'pt-tee', kind: 'SIMPLE', name: 'Classic tee', locale: 'en', defaultPrice: 0 }),
    )
    expect(writes.attrs).toHaveBeenCalledWith(
      expect.objectContaining({ productId: 'p1', assignments: [expect.objectContaining({ attributeId: 'attr-material' })] }),
    )
  })
})
