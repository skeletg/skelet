import { describe, expect, it } from 'vitest'

import { formatCents } from './format'

// Baseline smoke tests for the money formatter - without a registry hint the Intl fallback formats by code
describe('formatCents', () => {
  it('formats whole dollars', () => {
    expect(formatCents(9900, 'USD')).toMatch(/99/)
  })

  it('keeps cents precision', () => {
    expect(formatCents(7901, 'USD')).toMatch(/79[.,]01/)
  })

  it('handles negative amounts (ledger debits)', () => {
    expect(formatCents(-5000, 'USD')).toMatch(/50/)
    expect(formatCents(-5000, 'USD')).toMatch(/-/)
  })

  it('never throws on unknown currency codes', () => {
    expect(() => formatCents(1234, 'XXX')).not.toThrow()
  })
})
