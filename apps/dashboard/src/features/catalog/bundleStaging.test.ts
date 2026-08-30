import { describe, expect, it } from 'vitest'
import {
  applyOps,
  isTemp,
  newTempId,
  reduceOps,
  type AddOptionOp,
  type AddSelectionOp,
  type BundleOption,
  type PendingOp,
  type SelectionProduct,
} from './bundleStaging'

// reduceOps sits between the admin's edits and the mutations that go out, and the diff count the
// header shows is computed from the same reduced list - so an op it drops or merges wrongly is a
// lost edit that nothing in the UI reports

const product = (over: Partial<SelectionProduct> = {}): SelectionProduct => ({
  id: 'p1',
  sku: 'SKU-1',
  name: 'Widget',
  kind: 'SIMPLE',
  defaultPrice: 1000,
  defaultCurrencyCode: 'USD',
  ...over,
})

const addOption = (tempId: string, over: Partial<AddOptionOp> = {}): PendingOp => ({
  kind: 'add-option',
  tempId,
  title: 'Choose one',
  inputType: 'RADIO',
  isRequired: false,
  sortOrder: 0,
  locale: 'en',
  ...over,
}) as PendingOp

const addSelection = (tempId: string, over: Partial<AddSelectionOp> = {}): PendingOp => ({
  kind: 'add-selection',
  tempId,
  optionRef: 'tmp_o1',
  product: product(),
  quantity: 1,
  isDefault: false,
  sortOrder: 0,
  ...over,
}) as PendingOp

const option = (over: Partial<BundleOption> = {}): BundleOption => ({
  id: 'o1',
  inputType: 'RADIO',
  isRequired: false,
  sortOrder: 0,
  title: 'Choose one',
  content: [{ locale: 'en', title: 'Choose one' }],
  selections: [],
  ...over,
})

describe('temp ids', () => {
  it('marks what it mints, and nothing else', () => {
    expect(isTemp(newTempId())).toBe(true)
    expect(isTemp('019f7963-0847-7a2c-b415-5598772972b6')).toBe(false)
  })
})

describe('reduceOps', () => {
  it('drops an add that was deleted again, and the delete with it', () => {
    expect(reduceOps([addOption('tmp_o1'), { kind: 'delete-option', id: 'tmp_o1' }])).toEqual([])
    expect(reduceOps([addSelection('tmp_s1'), { kind: 'delete-selection', id: 'tmp_s1' }])).toEqual([])
  })

  it('keeps a delete of something already saved', () => {
    expect(reduceOps([{ kind: 'delete-option', id: 'o1' }])).toHaveLength(1)
  })

  it('folds an update into the add it belongs to, so only one create fires', () => {
    const out = reduceOps([addOption('tmp_o1'), { kind: 'update-option', id: 'tmp_o1', title: 'Pick a size' }])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'add-option', tempId: 'tmp_o1', title: 'Pick a size' })
  })

  // the middle op is the one at risk: merging into the op that STARTED the run rather than into the
  // accumulated result drops whatever only it touched
  it('keeps every field when three updates land on one option', () => {
    const out = reduceOps([
      { kind: 'update-option', id: 'o1', title: 'First' },
      { kind: 'update-option', id: 'o1', isRequired: true },
      { kind: 'update-option', id: 'o1', title: 'Second' },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'update-option', id: 'o1', title: 'Second', isRequired: true })
  })

  it('keeps every field when three updates land on one selection', () => {
    const out = reduceOps([
      { kind: 'update-selection', id: 's1', quantity: 2 },
      { kind: 'update-selection', id: 's1', isDefault: true },
      { kind: 'update-selection', id: 's1', quantity: 5 },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'update-selection', id: 's1', quantity: 5, isDefault: true })
  })

  it('keeps every field when two updates fold into one add', () => {
    const out = reduceOps([
      addOption('tmp_o1'),
      { kind: 'update-option', id: 'tmp_o1', title: 'Pick a size' },
      { kind: 'update-option', id: 'tmp_o1', isRequired: true },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'add-option', title: 'Pick a size', isRequired: true })
  })

  it('does not merge an option update into a selection update sharing an id', () => {
    const out = reduceOps([
      { kind: 'update-option', id: 'x', title: 'A' },
      { kind: 'update-selection', id: 'x', quantity: 2 },
    ])
    expect(out).toHaveLength(2)
  })

  it('keeps updates to different ids apart', () => {
    const out = reduceOps([
      { kind: 'update-option', id: 'o1', title: 'A' },
      { kind: 'update-option', id: 'o2', title: 'B' },
    ])
    expect(out).toHaveLength(2)
  })

  // the option is going away, so the update would be work the server throws out
  it('drops an update to something deleted in the same batch', () => {
    const out = reduceOps([
      { kind: 'update-option', id: 'o1', title: 'A' },
      { kind: 'delete-option', id: 'o1' },
    ])
    expect(out).toEqual([{ kind: 'delete-option', id: 'o1' }])
  })

  it('leaves an untouched queue alone', () => {
    const ops: PendingOp[] = [addOption('tmp_o1'), { kind: 'update-option', id: 'o2', title: 'B' }]
    expect(reduceOps(ops)).toHaveLength(2)
  })
})

describe('applyOps', () => {
  it('leaves the original list untouched', () => {
    const original = [option()]
    applyOps(original, [{ kind: 'update-option', id: 'o1', isRequired: true }])
    expect(original[0].isRequired).toBe(false)
  })

  it('shows a staged option, seeded with a content row for its locale', () => {
    const out = applyOps([], [addOption('tmp_o1', { title: 'Pick a size', locale: 'fr' })])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ id: 'tmp_o1', title: 'Pick a size' })
    expect(out[0].content).toEqual([{ locale: 'fr', title: 'Pick a size' }])
  })

  it('applies only the fields an update carries', () => {
    const out = applyOps([option({ title: 'Keep', sortOrder: 4 })], [{ kind: 'update-option', id: 'o1', isRequired: true }])
    expect(out[0]).toMatchObject({ title: 'Keep', sortOrder: 4, isRequired: true })
  })

  it('drops a deleted option', () => {
    expect(applyOps([option()], [{ kind: 'delete-option', id: 'o1' }])).toEqual([])
  })

  it('nests a staged selection under its staged option', () => {
    const out = applyOps([], [addOption('tmp_o1'), addSelection('tmp_s1', { optionRef: 'tmp_o1' })])
    expect(out).toHaveLength(1)
    expect(out[0].selections.map((s) => s.id)).toEqual(['tmp_s1'])
  })

  it('drops a deleted selection but keeps its option', () => {
    const out = applyOps([], [addOption('tmp_o1'), addSelection('tmp_s1'), { kind: 'delete-selection', id: 'tmp_s1' }])
    expect(out).toHaveLength(1)
    expect(out[0].selections).toEqual([])
  })
})
