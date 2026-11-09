import { useState, type ReactNode } from 'react'
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
  MonitorIcon,
  MoreVerticalIcon,
  PlusIcon,
  ReplyIcon,
  SmartphoneIcon,
  StarIcon,
  Trash2Icon,
} from 'lucide-react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { BlockMenu, type PaletteOptions } from './BlockPalette'
import { BlockView } from './BlockView'
import { InlineEditor } from './InlineEdit'
import {
  blockLabel,
  interpolateSample,
  isInlineEditable,
  SAMPLE_VARS,
  type Column,
  type BlockComponent,
  type EmailBlock,
  type EmailStyles,
} from './blocks'
import { STARTER_GROUPS, STARTER_TEMPLATES, type StarterTemplate } from './starters'
import { childSel } from './treeops'

export type CanvasItem = { id: string; block: EmailBlock }

// Sentinel drop target: append at the end of the list (dropped on the canvas,
// not before a specific block)
export const END_DROP = '__end__'

function DropLine({ edge }: { edge: 'top' | 'bottom' }) {
  return (
    <div className={cn('pointer-events-none absolute inset-x-0 z-30 h-0.5 rounded-full bg-primary', edge === 'top' ? '-top-[6px]' : '-bottom-[6px]')}>
      <span className="absolute -left-1 -top-[3px] size-2 rounded-full bg-primary" />
    </div>
  )
}

type Device = 'desktop' | 'mobile'

type CanvasShared = {
  sel: string | null
  onSelect: (sel: string | null) => void
  onDelete: (sel: string) => void
  onDuplicate?: (sel: string) => void
  onMoveChild: (rowId: string, col: number, idx: number, dir: -1 | 1) => void
  onEdit?: (sel: string, block: EmailBlock) => void
  onInsertAt?: (index: number, type: string) => void
  insertOptions: PaletteOptions
  mergeTags: string[]
  dragging: boolean
  device: Device
  theme: EmailStyles
  components: Record<string, BlockComponent>
}

function inlineEditor(id: string, block: EmailBlock, shared: CanvasShared): ReactNode {
  if (shared.sel !== id || !shared.onEdit || shared.dragging || !isInlineEditable(block) || shared.components[block.type]) return undefined
  const onEdit = shared.onEdit
  return <InlineEditor block={block} theme={shared.theme} tags={shared.mergeTags} onChange={(b) => onEdit(id, b)} />
}

// A block hidden on phones fades in the phone preview rather than vanishing, so it can still be picked
function hiddenHere(block: EmailBlock, shared: CanvasShared): boolean {
  return shared.device === 'mobile' && !!block.style?.hideMobile
}

