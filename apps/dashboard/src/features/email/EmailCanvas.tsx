import { Fragment } from 'react'
import { DragOverlay, useDndContext, useDroppable } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ChevronDownIcon,
  CopyIcon,
  GripVerticalIcon,
  LayoutGridIcon,
  MoreVerticalIcon,
  ReplyIcon,
  StarIcon,
  Trash2Icon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { BlockView } from './BlockView'
import {
  blockLabel,
  interpolateSample,
  SAMPLE_VARS,
  type Column,
  type BlockComponent,
  type EmailBlock,
  type EmailStyles,
} from './blocks'
import { childSel } from './treeops'

export type CanvasItem = { id: string; block: EmailBlock }

// Sentinel drop target: append at the end of the list (dropped on the canvas,
// not before a specific block)
export const END_DROP = '__end__'

// DropLine is the insertion indicator shown between blocks while dragging a
// palette block, marking where it will land
function DropLine() {
  return (
    <div className="relative my-0.5 h-0.5 rounded-full bg-primary">
      <span className="absolute -left-1 -top-[3px] size-2 rounded-full bg-primary" />
    </div>
  )
}

type CanvasShared = {
  sel: string | null
  onSelect: (sel: string | null) => void
  onDelete: (sel: string) => void
  onDuplicate?: (sel: string) => void
  onMoveChild: (rowId: string, col: number, idx: number, dir: -1 | 1) => void
  theme: EmailStyles
  components: Record<string, BlockComponent>
}

function ChildBlock({
  rowId,
  col,
  idx,
  count,
  block,
  shared,
}: {
  rowId: string
  col: number
  idx: number
  count: number
  block: EmailBlock
  shared: CanvasShared
}) {
  const id = childSel(rowId, col, idx)
  const selected = shared.sel === id
  return (
    <div
      className={cn(
        'group/child relative rounded-sm outline-offset-2 transition-all',
        selected ? 'outline outline-2 outline-primary' : 'hover:outline hover:outline-1 hover:outline-primary/40',
      )}
      onClick={(e) => {
        e.stopPropagation()
        shared.onSelect(id)
      }}
    >
      <div
        className={cn(
          'absolute -top-2.5 right-1 z-10 flex items-center gap-0.5 rounded-full border bg-background/95 px-0.5 py-0.5 shadow-sm backdrop-blur transition-opacity',
          selected ? 'opacity-100' : 'opacity-0 group-hover/child:opacity-100',
        )}
      >
        <button
          type="button"
          disabled={idx === 0}
          className="rounded p-0.5 text-muted-foreground hover:bg-accent disabled:opacity-30"
          aria-label="Move up"
          onClick={(e) => {
            e.stopPropagation()
            shared.onMoveChild(rowId, col, idx, -1)
          }}
        >
          <ArrowUpIcon className="size-3" />
        </button>
        <button
          type="button"
          disabled={idx === count - 1}
          className="rounded p-0.5 text-muted-foreground hover:bg-accent disabled:opacity-30"
          aria-label="Move down"
          onClick={(e) => {
            e.stopPropagation()
            shared.onMoveChild(rowId, col, idx, 1)
          }}
        >
          <ArrowDownIcon className="size-3" />
        </button>
        {shared.onDuplicate && (
          <button
            type="button"
            className="rounded p-0.5 text-muted-foreground hover:bg-accent"
            aria-label="Duplicate"
            onClick={(e) => {
              e.stopPropagation()
              shared.onDuplicate?.(id)
            }}
          >
            <CopyIcon className="size-3" />
          </button>
        )}
        <button
          type="button"
          className="rounded p-0.5 text-muted-foreground hover:text-destructive"
          aria-label="Delete"
          onClick={(e) => {
            e.stopPropagation()
            shared.onDelete(id)
          }}
        >
          <Trash2Icon className="size-3" />
        </button>
      </div>
      <BlockView block={block} theme={shared.theme} components={shared.components} />
    </div>
  )
}

