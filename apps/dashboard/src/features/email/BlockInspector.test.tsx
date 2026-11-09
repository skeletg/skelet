import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { BlockInspector } from './BlockInspector'
import { EmailBlockProductsDocument } from './Email.gql'

// Catalog selection has its own tests; here it supplies one additional product to the block.
vi.mock('@/features/catalog/ProductPickerDialog', () => ({
  ProductPickerDialog: ({ open, onConfirm }: { open: boolean; onConfirm: (picks: Array<{ product: { sku: string } }>) => void }) =>
    open ? <button onClick={() => onConfirm([{ product: { sku: 'D' } }])}>Confirm picks</button> : null,
}))

function mount() {
  const onChange = vi.fn()
  render(
    <MockedProvider mocks={[{
      request: { query: EmailBlockProductsDocument, variables: { skus: ['A', 'B', 'C'] } },
      result: { data: { productsBySkus: [] } },
    }]}>
      <BlockInspector
        block={{ type: 'products', products: { mode: 'skus', skus: ['A', 'B', 'C'], limit: 1, columns: 1 } }}
        onChangeBlock={onChange}
        subject=""
        onSubjectChange={vi.fn()}
        mergeTags={[]}
        snippets={[]}
        showSubject={false}
      />
    </MockedProvider>,
  )
  return onChange
}

describe('an email products block', () => {
  it('keeps Max products when moving or removing products', () => {
    const onChange = mount()
    fireEvent.click(screen.getAllByRole('button', { name: 'Move down' })[0])
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ products: { mode: 'skus', skus: ['B', 'A', 'C'], limit: 1, columns: 1 } }))
    fireEvent.click(screen.getByRole('button', { name: 'Remove C' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ products: { mode: 'skus', skus: ['A', 'B'], limit: 1, columns: 1 } }))
  })

  it('raises Max products when choosing additional products', () => {
    const onChange = mount()
    fireEvent.click(screen.getByRole('button', { name: 'Choose products' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm picks' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ products: { mode: 'skus', skus: ['A', 'B', 'C', 'D'], limit: 4, columns: 1 } }))
  })
})
