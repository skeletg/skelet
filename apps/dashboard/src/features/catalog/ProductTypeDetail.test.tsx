import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { ProductTypeDetail } from './ProductTypeDetail'
import {
  AttributesDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductTypeUrlPathTemplateDocument,
  SetProductTypeDeliveryKindsDocument,
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
  deliveryKinds: [],
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


it.each([false, true])('saves explicit delivery-kind restrictions and the all-kinds policy (all=%s)', async (all) => {
  const onKinds = vi.fn()
  const record = { ...productType, deliveryKinds: all ? ['ship_to_address'] : [] }
  renderPage({
    path: '/catalog/product-types/pt1', adminId: 'admin-1',
    routes: { '/catalog/product-types/:id': <ProductTypeDetail productTypeId="pt1" /> },
    mocks: [
      meMock(),
      { request: { query: ProductTypeDocument, variables: { id: 'pt1' } }, result: { data: { productType: record } }, maxUsageCount: 5 },
      { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 5 },
      { request: { query: AttributesDocument }, result: { data: { attributes: [] } }, maxUsageCount: 5 },
      { request: { query: UpdateProductTypeDocument, variables: () => true }, result: { data: { updateProductType: record } } },
      { request: { query: SetProductTypeDeliveryKindsDocument, variables: (v: Record<string, unknown>) => (onKinds(v), true) }, result: { data: { setProductTypeDeliveryKinds: { __typename: 'ProductType', id: 'pt1' } } } },
    ],
  })
  fireEvent.click(await screen.findByRole('radio', { name: all ? 'All offered kinds' : 'Selected kinds' }, SLOW))
  if (!all) {
    fireEvent.click(screen.getByRole('checkbox', { name: 'Pickup point' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Nearest pickup point' }))
  }
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('Allowed delivery kinds')).toBeDefined()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await vi.waitFor(() => expect(onKinds).toHaveBeenCalledWith({ productTypeId: 'pt1', kinds: all ? null : ['ship_to_address', 'store_pickup'] }), SLOW)
})
