import { useMemo, useState, type ReactNode } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { useIsMobile } from '@/hooks/use-mobile'
import { cn } from '@/lib/utils'
import { useWorkspaceTabs } from '@/lib/workspace-tabs-context'
import type { Edge } from '@/lib/workspace-layout'
import { DndCtx, useWorkspaceDnd, type DropData, type DropTarget } from './workspace-dnd-context'

// Dragging a workspace tab: along a strip it reorders, onto another strip it moves the page over,
// and onto a pane's edge it splits the workspace and takes the page with it
//
// Strip slots and pane bodies never overlap on screen, so plain pointerWithin picks the right one
// without any collision tie-breaking

function pointerAt(event: DragMoveEvent | DragEndEvent): { x: number; y: number } | null {
  const start = event.activatorEvent as PointerEvent | null
  if (!start || typeof start.clientX !== 'number') return null
  return { x: start.clientX + event.delta.x, y: start.clientY + event.delta.y }
}

type Box = { left: number; top: number; width: number; height: number }

// An edge band scaled to the pane: a narrow strip on a wide pane, but never so wide on a small one
// that the middle disappears
function edgeAt(rect: Box, x: number, y: number): Edge | 'center' {
  const band = (size: number) => Math.min(0.3, Math.max(0.12, 160 / Math.max(size, 1)))
  const fx = (x - rect.left) / rect.width
  const fy = (y - rect.top) / rect.height
  const candidates: Array<[Edge, number, number]> = [
    ['left', fx, band(rect.width)],
    ['right', 1 - fx, band(rect.width)],
    ['top', fy, band(rect.height)],
    ['bottom', 1 - fy, band(rect.height)],
  ]
  const [edge, depth, limit] = candidates.sort((a, b) => a[1] - b[1])[0]
  return depth < limit ? edge : 'center'
}

type Hit = {
  data?: { droppableContainer?: { data: { current?: DropData }; rect: { current: Box | null } } }
}

// dnd-kit publishes `over` a render LATER than the collisions it was derived from, so on the frame a
// drag crosses into a new target `over` is still the old one (and null on the frame after that) -
// reading the collision directly is what makes a drop land where the pointer actually is
function targetOf(event: DragMoveEvent | DragEndEvent): DropTarget | null {
  const point = pointerAt(event)
  if (!point) return null
  const container = (event.collisions?.[0] as Hit | undefined)?.data?.droppableContainer
  const data = (container?.data.current ?? event.over?.data.current) as DropData | undefined
  const rect: Box | null = container?.rect.current ?? event.over?.rect ?? null
  if (!data || !rect) return null
  if (data.kind === 'pane') {
    return { kind: 'pane', paneId: data.paneId, edge: edgeAt(rect, point.x, point.y) }
  }
  if (data.kind === 'strip') return { kind: 'strip', paneId: data.paneId, index: data.count }
  // past a tab's midpoint the drop belongs on its far side
  const past = point.x > rect.left + rect.width / 2
  return { kind: 'strip', paneId: data.paneId, index: data.index + (past ? 1 : 0) }
}

export function WorkspaceDndProvider({ children }: { children: ReactNode }) {
  const { panes, moveTab, splitWithTab } = useWorkspaceTabs()
  // Split panes are unusable at phone width, and a draggable pill has to claim touch-action, which
  // would cost the strip its swipe-scroll - so on a phone tabs stay plain buttons
  const enabled = !useIsMobile()
  const [dragging, setDragging] = useState<{ id: string; title: string } | null>(null)
  const [drop, setDrop] = useState<DropTarget | null>(null)
  // the same 8px the rest of the dashboard's drag surfaces use - below it a press is still a click
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }))

  const onStart = (event: DragStartEvent) => {
    const data = event.active.data.current as { tabId: string; title: string } | undefined
    if (data) setDragging({ id: data.tabId, title: data.title })
  }

  const onMove = (event: DragMoveEvent) => {
    setDrop(targetOf(event))
  }

  const onEnd = (event: DragEndEvent) => {
    const data = event.active.data.current as { tabId: string } | undefined
    const target = targetOf(event) ?? drop
    setDragging(null)
    setDrop(null)
    if (!data || !target) return
    if (target.kind === 'strip') {
      moveTab(data.tabId, target.paneId, target.index)
      return
    }
    if (target.edge === 'center') {
      // the middle of a pane means "put it here", appended like any other tab
      const count = panes.find((p) => p.id === target.paneId)?.tabs.length ?? 0
      moveTab(data.tabId, target.paneId, count)
      return
    }
    splitWithTab(data.tabId, target.paneId, target.edge)
  }

  const onCancel = () => {
    setDragging(null)
    setDrop(null)
  }

  const api = useMemo(() => ({ dragging, drop, enabled }), [dragging, drop, enabled])

  return (
    <DndCtx.Provider value={api}>
      <DndContext
        sensors={sensors}
        collisionDetection={pointerWithin}
        onDragStart={onStart}
        onDragMove={onMove}
        onDragEnd={onEnd}
        onDragCancel={onCancel}
      >
        {children}
        <DragOverlay dropAnimation={null}>
          {dragging ? (
            <div className="pointer-events-none flex h-8 w-36 items-center rounded-md border border-primary/30 bg-background px-2.5 text-sm shadow-lg">
              <span className="min-w-0 flex-1 truncate text-left">{dragging.title || 'New tab'}</span>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </DndCtx.Provider>
  )
}

// Wraps the New Tab button rather than reserving a gap of its own - it sits immediately past the
// last pill, so it is exactly where a drop that overshoots the strip lands, and appends there
export function StripTail({ paneId, count, children }: { paneId: string; count: number; children: ReactNode }) {
  const { enabled } = useWorkspaceDnd()
  const { setNodeRef } = useDroppable({
    id: `strip:${paneId}`,
    data: { kind: 'strip', paneId, count } satisfies DropData,
    disabled: !enabled,
  })
  return (
    <div ref={setNodeRef} className="shrink-0">
      {children}
    </div>
  )
}

const previewFor: Record<Edge | 'center', string> = {
  left: 'inset-y-0 left-0 w-1/2',
  right: 'inset-y-0 right-0 w-1/2',
  top: 'inset-x-0 top-0 h-1/2',
  bottom: 'inset-x-0 bottom-0 h-1/2',
  center: 'inset-0',
}

// Always mounted, never clickable: dnd-kit measures droppables when a drag STARTS, so a zone that
// only appeared mid-drag would never get a rect
export function PaneDropZone({ paneId, gutter }: { paneId: string; gutter?: boolean }) {
  const { dragging, drop, enabled } = useWorkspaceDnd()
  const { setNodeRef } = useDroppable({
    id: `pane:${paneId}`,
    data: { kind: 'pane', paneId } satisfies DropData,
    disabled: !enabled,
  })
  const showing = dragging && drop?.kind === 'pane' && drop.paneId === paneId ? drop.edge : null
  return (
    <>
      {/* Once panes are inset, the zone spills back over the gutters so slamming a tab into the very
          edge of the window still lands - the top stays flush, that is the pane's own strip */}
      <div
        ref={setNodeRef}
        className={cn('pointer-events-none absolute', gutter ? '-inset-x-2 -bottom-2 top-0' : 'inset-0')}
      />
      {/* Separate from the measured zone, so the preview draws on the pane itself, gutters excluded */}
      {showing ? (
        <div
          className={cn(
            'pointer-events-none absolute rounded-md border-2 border-primary/60 bg-primary/15 transition-all duration-75',
            previewFor[showing],
          )}
        />
      ) : null}
    </>
  )
}
