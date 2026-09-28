import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { AddProductToCategoryDocument, RemoveProductFromCategoryDocument, SetProductPrimaryCategoryDocument } from '../Catalog.gql'
import { RootCategoriesDocument } from '../Categories.gql'
import { ProductCategoriesPicker } from './ProductCategoriesPicker'

// What this pins: moving a product to a new category adds it first, makes it primary, and only then
// removes the old one - so the product never sits without a primary

const category = (id: string, name: string) => ({
  __typename: 'Category' as const,
  id,
  slug: name.toLowerCase(),
  urlPath: name.toLowerCase(),
  position: 0,
  level: 0,
  isPublished: true,
  seoNoindex: false,
  name,
  productCount: 0,
  content: [],
  children: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

const recorder = (calls: string[], name: string) => (vars: Record<string, unknown>) => {
  calls.push(`${name}:${String(vars.categoryId)}`)
  return true
}

describe('ProductCategoriesPicker', () => {
  it('adds, sets the primary, then removes', async () => {
    const calls: string[] = []
    const shirts = { id: 'k1', slug: 'shirts', urlPath: 'shirts', name: 'Shirts' }
    const moved = { __typename: 'Product' as const, id: 'p1', categories: [], primaryCategory: null }
    renderSection(
      <ProductCategoriesPicker productId="p1" categories={[shirts]} primaryCategory={shirts} canManage productVariables={{ id: 'p1' }} />,
      [
        { request: { query: RootCategoriesDocument, variables: {} }, result: { data: { rootCategories: [category('k1', 'Shirts'), category('k2', 'Sale')] } }, maxUsageCount: 3 },
        { request: { query: AddProductToCategoryDocument, variables: recorder(calls, 'add') }, result: { data: { addProductToCategory: moved } } },
        { request: { query: SetProductPrimaryCategoryDocument, variables: recorder(calls, 'primary') }, result: { data: { setProductPrimaryCategory: moved } } },
        { request: { query: RemoveProductFromCategoryDocument, variables: recorder(calls, 'remove') }, result: { data: { removeProductFromCategory: moved } } },
      ],
    )
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Assign Sale' }, SLOW))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Unassign Shirts' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 3 changes/ }))
    await vi.waitFor(() => expect(calls).toHaveLength(3), SLOW)
    expect(calls).toEqual(['add:k2', 'primary:k2', 'remove:k1'])
  })
})
