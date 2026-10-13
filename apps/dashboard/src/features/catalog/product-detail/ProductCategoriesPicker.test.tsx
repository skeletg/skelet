import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { AddProductToCategoryDocument, RemoveProductFromCategoryDocument, SetProductPrimaryCategoryDocument } from '../Catalog.gql'
import { CategoryOptionsDocument } from '../category-picker/CategoryOptions.gql'
import { ProductCategoriesPicker } from './ProductCategoriesPicker'

// What this pins: the assigned categories list by path, a pick from the dialog only stages, and moving
// a product to a new category adds it first, makes it primary, and only then removes the old one -
// so the product never sits without a primary

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

const options = [option('k0', 'Apparel'), option('k1', 'Shirts', 'k0'), option('k2', 'Sale')]

const recorder = (calls: string[], name: string) => (vars: Record<string, unknown>) => {
  calls.push(`${name}:${String(vars.categoryId)}`)
  return true
}

function render(calls: string[] = []) {
  const shirts = { id: 'k1', slug: 'shirts', urlPath: 'apparel/shirts', name: 'Shirts' }
  const moved = { __typename: 'Product' as const, id: 'p1', categories: [], primaryCategory: null }
  renderSection(
    <ProductCategoriesPicker productId="p1" categories={[shirts]} primaryCategory={shirts} canManage productVariables={{ id: 'p1' }} />,
    [
      { request: { query: CategoryOptionsDocument }, result: { data: { categoryOptions: options } }, maxUsageCount: 3 },
      { request: { query: AddProductToCategoryDocument, variables: recorder(calls, 'add') }, result: { data: { addProductToCategory: moved } } },
      { request: { query: SetProductPrimaryCategoryDocument, variables: recorder(calls, 'primary') }, result: { data: { setProductPrimaryCategory: moved } } },
      { request: { query: RemoveProductFromCategoryDocument, variables: recorder(calls, 'remove') }, result: { data: { removeProductFromCategory: moved } } },
    ],
  )
}

async function addFromDialog(name: string) {
  fireEvent.click(await screen.findByRole('button', { name: 'Add categories' }, SLOW))
  const dialog = await screen.findByRole('dialog', {}, SLOW)
  fireEvent.change(within(dialog).getByRole('combobox', { name: 'Search categories' }), { target: { value: name } })
  fireEvent.click(await within(dialog).findByRole('checkbox', { name: `Select ${name}` }, SLOW))
  fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 category' }))
}

describe('ProductCategoriesPicker', () => {
  it('lists an assigned category by its path, and locks it in the dialog', async () => {
    render()
    const list = await screen.findByRole('list', { name: 'Assigned categories' })
    await within(list).findByTitle('Apparel › Shirts', {}, SLOW)

    fireEvent.click(screen.getByRole('button', { name: 'Add categories' }))
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    const taken = await within(dialog).findByRole('checkbox', { name: 'Select Shirts' }, SLOW)
    expect(taken.hasAttribute('disabled')).toBe(true)
    expect(within(dialog).getByText('Assigned')).toBeTruthy()
  })

  it('adds, sets the primary, then removes', async () => {
    const calls: string[] = []
    render(calls)
    await addFromDialog('Sale')
    expect(await screen.findByText('Will add')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Make Sale the primary category' }))
    fireEvent.click(screen.getByRole('button', { name: 'Remove Shirts' }))
    expect(screen.getByText('Will remove')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const confirm = await screen.findByRole('alertdialog')
    fireEvent.click(within(confirm).getByRole('button', { name: /Apply 3 changes/ }))
    await vi.waitFor(() => expect(calls).toHaveLength(3), SLOW)
    expect(calls).toEqual(['add:k2', 'primary:k2', 'remove:k1'])
  })

  it('undoes a staged removal in place', async () => {
    render()
    fireEvent.click(await screen.findByRole('button', { name: 'Remove Shirts' }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Undo removing Shirts' }))
    expect(screen.queryByText('Will remove')).toBeNull()
    expect(screen.getByRole('button', { name: 'Shirts is the primary category' })).toBeTruthy()
  })
})
