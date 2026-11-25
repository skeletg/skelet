import { describe, expect, it } from 'vitest'
import { addOnQuantity, addOnsOf } from './addOns'

const jacket = { id: 'l1', quantity: 2, parentLineId: null }
const plan = { id: 'l2', quantity: 2, parentLineId: 'l1' }
const buttons = { id: 'l3', quantity: 4, parentLineId: 'l1' }
const mug = { id: 'l4', quantity: 1, parentLineId: null }

describe('add-ons of an order line', () => {
  it('are the lines bought with it, and no others', () => {
    expect(addOnsOf([jacket, plan, buttons, mug], 'l1')).toEqual([plan, buttons])
    expect(addOnsOf([jacket, plan, buttons, mug], 'l4')).toEqual([])
  })

  it('go back in the ratio they were sold at', () => {
    expect(addOnQuantity(plan, jacket, 1)).toBe(1)
    expect(addOnQuantity(buttons, jacket, 1)).toBe(2)
    expect(addOnQuantity(buttons, jacket, 2)).toBe(4)
  })

  it('never come to more than were bought, nor to anything for nothing', () => {
    expect(addOnQuantity(plan, jacket, 5)).toBe(2)
    expect(addOnQuantity(plan, jacket, 0)).toBe(0)
    expect(addOnQuantity(plan, { ...jacket, quantity: 0 }, 1)).toBe(0)
  })
})
