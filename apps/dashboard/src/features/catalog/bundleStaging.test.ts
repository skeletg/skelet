import { describe, expect, it, vi } from 'vitest'
import {
  CommitError,
  applyOps,
  selectionLimitText,
  commitOps,
  opsToDiff,
  isTemp,
  newTempId,
  reduceOps,
  type AddOptionOp,
  type AddSelectionOp,
  type BundleOption,
  type Committers,
  type PendingOp,
  type SelectionProduct,
} from './bundleStaging'
import { toStructureOps } from './bundleStaging'

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
  minSelections: 0,
  maxSelections: null,
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
  minSelections: 0,
  maxSelections: null,
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

describe('commitOps', () => {
  it('hands back only the unapplied tail, pointed at the rows it already made', async () => {
    const noop = vi.fn(async () => {})
    const c: Committers = {
      addOption: vi.fn(async () => 'o-real'), updateOption: noop, upsertOptionContent: noop, deleteOption: noop, deleteOptionContent: noop,
      addSelection: vi.fn(async () => {
        throw new Error('boom')
      }),
      updateSelection: noop, deleteSelection: noop,
    }
    const err = await commitOps([addOption('tmp_o1'), addSelection('tmp_s1', { optionRef: 'tmp_o1' })], 'p1', c).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(CommitError)
    expect(c.addOption).toHaveBeenCalledTimes(1)
    expect((err as CommitError).remaining).toEqual([expect.objectContaining({ kind: 'add-selection', tempId: 'tmp_s1', optionRef: 'o-real' })])
  })
})

describe('a staged translation removal', () => {
  const translated = option({ content: [{ locale: 'en', title: 'Size' }, { locale: 'sr', title: 'Veličina' }] })
  const drop: PendingOp = { kind: 'delete-content', id: 'o1', locale: 'sr' }

  it('hides the locale until saved, and words the change', () => {
    expect(applyOps([translated], [drop])[0].content).toEqual([{ locale: 'en', title: 'Size' }])
    expect(opsToDiff([translated], [drop])).toEqual([expect.objectContaining({ kind: 'remove', label: 'Option · Choose one · sr title' })])
  })

  it('is dropped when the option itself is deleted', () => {
    expect(reduceOps([drop, { kind: 'delete-option', id: 'o1' }])).toEqual([{ kind: 'delete-option', id: 'o1' }])
  })

  it('runs on save', async () => {
    const noop = vi.fn(async () => {})
    const deleteOptionContent = vi.fn(async () => {})
    const c: Committers = {
      addOption: vi.fn(async () => ''), updateOption: noop, upsertOptionContent: noop, deleteOption: noop, deleteOptionContent,
      addSelection: vi.fn(async () => ''), updateSelection: noop, deleteSelection: noop,
    }
    await commitOps([drop], 'p1', c)
    expect(deleteOptionContent).toHaveBeenCalledWith({ optionId: 'o1', locale: 'sr' })
  })
})

describe('pick limits', () => {
  it('reads as the customer would hear it', () => {
    expect([selectionLimitText(3, 3), selectionLimitText(1, 3), selectionLimitText(2, null), selectionLimitText(0, 4), selectionLimitText(0, null)])
      .toEqual(['pick 3', 'pick 1–3', 'pick 2+', 'up to 4', null])
  })

  it('stages, words and commits a cleared maximum', async () => {
    const op: PendingOp = { kind: 'update-option', id: 'o1', minSelections: 2, maxSelections: null }
    expect(applyOps([option({ maxSelections: 5 })], [op])[0]).toMatchObject({ minSelections: 2, maxSelections: null })
    expect(opsToDiff([option({ maxSelections: 5 })], [op])).toEqual([
      expect.objectContaining({ label: 'Option · Choose one · at least', before: '0', after: '2' }),
      expect.objectContaining({ label: 'Option · Choose one · at most', before: '5', after: 'no limit' }),
    ])
  })

  it('folds limits set before the first save into the add', () => {
    const [add] = reduceOps([addOption('tmp_o1'), { kind: 'update-option', id: 'tmp_o1', minSelections: 3, maxSelections: 3 }])
    expect(add).toMatchObject({ kind: 'add-option', minSelections: 3, maxSelections: 3 })
  })
})

// What this pins: the staged queue becomes one ordered setBundleStructure call - adds carry their tempIds for later edits
// to name, a title edit becomes a translation write, a cleared bound says so, and edits to never-saved rows are dropped
describe('toStructureOps', () => {
  it('turns the queue into one ordered batch', () => {
    const product = { id: 'p2', sku: 'MUG', name: 'Mug', kind: 'SIMPLE', defaultPrice: 500, defaultCurrencyCode: 'EUR' }
    const ops = toStructureOps([
      { kind: 'add-option', tempId: 'tmp_1', title: 'Mug', inputType: 'RADIO', isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, locale: 'en' },
      { kind: 'add-selection', tempId: 'tmp_2', optionRef: 'tmp_1', product, quantity: 2, isDefault: true, sortOrder: 0 },
      { kind: 'update-option', id: 'o1', title: 'Cups', titleLocale: 'sr', maxSelections: null },
      { kind: 'delete-selection', id: 'tmp_9' },
      { kind: 'delete-selection', id: 's1' },
    ], 'p1')
    expect(ops).toEqual([
      { tempId: 'tmp_1', addOption: { productId: 'p1', inputType: 'RADIO', isRequired: true, sortOrder: 0, minSelections: 0, locale: 'en', title: 'Mug' } },
      { tempId: 'tmp_2', addSelection: { optionId: 'tmp_1', productId: 'p2', quantity: 2, isDefault: true, sortOrder: 0 } },
      { updateOption: { id: 'o1', clearMaxSelections: true } },
      { upsertContent: { optionId: 'o1', locale: 'sr', title: 'Cups' } },
      { deleteSelection: 's1' },
    ])
  })
})

// What this pins: a rule staged on a new option follows its add in the batch, and dropping one sends a null category
describe('toStructureOps with rules', () => {
  it('sets and drops option rules', () => {
    const rule = { categoryId: 'c1', attributeValueIds: ['v1'], quantity: 2 }
    expect(toStructureOps([
      { kind: 'add-option', tempId: 'tmp_1', title: 'Cups', inputType: 'CHECKBOX', isRequired: false, sortOrder: 0, minSelections: 0, maxSelections: null, locale: 'en', rule },
      { kind: 'update-option', id: 'o2', rule: null },
    ], 'p1').slice(1)).toEqual([
      { setRule: { optionId: 'tmp_1', categoryId: 'c1', attributeValueIds: ['v1'], quantity: 2 } },
      { setRule: { optionId: 'o2', categoryId: null } },
    ])
  })
})
