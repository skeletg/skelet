import { describe, expect, it } from 'vitest'
import {
  applyOps,
  isTemp,
  newTempId,
  optionContentFor,
  parseVisibleIfGroup,
  reduceOps,
  valueContentFor,
  type AddOptionOp,
  type AddValueOp,
  type PendingOp,
  type ProductOption,
} from './productOptionsStaging'

// Everything here stands between an admin's edits and the commit that sends them. An op dropped or
// merged wrongly is a silently lost edit, and a phantom op referencing a temp id that never became
// real fails parseID at commit and takes the whole save with it

const addOption = (over: Partial<AddOptionOp> = {}): AddOptionOp => ({
  kind: 'add-option',
  tempId: 'tmp_o1',
  inputType: 'DROPDOWN',
  isRequired: false,
  sortOrder: 0,
  title: 'Size',
  helpText: '',
  locale: 'en',
  priceValue: null,
  priceType: null,
  maxChars: null,
  maxFileBytes: null,
  fileExtensions: null,
  ...over,
})

const addValue = (over: Partial<AddValueOp> = {}): AddValueOp => ({
  kind: 'add-value',
  tempId: 'tmp_v1',
  optionRef: 'tmp_o1',
  sortOrder: 0,
  priceValue: 0,
  priceType: 'FIXED',
  isDefault: false,
  quantity: 1,
  title: 'Small',
  locale: 'en',
  ...over,
})

const option = (over: Partial<ProductOption> = {}): ProductOption =>
  ({
    id: 'o1',
    inputType: 'DROPDOWN',
    isRequired: false,
    sortOrder: 0,
    title: 'Size',
    helpText: '',
    values: [],
    ...over,
  }) as ProductOption

const kinds = (ops: PendingOp[]) => ops.map((o) => o.kind)

