import { describe, expect, it } from 'vitest'
import { optionRuleEffects } from './optionRules'
import { visibleOptionPicks } from './optionVisibility'

// What these pin: the console reads the same rules the cart applies - a hidden option's pick never counts, require
// beats optional, and quantities that disagree are flagged

const gift = { id: 'gift', visibleIf: null }
const note = { id: 'note', visibleIf: { rules: [{ option_id: 'gift', op: 'anyOf', value_ids: ['yes'] }] } }
const whenGift = { rules: [{ option_id: 'gift', op: 'anyOf', value_ids: ['yes'] }] }

describe('console option rules', () => {
  it('fires on visible picks only', () => {
    const { visible, picks } = visibleOptionPicks([gift, note], (id) => (id === 'gift' ? ['no'] : []), () => false)
    expect(visible.has('note')).toBe(false)
    const effects = optionRuleEffects([{ condition: whenGift, actions: [{ type: 'REQUIRE', optionId: 'note' }] }], picks)
    expect(effects.isRequired({ id: 'note', isRequired: false })).toBe(false)
  })

  it('requires over optional, and flags quantities that disagree', () => {
    const { picks } = visibleOptionPicks([gift, note], (id) => (id === 'gift' ? ['yes'] : []), () => false)
    const effects = optionRuleEffects(
      [
        { condition: whenGift, actions: [{ type: 'OPTIONAL', optionId: 'note' }, { type: 'SET_QUANTITY', quantity: 2 }] },
        { condition: whenGift, actions: [{ type: 'REQUIRE', optionId: 'note' }, { type: 'SET_QUANTITY', quantity: 3 }] },
      ],
      picks,
    )
    expect(effects.isRequired({ id: 'note', isRequired: false })).toBe(true)
    expect(effects.conflict).toBe(true)
  })
})
