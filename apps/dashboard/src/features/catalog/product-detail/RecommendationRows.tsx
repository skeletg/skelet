import { cloneElement, createContext, useContext, useMemo, useState, type ComponentProps, type ReactElement } from 'react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowDownIcon, ArrowUpIcon, GripVerticalIcon, ImageIcon, XIcon } from 'lucide-react'
import { DataTable } from '@/components/data-table'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { Button } from '@/components/ui/button'
import { recommendationName, type RecommendationProduct } from './recommendation-products'

export function RecommendationImage({ product }: { product?: RecommendationProduct }) {
  const image = [product?.thumbnail, product?.displayImage].find((img) => img?.url && (!img.status || img.status === 'READY'))
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  return image && failedUrl !== image.url ? (
    <img
      src={image.url} alt={image.altText || recommendationName(product)} loading="lazy"
      onError={() => setFailedUrl(image.url)} className="size-10 shrink-0 rounded-md border object-cover"
    />
  ) : (
    <span className="flex size-10 shrink-0 items-center justify-center rounded-md border bg-muted" aria-label="No product image">
      <ImageIcon className="size-4 text-muted-foreground" />
    </span>
  )
}

const DragContext = createContext<Pick<ReturnType<typeof useSortable>, 'attributes' | 'listeners' | 'setActivatorNodeRef'> | null>(null)

function SortableRow({ id, disabled, element }: { id: string; disabled: boolean; element: ReactElement }) {
  const sortable = useSortable({ id, disabled })
  const props = element.props as ComponentProps<'tr'>
  return (
    <DragContext.Provider value={{ attributes: sortable.attributes, listeners: sortable.listeners, setActivatorNodeRef: sortable.setActivatorNodeRef }}>
      {cloneElement(element as ReactElement<ComponentProps<'tr'>>, {
        ref: sortable.setNodeRef,
        style: { ...props.style, transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition },
        className: `${props.className ?? ''} bg-background ${sortable.isDragging ? 'relative z-10 shadow-md' : ''}`,
      })}
    </DragContext.Provider>
  )
}

function DragHandle({ name, disabled }: { name: string; disabled: boolean }) {
  const { attributes, listeners, setActivatorNodeRef } = useContext(DragContext)!
  return (
    <Button
      type="button" variant="ghost" size="icon" className="size-7 touch-none cursor-grab active:cursor-grabbing"
      disabled={disabled} aria-label={`Reorder ${name}`} ref={setActivatorNodeRef} {...attributes} {...listeners}
    >
      <GripVerticalIcon className="size-4" />
    </Button>
  )
}

type RecommendationRow = { id: string; product?: RecommendationProduct; index: number }

type RecommendationRowsProps = {
  value: string[]
  byId: Map<string, RecommendationProduct>
  label: string
  disabled: boolean
  loading: boolean
  onChange: (ids: string[]) => void
}
const RecommendationStateContext = createContext<RecommendationRowsProps | null>(null)

function RecommendationRowCell({ row: { original: r } }: { row: { original: RecommendationRow } }) {
  const { value, label, disabled, loading, onChange } = useContext(RecommendationStateContext)!
  return (
    <div className="flex items-center gap-1.5">
      <DragHandle name={recommendationName(r.product)} disabled={disabled} />
      <span className="text-xs tabular-nums text-muted-foreground">{r.index + 1}</span>
      <RecommendationImage product={r.product} />
      <div className="min-w-0 flex-1 whitespace-normal break-words text-sm">
        <p className="font-medium">{!r.product && loading ? 'Loading product…' : recommendationName(r.product)}</p>
        <p className="font-mono text-xs text-muted-foreground">{r.product?.sku || 'No SKU'}</p>
        <div className="mt-1 flex items-center gap-1">
          <Button
            type="button" variant="ghost" size="icon" className="size-7" disabled={disabled || r.index === 0}
            aria-label={`Move ${recommendationName(r.product)} up in ${label}`}
            onClick={() => onChange(arrayMove(value, r.index, r.index - 1))}
          ><ArrowUpIcon className="size-3.5" /></Button>
          <Button
            type="button" variant="ghost" size="icon" className="size-7" disabled={disabled || r.index === value.length - 1}
            aria-label={`Move ${recommendationName(r.product)} down in ${label}`}
            onClick={() => onChange(arrayMove(value, r.index, r.index + 1))}
          ><ArrowDownIcon className="size-3.5" /></Button>
          <Button
            type="button" variant="ghost" size="sm" className="h-7 px-1.5 text-muted-foreground hover:text-destructive" disabled={disabled}
            aria-label={`Remove ${recommendationName(r.product)} from ${label}`}
            onClick={() => onChange(value.filter((id) => id !== r.id))}
          ><XIcon className="size-3.5" />Remove</Button>
        </div>
      </div>
    </div>
  )
}

const RECOMMENDATION_COLUMNS: ColumnDef<RecommendationRow>[] = [{
  id: 'product', header: () => 'Product', enableSorting: false, enableResizing: false,
  cell: RecommendationRowCell,
}]

export function RecommendationRows(props: RecommendationRowsProps) {
  const { value, byId, label, disabled, onChange } = props
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const rows = useMemo(() => value.map((id, index) => ({ id, product: byId.get(id), index })), [value, byId])
  const name = (id: string | number) => recommendationName(byId.get(String(id)))
  return (
    <RecommendationStateContext.Provider value={props}>
      <DndContext
        sensors={sensors} collisionDetection={closestCenter}
        accessibility={{
          screenReaderInstructions: { draggable: 'Press Space to pick up a product. Use the up and down arrows to reorder within this group. Press Space to drop, or Escape to cancel.' },
          announcements: {
            onDragStart: ({ active }) => `Picked up ${name(active.id)} in ${label}.`,
            onDragOver: ({ active, over }) => over ? `${name(active.id)} is over ${name(over.id)}, position ${value.indexOf(String(over.id)) + 1} of ${value.length}.` : undefined,
            onDragEnd: ({ active, over }) => over ? `${name(active.id)} dropped at position ${value.indexOf(String(over.id)) + 1} in ${label}.` : 'Order unchanged.',
            onDragCancel: () => 'Reordering cancelled. Order unchanged.',
          },
        }}
        onDragEnd={({ active, over }) => {
          if (!disabled && over && active.id !== over.id) {
            const from = value.indexOf(String(active.id))
            const to = value.indexOf(String(over.id))
            if (from >= 0 && to >= 0) onChange(arrayMove(value, from, to))
          }
        }}
      >
        <SortableContext items={value} strategy={verticalListSortingStrategy}>
          <div
            role="region" aria-label={`${label} selected products`} tabIndex={value.length > 3 ? 0 : undefined}
            className="max-h-80 overflow-y-auto overscroll-contain rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <DataTable
              compact columns={RECOMMENDATION_COLUMNS} data={rows} pageSize={Math.max(1, rows.length)} getRowId={(r) => r.id}
              renderRow={(row, element) => <SortableRow id={row.id} disabled={disabled} element={element} />}
            />
          </div>
        </SortableContext>
      </DndContext>
    </RecommendationStateContext.Provider>
  )
}
