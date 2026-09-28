import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { ProductTypeDetail } from './ProductTypeDetail'
import {
  AttributesDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductTypeUrlPathTemplateDocument,
  UpdateProductTypeDocument,
} from './Catalog.gql'

// What these pin: the general fields go through the page's review, and a changed URL template takes
// the mutation that recomputes the products' paths instead of the plain update (which leaves them stale)

const productType = {
  __typename: 'ProductType' as const,
  id: 'pt1',
  slug: 'tee',
  name: 'T-shirt',
  isDigital: false,
  isShippingRequired: true,
  urlTemplate: '{slug}',
  tracksInventory: false,
  attributes: [],
  groups: [],
}

afterEach(() => localStorage.clear())

describe('ProductTypeDetail', () => {
  it('saves the name through the update and the template through its own mutation', async () => {
    const onUpdate = vi.fn()
    const onTemplate = vi.fn()
    renderPage({
      path: '/catalog/product-types/pt1',
      adminId: 'admin-1',
      routes: { '/catalog/product-types/:id': <ProductTypeDetail productTypeId="pt1" /> },
      mocks: [
        meMock(),
        { request: { query: ProductTypeDocument, variables: { id: 'pt1' } }, result: { data: { productType } }, maxUsageCount: 5 },
        { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 3 },
        { request: { query: AttributesDocument }, result: { data: { attributes: [] } }, maxUsageCount: 3 },
        {
          request: { query: UpdateProductTypeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateProductType: { ...productType, name: 'Tee' } } },
        },
        {
          request: {
            query: SetProductTypeUrlPathTemplateDocument,
            variables: (vars: Record<string, unknown>) => {
              onTemplate(vars)
              return true
            },
          },
          result: { data: { setProductTypeUrlPathTemplate: { __typename: 'ProductType', id: 'pt1' } } },
        },
      ],
    })

    fireEvent.change(await screen.findByLabelText('Name', {}, SLOW), { target: { value: 'Tee' } })
    fireEvent.change(screen.getByLabelText('URL path template'), { target: { value: '{category}/{slug}' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))

    await vi.waitFor(() => expect(onTemplate).toHaveBeenCalledWith({ productTypeId: 'pt1', template: '{category}/{slug}' }), SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'pt1', name: 'Tee' }))
    expect(onUpdate.mock.calls[0][0]).not.toHaveProperty('urlTemplate')
  })
})
