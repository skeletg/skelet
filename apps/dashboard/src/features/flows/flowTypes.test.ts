import { describe, expect, it } from 'vitest'

import { ADDABLE_NODES, NODE_HINTS, NODE_LABELS, STEP_GROUPS, TRIGGER_HELP, TRIGGER_LABELS, defaultNodeData, needsEmail } from './flowTypes'

describe('the AI abandoned-cart trigger', () => {
  it('reads as what it is, and says it stands in for the usual one', () => {
    expect(TRIGGER_LABELS.assisted_cart_abandoned).toBe('Abandoned cart the assistant filled')
    expect(TRIGGER_HELP.assisted_cart_abandoned).toMatch(/instead of the usual abandoned-cart flow/)
  })

})

describe('the step rail', () => {
  it('lists every step an operator can add, once, each with a name and a hint', () => {
    const listed = STEP_GROUPS.flatMap((g) => g.steps)
    expect([...listed].sort()).toEqual([...ADDABLE_NODES].sort())
    for (const type of listed) {
      expect(NODE_LABELS[type]).toBeTruthy()
      expect(NODE_HINTS[type]).toBeTruthy()
    }
  })
})

describe('a send-email step', () => {
  it('needs an email until it has one from the library or carries its own', () => {
    expect(needsEmail(defaultNodeData('send_email'))).toBe(true)
    expect(needsEmail({ emailId: 'e1' })).toBe(false)
    expect(needsEmail({ subject: 'Shipped', blocks: [{ type: 'heading', text: 'On its way' }] })).toBe(false)
  })

  it('still needs one when it only carries the placeholder older flows were saved with', () => {
    expect(needsEmail({ subject: 'Hello {{customer.firstName}}', blocks: [{ type: 'heading', text: 'Hi {{customer.firstName}}' }], styles: {} })).toBe(true)
  })
})
