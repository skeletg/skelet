import { PrefsContext } from '@/lib/admin-prefs'
import { draftKey, writeDraft } from '@/lib/form-drafts'
import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { RecommendationProductsDocument, ProductLinksDocument, RecommendationSearchDocument, ProductPickerPagedDocument, ProductTypesDocument, SetProductLinksDocument } from '../Catalog.gql'
import { RecommendationsSection } from './RecommendationsSection'

// What these pin: the three lists stage as one section, and only the kinds that changed are written

const links = (kind: string, ids: string[]) => ({
  request: { query: ProductLinksDocument, variables: { productId: 'p1', kind } },
  result: { data: { productLinks: ids } },
  maxUsageCount: 3,
})

const slim = (id: string, name: string) => ({ __typename: 'Product', id, name, sku: name.toUpperCase(), thumbnail: null, displayImage: null })

function renderLinks(canManage = true, options: { metadataError?: boolean; missing?: boolean; searchError?: boolean; emptySearch?: boolean; adminId?: string } = {}) {
  const write = vi.fn()
  renderSection(<PrefsContext.Provider value={options.adminId ?? ''}><RecommendationsSection productId="p1" canManage={canManage} /></PrefsContext.Provider>, [
    links('RELATED', ['p2', 'p3']),
    links('UPSELL', []),
    links('CROSSSELL', []),
    ...(options.metadataError ? [{ request: { query: RecommendationProductsDocument, variables: () => true }, error: new Error('Metadata unavailable') }] : []),
    ...(options.missing ? [{ request: { query: RecommendationProductsDocument, variables: () => true }, result: { data: { productsByIds: [slim('p3', 'Cap')] } } }] : []),
    {
      request: { query: RecommendationProductsDocument, variables: () => true },
      result: { data: { productsByIds: [slim('p2', 'Mug'), slim('p3', 'Cap')] } },
      maxUsageCount: 20,
    },
    ...(options.searchError ? [{ request: { query: RecommendationSearchDocument, variables: { search: 'soc', limit: 10 } }, error: new Error('Search unavailable') }] : []),
    {
      request: { query: RecommendationSearchDocument, variables: { search: 'soc', limit: 10 } },
      result: {
        data: {
          productsPaged: {
            __typename: 'ProductPage',
            items: options.emptySearch ? [] : [slim('p1', 'Self'), slim('p2', 'Mug'), slim('p4', 'Socks')],
          },
        },
      },
    },
    {
      request: { query: ProductPickerPagedDocument, variables: () => true },
      result: { data: { productsPaged: { totalCount: 5, items: ['p1', 'p2', 'p3', 'p4', 'p5'].map((id, i) => ({ ...slim(id, ['Self', 'Mug', 'Cap', 'Socks', 'Shirt'][i]), kind: 'SIMPLE', defaultPrice: 500, defaultCurrencyCode: 'EUR', productType: { id: 'simple', name: 'Simple', slug: 'simple' } })) } } },
      maxUsageCount: 10,
    },
    { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 10 },
    {
      request: {
        query: SetProductLinksDocument,
        variables: (vars: Record<string, unknown>) => {
          write(vars)
          return true
        },
      },
      result: { data: { setProductLinks: true } },
      maxUsageCount: 3,
    },
  ])
  return write
}

async function apply(changes: RegExp) {
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
  return dialog
}

