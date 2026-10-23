import { describe, expect, it } from 'vitest'
import type { ProductOption } from '../productOptionsStaging'
import { buildOptionsModel } from './options-graph'
import { estimateBoxes, layoutOptionsMap } from './options-layout'
import { ruleToInput, rulesDiff, rulesUsing, type OptionRule } from './rules'

// What these pin: a rule reads back in words, a new one saves without an id, the diff names what changed or moved,
// and the map reads top to bottom like the product page

const option = (id: string, title: string, over: Partial<ProductOption> = {}): ProductOption => ({
  id, title, helpText: '', inputType: 'DROPDOWN', isRequired: false, sortOrder: 0, values: [],
  content: [{ locale: 'en', title, helpText: '' }], ...over,
})

const finish = option('o1', 'Finish', {
  values: [
    { id: 'v1', title: 'Gloss', sortOrder: 0, priceValue: 0, priceType: 'FIXED', isDefault: false, quantity: 1 },
    { id: 'v2', title: 'Matte', sortOrder: 1, priceValue: 0, priceType: 'FIXED', isDefault: false, quantity: 1 },
  ],
})
const note = option('o2', 'Gift note', { inputType: 'AREA', sortOrder: 1 })
const colour = option('o3', 'Engraving colour', { inputType: 'FIELD', sortOrder: 2, visibleIf: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v1'] }] } })
const all = [finish, note, colour]

const saved: OptionRule = {
  id: 'r1',
  labels: { en: 'Matte set' },
  condition: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v2'] }] },
  actions: [{ type: 'REQUIRE', optionId: 'o2' }, { type: 'ADJUST_PRICE', priceType: 'FIXED', priceValue: -250, channelPrices: [] }],
}

describe('option rules', () => {
  it('saves a new rule without an id and drops blank labels', () => {
    expect(ruleToInput({ ...saved, id: 'tmp_x', labels: { en: 'Matte set', de: ' ' } })).toEqual({
      labels: [{ locale: 'en', label: 'Matte set' }],
      condition: saved.condition,
      actions: [{ type: 'REQUIRE', optionId: 'o2' }, { type: 'ADJUST_PRICE', priceType: 'FIXED', priceValue: -250, channelPrices: [] }],
    })
    expect(ruleToInput(saved).id).toBe('r1')
  })

  it('words additions, edits and removals for the save dialog', () => {
    const edited: OptionRule = { ...saved, actions: [{ type: 'SET_QUANTITY', quantity: 2 }] }
    const added: OptionRule = { ...saved, id: 'tmp_new' }
    const rows = rulesDiff([saved], [edited, added], all, 'en', 'USD')
    expect(rows).toEqual([
      { kind: 'change', label: 'Rule 1', before: 'When Finish is Matte: Require Gift note, Price −$2.50', after: 'When Finish is Matte: Set quantity to 2' },
      expect.objectContaining({ kind: 'add', label: 'Rule 2' }),
    ])
    expect(rulesDiff([saved], [], all, 'en', 'USD')).toEqual([expect.objectContaining({ kind: 'remove' })])
  })

  it('words a move as positions, and an add or delete alone moves nothing', () => {
    const other: OptionRule = { ...saved, id: 'r2', labels: {}, actions: [{ type: 'SET_QUANTITY', quantity: 2 }] }
    expect(rulesDiff([saved, other], [other, saved], all, 'en', 'USD')).toEqual([
      { kind: 'change', label: 'Rule · When Finish is Matte · position', before: '2', after: '1' },
      { kind: 'change', label: 'Rule · Matte set · position', before: '1', after: '2' },
    ])
    const fresh: OptionRule = { ...saved, id: 'tmp_new' }
    expect(rulesDiff([saved, other], [fresh, other], all, 'en', 'USD')).toEqual([
      expect.objectContaining({ kind: 'add', label: 'Rule 1' }),
      expect.objectContaining({ kind: 'remove' }),
    ])
  })

  it('knows which rules read a choice or act on an option', () => {
    expect(rulesUsing([saved], { valueId: 'v2' })).toEqual([saved])
    expect(rulesUsing([saved], { optionId: 'o2' })).toEqual([saved])
    expect(rulesUsing([saved], { valueId: 'v1' })).toEqual([])
  })

  it('maps the options top to bottom as a customer meets them, branches right, rules left', () => {
    const model = buildOptionsModel(all, [saved], 'en', 'en', 'USD')
    expect(model.options.map((o) => [o.id, o.row, o.branch])).toEqual([['opt:o1', 0, false], ['opt:o2', 1, false], ['opt:o3', 2, true]])
    expect(model.rules.map((r) => [r.id, r.row])).toEqual([['rule:r1', 1]])
    const links = Object.fromEntries(model.links.map((l) => [l.id, l]))
    expect(['path:start', 'path:o1', 'path:o2', 'path:o3'].map((id) => [links[id].kind, links[id].to])).toEqual([
      ['path', 'opt:o1'], ['path', 'opt:o2'], ['path', 'cart'], ['rejoin', 'cart'],
    ])
    expect(links['show:o3<-o1:v1']).toMatchObject({ kind: 'show', from: 'opt:o1', choice: 'v1' })
    expect(links['rule:r1<-o1:v2']).toMatchObject({ kind: 'read', choice: 'v2' })
    expect(links['rule:r1->o2']).toMatchObject({ kind: 'act', to: 'opt:o2' })
    expect(links['rule:r1->cart']).toMatchObject({ kind: 'fx' })
    expect(model.rules[0].data).toMatchObject({ title: 'Matte set', effects: ['Require Gift note', 'Price −$2.50'] })

    const boxes = estimateBoxes(model)
    const { positions } = layoutOptionsMap(model, (id) => boxes.get(id) ?? { w: 0, h: 0, ports: {} })
    const at = (id: string) => positions.get(id) ?? { x: NaN, y: NaN }
    expect(at('opt:o1').y < at('opt:o2').y && at('opt:o2').y < at('opt:o3').y && at('opt:o3').y < at('cart').y).toBe(true)
    expect(at('opt:o3').x).toBeGreaterThan(at('opt:o1').x)
    expect(at('rule:r1').x).toBeLessThan(at('opt:o1').x)
  })

  it('keeps a choice a condition reads on its card past the cap, chains a branch, and draws a shared condition once', () => {
    const many = option('o1', 'Size', {
      values: Array.from({ length: 9 }, (_, n) => ({ id: `s${n}`, title: `S${n}`, sortOrder: n, priceValue: 0, priceType: 'FIXED' as const, isDefault: false, quantity: 1 })),
    })
    const late = option('o2', 'Sleeve', { sortOrder: 1, visibleIf: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['s8'] }] } })
    const filled = option('o3', 'Note', { inputType: 'FIELD', sortOrder: 2, visibleIf: { rules: [{ option_id: 'o2', op: 'filled' }] } })
    const twin = option('o4', 'Note colour', { inputType: 'FIELD', sortOrder: 3, visibleIf: filled.visibleIf })
    const model = buildOptionsModel([many, late, filled, twin], [], 'en', 'en', 'USD')
    expect(model.options[0].data).toMatchObject({ choices: expect.arrayContaining([{ id: 's8', title: 'S8' }]), more: 2, linked: ['s8'] })
    expect(model.links.find((l) => l.id === 'show:o3<-o2')).toMatchObject({ choice: null })
    expect(model.links.find((l) => l.id === 'path:o2')).toMatchObject({ kind: 'chain', to: 'opt:o3' })
    expect(model.links.filter((l) => l.to === 'opt:o4').map((l) => l.id)).toEqual(['path:o3'])
  })
})
