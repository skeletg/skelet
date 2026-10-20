import { describe, expect, it } from 'vitest'
import { formatCents } from './promoFormat'
import { summariseType } from './typeCategory'

// What these pin: promotion amounts carry no currency, so they render as plain numbers, never a guessed "$"

describe('promotion money', () => {
  it('formats analytics totals without a currency symbol', () => {
    expect(formatCents(123450)).toMatch(/^1[,.]?234[.,]50$/)
  })

  it('summarises fixed-amount rules without "$"', () => {
    expect(summariseType('core.fixed_off_subtotal', 'Fixed off', { cents: 1000 })).toMatch(/^10[.,]00 off cart$/)
    expect(summariseType('core.cart_subtotal_range', 'Subtotal', { min_cents: 5000, max_cents: 10000 })).not.toContain('$')
  })
})
