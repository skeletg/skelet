import type { CategorySummaryFieldsFragment } from './Categories.gql'

// The tree query nests a fixed number of levels, each carrying the same fields, and the deepest
// leaves `children` out - so a walker can take any level as this node, children optional
type Deepest = CategorySummaryFieldsFragment['children'][number]['children'][number]['children'][number]['children'][number]

export type CategorySummary = Deepest & { children?: CategorySummary[] }

export type DropZone = 'before' | 'on' | 'after'

// Where a drag lands on a tree row: the top quarter drops before it, the bottom quarter after it,
// and the middle nests the dragged category under it
export function dropZone(pointerY: number, rowTop: number, rowHeight: number): DropZone {
  const pct = rowHeight > 0 ? (pointerY - rowTop) / rowHeight : 0.5
  if (pct < 0.25) return 'before'
  if (pct > 0.75) return 'after'
  return 'on'
}

// True when `targetId` is `ancestorId` itself or sits anywhere beneath it - a category dropped
// there would become its own ancestor, which the tree can't represent
export function isDescendant(cats: CategorySummary[], ancestorId: string, targetId: string): boolean {
  if (ancestorId === targetId) return true
  const find = (list: CategorySummary[]): CategorySummary | null => {
    for (const c of list) {
      if (c.id === ancestorId) return c
      const hit = find(c.children ?? [])
      if (hit) return hit
    }
    return null
  }
  const contains = (list: CategorySummary[]): boolean =>
    list.some((c) => c.id === targetId || contains(c.children ?? []))
  const ancestor = find(cats)
  return ancestor ? contains(ancestor.children ?? []) : false
}
