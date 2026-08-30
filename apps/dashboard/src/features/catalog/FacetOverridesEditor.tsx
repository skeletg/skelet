import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { EyeIcon, EyeOffIcon, GripVerticalIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { FacetRow } from './facet-overrides-draft'

// Shared "storefront filters" editor for category + collection detail pages -
// drag to order, eye to hide, one reset back to attribute-order defaults.
// Draft state lives in ./facet-overrides-draft; each page wires the pair into
// its own save registry and mutation

export function FacetOverridesEditor({
  rows,
  customized,
  canManage,
  onReorder,
  onToggleHidden,
  onReset,
}: {
  rows: FacetRow[]
  customized: boolean
  canManage: boolean
  onReorder: (activeId: string, overId: string) => void
  onToggleHidden: (attributeId: string) => void
  onReset: () => void
}) {
  const sensors = useSensors(
    // The 4px activation distance keeps the eye toggle clickable inside a draggable row
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const onDragEnd = (e: DragEndEvent) => {
    if (e.over && e.active.id !== e.over.id) onReorder(String(e.active.id), String(e.over.id))
  }

  return (
    <div className="grid gap-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {customized
            ? 'Custom order for this page.'
            : 'Default order (attribute sort order). Drag or hide a filter to customize.'}
        </p>
        {customized ? (
          <Button type="button" variant="outline" size="sm" onClick={onReset} disabled={!canManage}>
            Use default order
          </Button>
        ) : null}
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext
          items={rows.map((r) => r.attributeId)}
          strategy={verticalListSortingStrategy}
        >
          <ul className="divide-y rounded-md border">
            {rows.map((r, i) => (
              <FacetOverrideRow
                key={r.attributeId}
                row={r}
                index={i}
                canManage={canManage}
                onToggleHidden={onToggleHidden}
              />
            ))}
          </ul>
        </SortableContext>
      </DndContext>
    </div>
  )
}

function FacetOverrideRow({
  row,
  index,
  canManage,
  onToggleHidden,
}: {
  row: FacetRow
  index: number
  canManage: boolean
  onToggleHidden: (attributeId: string) => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: row.attributeId,
    disabled: !canManage,
  })
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`flex items-center gap-2 bg-background px-3 py-2 first:rounded-t-md last:rounded-b-md ${
        row.hidden ? 'opacity-60' : ''
      } ${isDragging ? 'relative z-10 shadow-md' : ''}`}
    >
      <button
        type="button"
        {...attributes}
        {...listeners}
        disabled={!canManage}
        aria-label={`Reorder ${row.name}`}
        className="cursor-grab touch-none rounded p-1 text-muted-foreground transition-colors hover:text-foreground active:cursor-grabbing disabled:opacity-30"
      >
        <GripVerticalIcon className="size-4" />
      </button>
      <span className="w-6 text-center font-mono text-xs tabular-nums text-muted-foreground">
        {index + 1}
      </span>
      <span className="min-w-0 flex-1">
        <span
          className={`block truncate text-sm font-medium ${row.hidden ? 'line-through' : ''}`}
        >
          {row.name}
        </span>
        <span className="block truncate font-mono text-[11px] text-muted-foreground">
          {row.slug}
        </span>
      </span>
      {row.hidden ? (
        <span className="rounded-sm bg-muted px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Hidden
        </span>
      ) : null}
      <button
        type="button"
        onClick={() => onToggleHidden(row.attributeId)}
        disabled={!canManage}
        aria-pressed={row.hidden}
        aria-label={row.hidden ? `Show ${row.name} filter` : `Hide ${row.name} filter`}
        title={row.hidden ? 'Show this filter' : 'Hide this filter on this page'}
        className="grid size-7 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      >
        {row.hidden ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
      </button>
    </li>
  )
}
