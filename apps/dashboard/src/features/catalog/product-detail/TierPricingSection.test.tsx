import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { CustomerGroupsDocument } from '../../customers/CustomerGroups.gql'
import { SetProductGroupPricesDocument } from '../Catalog.gql'
import { TierPricingSection } from './TierPricingSection'

// What these pin: the whole set is sent on save with prices in cents, and a half-filled row is
// refused instead of being dropped from the replace-all write

const groups = {
  customerGroups: [
    { __typename: 'CustomerGroup', id: 'g1', slug: 'vip', name: 'VIP', description: '', isDefault: false, memberCount: 3, createdAt: '2026-01-01T00:00:00Z' },
  ],
}

function renderTiers() {
  const write = vi.fn()
  renderSection(
    <TierPricingSection
      productId="p1"
      groupPrices={[{ id: 't1', customerGroupId: 'g1', minQty: 5, price: 900 }]}
      currencyCode="EUR"
      canManage
      productVariables={{ id: 'p1' }}
    />,
    [
      { request: { query: CustomerGroupsDocument }, result: { data: groups }, maxUsageCount: 3 },
      {
        request: {
          query: SetProductGroupPricesDocument,
          variables: (vars: Record<string, unknown>) => {
            write(vars)
            return true
          },
        },
        result: { data: { setProductGroupPrices: [] } },
      },
    ],
  )
  return write
}

async function apply(changes: RegExp) {
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  return dialog
}

describe('TierPricingSection', () => {
  it('sends the whole set with prices in cents', async () => {
    const write = renderTiers()
    fireEvent.click(screen.getByRole('button', { name: 'Add tier' }))
    fireEvent.change(screen.getByLabelText('Min qty, tier 2'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Price, tier 2'), { target: { value: '7.5' } })
    fireEvent.change(screen.getByLabelText('Price, tier 1'), { target: { value: '8' } })
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(write).toHaveBeenCalled(), SLOW)
    expect(write).toHaveBeenCalledWith({
      productId: 'p1',
      prices: [
        { customerGroupId: 'g1', minQty: 5, price: 800 },
        { customerGroupId: null, minQty: 10, price: 750 },
      ],
    })
  })

  it('refuses a tier without a price', async () => {
    const write = renderTiers()
    fireEvent.click(screen.getByRole('button', { name: 'Add tier' }))
    fireEvent.change(screen.getByLabelText('Min qty, tier 2'), { target: { value: '10' } })
    const dialog = await apply(/Apply/)
    expect(await within(dialog).findByText(/Zero or more/)).toBeDefined()
    expect(write).not.toHaveBeenCalled()
  })
})
