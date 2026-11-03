import { describe, expect, it, vi } from 'vitest'
import {
  CommitError,
  applyOps,
  commitOps,
  isTemp,
  newTempId,
  opsToDiff,
  reduceOps,
  type Committers,
  type GroupedMember,
  type MemberProduct,
  type PendingOp,
} from './groupedStaging'

// reduceOps runs between the admin's edits and the mutations that go out, so an op it drops or
// merges wrongly is a lost edit nobody is told about - the diff count still reads right, because it
// is computed from the same reduced list

const product = (over: Partial<MemberProduct> = {}): MemberProduct => ({
  id: 'p1',
  sku: 'SKU-1',
  name: 'Widget',
  kind: 'SIMPLE',
  defaultPrice: 1000,
  defaultCurrencyCode: 'USD',
  ...over,
})

const member = (over: Partial<GroupedMember> = {}): GroupedMember => ({
  id: 'm1',
  member: product(),
  defaultQuantity: 1,
  sortOrder: 0,
  ...over,
})

const add = (tempId: string, over: Partial<PendingOp> = {}): PendingOp =>
  ({ kind: 'add-member', tempId, product: product(), defaultQuantity: 1, sortOrder: 0, ...over }) as PendingOp

describe('temp ids', () => {
  it('marks what it mints, and nothing else', () => {
    expect(isTemp(newTempId())).toBe(true)
    expect(isTemp('019f7963-0847-7a2c-b415-5598772972b6')).toBe(false)
  })
})

describe('reduceOps', () => {
  it('drops an add that was removed again, and the remove with it', () => {
    expect(reduceOps([add('tmp_1'), { kind: 'remove-member', id: 'tmp_1' }])).toEqual([])
  })

  it('keeps a remove of a member that was already saved', () => {
    const out = reduceOps([{ kind: 'remove-member', id: 'm1' }])
    expect(out).toHaveLength(1)
  })

  it('folds an update into the add it belongs to, so only one create fires', () => {
    const out = reduceOps([add('tmp_1'), { kind: 'update-member', id: 'tmp_1', defaultQuantity: 5 }])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'add-member', tempId: 'tmp_1', defaultQuantity: 5 })
  })

  // the middle op is the one at risk: merging into the op that STARTED the run rather than into the
  // accumulated result drops whatever only it touched
  it('keeps every field when three updates land on one member', () => {
    const out = reduceOps([
      { kind: 'update-member', id: 'm1', defaultQuantity: 2 },
      { kind: 'update-member', id: 'm1', sortOrder: 9 },
      { kind: 'update-member', id: 'm1', defaultQuantity: 4 },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'update-member', id: 'm1', defaultQuantity: 4, sortOrder: 9 })
  })

  it('keeps every field when two updates fold into one add', () => {
    const out = reduceOps([
      add('tmp_1'),
      { kind: 'update-member', id: 'tmp_1', defaultQuantity: 7 },
      { kind: 'update-member', id: 'tmp_1', sortOrder: 3 },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ kind: 'add-member', defaultQuantity: 7, sortOrder: 3 })
  })

  it('keeps updates to different members apart', () => {
    const out = reduceOps([
      { kind: 'update-member', id: 'm1', defaultQuantity: 2 },
      { kind: 'update-member', id: 'm2', defaultQuantity: 3 },
    ])
    expect(out).toHaveLength(2)
  })

  // the member is going away, so the mutation would be work the server throws out
  it('drops an update to a member that is removed in the same batch', () => {
    const out = reduceOps([
      { kind: 'update-member', id: 'm1', defaultQuantity: 2 },
      { kind: 'remove-member', id: 'm1' },
    ])
    expect(out).toEqual([{ kind: 'remove-member', id: 'm1' }])
  })

  it('leaves an untouched queue alone', () => {
    const ops: PendingOp[] = [add('tmp_1'), { kind: 'update-member', id: 'm2', sortOrder: 1 }]
    expect(reduceOps(ops)).toHaveLength(2)
  })
})

describe('applyOps', () => {
  it('leaves the original list untouched', () => {
    const original = [member()]
    applyOps(original, [{ kind: 'update-member', id: 'm1', defaultQuantity: 9 }])
    expect(original[0].defaultQuantity).toBe(1)
  })

  it('shows a staged add', () => {
    const out = applyOps([], [add('tmp_1', { defaultQuantity: 3 })])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ id: 'tmp_1', defaultQuantity: 3 })
  })

  it('applies only the fields an update carries', () => {
    const out = applyOps([member({ defaultQuantity: 1, sortOrder: 5 })], [{ kind: 'update-member', id: 'm1', defaultQuantity: 9 }])
    expect(out[0].defaultQuantity).toBe(9)
    expect(out[0].sortOrder).toBe(5)
  })

  it('drops a removed member', () => {
    expect(applyOps([member()], [{ kind: 'remove-member', id: 'm1' }])).toEqual([])
  })
})

describe('opsToDiff', () => {
  it('names an added member by its product', () => {
    const rows = opsToDiff([], [add('tmp_1', { defaultQuantity: 2 })])
    expect(rows).toHaveLength(1)
    expect(rows[0].label).toContain('Widget')
  })

  it('reports only the fields an update actually changes', () => {
    const rows = opsToDiff([member({ defaultQuantity: 1, sortOrder: 0 })], [
      { kind: 'update-member', id: 'm1', defaultQuantity: 4 },
    ])
    expect(rows).toHaveLength(1)
    expect(rows[0].label).toContain('qty')
  })

  it('says nothing about an update that changes nothing', () => {
    const rows = opsToDiff([member({ defaultQuantity: 1 })], [{ kind: 'update-member', id: 'm1', defaultQuantity: 1 }])
    expect(rows).toEqual([])
  })

  it('names a removed member', () => {
    const rows = opsToDiff([member()], [{ kind: 'remove-member', id: 'm1' }])
    expect(rows[0].label).toContain('Widget')
  })
})

// Grouped and kit sections both commit through here
describe('commitOps', () => {
  it('hands back only the unapplied tail, pointed at the rows it already made', async () => {
    const c: Committers = {
      addMember: vi.fn(async () => 'm-real'),
      updateMember: vi.fn(async () => {
        throw new Error('boom')
      }),
      removeMember: vi.fn(async () => {}),
    }
    const err = await commitOps([add('tmp_m1'), { kind: 'update-member', id: 'tmp_m1', defaultQuantity: 3 }], 'p1', c).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(CommitError)
    expect(c.addMember).toHaveBeenCalledTimes(1)
    expect((err as CommitError).remaining).toEqual([{ kind: 'update-member', id: 'm-real', defaultQuantity: 3 }])
  })
})
