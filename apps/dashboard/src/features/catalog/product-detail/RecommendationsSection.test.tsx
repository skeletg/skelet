import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { ProductLabelsDocument, ProductLinksDocument, ProductPickerDocument, SetProductLinksDocument } from '../Catalog.gql'
import { RecommendationsSection } from './RecommendationsSection'

// What these pin: the three lists stage as one section, and only the kinds that changed are written

const links = (kind: string, ids: string[]) => ({
  request: { query: ProductLinksDocument, variables: { productId: 'p1', kind } },
  result: { data: { productLinks: ids } },
  maxUsageCount: 3,
})

const slim = (id: string, name: string) => ({ __typename: 'Product', id, name, sku: null })

function renderLinks() {
  const write = vi.fn()
  renderSection(<RecommendationsSection productId="p1" canManage />, [
    links('RELATED', ['p2', 'p3']),
    links('UPSELL', []),
    links('CROSSSELL', []),
    {
      request: { query: ProductLabelsDocument, variables: { ids: ['p2', 'p3'] } },
      result: { data: { productsByIds: [slim('p2', 'Mug'), slim('p3', 'Cap')] } },
      maxUsageCount: 3,
    },
    {
      request: { query: ProductPickerDocument, variables: { search: 'soc', limit: 10 } },
      result: {
        data: {
          productsPaged: {
            __typename: 'ProductPage',
            items: [{ __typename: 'Product', id: 'p4', name: 'Socks', sku: 'SOCK', kind: 'SIMPLE', defaultPrice: 500, defaultCurrencyCode: 'EUR' }],
          },
        },
      },
    },
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
  return dialog
}

describe('RecommendationsSection', () => {
  it('writes only the kinds that changed', async () => {
    const write = renderLinks()
    fireEvent.click(await screen.findByRole('button', { name: 'Remove Mug from Related' }, SLOW))
    fireEvent.change(screen.getByLabelText('Add to Cross-sells'), { target: { value: 'soc' } })
    fireEvent.click(await screen.findByRole('button', { name: /Socks/ }, SLOW))
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(2), SLOW)
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'RELATED', linkedProductIds: ['p3'] })
    expect(write).toHaveBeenCalledWith({ productId: 'p1', kind: 'CROSSSELL', linkedProductIds: ['p4'] })
  })
})
