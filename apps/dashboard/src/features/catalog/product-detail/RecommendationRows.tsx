import { cloneElement, createContext, useContext, useMemo, useState, type ComponentProps, type ReactElement } from 'react'
import { Link } from 'react-router-dom'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowDownIcon, ArrowUpIcon, GripVerticalIcon, ImageIcon, Trash2Icon } from 'lucide-react'
import { DataTable } from '@/components/data-table'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { LONG_LIST, matchesRecommendation, recommendationName, type RecommendationProduct } from './recommendation-products'

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

type RecommendationRow = { id: string; product?: RecommendationProduct; index: number }

type RecommendationRowsProps = {
  value: string[]
  byId: Map<string, RecommendationProduct>
  label: string
  disabled: boolean
  loading: boolean
  // Not in the saved list yet, so the row says it joins on save
  added?: ReadonlySet<string>
  // Narrows the rows shown - reordering pauses meanwhile, since a move past hidden rows can't be seen
  query?: string
  onChange: (ids: string[]) => void
}
const RecommendationStateContext = createContext<(RecommendationRowsProps & { locked: boolean }) | null>(null)

function HandleCell({ row: { original: r } }: { row: { original: RecommendationRow } }) {
  const { disabled, locked } = useContext(RecommendationStateContext)!
  const { attributes, listeners, setActivatorNodeRef } = useContext(DragContext)!
  return (
    <Button
      type="button" variant="ghost" size="icon" className="size-7 touch-none cursor-grab text-muted-foreground active:cursor-grabbing"
      disabled={disabled || locked} aria-label={`Reorder ${recommendationName(r.product)}`} ref={setActivatorNodeRef} {...attributes} {...listeners}
    >
      <GripVerticalIcon className="size-4" />
    </Button>
  )
}

function ProductCell({ row: { original: r } }: { row: { original: RecommendationRow } }) {
  const { loading, added } = useContext(RecommendationStateContext)!
  return (
    <div className="flex min-w-0 items-center gap-2">
      {r.product ? (
        <Link to={`/products/${r.id}`} className="truncate font-medium hover:underline">{recommendationName(r.product)}</Link>
      ) : (
        <span className="truncate text-muted-foreground">{loading ? 'Loading product…' : recommendationName()}</span>
      )}
      {added?.has(r.id) ? <span className="shrink-0 text-xs text-success">Will add</span> : null}
    </div>
  )
}

function ActionsCell({ row: { original: r } }: { row: { original: RecommendationRow } }) {
  const { value, label, disabled, locked, onChange } = useContext(RecommendationStateContext)!
  const name = recommendationName(r.product)
  return (
    <div className="flex items-center justify-end gap-0.5">
      <Button
        type="button" variant="ghost" size="icon" className="size-7" disabled={disabled || locked || r.index === 0}
        aria-label={`Move ${name} up in ${label}`}
        onClick={() => onChange(arrayMove(value, r.index, r.index - 1))}
      ><ArrowUpIcon className="size-3.5" /></Button>
      <Button
        type="button" variant="ghost" size="icon" className="size-7" disabled={disabled || locked || r.index === value.length - 1}
        aria-label={`Move ${name} down in ${label}`}
        onClick={() => onChange(arrayMove(value, r.index, r.index + 1))}
      ><ArrowDownIcon className="size-3.5" /></Button>
      <Button
        type="button" variant="ghost" size="icon" className="size-7 text-muted-foreground hover:text-destructive" disabled={disabled}
        aria-label={`Remove ${name} from ${label}`}
        onClick={() => onChange(value.filter((id) => id !== r.id))}
      ><Trash2Icon className="size-3.5" /></Button>
    </div>
  )
}

const RECOMMENDATION_COLUMNS: ColumnDef<RecommendationRow>[] = [
  { id: 'handle', header: () => <span className="sr-only">Reorder</span>, size: 44, enableSorting: false, enableResizing: false, cell: HandleCell },
  {
    id: 'position', header: () => '#', size: 40, enableSorting: false, enableResizing: false,
    cell: ({ row }) => <span className="text-xs tabular-nums text-muted-foreground">{row.original.index + 1}</span>,
  },
  {
    id: 'image', header: () => '', size: 56, enableSorting: false, enableResizing: false,
    cell: ({ row }) => <RecommendationImage product={row.original.product} />,
  },
  { id: 'product', header: () => 'Product', enableSorting: false, enableResizing: false, cell: ProductCell },
  {
    id: 'sku', header: () => 'SKU', size: 180, enableSorting: false, enableResizing: false,
    cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.product?.sku || '-'}</span>,
  },
  { id: 'actions', header: () => <span className="sr-only">Actions</span>, size: 112, enableSorting: false, enableResizing: false, cell: ActionsCell },
]

export function RecommendationRows(props: RecommendationRowsProps) {
  const { value, byId, label, disabled, query = '', onChange } = props
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const locked = query.trim() !== ''
  const rows = useMemo(
    () => value.map((id, index) => ({ id, product: byId.get(id), index })).filter((r) => matchesRecommendation(r.product, query)),
    [value, byId, query],
  )
  const name = (id: string | number) => recommendationName(byId.get(String(id)))
  const scrolls = value.length > LONG_LIST
  return (
    <RecommendationStateContext.Provider value={{ ...props, locked }}>
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
          if (!disabled && !locked && over && active.id !== over.id) {
            const from = value.indexOf(String(active.id))
            const to = value.indexOf(String(over.id))
            if (from >= 0 && to >= 0) onChange(arrayMove(value, from, to))
          }
        }}
      >
        <SortableContext items={rows.map((r) => r.id)} strategy={verticalListSortingStrategy}>
          {/* Every row on one page - a drag can't cross a page boundary - so a long list scrolls in its frame instead */}
          <div
            role="region" aria-label={`${label} selected products`} tabIndex={scrolls ? 0 : undefined}
            className={cn('rounded-md', scrolls && 'max-h-[36rem] overflow-y-auto overscroll-contain focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring')}
          >
            <DataTable
              compact columns={RECOMMENDATION_COLUMNS} data={rows} pageSize={Math.max(1, rows.length)} getRowId={(r) => r.id}
              emptyState={`No products in ${label} match "${query.trim()}".`}
              renderRow={(row, element) => <SortableRow id={row.id} disabled={disabled || locked} element={element} />}
            />
          </div>
        </SortableContext>
      </DndContext>
    </RecommendationStateContext.Provider>
  )
}
