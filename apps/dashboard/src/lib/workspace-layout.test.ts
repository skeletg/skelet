import { describe, expect, it } from 'vitest'
import {
  layoutPaneIds,
  paneNode,
  parseLayout,
  removePane,
  setSplitSizes,
  splitPane,
  type LayoutNode,
  type SplitNode,
} from './workspace-layout'

const asSplit = (node: LayoutNode): SplitNode => {
  if (node.kind !== 'split') throw new Error('expected a split')
  return node
}

describe('splitPane', () => {
  it('turns a lone pane into a split, on the side the drop landed', () => {
    const left = asSplit(splitPane(paneNode('a'), 'a', 'b', 'left'))
    expect(left.dir).toBe('row')
    expect(layoutPaneIds(left)).toEqual(['b', 'a'])

    const below = asSplit(splitPane(paneNode('a'), 'a', 'b', 'bottom'))
    expect(below.dir).toBe('col')
    expect(layoutPaneIds(below)).toEqual(['a', 'b'])
  })

  it('joins a split already running the same way instead of nesting', () => {
    const two = splitPane(paneNode('a'), 'a', 'b', 'right')
    const three = asSplit(splitPane(two, 'b', 'c', 'right'))
    expect(three.children.every((c) => c.kind === 'pane')).toBe(true)
    expect(layoutPaneIds(three)).toEqual(['a', 'b', 'c'])
    // the pane that was dropped on gives up half its own weight, nobody else moves
    expect(three.sizes).toEqual([50, 25, 25])
  })

  it('nests when the drop runs across the existing split', () => {
    const row = splitPane(paneNode('a'), 'a', 'b', 'right')
    const nested = asSplit(splitPane(row, 'b', 'c', 'bottom'))
    expect(nested.dir).toBe('row')
    expect(asSplit(nested.children[1]).dir).toBe('col')
    expect(layoutPaneIds(nested)).toEqual(['a', 'b', 'c'])
  })
})

describe('removePane', () => {
  it('collapses a split back to its last remaining child', () => {
    const two = splitPane(paneNode('a'), 'a', 'b', 'right')
    expect(removePane(two, 'b')).toEqual(paneNode('a'))
  })

  it("hands the leaver's weight to the panes that stay", () => {
    const three = splitPane(splitPane(paneNode('a'), 'a', 'b', 'right'), 'b', 'c', 'right')
    const left = asSplit(removePane(three, 'c')!)
    expect(layoutPaneIds(left)).toEqual(['a', 'b'])
    expect(left.sizes.reduce((x, y) => x + y, 0)).toBeCloseTo(100)
  })

  it('reports an emptied tree rather than returning a husk', () => {
    expect(removePane(paneNode('a'), 'a')).toBeNull()
  })

  it('reaches panes nested inside other splits', () => {
    const nested = splitPane(splitPane(paneNode('a'), 'a', 'b', 'right'), 'b', 'c', 'bottom')
    expect(layoutPaneIds(removePane(nested, 'c')!)).toEqual(['a', 'b'])
  })
})

describe('setSplitSizes', () => {
  it('only touches the split it names', () => {
    const nested = splitPane(splitPane(paneNode('a'), 'a', 'b', 'right'), 'b', 'c', 'bottom')
    const inner = asSplit(asSplit(nested).children[1])
    const next = asSplit(setSplitSizes(nested, inner.id, [70, 30]))
    expect(asSplit(next.children[1]).sizes).toEqual([70, 30])
    expect(next.sizes).toEqual(asSplit(nested).sizes)
  })
})

describe('parseLayout', () => {
  it('accepts what it wrote', () => {
    const tree = splitPane(paneNode('a'), 'a', 'b', 'right')
    expect(parseLayout(JSON.parse(JSON.stringify(tree)))).toEqual(tree)
  })

  it('rejects trees that would render holes', () => {
    expect(parseLayout(null)).toBeNull()
    expect(parseLayout({ kind: 'pane' })).toBeNull()
    expect(parseLayout({ kind: 'split', dir: 'row', sizes: [50], children: [paneNode('a')] })).toBeNull()
    expect(
      parseLayout({ kind: 'split', dir: 'diagonal', sizes: [50, 50], children: [paneNode('a'), paneNode('b')] }),
    ).toBeNull()
    expect(
      parseLayout({ kind: 'split', dir: 'row', sizes: [50, 50], children: [paneNode('a'), { kind: 'pane' }] }),
    ).toBeNull()
  })
})
