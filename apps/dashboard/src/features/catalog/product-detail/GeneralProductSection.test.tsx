import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { SetProductTaxClassDocument, UpdateProductDocument, UpsertProductContentDocument } from '../Catalog.gql'
import { GeneralProductSection } from './GeneralProductSection'

// What these pin: a blank name is refused before anything is written, and each change goes through
// its own write - price in cents on the product, the name on the locale's content

const product = {
  id: 'p1',
  name: 'Tee',
  sku: 'TEE-1',
  taxClass: 'standard',
  defaultPrice: 1500,
  defaultCurrencyCode: 'EUR',
  content: [{ locale: 'en', name: 'Tee' }],
} as never

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderGeneral() {
  const writes = { update: vi.fn(), content: vi.fn(), tax: vi.fn() }
  renderSection(
    <GeneralProductSection
      product={product}
      locale="en"
      scopeLabel="Global / en"
      canManage
      productVariables={{ id: 'p1' }}
      categories={[]}
      primaryCategory={null}
    />,
    [
      { request: { query: UpdateProductDocument, variables: recorder(writes.update) }, result: { data: { updateProduct: null } } },
      { request: { query: UpsertProductContentDocument, variables: recorder(writes.content) }, result: { data: { upsertProductContent: null } } },
      { request: { query: SetProductTaxClassDocument, variables: recorder(writes.tax) }, result: { data: { setProductTaxClass: null } } },
    ],
  )
  return writes
}

async function apply(changes: RegExp) {
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  return dialog
}

describe('GeneralProductSection', () => {
  it('writes the price and name through their own mutations, and leaves the tax class alone', async () => {
    const writes = renderGeneral()
    fireEvent.change(screen.getByLabelText('Default price'), { target: { value: '12.5' } })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Classic tee' } })
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(writes.content).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: { id: 'p1', sku: 'TEE-1', defaultPrice: 1250 } })
    expect(writes.content).toHaveBeenCalledWith({ productId: 'p1', locale: 'en', input: { name: 'Classic tee' } })
    expect(writes.tax).not.toHaveBeenCalled()
  })

  it('refuses a blank name', async () => {
    const writes = renderGeneral()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: ' ' } })
    const dialog = await apply(/Apply/)
    expect(await within(dialog).findByText(/Name is required/)).toBeDefined()
    expect(writes.content).not.toHaveBeenCalled()
  })
})
