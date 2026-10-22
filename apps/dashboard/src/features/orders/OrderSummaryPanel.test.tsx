import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { OrderSummaryPanel } from './OrderSummaryPanel'

// What this pins: payment rows arrive with the backend's lowercase kind and status ("capture", "succeeded"), so a
// card-paid order reads as collected and settled rather than fully due

const txn = (kind: string, amount: number, status = 'succeeded') => ({ id: `${kind}-${amount}`, kind, amount, status })

const order = {
  currencyCode: 'USD',
  subtotal: 5000,
  discountAmount: 0,
  appliedPromotions: [],
  shippingAmount: 0,
  taxAmount: 0,
  taxLines: [],
  total: 5000,
  giftCardAmount: 0,
  storeCreditAmount: 0,
  lines: [],
  fulfillments: [],
  shippingAddress: { firstName: 'Ada', lastName: 'Lovelace' },
  customerId: null,
  email: 'a@example.com',
  createdAt: '2026-01-01T00:00:00Z',
  paymentTransactions: [txn('authorize', 5000), txn('capture', 5000), txn('refund', 1000), txn('refund', 999, 'failed')],
} as never

const rowValue = (label: string) => screen.getByText(label).nextElementSibling?.textContent

describe('OrderSummaryPanel', () => {
  it('counts succeeded captures and refunds', () => {
    render(
      <MemoryRouter>
        <OrderSummaryPanel order={order} isDraft={false} />
      </MemoryRouter>,
    )
    expect(rowValue('Collected')).toMatch(/50\.00/)
    expect(rowValue('Refunded')).toMatch(/10\.00/)
    expect(rowValue('Balance')).toBe('Settled')
  })
})
