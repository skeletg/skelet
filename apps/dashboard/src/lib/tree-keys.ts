import type { KeyboardEvent } from 'react'

// Keyboard for a row in a role="tree" of focusable treeitems (one of them tabbable): Enter or Space opens, the
// side arrows expand and collapse, and up / down / Home / End move focus between the tree's rows
export function onTreeItemKeyDown(
  e: KeyboardEvent<HTMLElement>,
  row: { expanded: boolean; hasChildren: boolean; onToggle: () => void; onOpen: () => void },
) {
  if (e.target !== e.currentTarget) return // a control inside the row keeps its own keys
  const rows = () => [...(e.currentTarget.closest('[role="tree"]')?.querySelectorAll<HTMLElement>('[role="treeitem"]') ?? [])]
  const move = (pick: (all: HTMLElement[], at: number) => HTMLElement | undefined) => {
    const all = rows()
    pick(all, all.indexOf(e.currentTarget))?.focus()
  }
  switch (e.key) {
    case 'Enter':
    case ' ':
      row.onOpen()
      break
    case 'ArrowRight':
      if (row.hasChildren && !row.expanded) row.onToggle()
      else move((all, at) => all[at + 1])
      break
    case 'ArrowLeft':
      if (row.hasChildren && row.expanded) row.onToggle()
      break
    case 'ArrowDown':
      move((all, at) => all[at + 1])
      break
    case 'ArrowUp':
      move((all, at) => all[at - 1])
      break
    case 'Home':
      move((all) => all[0])
      break
    case 'End':
      move((all) => all[all.length - 1])
      break
    default:
      return
  }
  e.preventDefault()
}
