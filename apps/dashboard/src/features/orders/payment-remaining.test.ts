import { describe, expect, it } from 'vitest'
import { paymentRemaining } from './payment-remaining'

const txn = (kind: string, amount: number, status = 'succeeded') => ({ kind, amount, status })

describe('paymentRemaining', () => {
  it('capture: authorized minus captured', () => {
    expect(paymentRemaining('capture', [txn('authorize', 5000), txn('capture', 2000)])).toBe(3000)
  })

  it('refund: captured minus refunded', () => {
    expect(paymentRemaining('refund', [txn('authorize', 5000), txn('capture', 5000), txn('refund', 1250)])).toBe(3750)
  })

  it('counts pending rows like the backend guard, skips failed ones', () => {
    const txns = [txn('capture', 5000), txn('refund', 1000, 'pending'), txn('refund', 4000, 'failed')]
    expect(paymentRemaining('refund', txns)).toBe(4000)
  })

  it('never goes below zero', () => {
    expect(paymentRemaining('refund', [txn('refund', 100)])).toBe(0)
  })
})
