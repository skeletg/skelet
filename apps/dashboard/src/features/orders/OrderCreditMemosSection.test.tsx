import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderDialog } from '@/test/page-harness'
import { OrderCreditMemosSection } from './OrderCreditMemosSection'
import { CreditMemosDocument, CreditableQuantitiesDocument, IssueCreditMemoDocument, OrderDocument } from './Orders.gql'

// What these pin: memo amounts are typed in the order currency's major units, negative amounts are refused, and
// quantities may be fractional (lines are Float end to end) but never above what is left to credit

const order = {
  id: 'o1',
  channelId: 'c1',
  currencyCode: 'USD',
  giftCardAmount: 1000,
  storeCreditAmount: 0,
  shippingRefundable: 500,
  paymentTransactions: [],
  returns: [],
  lines: [{ id: 'l1', name: 'Fabric', sku: 'FAB-1', quantity: 3 }],
} as never

function renderMemos(onInput: (v: Record<string, unknown>) => void) {
  renderDialog(<OrderCreditMemosSection order={order} />, [
    meMock(),
    { request: { query: CreditMemosDocument, variables: { orderId: 'o1' } }, result: { data: { creditMemos: [] } }, maxUsageCount: 3 },
    {
      request: { query: CreditableQuantitiesDocument, variables: { orderId: 'o1' } },
      result: { data: { creditableQuantities: [{ __typename: 'OrderLineQuantity', orderLineId: 'l1', quantity: 2.5 }] } },
      maxUsageCount: 3,
    },
    {
      request: { query: IssueCreditMemoDocument, variables: captureInput(onInput) },
      result: { data: { issueCreditMemo: { __typename: 'CreditMemo', id: 'm1', number: 'CM-1', total: 1500 } } },
    },
    { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
  ])
}

async function openMemo() {
  fireEvent.click(await screen.findByRole('button', { name: 'Issue credit memo' }, SLOW))
  // the line list waits on creditableQuantities
  fireEvent.click(await screen.findByRole('checkbox', {}, SLOW))
}

const quantity = () => screen.getAllByRole('spinbutton')[0]
const submit = () => fireEvent.click(screen.getAllByRole('button', { name: 'Issue credit memo' }).at(-1)!)

describe('OrderCreditMemosSection', () => {
  it('sends a fractional quantity and a decimal shipping refund', async () => {
    const calls: Record<string, unknown>[] = []
    renderMemos((v) => calls.push(v))
    await openMemo()
    fireEvent.change(quantity(), { target: { value: '1.5' } })
    fireEvent.change(screen.getByLabelText('Shipping refund'), { target: { value: '4.99' } })
    submit()
    await vi.waitFor(
      () =>
        expect(calls[0]).toMatchObject({
          lines: [{ orderLineId: 'l1', quantity: 1.5 }],
          shippingRefund: 499,
          adjustmentRefund: 0,
          adjustmentFee: 0,
        }),
      SLOW,
    )
  })

  it('refuses a negative amount', async () => {
    const calls: Record<string, unknown>[] = []
    renderMemos((v) => calls.push(v))
    await openMemo()
    fireEvent.change(screen.getByLabelText('Adjustment'), { target: { value: '-5' } })
    submit()
    expect(await screen.findByText(/Amounts must be positive/)).toBeTruthy()
    expect(calls).toHaveLength(0)
  })

  it('refuses more than is left to credit', async () => {
    const calls: Record<string, unknown>[] = []
    renderMemos((v) => calls.push(v))
    await openMemo()
    fireEvent.change(quantity(), { target: { value: '3' } })
    submit()
    expect(await screen.findByText(/what is left to credit/)).toBeTruthy()
    expect(calls).toHaveLength(0)
  })
})
