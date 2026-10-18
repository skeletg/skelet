import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { CreateShipmentDialog } from './CreateShipmentDialog'
import { CreateFulfillmentDocument, OrderDocument } from './Orders.gql'

// Two of the three tees already shipped; the cancelled shipment of the mug doesn't count
const order = {
  id: 'o1',
  lines: [
    { id: 'l1', name: 'Tee', sku: 'TEE-1', quantity: 3 },
    { id: 'l2', name: 'Mug', sku: 'MUG-1', quantity: 1 },
  ],
  fulfillments: [
    { status: 'shipped', lines: [{ orderLineId: 'l1', quantity: 2 }] },
    { status: 'cancelled', lines: [{ orderLineId: 'l2', quantity: 1 }] },
  ],
} as never

describe('CreateShipmentDialog', () => {
  it('defaults to the unshipped remainder and refuses more than that', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(<CreateShipmentDialog order={order} open onOpenChange={onOpenChange} />, [
      {
        request: { query: CreateFulfillmentDocument, variables: captureInput(onCreate) },
        result: { data: { createFulfillment: { __typename: 'Fulfillment', id: 'f1' } } },
      },
      { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
    ])
    expect((screen.getByLabelText('Tee quantity') as HTMLInputElement).value).toBe('1')
    expect(screen.getByText('TEE-1 · 1 unshipped')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Tee quantity'), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create shipment' }))
    expect(await screen.findByText('Between 1 and what is still unshipped')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Tee quantity'), { target: { value: '1' } })
    fireEvent.click(screen.getByLabelText('Ship Mug'))
    fireEvent.change(screen.getByLabelText('Tracking (optional)'), { target: { value: '1Z999' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create shipment' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith({
      orderId: 'o1',
      lines: [{ orderLineId: 'l1', quantity: 1 }],
      carrier: null,
      trackingNumber: '1Z999',
      insuredValueCents: null,
    })
  })
})
