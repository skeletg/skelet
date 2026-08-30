import { describe, expect, it } from 'vitest'
import type { CanvasItem } from './EmailCanvas'
import type { EmailBlock } from './blocks'
import {
  addToColumn,
  childSel,
  duplicateBlock,
  findBlock,
  moveChild,
  parseSel,
  removeBlock,
  replaceBlock,
  setRowColumns,
} from './treeops'

// Every edit here rewrites what the admin is composing, so the things worth pinning are that an
// edit lands on exactly one block and that a selector pointing nowhere is a no-op rather than a
// crash or, worse, a write to the wrong block

const text = (t: string): EmailBlock => ({ type: 'text', text: t })

// one leaf at the top, then a two-column row holding two blocks and one
const tree = (): CanvasItem[] => [
  { id: 'a', block: text('top') },
  {
    id: 'row',
    block: {
      type: 'row',
      columns: [
        { width: 50, blocks: [text('l1'), text('l2')] },
        { width: 50, blocks: [text('r1')] },
      ],
    },
  },
]

const colBlocks = (items: CanvasItem[], col: number) =>
  (items.find((i) => i.id === 'row')!.block.columns?.[col].blocks ?? []).map((b) => b.text)

describe('selectors', () => {
  it('reads a top-level selection as just an id', () => {
    expect(parseSel('a')).toEqual({ id: 'a' })
  })

  it('reads a column selection as id, column and index', () => {
    expect(parseSel('row::1::0')).toEqual({ id: 'row', col: 1, idx: 0 })
  })

  it('round-trips through childSel', () => {
    expect(parseSel(childSel('row', 1, 2))).toEqual({ id: 'row', col: 1, idx: 2 })
  })
})

describe('findBlock', () => {
  it('finds a top-level block and one nested in a column', () => {
    expect(findBlock(tree(), 'a')?.text).toBe('top')
    expect(findBlock(tree(), 'row::0::1')?.text).toBe('l2')
  })

  it('answers null for no selection at all', () => {
    expect(findBlock(tree(), null)).toBeNull()
  })

  // a selector can outlive the block it names - the row shrank, the block was deleted elsewhere
  it('answers null for a selector pointing past the end, not the wrong block', () => {
    expect(findBlock(tree(), 'nope')).toBeNull()
    expect(findBlock(tree(), 'row::9::0')).toBeNull()
    expect(findBlock(tree(), 'row::0::9')).toBeNull()
  })
})

describe('replaceBlock', () => {
  it('swaps a top-level block and leaves its neighbours alone', () => {
    const next = replaceBlock(tree(), 'a', text('new'))
    expect(next[0].block.text).toBe('new')
    expect(next[1]).toEqual(tree()[1])
  })

  it('swaps exactly one block inside a column', () => {
    const next = replaceBlock(tree(), 'row::0::1', text('new'))
    expect(colBlocks(next, 0)).toEqual(['l1', 'new'])
    expect(colBlocks(next, 1)).toEqual(['r1'])
  })

  it('does not mutate the tree it was given', () => {
    const before = tree()
    replaceBlock(before, 'row::0::0', text('new'))
    expect(colBlocks(before, 0)).toEqual(['l1', 'l2'])
  })
})

describe('removeBlock', () => {
  it('drops a top-level item', () => {
    expect(removeBlock(tree(), 'a').map((i) => i.id)).toEqual(['row'])
  })

  it('drops one block out of a column and shifts the rest up', () => {
    const next = removeBlock(tree(), 'row::0::0')
    expect(colBlocks(next, 0)).toEqual(['l2'])
    expect(colBlocks(next, 1)).toEqual(['r1'])
  })

  it('leaves the tree as it was for a selector that matches nothing', () => {
    expect(removeBlock(tree(), 'ghost')).toEqual(tree())
    expect(removeBlock(tree(), 'row::0::9')).toEqual(tree())
  })
})

describe('addToColumn', () => {
  it('appends to the named column only', () => {
    const next = addToColumn(tree(), 'row', 1, text('r2'))
    expect(colBlocks(next, 1)).toEqual(['r1', 'r2'])
    expect(colBlocks(next, 0)).toEqual(['l1', 'l2'])
  })

  it('ignores a row that is not there', () => {
    expect(addToColumn(tree(), 'ghost', 0, text('x'))).toEqual(tree())
  })
})

describe('moveChild', () => {
  it('swaps a block with its neighbour in either direction', () => {
    expect(colBlocks(moveChild(tree(), 'row', 0, 0, 1), 0)).toEqual(['l2', 'l1'])
    expect(colBlocks(moveChild(tree(), 'row', 0, 1, -1), 0)).toEqual(['l2', 'l1'])
  })

  // the first block cannot rise and the last cannot sink - a wrap-around would look like a delete
  it('refuses to move past either end', () => {
    expect(colBlocks(moveChild(tree(), 'row', 0, 0, -1), 0)).toEqual(['l1', 'l2'])
    expect(colBlocks(moveChild(tree(), 'row', 0, 1, 1), 0)).toEqual(['l1', 'l2'])
  })

  it('leaves the other column untouched', () => {
    expect(colBlocks(moveChild(tree(), 'row', 0, 0, 1), 1)).toEqual(['r1'])
  })
})

describe('setRowColumns', () => {
  it('replaces a row’s columns wholesale', () => {
    const next = setRowColumns(tree(), 'row', [{ width: 100, blocks: [text('only')] }])
    expect(next.find((i) => i.id === 'row')!.block.columns).toHaveLength(1)
    expect(colBlocks(next, 0)).toEqual(['only'])
  })
})

describe('duplicateBlock', () => {
  const ids = () => {
    let n = 0
    return () => `copy-${++n}`
  }

  it('puts a top-level copy directly after the original, with a fresh id', () => {
    const next = duplicateBlock(tree(), 'a', ids())
    expect(next.map((i) => i.id)).toEqual(['a', 'copy-1', 'row'])
    expect(next[1].block.text).toBe('top')
  })

  it('puts a column copy directly after the original', () => {
    const next = duplicateBlock(tree(), 'row::0::0', ids())
    expect(colBlocks(next, 0)).toEqual(['l1', 'l1', 'l2'])
  })

  // a shallow copy would have the two sharing nested state, so editing one would edit both
  it('deep-copies, so the copy and the original are independent', () => {
    const nested: CanvasItem[] = [
      { id: 'a', block: { type: 'row', columns: [{ width: 100, blocks: [text('inner')] }] } },
    ]
    const next = duplicateBlock(nested, 'a', ids())
    next[1].block.columns![0].blocks![0].text = 'changed'
    expect(next[0].block.columns![0].blocks![0].text).toBe('inner')
  })

  it('is a no-op when the selector names nothing', () => {
    expect(duplicateBlock(tree(), 'ghost', ids())).toEqual(tree())
    expect(duplicateBlock(tree(), 'row::0::9', ids())).toEqual(tree())
  })
})
