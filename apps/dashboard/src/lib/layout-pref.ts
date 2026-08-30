// The shape every "reorder and hide a list of things" preference uses - Home sections, Home quick
// actions, the stat tiles. Unknown ids are dropped and new ones append in their default position,
// so adding a widget doesn't invalidate every admin's saved arrangement
//
// Two lists rather than one because items have two different defaults: most start visible and go
// into `hidden` when switched off, while an opt-in item (a Home section from the library) starts
// hidden and goes into `shown` when switched on. Absence from both means "whatever the item's own
// default is", so a new item never has to be written into every admin's stored layout
export type OrderedLayout = { order: string[]; hidden: string[]; shown?: string[] }

export const EMPTY_LAYOUT: OrderedLayout = { order: [], hidden: [] }

export function isHiddenInLayout(
  layout: OrderedLayout,
  id: string,
  defaultHidden = false,
): boolean {
  if (layout.hidden.includes(id)) return true
  return defaultHidden && !(layout.shown ?? []).includes(id)
}

export function applyLayout<T extends { id: string }>(items: T[], layout: OrderedLayout): T[] {
  return [
    ...layout.order.map((id) => items.find((i) => i.id === id)).filter((i) => i !== undefined),
    ...items.filter((i) => !layout.order.includes(i.id)),
  ]
}

// The list is what's on screen right now, so a move is expressed against the resolved order rather
// than the stored one - which may be shorter, or empty on the first drag
export function moveInLayout(
  layout: OrderedLayout,
  ids: string[],
  id: string,
  dir: -1 | 1,
): OrderedLayout {
  const from = ids.indexOf(id)
  const to = from + dir
  if (from < 0 || to < 0 || to >= ids.length) return layout
  const order = [...ids]
  const [moved] = order.splice(from, 1)
  order.splice(to, 0, moved)
  return { ...layout, order }
}

// Writes both lists, so a toggle means the same thing whichever default the item carries - showing
// clears it from `hidden` AND records it in `shown`, hiding does the reverse
export function toggleInLayout(
  layout: OrderedLayout,
  id: string,
  defaultHidden = false,
): OrderedLayout {
  const shown = layout.shown ?? []
  if (isHiddenInLayout(layout, id, defaultHidden)) {
    return {
      ...layout,
      hidden: layout.hidden.filter((h) => h !== id),
      shown: shown.includes(id) ? shown : [...shown, id],
    }
  }
  return {
    ...layout,
    hidden: layout.hidden.includes(id) ? layout.hidden : [...layout.hidden, id],
    shown: shown.filter((s) => s !== id),
  }
}
