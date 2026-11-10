import { describe, expect, it } from 'vitest'

import { TRANSACTIONAL_TRIGGERS, TRIGGER_HELP, TRIGGER_LABELS } from './flowTypes'

describe('the AI abandoned-cart trigger', () => {
  it('reads as what it is, and says it stands in for the usual one', () => {
    expect(TRIGGER_LABELS.assisted_cart_abandoned).toBe('Abandoned cart the assistant filled')
    expect(TRIGGER_HELP.assisted_cart_abandoned).toMatch(/instead of the usual abandoned-cart flow/)
  })

  it('is marketing, held to consent like the usual abandoned cart', () => {
    expect(TRANSACTIONAL_TRIGGERS.has('assisted_cart_abandoned')).toBe(false)
    expect(TRANSACTIONAL_TRIGGERS.has('cart_abandoned')).toBe(false)
  })
})
