import { useEffect, useEffectEvent } from 'react'
import { useTabPanelActive } from '@/lib/tab-panel-context'

const MOD = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

export const FLOW_SHORTCUTS: Array<{ keys: string[]; what: string }> = [
  { keys: [MOD, 'D'], what: 'Duplicate the selected step' },
  { keys: ['Backspace'], what: 'Remove the selected step or connection' },
  { keys: [MOD, 'Z'], what: 'Undo' },
  { keys: [MOD, 'Shift', 'Z'], what: 'Redo' },
  { keys: [MOD, 'S'], what: 'Save' },
]

// Save works from anywhere on the page, undo anywhere but a field, and the rest only while the canvas has the keys
export function useFlowShortcuts(actions: { undo: () => void; redo: () => void; duplicate: () => void; remove: () => void; deselect: () => void; save: () => void }) {
  const active = useTabPanelActive()
  const onKey = useEffectEvent((e: KeyboardEvent) => {
    if (!active || e.defaultPrevented) return
    const t = e.target instanceof HTMLElement ? e.target : null
    if (t?.closest('[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"]')) return
    const mod = e.metaKey || e.ctrlKey
    const key = e.key.toLowerCase()
    if (mod && key === 's') {
      e.preventDefault()
      actions.save()
      return
    }
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return
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
    if (t && t !== document.body && !t.closest('[data-flow-canvas],[data-flow-rail]')) return
    if (mod && key === 'd') {
      e.preventDefault()
      actions.duplicate()
    } else if (e.key === 'Backspace' || e.key === 'Delete') {
      e.preventDefault()
      actions.remove()
    } else if (e.key === 'Escape') {
      actions.deselect()
    }
  })
  useEffect(() => {
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}
