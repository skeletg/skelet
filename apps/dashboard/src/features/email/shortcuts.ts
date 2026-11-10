import { useEffect, useEffectEvent } from 'react'
import { useTabPanelActive } from '@/lib/tab-panel-context'

export type CanvasActions = {
  sel: string | null
  order: string[]
  select: (sel: string | null) => void
  remove: (sel: string) => void
  duplicate: (sel: string) => void
  move: (sel: string, dir: -1 | 1) => void
  undo: () => void
  redo: () => void
}

function editable(t: HTMLElement): boolean {
  return t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)
}

export function scrollToBlock(id: string) {
  document.querySelector(`[data-block-id="${id}"]`)?.scrollIntoView?.({ block: 'nearest' })
}

// Keys act only while the canvas or palette has focus, so a field, dialog or menu keeps its own
export function useCanvasShortcuts(actions: CanvasActions) {
  const active = useTabPanelActive()
  const onKey = useEffectEvent((e: KeyboardEvent) => {
    if (!active || e.defaultPrevented) return
    const t = e.target instanceof HTMLElement ? e.target : null
    if (t?.closest('[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"]')) return
    const canvas = t?.closest<HTMLElement>('[data-email-canvas]')
    if (t && editable(t)) {
      if (e.key === 'Escape' && canvas) {
        e.preventDefault()
        canvas.focus({ preventScroll: true })
      }
      return
    }
    if (t && t !== document.body && !canvas && !t.closest('[data-email-palette]')) return

    const mod = e.metaKey || e.ctrlKey
    const key = e.key.toLowerCase()
    if (mod && key === 'z') {
      e.preventDefault()
      if (e.shiftKey) actions.redo()
      else actions.undo()
      return
    }
    if (mod && key === 'y') {
      e.preventDefault()
      actions.redo()
      return
    }
    const { sel, order } = actions
    if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      const dir = e.key === 'ArrowUp' ? -1 : 1
      e.preventDefault()
      if (e.altKey && sel) return actions.move(sel, dir)
      if (order.length === 0) return
      const at = sel ? order.indexOf(sel.split('::')[0]) : -1
      const next = order[at === -1 ? (dir === 1 ? 0 : order.length - 1) : Math.max(0, Math.min(order.length - 1, at + dir))]
      actions.select(next)
      scrollToBlock(next)
      return
    }
    if (!sel) return
    if (e.key === 'Escape') {
      actions.select(null)
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      actions.remove(sel)
    } else if (mod && key === 'd') {
      e.preventDefault()
      actions.duplicate(sel)
    }
  })
  useEffect(() => {
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}

const MOD = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

export const SHORTCUTS: Array<{ keys: string[]; what: string }> = [
  { keys: ['↑', '↓'], what: 'Select the block above or below' },
  { keys: ['Alt', '↑ / ↓'], what: 'Move the selected block' },
  { keys: [MOD, 'D'], what: 'Duplicate it' },
  { keys: ['Delete'], what: 'Delete it' },
  { keys: ['Esc'], what: 'Stop writing, then deselect' },
  { keys: [MOD, 'Z'], what: 'Undo' },
  { keys: [MOD, 'Shift', 'Z'], what: 'Redo' },
]