function ColumnZone({
  rowId,
  colIndex,
  column,
  shared,
}: {
  rowId: string
  colIndex: number
  column: Column
  shared: CanvasShared
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${rowId}:${colIndex}` })
  const blocks = column.blocks ?? []
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'min-h-16 rounded-md border border-dashed p-1.5 transition-colors',
        isOver ? 'border-primary bg-primary/10 ring-1 ring-primary/30' : 'border-muted-foreground/20',
      )}
    >
      {blocks.length === 0 ? (
        <div className="grid h-14 place-items-center text-center text-[11px] text-muted-foreground">
          Drop a block here
        </div>
      ) : (
        <div className="grid gap-1.5">
          {blocks.map((b, i) => (
            <ChildBlock
              key={i}
              rowId={rowId}
              col={colIndex}
              idx={i}
              count={blocks.length}
              block={b}
              shared={shared}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function SortableBlock({ item, shared }: { item: CanvasItem; shared: CanvasShared }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id })
  const selected = shared.sel === item.id
  const isRow = item.block.type === 'row'
  return (
    <div
      ref={setNodeRef}
      // While this item is the one being dragged, keep it static (a dimmed
      // placeholder) - the DragOverlay follows the cursor, siblings still shift
      style={{ transform: isDragging ? undefined : CSS.Transform.toString(transform), transition }}
      className={cn(
        'group relative rounded-md outline-offset-4 transition-all',
        selected ? 'outline outline-2 outline-primary' : 'hover:outline hover:outline-1 hover:outline-primary/40',
        isDragging && 'opacity-30',
      )}
      onClick={(e) => {
        e.stopPropagation()
        shared.onSelect(item.id)
      }}
    >
      <div
        className={cn(
          'absolute -top-6 right-1 z-20 flex items-center gap-0.5 rounded-full border bg-background/95 px-1 py-0.5 shadow-md backdrop-blur transition-opacity',
          selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100',
          isDragging && 'opacity-0',
        )}
      >
        <span className="px-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          {blockLabel(item.block.type)}
        </span>
        <button
          type="button"
          className="cursor-grab rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          aria-label="Drag to reorder"
          {...listeners}
          {...attributes}
          onClick={(e) => e.stopPropagation()}
        >
          <GripVerticalIcon className="size-3.5" />
        </button>
        {shared.onDuplicate && (
          <button
            type="button"
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="Duplicate block"
            onClick={(e) => {
              e.stopPropagation()
              shared.onDuplicate?.(item.id)
            }}
          >
            <CopyIcon className="size-3.5" />
          </button>
        )}
        <button
          type="button"
          className="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          aria-label="Delete block"
          onClick={(e) => {
            e.stopPropagation()
            shared.onDelete(item.id)
          }}
        >
          <Trash2Icon className="size-3.5" />
        </button>
      </div>
      <BlockView
        block={item.block}
        theme={shared.theme}
        components={shared.components}
        renderColumn={
          isRow
            ? (colIndex) => (
                <ColumnZone
                  rowId={item.id}
                  colIndex={colIndex}
                  column={item.block.columns?.[colIndex] ?? {}}
                  shared={shared}
                />
              )
            : undefined
        }
      />
    </div>
  )
}

/**
 * EmailCanvas is the WYSIWYG editing surface: blocks rendered client-side with
 * sample data inside a themed email frame, each selectable + reorderable - Row
 * blocks expose per-column drop zones - DnD is driven by the parent's
 * DndContext; this declares the sortable list, the canvas droppable, and the
 * per-column droppables
 */
export function EmailCanvas({
  items,
  sel,
  onSelect,
  onDelete,
  onDuplicate,
  onMoveChild,
  theme,
  components = {},
  subject,
  senderName,
  emailFrame = false,
  dropBefore = null,
}: {
  items: CanvasItem[]
  sel: string | null
  onSelect: (sel: string | null) => void
  onDelete: (sel: string) => void
  onDuplicate?: (sel: string) => void
  onMoveChild: (rowId: string, col: number, idx: number, dir: -1 | 1) => void
  theme: EmailStyles
  components?: Record<string, BlockComponent>
  /** Subject line shown in the Gmail-style frame (interpolated with sample data) */
  subject?: string
  /** Sender display name; defaults to the sample store name */
  senderName?: string
  /** Wrap the rendered email in a Gmail-style message frame (subject + sender) */
  emailFrame?: boolean
  /** While dragging a palette block, the id of the block it will drop BEFORE
   *  (or the sentinel END_DROP to append) - renders an insertion line */
  dropBefore?: string | null
}) {
  const { setNodeRef, isOver } = useDroppable({ id: 'canvas' })
  const width = theme.contentWidth && theme.contentWidth >= 200 ? theme.contentWidth : 600
  const fontFamily = theme.fontFamily || 'Helvetica, Arial, sans-serif'
  const shared: CanvasShared = { sel, onSelect, onDelete, onDuplicate, onMoveChild, theme, components }

  // A DragOverlay renders the dragged thing as a clean floating copy that follows the cursor - so reordering doesn't
  // distort/squash the real block, and palette drags show a labelled chip
  const { active } = useDndContext()
  const fromPalette = !!active?.data.current?.fromPalette
  const activeItem = active && !fromPalette ? items.find((i) => i.id === active.id) : undefined
  const activePaletteType = fromPalette ? String(active?.data.current?.blockType) : undefined
  const dragOverlay = (
    <DragOverlay dropAnimation={null}>
      {activeItem ? (
        <div className="cursor-grabbing rounded-md bg-white opacity-95 shadow-2xl ring-1 ring-primary/40" style={{ fontFamily }}>
          <BlockView block={activeItem.block} theme={theme} components={components} />
        </div>
      ) : activePaletteType ? (
        <div className="inline-flex cursor-grabbing items-center gap-1.5 rounded-full bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground shadow-lg">
          {components[activePaletteType]?.name ?? blockLabel(activePaletteType)}
        </div>
      ) : null}
    </DragOverlay>
  )

  const list = (
    <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy}>
      {items.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border-2 border-dashed px-4 py-16 text-center">
          <span className="grid size-10 place-items-center rounded-full bg-muted text-muted-foreground">
            <LayoutGridIcon className="size-5" />
          </span>
          <p className="text-sm font-medium">Start building</p>
          <p className="max-w-xs text-xs text-muted-foreground">
            Drag a block from the left onto the canvas.
          </p>
        </div>
      ) : (
        <div className="grid gap-2.5">
          {items.map((item) => (
            <Fragment key={item.id}>
              {dropBefore === item.id && <DropLine />}
              <SortableBlock item={item} shared={shared} />
            </Fragment>
          ))}
          {dropBefore === END_DROP && <DropLine />}
        </div>
      )}
    </SortableContext>
  )

  if (emailFrame) {
    const sender = senderName || SAMPLE_VARS['store.name'] || 'Your Store'
    const senderEmail = `hello@${(sender.toLowerCase().replace(/[^a-z0-9]+/g, '') || 'store')}.com`
    const subjectLine = interpolateSample(subject ?? '').trim()
    return (
      <>
      <div className="min-h-full bg-muted/40 p-4 md:p-8" onClick={() => onSelect(null)}>
        {/* Gmail-style opened-message frame around the rendered email */}
        <div className="mx-auto max-w-3xl overflow-hidden rounded-xl border bg-background shadow-sm">
          <div className="px-6 pt-5">
            <div className="flex items-start gap-3">
              <h2 className="min-w-0 flex-1 text-[1.375rem] font-normal leading-snug text-foreground">
                {subjectLine || <span className="text-muted-foreground">(no subject)</span>}
              </h2>
              <span className="mt-1 shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                Inbox
              </span>
            </div>
            <div className="mt-4 flex items-center gap-3 pb-4">
              <div
                className="grid size-10 shrink-0 place-items-center rounded-full text-base font-medium text-white"
                style={{ background: '#7c4dcc' }}
                aria-hidden
              >
                {sender.charAt(0).toUpperCase()}
              </div>
              <div className="min-w-0 flex-1 leading-tight">
                <div className="flex items-center gap-1 text-sm">
                  <span className="font-medium text-foreground">{sender}</span>
                  <span className="truncate text-muted-foreground">&lt;{senderEmail}&gt;</span>
                </div>
                <div className="flex items-center gap-0.5 text-xs text-muted-foreground">
                  to me <ChevronDownIcon className="size-3" />
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <span className="text-xs">9:41 AM</span>
                <StarIcon className="size-4" />
                <ReplyIcon className="size-4" />
                <MoreVerticalIcon className="size-4" />
              </div>
            </div>
          </div>
          {/* Email body - rendered with the email's own theme background */}
          <div className="border-t px-4 py-6" style={{ background: theme.background || '#ffffff' }}>
            <div ref={setNodeRef} className={cn('mx-auto', isOver && 'rounded ring-2 ring-primary/50')} style={{ maxWidth: width, fontFamily }}>
              {list}
            </div>
          </div>
        </div>
      </div>
      {dragOverlay}
      </>
    )
  }

  return (
    <>
    <div
      className="min-h-full p-6"
      style={{ background: theme.background || '#f6f6f6' }}
      onClick={() => onSelect(null)}
    >
      <div
        ref={setNodeRef}
        className={cn('mx-auto rounded-md bg-white p-6 shadow-sm', isOver && 'ring-2 ring-primary/50')}
        style={{ maxWidth: width, fontFamily }}
      >
        {list}
      </div>
    </div>
    {dragOverlay}
    </>
  )
}
