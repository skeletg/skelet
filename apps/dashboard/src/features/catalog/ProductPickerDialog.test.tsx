import { useMemo, type ReactElement } from 'react'
import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderPage } from '@/test/page-harness'
import { ProductPickerPagedDocument, ProductTypesDocument } from './Catalog.gql'
import { ProductPickerDialog, type PickExtra, type ProductPick } from './ProductPickerDialog'
import { quantityColumn } from './product-picker-columns'

// What these pin: per-pick fields reach onConfirm with what was typed (and typing keeps focus),
// a failed validation keeps the dialog open, and row errors keep only the failed picks ticked

const item = (id: string, name: string) => ({
  __typename: 'Product' as const,
  id,
  name,
  sku: id.toUpperCase(),
  kind: 'SIMPLE' as const,
  defaultPrice: 500,
  defaultCurrencyCode: 'USD',
  productType: { __typename: 'ProductType' as const, id: 'pt1', slug: 'shoe', name: 'Shoe' },
  thumbnail: null,
  displayImage: null,
})

const mocks = [
  {
    request: { query: ProductPickerPagedDocument, variables: () => true },
    result: {
      data: {
        productsPaged: { __typename: 'ProductPage' as const, totalCount: 2, items: [item('p1', 'Air Runner'), item('p2', 'Trail Max')] },
      },
    },
    maxUsageCount: 10,
  },
  { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 10 },
]

type Qty = { quantity: number }

function Harness({
  onConfirm,
  validate,
}: {
  onConfirm: (picks: ProductPick<Qty>[]) => unknown
  validate?: (picks: ProductPick<Qty>[]) => string | null
}) {
  const extra = useMemo<PickExtra<Qty>>(() => ({ initial: () => ({ quantity: 1 }), columns: [quantityColumn()] }), [])
  return (
    <ProductPickerDialog<Qty>
      open
      onOpenChange={() => {}}
      title="Pick"
      extra={extra}
      validate={validate}
      onConfirm={onConfirm as never}
    />
  )
}

// renderPage rather than renderDialog - the table reads the admin's list preferences
function render(node: ReactElement) {
  renderPage({ path: '/pick', adminId: 'admin-1', routes: { '/pick': node }, mocks })
}

async function pick(name: string) {
  const dialog = await screen.findByRole('dialog', {}, SLOW)
  fireEvent.click(await within(dialog).findByRole('checkbox', { name: `Select ${name}` }, SLOW))
  return dialog
}

describe('ProductPickerDialog', () => {
  it('hands each pick back with its typed quantity', async () => {
    const onConfirm = vi.fn()
    render(<Harness onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    const qty = within(dialog).getAllByLabelText('Quantity')[0] as HTMLInputElement
    qty.focus()
    fireEvent.change(qty, { target: { value: '4' } })
    // The same input survives the edit - a remount would drop focus mid-typing
    expect(document.activeElement).toBe(qty)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalled(), SLOW)
    expect(onConfirm.mock.calls[0][0]).toEqual([expect.objectContaining({ product: expect.objectContaining({ id: 'p1' }), extra: { quantity: 4 } })])
  })

  it('keeps the picks when validation refuses them', async () => {
    const onConfirm = vi.fn()
    render(<Harness onConfirm={onConfirm} validate={() => 'Only one default allowed.'} />)
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    expect(await within(dialog).findByText('Only one default allowed.')).toBeTruthy()
    expect(onConfirm).not.toHaveBeenCalled()
    expect(within(dialog).getByRole('button', { name: 'Add 1 product' })).toBeTruthy()
  })

  it('keeps only the failed picks ticked, with their errors', async () => {
    const onConfirm = vi.fn(() => ({ error: 'Linked 1 of 2.', rowErrors: { p2: 'Missing option values.' } }))
    render(<Harness onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' }))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 2 products' }))
    expect(await within(dialog).findByText('Missing option values.', {}, SLOW)).toBeTruthy()
    expect(within(dialog).getByText('Linked 1 of 2.')).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: 'Add 1 product' })).toBeTruthy()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' }).getAttribute('data-state')).toBe('checked')
  })
})
