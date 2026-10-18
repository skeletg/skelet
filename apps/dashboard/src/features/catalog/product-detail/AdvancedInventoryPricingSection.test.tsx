import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { UpdateProductDocument } from '../Catalog.gql'
import { AdvancedInventoryPricingSection } from './AdvancedInventoryPricingSection'

// What these pin: only the groups that changed are sent, an emptied compare-at price clears it, and
// a maximum below the minimum is refused before anything is written

const product = {
  id: 'p1',
  defaultCurrencyCode: 'EUR',
  costCents: 400,
  compareAtPrice: 2000,
  specialPrice: null,
  specialFrom: null,
  specialTo: null,
  weightGrams: 250,
  lengthMm: 0,
  widthMm: 0,
  heightMm: 0,
  minOrderQty: 2,
  maxOrderQty: 10,
} as never

function renderAdvanced() {
  const update = vi.fn()
  renderSection(<AdvancedInventoryPricingSection product={product} canManage productVariables={{ id: 'p1' }} />, [
    {
      request: {
        query: UpdateProductDocument,
        variables: (vars: Record<string, unknown>) => {
          update(vars)
          return true
        },
      },
      result: { data: { updateProduct: null } },
    },
  ])
  return update
}

async function apply(changes: RegExp) {
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  return dialog
}

describe('AdvancedInventoryPricingSection', () => {
  it('sends only the changed groups, clearing an emptied compare-at price', async () => {
    const update = renderAdvanced()
    fireEvent.change(screen.getByLabelText('Unit cost (COGS)'), { target: { value: '5.25' } })
    fireEvent.change(screen.getByLabelText('Compare-at price'), { target: { value: '' } })
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(update).toHaveBeenCalled(), SLOW)
    expect(update).toHaveBeenCalledWith({ input: { id: 'p1', costCents: 525, clearCompareAtPrice: true } })
  })

  it('refuses a maximum below the minimum', async () => {
    const update = renderAdvanced()
    fireEvent.change(screen.getByLabelText('Max order qty'), { target: { value: '1' } })
    const dialog = await apply(/Apply/)
    expect(await within(dialog).findByText(/At least the minimum/)).toBeDefined()
    expect(update).not.toHaveBeenCalled()
  })
})
