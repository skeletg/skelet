import { describe, expect, it } from 'vitest'
import type { ClientRect, CollisionDescriptor, DroppableContainer } from '@dnd-kit/core'
import { applyDrop, canvasCollision, parseColumn, slotBefore } from './canvasDnd'
import { END_DROP, type CanvasItem } from './EmailCanvas'

// What these pin: a palette block lands on the side of the block the pointer is on, the bare canvas never steals a
// drop from the blocks, a column takes only blocks it can hold, and moving a block into a column takes it out of the
// list
const items: CanvasItem[] = [
  { id: 'a', block: { type: 'heading', text: 'A' } },
  { id: 'b', block: { type: 'text', content: 'B' } },
  { id: 'r', block: { type: 'row', columns: [{ width: 1, blocks: [] }, { width: 1, blocks: [{ type: 'divider' }] }] } },
]
let n = 0
const mkId = () => `new_${++n}`

function rect(top: number, height: number): ClientRect {
  return { top, height, bottom: top + height, left: 0, right: 600, width: 600 }
}

describe('placing a dragged block', () => {
  it('goes before the block from its top half and after it from its bottom half', () => {
    expect(slotBefore(items, 'a', rect(100, 40), 110)).toBe('a')
    expect(slotBefore(items, 'a', rect(100, 40), 130)).toBe('b')
    expect(slotBefore(items, 'r', rect(300, 100), 390)).toBe(END_DROP)
    expect(slotBefore(items, 'gone', rect(0, 10), 5)).toBe(END_DROP)
  })

  it('inserts, moves and nests as asked', () => {
    const inserted = applyDrop(items, { kind: 'insert', block: { type: 'divider' }, before: 'b' }, mkId)
    expect(inserted.items.map((i) => i.id)).toEqual(['a', inserted.sel, 'b', 'r'])
    expect(applyDrop(items, { kind: 'insert', block: { type: 'divider' }, before: END_DROP }, mkId).items.at(-1)?.block.type).toBe('divider')

    expect(applyDrop(items, { kind: 'reorder', activeId: 'a', overId: 'r' }, mkId).items.map((i) => i.id)).toEqual(['b', 'r', 'a'])

    const nested = applyDrop(items, { kind: 'insert-into-column', block: { type: 'spacer' }, rowId: 'r', col: 1 }, mkId)
    expect(nested.sel).toBe('r::1::1')
    expect(nested.items[2].block.columns?.[1].blocks?.map((b) => b.type)).toEqual(['divider', 'spacer'])

    const moved = applyDrop(items, { kind: 'move-into-column', activeId: 'a', rowId: 'r', col: 0 }, mkId)
    expect(moved.items.map((i) => i.id)).toEqual(['b', 'r'])
    expect(moved.items[1].block.columns?.[0].blocks).toEqual([{ type: 'heading', text: 'A' }])
    expect(moved.sel).toBe('r::0::0')
    expect(applyDrop(items, { kind: 'move-into-column', activeId: 'r', rowId: 'r', col: 0 }, mkId).items).toBe(items)
  })

  it('reads a column drop target', () => {
    expect(parseColumn('col:le_ab12_3:1')).toEqual({ rowId: 'le_ab12_3', col: 1 })
    expect(parseColumn('canvas')).toBeNull()
  })
})

describe('the drop target', () => {
  const rects = new Map<string, ClientRect>([
    ['canvas', rect(0, 1000)],
    ['a', rect(100, 40)],
    ['b', rect(160, 40)],
    ['r', rect(220, 120)],
    ['col:r:0', rect(230, 100)],
  ])
  const containers = [...rects.keys()].map((id) => ({ id }) as unknown as DroppableContainer)
  const detect = (pointer: { x: number; y: number } | null, active: { id: string; data: Record<string, unknown> }, only = containers) =>
    canvasCollision((type) => type !== 'row')({
      active: { id: active.id, data: { current: active.data } } as never,
      collisionRect: rect(pointer?.y ?? 0, 10),
      droppableRects: rects as never,
      droppableContainers: only,
      pointerCoordinates: pointer,
    }).map((c: CollisionDescriptor | { id: unknown }) => c.id)

  it('is the block nearest the pointer for a palette drag, never the bare canvas', () => {
    const palette = { id: 'palette:text', data: { fromPalette: true, blockType: 'text' } }
    expect(detect({ x: 10, y: 150 }, palette)).toEqual(['a'])
    expect(detect({ x: 10, y: 600 }, palette)).toEqual(['r'])
    expect(detect({ x: 10, y: 600 }, palette, containers.filter((c) => c.id === 'canvas'))).toEqual(['canvas'])
    expect(detect({ x: 10, y: 2000 }, palette)).toEqual([])
  })

  it('is a column only for a block that fits in one, and never the row being dragged', () => {
    expect(detect({ x: 10, y: 260 }, { id: 'palette:text', data: { fromPalette: true, blockType: 'text' } })).toEqual(['col:r:0'])
    expect(detect({ x: 10, y: 260 }, { id: 'palette:row', data: { fromPalette: true, blockType: 'row' } })).toEqual(['r'])
    expect(detect({ x: 10, y: 260 }, { id: 'a', data: { blockType: 'heading' } })).toEqual(['col:r:0'])
  })
})
