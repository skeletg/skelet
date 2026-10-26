import { PrefsContext } from '@/lib/admin-prefs'
import { draftKey, writeDraft } from '@/lib/form-drafts'
import { fireEvent, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { RecommendationProductsDocument, ProductLinksDocument, ProductPickerPagedDocument, ProductTypesDocument, SetProductLinksDocument } from '../Catalog.gql'
import { RecommendationsSection } from './RecommendationsSection'

// What these pin: the three lists stage as one section, and only the kinds that changed are written

const links = (kind: string, ids: string[]) => ({
  request: { query: ProductLinksDocument, variables: { productId: 'p1', kind } },
  result: { data: { productLinks: ids } },
  maxUsageCount: 3,
})

const slim = (id: string, name: string) => ({ __typename: 'Product', id, name, sku: name.toUpperCase(), thumbnail: null, displayImage: null })

function renderLinks(canManage = true, options: { metadataError?: boolean; missing?: boolean; adminId?: string } = {}) {
  const write = vi.fn()
  renderSection(<MemoryRouter><PrefsContext.Provider value={options.adminId ?? ''}><RecommendationsSection productId="p1" canManage={canManage} /></PrefsContext.Provider></MemoryRouter>, [
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

  it('disables edits for read-only viewers', async () => {
    renderLinks(false)
    expect(await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)).toHaveProperty('disabled', true)
    expect(screen.getByRole('button', { name: 'Reorder Mug' })).toHaveProperty('disabled', true)
    expect(screen.queryByRole('button', { name: /Add products/ })).toBeNull()
  })

  it('stages a multi-product Browse selection at the end of the list', async () => {
    const write = renderLinks()
    await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Add products to Related' }))
    const dialog = await screen.findByRole('dialog')
    const socks = await within(dialog).findByRole('checkbox', { name: 'Select Socks' }, SLOW)
    expect(within(dialog).queryByRole('checkbox', { name: 'Select Self' })).toBeNull()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Mug' })).toHaveProperty('disabled', true)
    fireEvent.click(socks)
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Select Shirt' }))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 2 products' }))
    expect(await screen.findByRole('button', { name: 'Remove Shirt from Related' }, SLOW)).toBeTruthy()
    expect(screen.getAllByText('Will add')).toHaveLength(2)
    expect(screen.getByRole('tab', { name: /Related.*unsaved changes/ })).toBeTruthy()
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
    fireEvent.mouseDown(screen.getByRole('tab', { name: /Cross-sells/ }), { button: 0, ctrlKey: false })
    expect(screen.getByText('No cross-sells yet')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Add products to Cross-sells' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(await within(dialog).findByRole('checkbox', { name: 'Select Socks' }, SLOW))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    expect(await screen.findByRole('button', { name: 'Remove Socks from Cross-sells' }, SLOW)).toBeTruthy()
    expect(write).not.toHaveBeenCalled()
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(2), SLOW)
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3'] })
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'CROSSSELL', linkedProductIds: ['p4'] })
  })

  it('names every product past the by-id batch limit', async () => {
    const asked: number[] = []
    const ids = Array.from({ length: 150 }, (_, i) => `p${i + 10}`)
    renderSection(<MemoryRouter><PrefsContext.Provider value=""><RecommendationsSection productId="p1" canManage /></PrefsContext.Provider></MemoryRouter>, [
      links('RELATED', ids),
      links('UPSELL', []),
      links('CROSSSELL', []),
      {
        request: { query: RecommendationProductsDocument, variables: () => true },
        result: (vars: Record<string, unknown>) => {
          const batch = vars.ids as string[]
          asked.push(batch.length)
          return { data: { productsByIds: batch.map((id) => slim(id, `Item ${id}`)) } }
        },
        maxUsageCount: 5,
      },
    ])
    expect(await screen.findByText('Item p159', {}, SLOW)).toBeTruthy()
    expect(screen.queryByText('Unavailable product')).toBeNull()
    expect(asked.sort((a, b) => b - a)).toEqual([100, 50])
  })

  it('filters a long list and pauses reordering meanwhile', async () => {
    const ids = Array.from({ length: 12 }, (_, i) => `p${i + 10}`)
    renderSection(<MemoryRouter><PrefsContext.Provider value=""><RecommendationsSection productId="p1" canManage /></PrefsContext.Provider></MemoryRouter>, [
      links('RELATED', ids),
      links('UPSELL', []),
      links('CROSSSELL', []),
      {
        request: { query: RecommendationProductsDocument, variables: () => true },
        result: (vars: Record<string, unknown>) => ({ data: { productsByIds: (vars.ids as string[]).map((id) => slim(id, `Item ${id}`)) } }),
        maxUsageCount: 5,
      },
    ])
    await screen.findByText('Item p21', {}, SLOW)
    fireEvent.change(screen.getByRole('searchbox', { name: 'Filter Related' }), { target: { value: 'item p2' } })
    expect(screen.getByText('2 of 12 shown. Clear the filter to reorder.')).toBeTruthy()
    expect(screen.queryByText('Item p10')).toBeNull()
    expect(screen.getByRole('button', { name: 'Reorder Item p20' })).toHaveProperty('disabled', true)
    expect(screen.getByRole('button', { name: 'Move Item p21 up in Related' })).toHaveProperty('disabled', true)
    fireEvent.click(screen.getByRole('button', { name: 'Remove Item p21 from Related' }))
    expect(screen.queryByText('Item p21')).toBeNull()
    fireEvent.change(screen.getByRole('searchbox', { name: 'Filter Related' }), { target: { value: '' } })
    expect(screen.getByText('Item p10')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Move Item p20 up in Related' })).toHaveProperty('disabled', false)
  })
})
