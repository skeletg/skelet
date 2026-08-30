import { createContext, useCallback, useContext } from 'react'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import type { Edge } from '@/lib/workspace-layout'

// Split from workspace-dnd so that file exports only components - fast refresh needs the separation

export type DropData =
  | { kind: 'tab'; paneId: string; index: number }
  | { kind: 'strip'; paneId: string; count: number }
  | { kind: 'pane'; paneId: string }

/** Where the drop would land - strip drops carry an insertion slot, pane drops an edge to split on */
export type DropTarget =
  | { kind: 'strip'; paneId: string; index: number }
  | { kind: 'pane'; paneId: string; edge: Edge | 'center' }

export type DndApi = { dragging: { id: string; title: string } | null; drop: DropTarget | null; enabled: boolean }

export const DndCtx = createContext<DndApi>({ dragging: null, drop: null, enabled: false })

export function useWorkspaceDnd(): DndApi {
  return useContext(DndCtx)
}

/** A tab pill: draggable, and itself the drop slot the strip measures insertions against */
export function useTabDrag(tabId: string, paneId: string, index: number, title: string) {
  const { enabled } = useWorkspaceDnd()
  const { listeners, setNodeRef: setDragRef, isDragging } = useDraggable({
    id: `tab:${tabId}`,
    data: { tabId, paneId, title },
    disabled: !enabled,
  })
  const { setNodeRef: setDropRef } = useDroppable({
    id: `slot:${tabId}`,
    data: { kind: 'tab', paneId, index } satisfies DropData,
    disabled: !enabled,
  })
  const ref = useCallback(
    (el: HTMLElement | null) => {
      setDragRef(el)
      setDropRef(el)
    },
    [setDragRef, setDropRef],
  )
  // dnd-kit's `attributes` are deliberately dropped: they announce the pill as draggable and point
  // at keyboard instructions, and with only a PointerSensor wired up neither is true
  return { ref, listeners, isDragging, enabled }
}
