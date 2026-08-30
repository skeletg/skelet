import type { CanvasItem } from './EmailCanvas'
import type { Column, EmailBlock } from './blocks'

// Selection + immutable tree edits for the canvas - A selection is a string:
//   "<itemId>"                  → a top-level block (leaf or row)
//   "<itemId>::<col>::<idx>"    → a leaf block inside a row's column
// Rows are depth-1 (a column holds only leaf blocks), so this two-level scheme
// covers every editable position

export type Sel = { id: string; col?: number; idx?: number }

export function parseSel(sel: string): Sel {
  const [id, col, idx] = sel.split('::')
  if (col === undefined) return { id }
  return { id, col: Number(col), idx: Number(idx) }
}

export function childSel(rowId: string, col: number, idx: number): string {
  return `${rowId}::${col}::${idx}`
}

export function findBlock(items: CanvasItem[], sel: string | null): EmailBlock | null {
  if (!sel) return null
  const p = parseSel(sel)
  const item = items.find((i) => i.id === p.id)
  if (!item) return null
  if (p.col === undefined) return item.block
  return item.block.columns?.[p.col]?.blocks?.[p.idx ?? -1] ?? null
}

export function replaceBlock(items: CanvasItem[], sel: string, next: EmailBlock): CanvasItem[] {
  const p = parseSel(sel)
  return items.map((i) => {
    if (i.id !== p.id) return i
    if (p.col === undefined) return { ...i, block: next }
    const cols = (i.block.columns ?? []).map((c, ci) =>
      ci !== p.col ? c : { ...c, blocks: (c.blocks ?? []).map((b, bi) => (bi === p.idx ? next : b)) },
    )
    return { ...i, block: { ...i.block, columns: cols } }
  })
}

export function removeBlock(items: CanvasItem[], sel: string): CanvasItem[] {
  const p = parseSel(sel)
  if (p.col === undefined) return items.filter((i) => i.id !== p.id)
  return items.map((i) =>
    i.id !== p.id
      ? i
      : {
          ...i,
          block: {
            ...i.block,
            columns: (i.block.columns ?? []).map((c, ci) =>
              ci !== p.col ? c : { ...c, blocks: (c.blocks ?? []).filter((_, bi) => bi !== p.idx) },
            ),
          },
        },
  )
}

export function addToColumn(items: CanvasItem[], rowId: string, col: number, block: EmailBlock): CanvasItem[] {
  return items.map((i) =>
    i.id !== rowId
      ? i
      : {
          ...i,
          block: {
            ...i.block,
            columns: (i.block.columns ?? []).map((c, ci) =>
              ci !== col ? c : { ...c, blocks: [...(c.blocks ?? []), block] },
            ),
          },
        },
  )
}

export function moveChild(
  items: CanvasItem[],
  rowId: string,
  col: number,
  idx: number,
  dir: -1 | 1,
): CanvasItem[] {
  return items.map((i) => {
    if (i.id !== rowId) return i
    const cols = (i.block.columns ?? []).map((c, ci) => {
      if (ci !== col) return c
      const bs = [...(c.blocks ?? [])]
      const t = idx + dir
      if (t < 0 || t >= bs.length) return c
      ;[bs[idx], bs[t]] = [bs[t], bs[idx]]
      return { ...c, blocks: bs }
    })
    return { ...i, block: { ...i.block, columns: cols } }
  })
}

export function setRowColumns(items: CanvasItem[], rowId: string, columns: Column[]): CanvasItem[] {
  return items.map((i) => (i.id !== rowId ? i : { ...i, block: { ...i.block, columns } }))
}

// cloneBlock deep-copies a block (blocks are plain JSON, so a round-trip is safe)
function cloneBlock(b: EmailBlock): EmailBlock {
  return JSON.parse(JSON.stringify(b)) as EmailBlock
}

// duplicateBlock inserts a copy of the selected block immediately after it Top-level copies get a fresh canvas id (via
// mkId); in-column copies are inlined (columns hold blocks directly, not CanvasItems)
export function duplicateBlock(items: CanvasItem[], sel: string, mkId: () => string): CanvasItem[] {
  const p = parseSel(sel)
  if (p.col === undefined) {
    const idx = items.findIndex((i) => i.id === p.id)
    if (idx === -1) return items
    const copy: CanvasItem = { id: mkId(), block: cloneBlock(items[idx].block) }
    const next = [...items]
    next.splice(idx + 1, 0, copy)
    return next
  }
  return items.map((i) => {
    if (i.id !== p.id) return i
    const cols = (i.block.columns ?? []).map((c, ci) => {
      if (ci !== p.col) return c
      const bs = [...(c.blocks ?? [])]
      const orig = bs[p.idx ?? -1]
      if (!orig) return c
      bs.splice((p.idx ?? 0) + 1, 0, cloneBlock(orig))
      return { ...c, blocks: bs }
    })
    return { ...i, block: { ...i.block, columns: cols } }
  })
}
