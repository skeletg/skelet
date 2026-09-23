import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { EditOrderDialog } from './EditOrderDialog'
import { EditOrderDocument, OrderDocument, OrdersDocument } from './Orders.gql'

const order = {
  id: 'o1',
  lines: [
    { id: 'l1', productName: 'Tee', productSku: 'TEE-1', quantity: 2 },
    { id: 'l2', productName: 'Mug', productSku: 'MUG-1', quantity: 1 },
  ],
} as never

describe('EditOrderDialog', () => {
  it('rejects a zeroed line, then sends the removal and the new line', async () => {
    const onEdit = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(<EditOrderDialog order={order} open onOpenChange={onOpenChange} />, [
      {
        request: { query: EditOrderDocument, variables: captureInput(onEdit) },
        result: { data: { editOrder: { __typename: 'Order', id: 'o1' } } },
      },
      { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
      { request: { query: OrdersDocument, variables: () => true }, result: { data: { orders: [] } }, maxUsageCount: 3 },
    ])
    const apply = screen.getByRole('button', { name: 'Apply changes' }) as HTMLButtonElement
    expect(apply.disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Tee quantity'), { target: { value: '0' } })
    fireEvent.click(apply)
    expect(await screen.findByText('Above 0 - use remove to drop the line')).toBeDefined()
    expect(onEdit).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Tee quantity'), { target: { value: '3' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove line' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Add product' }))
    fireEvent.change(screen.getByLabelText('New line 1 SKU'), { target: { value: 'CAP-9' } })
    fireEvent.click(apply)

    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onEdit).toHaveBeenCalledWith({
      orderId: 'o1',
      updateLines: [{ orderLineId: 'l1', quantity: 3 }],
      removeLines: ['l2'],
      addLines: [{ productSku: 'CAP-9', quantity: 1 }],
    })
  })
})