describe('RecommendationsSection', () => {
  it('renders product details and stages order changes through Save', async () => {
    const write = renderLinks()
    const down = await screen.findByRole('button', { name: 'Move Mug down in Related' }, SLOW)
    expect(screen.getByText('MUG')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Move Mug up in Related' })).toHaveProperty('disabled', true)
    fireEvent.click(down)
    expect(write).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Mug (MUG), Cap (CAP)')).toBeTruthy()
    expect(within(dialog).getByText('Cap (CAP), Mug (MUG)')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3', 'p2'] }), SLOW)
  })

  it('clears dirty state when the original order is restored', async () => {
    renderLinks()
    fireEvent.click(await screen.findByRole('button', { name: 'Move Mug down in Related' }, SLOW))
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveProperty('disabled', false)
    fireEvent.click(screen.getByRole('button', { name: 'Move Mug up in Related' }))
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveProperty('disabled', true)
  })

  it('supports keyboard search and excludes self and existing links', async () => {
    const write = renderLinks()
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    const input = screen.getByRole('combobox', { name: 'Add to Related' })
    fireEvent.change(input, { target: { value: 'soc' } })
    const option = await screen.findByRole('option', { name: /Socks/ }, SLOW)
    expect(screen.getAllByRole('option')).toHaveLength(1)
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.queryByRole('option')).toBeNull()
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    expect(input.getAttribute('aria-activedescendant')).toBe(option.id)
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(screen.getByRole('button', { name: 'Remove Socks from Related' })).toBeTruthy()
    expect(write).not.toHaveBeenCalled()
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p2', 'p3', 'p4'] }), SLOW)
  })

  it('disables edits for read-only viewers', async () => {
    renderLinks(false)
    expect(await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)).toHaveProperty('disabled', true)
    expect(screen.getByRole('button', { name: 'Reorder Mug' })).toHaveProperty('disabled', true)
    expect(screen.getByRole('combobox', { name: 'Add to Related' })).toHaveProperty('disabled', true)
    expect(screen.getByRole('button', { name: 'Browse products to add to Related' })).toHaveProperty('disabled', true)
  })

  it('stages a multi-product Browse selection at the end of the list', async () => {
    const write = renderLinks()
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Browse products to add to Related' }))
    const dialog = await screen.findByRole('dialog')
    const socks = await within(dialog).findByRole('checkbox', { name: 'Select Socks' }, SLOW)
    expect(within(dialog).queryByRole('checkbox', { name: 'Select Self' })).toBeNull()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Mug' })).toHaveProperty('disabled', true)
    fireEvent.click(socks)
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Select Shirt' }))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 2 products' }))
    expect(await screen.findByRole('button', { name: 'Remove Shirt from Related' }, SLOW)).toBeTruthy()
    expect(write).not.toHaveBeenCalled()
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p2', 'p3', 'p4', 'p5'] }), SLOW)
  })

  it('preserves staged ordering when missing metadata is refetched', async () => {
    const write = renderLinks(true, { missing: true })
    const up = await screen.findByRole('button', { name: 'Move Cap up in Related' }, SLOW)
    expect(screen.getByText('Unavailable product')).toBeTruthy()
    fireEvent.click(up)
    fireEvent.click(screen.getByRole('button', { name: 'Retry product details' }))
    await screen.findByRole('button', { name: 'Move Mug up in Related' }, SLOW)
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3', 'p2'] }), SLOW)
  })

  it('offers retry after metadata loading fails without deleting links', async () => {
    const write = renderLinks(true, { metadataError: true })
    fireEvent.click(await screen.findByRole('button', { name: 'Retry product details' }, SLOW))
    expect(await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)).toBeTruthy()
    expect(write).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveProperty('disabled', true)
  })

  it('shows a search error and recovers on retry', async () => {
    renderLinks(true, { searchError: true })
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    fireEvent.change(screen.getByLabelText('Add to Related'), { target: { value: 'soc' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Retry search' }, SLOW))
    expect(await screen.findByRole('option', { name: /Socks/ }, SLOW)).toBeTruthy()
  })

  it('shows loading and no-results search feedback', async () => {
    renderLinks(true, { emptySearch: true })
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    fireEvent.change(screen.getByLabelText('Add to Related'), { target: { value: 'soc' } })
    expect(screen.getByText('Searching…')).toBeTruthy()
    expect(await screen.findByText('No products available for this search.', {}, SLOW)).toBeTruthy()
  })

  it('offers a stored order draft and restores it without writing', async () => {
    const key = draftKey('recommendation-admin', null, 'product-links:p1')
    writeDraft(key, { related: { base: ['p2', 'p3'], value: ['p3', 'p2'] } })
    const write = renderLinks(true, { adminId: 'recommendation-admin' })
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    fireEvent.click(await screen.findByRole('button', { name: 'Review' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restore' }))
    expect(write).not.toHaveBeenCalled()
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3', 'p2'] }), SLOW)
    localStorage.removeItem(key)
  })

  it('writes only the kinds that changed', async () => {
    const write = renderLinks()
    fireEvent.click(await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW))
    fireEvent.change(screen.getByLabelText('Add to Cross-sells'), { target: { value: 'soc' } })
    fireEvent.click(await screen.findByRole('option', { name: /Socks/ }, SLOW))
    expect(write).not.toHaveBeenCalled()
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(2), SLOW)
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3'] })
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'CROSSSELL', linkedProductIds: ['p4'] })
  })
})
