import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { DeleteProductDocument, ProductDocument } from './Catalog.gql'
import { ProductDetail } from './ProductDetail'

// What this pins: deleting a product goes through the shared ConfirmDialog - a refused delete keeps
// it open with the server's message, a successful one goes back to the product list

// The smallest product the page renders: a simple product with no media, options or assignments
const product = {
  __typename: 'Product' as const,
  id: 'p1',
  name: 'Tee',
  kind: 'SIMPLE' as const,
  variantMode: null,
  contentLocale: 'en',
  shortDescription: null,
  canonicalUrl: null,
  url: 'tee',
  urlAuto: true,
  urlTemplateOverride: '',
  tracksInventoryOverride: null,
  tracksInventory: true,
  quantityDecimals: 0,
  quantityStep: 1,
  compareAtPrice: null,
  specialPrice: null,
  specialFrom: null,
  specialTo: null,
  price: 1000,
  onSale: false,
  weightGrams: null,
  lengthMm: null,
  widthMm: null,
  heightMm: null,
  minOrderQty: null,
  maxOrderQty: null,
  costCents: null,
  groupPrices: [],
  metaDescription: null,
  metaTitle: null,
  seoNoindex: false,
  description: null,
  descriptionRichText: null,
  categories: [],
  primaryCategory: null,
  content: [],
  mainImage: null,
  displayImage: null,
  thumbnail: null,
  ogImage: null,
  media: [],
  sku: 'TEE-1',
  taxClass: 'standard',
  defaultPrice: 1000,
  defaultCurrencyCode: 'EUR',
  metadata: [],
  privateMetadata: [],
  listings: [],
  productType: { __typename: 'ProductType' as const, id: 'pt1', slug: 'apparel', name: 'Apparel', attributes: [], groups: [] },
  attributeValues: [],
  children: [],
  configurableOptions: [],
  bundleOptions: [],
  kitComponents: [],
  groupedMembers: [],
  productOptions: [],
  downloads: [],
  subscriptionTerms: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('ProductDetail delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/products/p1',
      routes: { '/products/:id': <ProductDetail />, '/products': <p>Products list</p> },
      mocks: [meMock(), { request: { query: ProductDocument, variables: { id: 'p1' } }, result: { data: { product } }, maxUsageCount: 3 }, mock],
      adminId: 'admin-1',
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete product' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteProductDocument, variables: { id: 'p1' } }, result: { errors: [{ message: 'product has orders' }] } })
    expect(await within(dialog).findByText('product has orders', {}, SLOW)).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteProductDocument, variables: { id: 'p1' } }, result: { data: { deleteProduct: true } } })
    expect(await screen.findByText('Products list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
