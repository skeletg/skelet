import { useMemo, useState, type ReactElement } from 'react'
import { act, fireEvent, screen, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { SLOW, renderPage } from '@/test/page-harness'
import { ProductPickerPagedDocument, ProductTypesDocument } from './Catalog.gql'
import { ProductPickerDialog, type PickExtra, type ProductPick, type PickerOutcome } from './ProductPickerDialog'
import { defaultColumn, quantityColumn } from './product-picker-columns'

// Exercise the real server-paged table: selection outlives the loaded page, while closing the
// dialog discards it. Confirm outcomes and per-pick fields are part of the caller's contract.

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

const pickerVariables = {
  limit: 25, offset: 0, search: null, kinds: null, productTypeSlugs: null,
  published: null, attributeValueIds: null, withAttributes: false,
}

function productsPage(items: ReturnType<typeof item>[], totalCount = items.length) {
  return { data: { productsPaged: { __typename: 'ProductPage' as const, totalCount, items } } }
}

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
  onConfirm: (picks: ProductPick<Qty>[]) => PickerOutcome | Promise<PickerOutcome>
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
      onConfirm={onConfirm}
    />
  )
}

// renderPage rather than renderDialog - the table reads the admin's list preferences
function render(node: ReactElement, queryMocks: MockLink.MockedResponse[] = mocks) {
  return renderPage({ path: '/pick', adminId: 'admin-1', routes: { '/pick': node }, mocks: queryMocks })
}

async function pick(name: string) {
  const dialog = await screen.findByRole('dialog', {}, SLOW)
  fireEvent.click(await within(dialog).findByRole('checkbox', { name: `Select ${name}` }, SLOW))
  return dialog
}