function InsertPoint({ index, shared, edge }: { index: number; shared: CanvasShared; edge: 'top' | 'bottom' }) {
  const [open, setOpen] = useState(false)
  const onInsertAt = shared.onInsertAt
  if (!onInsertAt || shared.dragging) return null
  return (
    <div
      className={cn(
        'group/insert absolute inset-x-0 z-10 flex h-3 items-center justify-center',
        edge === 'top' ? '-top-[11px]' : '-bottom-[11px]',
      )}
      onClick={(e) => e.stopPropagation()}
    >
      <span className={cn('absolute inset-x-0 top-1/2 h-px bg-primary/60 opacity-0 transition-opacity group-hover/insert:opacity-100', open && 'opacity-100')} />
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label="Add a block here"
            title="Add a block here"
            className={cn(
              'relative grid size-5 place-items-center rounded-full border bg-background text-muted-foreground opacity-0 shadow-sm transition-opacity hover:text-foreground focus-visible:opacity-100 group-hover/insert:opacity-100',
              open && 'opacity-100',
            )}
          >
            <PlusIcon className="size-3" />
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-72 p-2" onClick={(e) => e.stopPropagation()}>
          <BlockMenu
            {...shared.insertOptions}
            components={Object.values(shared.components)}
            onPick={(type) => {
              setOpen(false)
              onInsertAt(index, type)
            }}
          />
        </PopoverContent>
      </Popover>
    </div>
  )
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
        hiddenHere(block, shared) && 'opacity-40',
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
      <BlockView block={block} theme={shared.theme} components={shared.components} editor={inlineEditor(id, block, shared)} />
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
      className={cn('min-h-16 rounded-md border border-dashed p-1.5 transition-colors', isOver && 'border-primary bg-primary/10 ring-1 ring-primary/30')}
      // Inside the email, so in the email's light colours rather than the dashboard's
      style={isOver ? undefined : { borderColor: '#d4d4d8' }}
    >
      {blocks.length === 0 ? (
        <div className="grid h-14 place-items-center text-center font-sans text-[11px]" style={{ color: '#8a8a8a' }}>
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

function SortableBlock({
  item,
  index,
  last,
  dropEdge,
  shared,
}: {
  item: CanvasItem
  index: number
  last: boolean
  dropEdge: 'top' | 'bottom' | null
  shared: CanvasShared
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id, data: { blockType: item.block.type } })
  const selected = shared.sel === item.id
  // Not draggable while it or a block in its columns is selected, so selecting their text never turns into a drag
  const editing = selected || !!shared.sel?.startsWith(`${item.id}::`)
  const isRow = item.block.type === 'row'
  return (
    <div
      ref={setNodeRef}
      data-block-id={item.id}
      {...(editing ? {} : listeners)}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={cn(
        'group relative rounded-md outline-offset-4 [&_img]:pointer-events-none',
        selected ? 'outline outline-2 outline-primary' : 'hover:outline hover:outline-1 hover:outline-primary/40',
        !editing && 'cursor-grab select-none',
        isDragging && 'opacity-40',
        hiddenHere(item.block, shared) && 'opacity-40',
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
      {dropEdge ? <DropLine edge={dropEdge} /> : null}
      <InsertPoint index={index} shared={shared} edge="top" />
      {last ? <InsertPoint index={index + 1} shared={shared} edge="bottom" /> : null}
      <BlockView
        block={item.block}
        theme={shared.theme}
        components={shared.components}
        editor={inlineEditor(item.id, item.block, shared)}
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

function EmptyState({ onStart, canAdd }: { onStart?: (blocks: EmailBlock[]) => void; canAdd: boolean }) {
  if (!onStart) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-xl border-2 border-dashed bg-background px-4 py-16 text-center font-sans">
        <span className="grid size-10 place-items-center rounded-full bg-muted text-muted-foreground">
          <LayoutGridIcon className="size-5" />
        </span>
        <p className="text-sm font-medium text-foreground">Start building</p>
        <p className="max-w-xs text-xs text-muted-foreground">{canAdd ? 'Click a block on the left, or drag one here.' : 'Drag a block from the left onto the canvas.'}</p>
      </div>
    )
  }
  return (
    <div className="grid gap-5 rounded-xl border-2 border-dashed bg-background p-5 font-sans" onClick={(e) => e.stopPropagation()}>
      <div className="text-center">
        <p className="text-sm font-medium text-foreground">Start from a layout</p>
        <p className="text-xs text-muted-foreground">Every part of it can be changed afterwards - or click a block on the left to start from a blank page.</p>
      </div>
      {STARTER_GROUPS.map((g) => (
        <section key={g.id} aria-label={g.label} className="grid gap-2">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{g.label}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {STARTER_TEMPLATES.filter((t) => t.group === g.id).map((t) => (
              <StarterCard key={t.id} starter={t} onPick={() => onStart(t.blocks)} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

// A starter as a card with a miniature of its layout - the canvas's own renderer at a fraction of the email's width
const THUMB_SCALE = 0.28

function StarterCard({ starter, onPick }: { starter: StarterTemplate; onPick: () => void }) {
  return (
    <button
      type="button"
      onClick={onPick}
      className="group grid gap-1.5 rounded-md border bg-muted/40 p-2 text-left transition-colors hover:border-primary/50 hover:bg-accent/40"
    >
      <span aria-hidden className="relative block h-32 overflow-hidden rounded border bg-white">
        <span
          className="pointer-events-none absolute left-1/2 top-0 block"
          style={{ width: 600, padding: 24, transform: `translateX(-50%) scale(${THUMB_SCALE})`, transformOrigin: 'top center', color: '#222', fontFamily: 'Helvetica, Arial, sans-serif' }}
        >
          {starter.blocks.map((b, i) => (
            <BlockView key={i} block={b} theme={{}} />
          ))}
        </span>
        <span className="absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-white to-transparent" />
      </span>
      <span className="px-0.5 text-sm font-medium text-foreground">{starter.name}</span>
      <span className="line-clamp-2 px-0.5 text-xs text-muted-foreground">{starter.description}</span>
    </button>
  )
}

function DeviceToggle({ device, onChange }: { device: Device; onChange: (d: Device) => void }) {
  const opts: Array<{ v: Device; label: string; icon: typeof MonitorIcon }> = [
    { v: 'desktop', label: 'Desktop', icon: MonitorIcon },
    { v: 'mobile', label: 'Mobile', icon: SmartphoneIcon },
  ]
  return (
    <div role="radiogroup" aria-label="Preview width" className="inline-flex items-center gap-0.5 rounded-md border bg-background p-0.5">
      {opts.map(({ v, label, icon: Icon }) => (
        <button
          key={v}
          type="button"
          role="radio"
          aria-checked={device === v}
          title={`${label} width`}
          onClick={() => onChange(v)}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded px-2 text-xs transition-colors',
            device === v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <Icon className="size-3.5" />
          {label}
        </button>
      ))}
    </div>
  )
}

// A phone's width, less the email's own side padding
const MOBILE_WIDTH = 343

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
  onEdit,
  onInsertAt,
  insertOptions = {},
  onStart,
  mergeTags = [],
  canAdd = false,
  toolbar,
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
  onEdit?: (sel: string, block: EmailBlock) => void
  onInsertAt?: (index: number, type: string) => void
  insertOptions?: PaletteOptions
  onStart?: (blocks: EmailBlock[]) => void
  mergeTags?: string[]
  canAdd?: boolean
  toolbar?: ReactNode
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
  const [device, setDevice] = useState<Device>('desktop')
  const desktopWidth = theme.contentWidth && theme.contentWidth >= 200 ? theme.contentWidth : 600
  const width = device === 'mobile' ? MOBILE_WIDTH : desktopWidth
  const fontFamily = theme.fontFamily || 'Helvetica, Arial, sans-serif'
  // The email's own text colour, as a send has it - never the dashboard's, which goes light in dark mode
  const textColor = theme.textColor || '#222'

  // A DragOverlay renders the dragged thing as a clean floating copy that follows the cursor - so reordering doesn't
  // distort/squash the real block, and palette drags show a labelled chip
  const { active } = useDndContext()
  const fromPalette = !!active?.data.current?.fromPalette
  const activeItem = active && !fromPalette ? items.find((i) => i.id === active.id) : undefined
  const activePaletteType = fromPalette ? String(active?.data.current?.blockType) : undefined
  const shared: CanvasShared = {
    sel,
    onSelect,
    onDelete,
    onDuplicate,
    onMoveChild,
    onEdit,
    onInsertAt,
    insertOptions,
    mergeTags,
    dragging: !!active,
    device,
    theme,
    components,
  }
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
        <EmptyState onStart={onStart} canAdd={canAdd} />
      ) : (
        <div className="grid gap-2.5">
          {items.map((item, i) => {
            const last = i === items.length - 1
            const dropEdge = dropBefore === item.id ? 'top' : last && dropBefore === END_DROP ? 'bottom' : null
            return <SortableBlock key={item.id} item={item} index={i} last={last} dropEdge={dropEdge} shared={shared} />
          })}
        </div>
      )}
    </SortableContext>
  )

  const bar = (
    <div className="mx-auto mb-2 flex max-w-3xl items-center justify-center gap-2 @min-[1400px]/editor:mb-3" onClick={(e) => e.stopPropagation()}>
      <DeviceToggle device={device} onChange={setDevice} />
      {toolbar}
    </div>
  )

  if (emailFrame) {
    const sender = senderName || SAMPLE_VARS['store.name'] || 'Your Store'
    const senderEmail = `hello@${(sender.toLowerCase().replace(/[^a-z0-9]+/g, '') || 'store')}.com`
    const subjectLine = interpolateSample(subject ?? '').trim()
    return (
      <>
      <div ref={setNodeRef} data-email-canvas tabIndex={-1} className="min-h-full p-3 outline-none @min-[1400px]/editor:p-8" onClick={() => onSelect(null)}>
        {bar}
        {/* Gmail-style opened-message frame around the rendered email */}
        <div className={cn('mx-auto overflow-hidden rounded-xl border bg-background shadow-sm transition-[max-width]', device === 'mobile' ? 'max-w-[400px]' : 'max-w-3xl')}>
          <div className={cn(device === 'mobile' ? 'px-4 pt-3' : 'px-4 pt-3 @min-[1400px]/editor:px-6 @min-[1400px]/editor:pt-5')}>
            <div className="flex items-start gap-3">
              <h2 className="min-w-0 flex-1 truncate text-lg font-normal leading-snug text-foreground @min-[1400px]/editor:text-[1.375rem]">
                {subjectLine || <span className="text-muted-foreground">(no subject)</span>}
              </h2>
              <span className="mt-1 shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                Inbox
              </span>
            </div>
            <div className="mt-2 flex items-center gap-3 pb-3 @min-[1400px]/editor:mt-4 @min-[1400px]/editor:pb-4">
              <div
                className="grid size-8 shrink-0 place-items-center rounded-full text-sm font-medium text-white @min-[1400px]/editor:size-10 @min-[1400px]/editor:text-base"
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
                {device === 'desktop' ? (
                  <>
                    <StarIcon className="size-4" />
                    <ReplyIcon className="size-4" />
                  </>
                ) : null}
                <MoreVerticalIcon className="size-4" />
              </div>
            </div>
          </div>
          {/* Email body - rendered with the email's own theme background */}
          <div className="border-t px-3 py-5 @min-[1400px]/editor:px-4 @min-[1400px]/editor:py-6" style={{ background: theme.background || '#ffffff' }}>
            <div
              className={cn('mx-auto', isOver && 'rounded ring-2 ring-primary/50', device === 'mobile' && '[&_.email-stack]:flex-col [&_.email-stack]:!items-stretch')}
              style={{ maxWidth: width, fontFamily, color: textColor }}
            >
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
      ref={setNodeRef}
      data-email-canvas
      tabIndex={-1}
      className="min-h-full p-6 outline-none"
      style={theme.background ? { background: theme.background } : undefined}
      onClick={() => onSelect(null)}
    >
      {bar}
      <div
        className={cn('mx-auto rounded-md bg-white p-6 shadow-sm', isOver && 'ring-2 ring-primary/50', device === 'mobile' && '[&_.email-stack]:flex-col [&_.email-stack]:!items-stretch')}
        style={{ maxWidth: device === 'mobile' ? MOBILE_WIDTH + 48 : desktopWidth, fontFamily, color: textColor }}
      >
        {list}
      </div>
    </div>
    {dragOverlay}
    </>
  )
}