describe('parseVisibleIfGroup', () => {
  it('accepts a group with rules', () => {
    const raw = { logic: 'and', rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v1'] }] }
    expect(parseVisibleIfGroup(raw)).toEqual(raw)
  })

  it('accepts a group that only nests other groups', () => {
    const raw = { logic: 'or', groups: [{ rules: [] }] }
    expect(parseVisibleIfGroup(raw)).toEqual(raw)
  })

  // the JSON scalar arrives as unknown from codegen, and anything malformed has to read as
  // "no condition" rather than half a condition that hides the option from every customer
  it('reads anything else as no condition at all', () => {
    expect(parseVisibleIfGroup(null)).toBeNull()
    expect(parseVisibleIfGroup(undefined)).toBeNull()
    expect(parseVisibleIfGroup('{"rules":[]}')).toBeNull()
    expect(parseVisibleIfGroup(42)).toBeNull()
    expect(parseVisibleIfGroup([{ option_id: 'o1' }])).toBeNull()
    expect(parseVisibleIfGroup({})).toBeNull()
    expect(parseVisibleIfGroup({ logic: 'and' })).toBeNull()
  })
})

describe('locale fallback', () => {
  it('prefers the row for the asked-for locale', () => {
    const o = option({ title: 'Size', helpText: 'Pick one', content: [{ locale: 'fr', title: 'Taille', helpText: 'Choisis' }] })
    expect(optionContentFor(o, 'fr')).toEqual({ title: 'Taille', helpText: 'Choisis' })
  })

  it('falls back to the resolved scalar when that locale has no row yet', () => {
    const o = option({ title: 'Size', helpText: 'Pick one', content: [{ locale: 'fr', title: 'Taille', helpText: 'Choisis' }] })
    expect(optionContentFor(o, 'de')).toEqual({ title: 'Size', helpText: 'Pick one' })
    expect(optionContentFor(option({ title: 'Size', helpText: 'h' }), 'en')).toEqual({ title: 'Size', helpText: 'h' })
  })

  it('does the same for a value', () => {
    const v = { id: 'v1', title: 'Small', content: [{ locale: 'fr', title: 'Petit' }] } as never
    expect(valueContentFor(v, 'fr')).toBe('Petit')
    expect(valueContentFor(v, 'de')).toBe('Small')
  })
})

describe('temp ids', () => {
  it('marks what it mints, and nothing else', () => {
    const id = newTempId()
    expect(isTemp(id)).toBe(true)
    expect(isTemp('019f7963-0847-7a2c-b415-5598772972b6')).toBe(false)
  })

  it('does not repeat itself', () => {
    expect(new Set(Array.from({ length: 100 }, newTempId)).size).toBe(100)
  })
})

describe('reduceOps', () => {
  it('leaves unrelated ops alone and in order', () => {
    const ops: PendingOp[] = [addOption(), { kind: 'delete-option', id: 'o9' }]
    expect(kinds(reduceOps(ops))).toEqual(['add-option', 'delete-option'])
  })

  // a staged add later deleted never existed - leaving the delete behind would send a temp id to
  // the backend, which fails parseID and takes the whole commit with it
  it('cancels an add that is later deleted, and the delete with it', () => {
    const ops: PendingOp[] = [addOption(), { kind: 'delete-option', id: 'tmp_o1' }]
    expect(reduceOps(ops)).toEqual([])
  })

  it('takes the values staged under a cancelled option with it', () => {
    const ops: PendingOp[] = [
      addOption(),
      addValue(),
      { kind: 'update-value', id: 'tmp_v1', title: 'Smaller' },
      { kind: 'delete-option', id: 'tmp_o1' },
    ]
    expect(reduceOps(ops)).toEqual([])
  })

  it('keeps a delete of something that was already real', () => {
    const ops: PendingOp[] = [{ kind: 'delete-option', id: 'o1' }]
    expect(kinds(reduceOps(ops))).toEqual(['delete-option'])
  })

  it('does not cancel an add that was deleted BEFORE it was staged', () => {
    // ids are minted fresh, so this can only be a stale op - the add still stands
    const ops: PendingOp[] = [{ kind: 'delete-option', id: 'tmp_o1' }, addOption()]
    expect(kinds(reduceOps(ops))).toEqual(['delete-option', 'add-option'])
  })

  it('merges consecutive updates to one op, last write winning per field', () => {
    const ops: PendingOp[] = [
      { kind: 'update-option', id: 'o1', title: 'First' },
      { kind: 'update-option', id: 'o1', helpText: 'Added' },
      { kind: 'update-option', id: 'o1', title: 'Second' },
    ]
    const out = reduceOps(ops)
    expect(out).toHaveLength(1)
    // the middle op is the one at risk: merging into the op that STARTED the run rather than into
    // the accumulated result drops whatever only it touched
    expect(out[0]).toMatchObject({ kind: 'update-option', id: 'o1', title: 'Second', helpText: 'Added' })
  })

  it('keeps updates to different ids apart', () => {
    const ops: PendingOp[] = [
      { kind: 'update-option', id: 'o1', title: 'A' },
      { kind: 'update-option', id: 'o2', title: 'B' },
    ]
    expect(reduceOps(ops)).toHaveLength(2)
  })

  it('does not merge an option update into a value update of the same id', () => {
    const ops: PendingOp[] = [
      { kind: 'update-option', id: 'x', title: 'A' },
      { kind: 'update-value', id: 'x', title: 'B' },
    ]
    expect(reduceOps(ops)).toHaveLength(2)
  })
})

describe('applyOps', () => {
  it('leaves the original list untouched', () => {
    const original = [option()]
    applyOps(original, [{ kind: 'update-option', id: 'o1', title: 'Renamed' }])
    expect(original[0].title).toBe('Size')
  })

  it('shows a staged add in the list', () => {
    const out = applyOps([], [addOption({ title: 'Colour' })])
    expect(out.map((o) => o.title)).toEqual(['Colour'])
    expect(isTemp(out[0].id)).toBe(true)
  })

  it('applies a non-copy update to an existing option', () => {
    const out = applyOps([option()], [{ kind: 'update-option', id: 'o1', isRequired: true, sortOrder: 3 }])
    expect(out[0].isRequired).toBe(true)
    expect(out[0].sortOrder).toBe(3)
  })

  // copy edits land on the targeted locale's row, never the resolved scalar - otherwise translating
  // one locale would clobber the fallback every other locale reads
  it('writes a title edit to its locale row and leaves the scalar alone', () => {
    const out = applyOps([option()], [{ kind: 'update-option', id: 'o1', title: 'Taille', titleLocale: 'fr' }])
    expect(out[0].title).toBe('Size')
    expect(optionContentFor(out[0], 'fr').title).toBe('Taille')
    expect(optionContentFor(out[0], 'de').title).toBe('Size')
  })

  it('ignores a copy edit that names no locale, rather than guessing one', () => {
    const out = applyOps([option()], [{ kind: 'update-option', id: 'o1', title: 'Renamed' }])
    expect(out[0].title).toBe('Size')
    expect(out[0].content ?? []).toEqual([])
  })

  it('drops a deleted option', () => {
    expect(applyOps([option()], [{ kind: 'delete-option', id: 'o1' }])).toEqual([])
  })

  it('nests a staged value under its staged option', () => {
    const out = applyOps([], [addOption(), addValue({ title: 'Small' })])
    expect(out[0].values.map((v) => v.title)).toEqual(['Small'])
  })

  it('drops a deleted value but keeps its option', () => {
    const staged = applyOps([], [addOption(), addValue(), { kind: 'delete-value', id: 'tmp_v1' }])
    expect(staged).toHaveLength(1)
    expect(staged[0].values).toEqual([])
  })
})