describe('ProductPickerDialog', () => {
  beforeAll(() => {
    // cmdk scrolls the active filter option into view; jsdom has no scrolling API.
    HTMLElement.prototype.scrollIntoView = vi.fn()
  })

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
    const onConfirm = vi.fn<(picks: ProductPick<Qty>[]) => PickerOutcome>()
      .mockReturnValueOnce({ rowErrors: { p2: 'Missing option values.' } })
      .mockReturnValueOnce(undefined)
    render(<Harness onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' }))
    fireEvent.change(within(dialog).getAllByLabelText('Quantity')[1], { target: { value: '7' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 2 products' }))
    expect(await within(dialog).findByText('Missing option values.', {}, SLOW)).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: 'Add 1 product' })).toBeTruthy()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' }).getAttribute('data-state')).toBe('checked')
    expect(within(dialog).getByRole('checkbox', { name: 'Select Air Runner' })).not.toBeChecked()
    expect(within(dialog).getAllByLabelText('Quantity')[1]).toHaveValue(7)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(2), SLOW)
    expect(onConfirm.mock.calls[1][0]).toEqual([
      expect.objectContaining({ product: expect.objectContaining({ id: 'p2' }), extra: { quantity: 7 } }),
    ])
    expect(within(dialog).queryByText('Missing option values.')).toBeNull()
  })

  it('locks taken products and leaves excluded products out of select-all', async () => {
    const onConfirm = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={onConfirm}
      takenIds={new Set(['p1'])} takenLabel="Already linked" excludeIds={new Set(['p2'])} />)
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    const taken = await within(dialog).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW)
    expect(taken).toBeChecked()
    expect(taken).toBeDisabled()
    expect(within(dialog).getByText('Already linked')).toBeVisible()
    expect(within(dialog).queryByText('Trail Max')).toBeNull()
    expect(within(dialog).getByRole('checkbox', { name: 'Select every product on this page' })).toBeDisabled()
    fireEvent.click(within(dialog).getByText('Air Runner'))
    expect(within(dialog).getByText('0 selected')).toBeVisible()
    expect(within(dialog).getByRole('button', { name: 'Add products' })).toBeDisabled()
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('selects only available rows and supports row clicks, indeterminate state and clearing', async () => {
    const onConfirm = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={onConfirm} />)
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    fireEvent.click(await within(dialog).findByText('Air Runner', {}, SLOW))
    const all = within(dialog).getByRole('checkbox', { name: 'Select every product on this page' })
    expect(all).toHaveAttribute('data-state', 'indeterminate')
    fireEvent.click(all)
    expect(within(dialog).getByText('2 selected')).toBeVisible()
    expect(all).toBeChecked()
    fireEvent.click(all)
    expect(within(dialog).getByText('0 selected')).toBeVisible()
    fireEvent.click(all)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear' }))
    expect(within(dialog).getByRole('checkbox', { name: 'Select Air Runner' })).not.toBeChecked()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' })).not.toBeChecked()
    expect(within(dialog).getByRole('button', { name: 'Add products' })).toBeDisabled()
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('keeps quantities and off-page selections through paging and trimmed search', async () => {
    const onConfirm = vi.fn()
    const searched = vi.fn(() => ({ data: { productsPaged: {
      __typename: 'ProductPage', totalCount: 1, items: [item('p2', 'Trail Max')],
    } } }))
    const variables = { limit: 25, offset: 0, search: null, kinds: null, productTypeSlugs: null,
      published: null, attributeValueIds: null, withAttributes: false }
    const pageMock = (offset: number, search: string | null, id: string, name: string): MockLink.MockedResponse => ({
      request: { query: ProductPickerPagedDocument, variables: { ...variables, offset, search } },
      result: { data: { productsPaged: { __typename: 'ProductPage', totalCount: 26, items: [item(id, name)] } } },
      maxUsageCount: 3,
    })
    render(<Harness onConfirm={onConfirm} />, [
      pageMock(0, null, 'p1', 'Air Runner'), pageMock(25, null, 'p2', 'Trail Max'),
      { request: { query: ProductPickerPagedDocument, variables: { ...variables, search: 'Trail' } }, result: searched }, mocks[1],
    ])
    const dialog = await pick('Air Runner')
    fireEvent.change(within(dialog).getByLabelText('Quantity'), { target: { value: '4' } })
    fireEvent.click(within(dialog).getByLabelText('Next page'))
    fireEvent.click(await within(dialog).findByRole('checkbox', { name: 'Select Trail Max' }, SLOW))
    fireEvent.click(within(dialog).getByLabelText('Previous page'))
    expect(await within(dialog).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW)).toBeChecked()
    expect(within(dialog).getByLabelText('Quantity')).toHaveValue(4)
    fireEvent.click(within(dialog).getByLabelText('Next page'))
    await within(dialog).findByRole('checkbox', { name: 'Select Trail Max' }, SLOW)
    fireEvent.change(within(dialog).getByPlaceholderText('Search by name or SKU…'), { target: { value: '  Trail  ' } })
    await vi.waitFor(() => expect(within(dialog).getByLabelText('Go to page')).toHaveValue(1), SLOW)
    await vi.waitFor(() => expect(searched).toHaveBeenCalledTimes(1), SLOW)
    expect(within(dialog).getByRole('checkbox', { name: 'Select Trail Max' })).toBeChecked()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 2 products' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith([
      expect.objectContaining({ product: expect.objectContaining({ id: 'p1' }), extra: { quantity: 4 } }),
      expect.objectContaining({ product: expect.objectContaining({ id: 'p2' }), extra: { quantity: 1 } }),
    ]), SLOW)
  })

  it('skips taken rows when selecting the whole page', async () => {
    const onConfirm = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={onConfirm} takenIds={new Set(['p1'])} />)
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    await within(dialog).findByText('Air Runner', {}, SLOW)
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Select every product on this page' }))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith([
      expect.objectContaining({ product: expect.objectContaining({ id: 'p2' }), extra: null }),
    ]), SLOW)
  })

  it('applies fixed server filters and returns to page one when the host changes attributes', async () => {
    function FilterHarness() {
      const [attributeValueIds, setAttributeValueIds] = useState(['v1'])
      return (
        <ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={() => {}}
          kinds={['SIMPLE']} productTypeSlugs={['shoe']} attributeValueIds={attributeValueIds}
          sidebar={<button onClick={() => setAttributeValueIds(['v2'])}>Use second attribute</button>} />
      )
    }
    const filteredPage = (offset: number, attribute: string, name: string): MockLink.MockedResponse => ({
      request: { query: ProductPickerPagedDocument, variables: {
        limit: 25, offset, search: null, kinds: ['SIMPLE'], productTypeSlugs: ['shoe'],
        published: null, attributeValueIds: [attribute], withAttributes: false,
      } },
      result: { data: { productsPaged: { __typename: 'ProductPage', totalCount: 26, items: [item(name, name)] } } },
    })
    render(<FilterHarness />, [filteredPage(0, 'v1', 'First match'), filteredPage(25, 'v1', 'Next match'), filteredPage(0, 'v2', 'New match')])
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    fireEvent.click(await within(dialog).findByRole('checkbox', { name: 'Select First match' }, SLOW))
    expect(within(dialog).queryByRole('button', { name: 'Kind' })).toBeNull()
    expect(within(dialog).queryByRole('button', { name: 'Type' })).toBeNull()
    fireEvent.click(within(dialog).getByLabelText('Next page'))
    await within(dialog).findByText('Next match', {}, SLOW)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Use second attribute' }))
    await within(dialog).findByText('New match', {}, SLOW)
    expect(within(dialog).getByLabelText('Go to page')).toHaveValue(1)
    expect(within(dialog).getByText('1 selected')).toBeVisible()
  })

  it('disables confirmation and cancellation until a successful confirm finishes', async () => {
    let finish!: (outcome: PickerOutcome) => void
    const onConfirm = vi.fn(() => new Promise<PickerOutcome>((resolve) => { finish = resolve }))
    const onOpenChange = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={onOpenChange} onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    expect(within(dialog).getByRole('button', { name: 'Working…' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(onOpenChange).not.toHaveBeenCalled()
    await act(async () => { finish() })
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['returned error', () => ({ error: 'Try again.' }), 'Try again.'],
    ['thrown error', () => { throw new Error('Network unavailable.') }, 'Network unavailable.'],
    ['non-Error rejection', () => Promise.reject('offline'), 'Something went wrong.'],
  ])('keeps selections after a %s and closes after retry succeeds', async (_name, failure, message) => {
    const onConfirm = vi.fn<() => PickerOutcome | Promise<PickerOutcome>>()
      .mockImplementationOnce(failure).mockResolvedValueOnce(undefined)
    const onOpenChange = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={onOpenChange} onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    expect(await within(dialog).findByText(message, {}, SLOW)).toBeVisible()
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(within(dialog).getByRole('checkbox', { name: 'Select Air Runner' })).toBeChecked()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false), SLOW)
    expect(within(dialog).queryByText(message)).toBeNull()
    expect(onConfirm).toHaveBeenCalledTimes(2)
  })

  it('starts with fresh selections and search after cancel and reopen', async () => {
    const onConfirm = vi.fn()
    function ReopenHarness() {
      const [open, setOpen] = useState(true)
      return <>
        <button onClick={() => setOpen(true)}>Open picker</button>
        <ProductPickerDialog open={open} title="Pick" onOpenChange={setOpen} onConfirm={onConfirm} />
      </>
    }
    render(<ReopenHarness />)
    const dialog = await pick('Air Runner')
    fireEvent.change(within(dialog).getByPlaceholderText('Search by name or SKU…'), { target: { value: 'Trail' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Open picker' }))
    const reopened = await screen.findByRole('dialog', {}, SLOW)
    expect(await within(reopened).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW)).not.toBeChecked()
    expect(within(reopened).getByPlaceholderText('Search by name or SKU…')).toHaveValue('')
    expect(within(reopened).getByRole('button', { name: 'Add products' })).toBeDisabled()
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('maps status choices to published, unpublished, or both and resets pagination', async () => {
    const pageMock = (published: boolean | null, offset: number, name: string): MockLink.MockedResponse => ({
      request: { query: ProductPickerPagedDocument, variables: { ...pickerVariables, published, offset } },
      result: productsPage([item(name, name)], 26),
    })
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={() => {}} />, [
      pageMock(null, 0, 'All statuses'), pageMock(null, 25, 'Second page'),
      pageMock(true, 0, 'Published match'), pageMock(false, 0, 'Unpublished match'), mocks[1],
    ])
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    await within(dialog).findByText('All statuses', {}, SLOW)
    fireEvent.click(within(dialog).getByLabelText('Next page'))
    await within(dialog).findByText('Second page', {}, SLOW)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Status' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Published' }))
    await within(dialog).findByText('Published match', {}, SLOW)
    expect(within(dialog).getByLabelText('Go to page')).toHaveValue(1)
    fireEvent.click(screen.getByRole('option', { name: 'Unpublished' }))
    await within(dialog).findByText('All statuses', {}, SLOW)
    fireEvent.click(screen.getByRole('option', { name: 'Published' }))
    await within(dialog).findByText('Unpublished match', {}, SLOW)
    fireEvent.click(screen.getByRole('option', { name: 'Unpublished' }))
    await within(dialog).findByText('All statuses', {}, SLOW)
  })

  it('combines kind and product-type filters using server enum values and slugs', async () => {
    const queryMock = (kinds: ['SIMPLE'] | null, productTypeSlugs: string[] | null, name: string): MockLink.MockedResponse => ({
      request: { query: ProductPickerPagedDocument, variables: { ...pickerVariables, kinds, productTypeSlugs } },
      result: productsPage([item(name, name)]),
    })
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={() => {}} />, [
      queryMock(null, null, 'All products'), queryMock(['SIMPLE'], null, 'Simple match'),
      queryMock(['SIMPLE'], ['shoe'], 'Shoe match'),
      { request: { query: ProductTypesDocument }, result: { data: { productTypes: [
        { __typename: 'ProductType', id: 'pt1', slug: 'shoe', name: 'Shoe',
          isDigital: false, isShippingRequired: true, urlTemplate: null, tracksInventory: true,
          attributes: [], createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      ] } } },
    ])
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    await within(dialog).findByText('All products', {}, SLOW)
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Kind' }).find((button) => button.getAttribute('aria-haspopup') === 'dialog')!)
    fireEvent.click(await screen.findByRole('option', { name: 'Simple' }))
    await within(dialog).findByText('Simple match', {}, SLOW)
    fireEvent.keyDown(screen.getByPlaceholderText('Kind'), { key: 'Escape' })
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Type' }).find((button) => button.getAttribute('aria-haspopup') === 'dialog')!)
    fireEvent.click(await screen.findByRole('option', { name: 'Shoe' }))
    await within(dialog).findByText('Shoe match', {}, SLOW)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear Type filter' }))
    await within(dialog).findByText('Simple match', {}, SLOW)
  })

  it('holds the loaded page and its selections while the next page is pending', async () => {
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={() => {}} />, [
      { request: { query: ProductPickerPagedDocument, variables: pickerVariables },
        result: productsPage([item('p1', 'Air Runner')], 26) },
      { request: { query: ProductPickerPagedDocument, variables: { ...pickerVariables, offset: 25 } },
        result: productsPage([item('p2', 'Trail Max')], 26), delay: Infinity },
      mocks[1],
    ])
    const dialog = await pick('Air Runner')
    fireEvent.click(within(dialog).getByLabelText('Next page'))
    expect(within(dialog).getByLabelText('Go to page')).toHaveValue(2)
    expect(within(dialog).getByRole('checkbox', { name: 'Select Air Runner' })).toBeChecked()
    expect(within(dialog).getByText('1 selected')).toBeVisible()
    expect(within(dialog).queryByText('Loading…')).toBeNull()
  })

  it('shows the query failure and allows cancellation without confirming', async () => {
    const onConfirm = vi.fn()
    const onOpenChange = vi.fn()
    render(<ProductPickerDialog open title="Pick" onOpenChange={onOpenChange} onConfirm={onConfirm} />, [
      { request: { query: ProductPickerPagedDocument, variables: pickerVariables }, error: new Error('Catalog unavailable.') },
      mocks[1],
    ])
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    expect(await within(dialog).findByText('Catalog unavailable.', {}, SLOW)).toBeVisible()
    expect(within(dialog).getByText("Couldn't load")).toBeVisible()
    expect(within(dialog).getByRole('button', { name: 'Add products' })).toBeDisabled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('shows the caller’s empty message when no products are available', async () => {
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={() => {}}
      emptyMessage="No eligible children." />, [
      { request: { query: ProductPickerPagedDocument, variables: pickerVariables }, result: productsPage([]) }, mocks[1],
    ])
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    expect(await within(dialog).findByText('No eligible children.', {}, SLOW)).toBeVisible()
    expect(within(dialog).getByRole('checkbox', { name: 'Select every product on this page' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: 'Add products' })).toBeDisabled()
  })

  it('restores initial per-product fields when a product is deselected and picked again', async () => {
    const onConfirm = vi.fn()
    render(<Harness onConfirm={onConfirm} />)
    const dialog = await pick('Air Runner')
    const qty = within(dialog).getAllByLabelText('Quantity')[0]
    fireEvent.change(qty, { target: { value: '9' } })
    const checkbox = within(dialog).getByRole('checkbox', { name: 'Select Air Runner' })
    fireEvent.click(checkbox)
    expect(qty).toBeDisabled()
    fireEvent.click(checkbox)
    expect(qty).toBeEnabled()
    expect(qty).toHaveValue(1)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith([
      expect.objectContaining({ product: expect.objectContaining({ id: 'p1' }), extra: { quantity: 1 } }),
    ]), SLOW)
  })

  it('enables per-pick fields only for selected rows and merges quantity and default edits', async () => {
    const onConfirm = vi.fn()
    const extra: PickExtra<{ quantity: number; isDefault: boolean }> = {
      initial: () => ({ quantity: 2, isDefault: false }), columns: [quantityColumn(), defaultColumn()],
    }
    render(<ProductPickerDialog open title="Pick" onOpenChange={() => {}} onConfirm={onConfirm} extra={extra} />)
    const dialog = await screen.findByRole('dialog', {}, SLOW)
    const checkbox = await within(dialog).findByRole('checkbox', { name: 'Select Air Runner' }, SLOW)
    const qty = within(dialog).getAllByLabelText('Quantity')[0]
    const defaultToggle = within(dialog).getAllByRole('checkbox', { name: 'Default selection' })[0]
    expect(qty).toBeDisabled()
    expect(defaultToggle).toBeDisabled()
    fireEvent.click(checkbox)
    expect(qty).toHaveValue(2)
    expect(defaultToggle).toBeEnabled()
    fireEvent.click(defaultToggle)
    fireEvent.change(qty, { target: { value: '0' } })
    expect(qty).toHaveValue(1)
    fireEvent.change(qty, { target: { value: '5' } })
    fireEvent.click(qty)
    expect(checkbox).toBeChecked()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add 1 product' }))
    await vi.waitFor(() => expect(onConfirm).toHaveBeenCalledWith([
      expect.objectContaining({ product: expect.objectContaining({ id: 'p1' }), extra: { quantity: 5, isDefault: true } }),
    ]), SLOW)
  })
})
