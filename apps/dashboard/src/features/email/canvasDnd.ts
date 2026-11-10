import { useRef, useState } from 'react'
import {
  closestCenter,
  pointerWithin,
  PointerSensor,
  useSensor,
  useSensors,
  type ClientRect,
  type Collision,
  type CollisionDetection,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
  type DroppableContainer,
} from '@dnd-kit/core'
import { arrayMove } from '@dnd-kit/sortable'
import type { EmailBlock } from './blocks'
import { END_DROP, type CanvasItem } from './EmailCanvas'
import { addToColumn, childSel, removeBlock } from './treeops'

export type Drop =
  | { kind: 'insert'; block: EmailBlock; before: string }
  | { kind: 'insert-into-column'; block: EmailBlock; rowId: string; col: number }
  | { kind: 'reorder'; activeId: string; overId: string }
  | { kind: 'move-into-column'; activeId: string; rowId: string; col: number }

export function parseColumn(id: string): { rowId: string; col: number } | null {
  const m = /^col:(.+):(\d+)$/.exec(id)
  return m ? { rowId: m[1], col: Number(m[2]) } : null
}

function columnLength(items: CanvasItem[], rowId: string, col: number): number {
  return items.find((i) => i.id === rowId)?.block.columns?.[col]?.blocks?.length ?? 0
}

// Before the block under the pointer, or after it from its lower half on - as the id it goes before, or END_DROP
export function slotBefore(items: CanvasItem[], overId: string, rect: Pick<ClientRect, 'top' | 'height'>, pointerY: number): string {
  const i = items.findIndex((it) => it.id === overId)
  if (i === -1) return END_DROP
  return pointerY > rect.top + rect.height / 2 ? (items[i + 1]?.id ?? END_DROP) : overId
}

export function applyDrop(items: CanvasItem[], drop: Drop, mkId: () => string): { items: CanvasItem[]; sel: string | null } {
  switch (drop.kind) {
    case 'insert': {
      const item = { id: mkId(), block: drop.block }
      const at = drop.before === END_DROP ? -1 : items.findIndex((i) => i.id === drop.before)
      const next = [...items]
      next.splice(at === -1 ? items.length : at, 0, item)
      return { items: next, sel: item.id }
    }
    case 'insert-into-column':
      return {
        items: addToColumn(items, drop.rowId, drop.col, drop.block),
        sel: childSel(drop.rowId, drop.col, columnLength(items, drop.rowId, drop.col)),
      }
    case 'reorder': {
      const from = items.findIndex((i) => i.id === drop.activeId)
      const to = items.findIndex((i) => i.id === drop.overId)
      if (from === -1 || to === -1 || from === to) return { items, sel: drop.activeId }
      return { items: arrayMove(items, from, to), sel: drop.activeId }
    }
    case 'move-into-column': {
      const moving = items.find((i) => i.id === drop.activeId)
      if (!moving || drop.rowId === drop.activeId || !items.some((i) => i.id === drop.rowId)) return { items, sel: null }
      return {
        items: addToColumn(removeBlock(items, drop.activeId), drop.rowId, drop.col, moving.block),
        sel: childSel(drop.rowId, drop.col, columnLength(items, drop.rowId, drop.col)),
      }
    }
  }
}

function nearestByPointer(y: number, blocks: DroppableContainer[], rects: Map<DroppableContainer['id'], ClientRect>): Collision[] {
  let best: Collision | null = null
  for (const c of blocks) {
    const r = rects.get(c.id)
    if (!r) continue
    const d = y < r.top ? r.top - y : y > r.bottom ? y - r.bottom : 0
    if (!best || d < (best.data?.value as number)) best = { id: c.id, data: { droppableContainer: c, value: d } }
  }
  return best ? [best] : []
}

// Columns first when they can hold the block; the bare canvas is a target only while it's empty
export function canvasCollision(canNest: (type: string) => boolean): CollisionDetection {
  return (args) => {
    const within = pointerWithin(args)
    if (!within.some((c) => c.id === 'canvas')) return []
    const data = args.active.data.current
    const activeId = String(args.active.id)
    if (canNest(String(data?.blockType ?? ''))) {
      const col = within.find((c) => String(c.id).startsWith('col:') && !String(c.id).startsWith(`col:${activeId}:`))
      if (col) return [col]
    }
    const blocks = args.droppableContainers.filter((c) => c.id !== 'canvas' && !String(c.id).startsWith('col:'))
    if (blocks.length === 0) return within.filter((c) => c.id === 'canvas')
    if (data?.fromPalette) return args.pointerCoordinates ? nearestByPointer(args.pointerCoordinates.y, blocks, args.droppableRects) : []
    return closestCenter({ ...args, droppableContainers: blocks })
  }
}

type Options = {
  items: CanvasItem[]
  setItems: (update: (items: CanvasItem[]) => CanvasItem[]) => void
  makeBlock: (type: string) => EmailBlock
  canNest: (type: string) => boolean
  nextId: () => string
  onPlaced: (sel: string | null) => void
}

export function useCanvasDnd({ items, setItems, makeBlock, canNest, nextId, onPlaced }: Options) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const [dropBefore, setDropBefore] = useState<string | null>(null)
  const pointerY = useRef(0)
  const untrack = useRef<(() => void) | null>(null)

  function stop() {
    untrack.current?.()
    untrack.current = null
    setDropBefore(null)
  }

  function onDragStart(e: DragStartEvent) {
    const start = e.activatorEvent as PointerEvent
    pointerY.current = start.clientY ?? 0
    const move = (ev: PointerEvent) => {
      pointerY.current = ev.clientY
    }
    window.addEventListener('pointermove', move, { passive: true })
    untrack.current = () => window.removeEventListener('pointermove', move)
  }

  function onDragMove(e: DragMoveEvent) {
    if (!e.active.data.current?.fromPalette) return
    const over = e.over
    const next = !over || parseColumn(String(over.id)) ? null : over.id === 'canvas' ? END_DROP : slotBefore(items, String(over.id), over.rect, pointerY.current)
    if (next !== dropBefore) setDropBefore(next)
  }

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e
    const before = over && active.data.current?.fromPalette && over.id !== 'canvas' ? slotBefore(items, String(over.id), over.rect, pointerY.current) : END_DROP
    stop()
    if (!over) return
    const overId = String(over.id)
    const column = parseColumn(overId)
    const data = active.data.current
    let drop: Drop
    if (data?.fromPalette) {
      const block = makeBlock(String(data.blockType))
      drop = column ? { kind: 'insert-into-column', block, ...column } : { kind: 'insert', block, before }
    } else if (column) {
      drop = { kind: 'move-into-column', activeId: String(active.id), ...column }
    } else {
      if (overId === 'canvas') return
      drop = { kind: 'reorder', activeId: String(active.id), overId }
    }
    const result = applyDrop(items, drop, nextId)
    if (result.items !== items) setItems(() => result.items)
    onPlaced(result.sel)
  }

  return {
    dropBefore,
    dndProps: { sensors, collisionDetection: canvasCollision(canNest), onDragStart, onDragMove, onDragEnd, onDragCancel: stop },
  }
}
