import { useDraggable } from '@dnd-kit/core'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { BLOCK_TYPES, type BlockComponent } from './blocks'
import { BlockIcon } from './BlockIcon'
import { FLOATING_PANEL, PANEL_TITLE } from './panel'

// Saved reusable blocks are listed dynamically (the `components` prop); the
// built-in palette carries no static "Reusable" group
const GROUPS: Array<{ label: string; group: 'content' | 'layout' | 'data'; note?: string }> = [
  { label: 'Content', group: 'content' },
  { label: 'Layout', group: 'layout' },
  // A campaign or a flow without an order renders these empty
  { label: 'Order data', group: 'data', note: 'order emails only' },
]

export type PaletteOptions = { allowRef?: boolean; allowData?: boolean; allowLayout?: boolean }

type Entry = { type: string; label: string; hint: string; icon: string }

function paletteGroups({ allowRef = true, allowData = true, allowLayout = true }: PaletteOptions, components: BlockComponent[]) {
  const groups: Array<{ label: string; note?: string; items: Entry[] }> = []
  for (const g of GROUPS) {
    if (g.group === 'data' && !allowData) continue
    if (g.group === 'layout' && !allowLayout) continue
    const items = BLOCK_TYPES.filter((b) => b.group === g.group)
    if (items.length > 0) groups.push({ label: g.label, note: g.note, items: items.map((b) => ({ type: b.type, label: b.label, hint: b.hint, icon: b.type })) })
  }
  if (allowRef && components.length > 0) {
    groups.push({
      label: 'Saved blocks',
      items: components.map((c) => ({ type: c.slug, label: c.name, hint: 'A block from your reusable library.', icon: 'ref' })),
    })
  }
  return groups
}

function PaletteItem({ entry, onAdd }: { entry: Entry; onAdd?: (type: string) => void }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `palette:${entry.type}`,
    data: { fromPalette: true, blockType: entry.type },
  })
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          ref={setNodeRef}
          {...listeners}
          {...attributes}
          role="button"
          tabIndex={0}
          aria-label={onAdd ? `Add ${entry.label}` : entry.label}
          // The pointer sensor only starts a drag after a few pixels, so a plain click lands here
          onClick={() => onAdd?.(entry.type)}
          onKeyDown={(e) => {
            if (onAdd && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onAdd(entry.type)
            }
          }}
          className={cn(
            'group flex cursor-grab touch-none select-none items-center justify-center rounded-md transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing',
            '@min-[1200px]/editor:flex-col @min-[1200px]/editor:gap-1.5 @min-[1200px]/editor:rounded-lg @min-[1200px]/editor:border @min-[1200px]/editor:bg-background @min-[1200px]/editor:p-2.5 @min-[1200px]/editor:text-center @min-[1200px]/editor:hover:-translate-y-0.5 @min-[1200px]/editor:hover:border-primary/40 @min-[1200px]/editor:hover:shadow-sm',
            isDragging && 'opacity-40',
          )}
        >
          <span className="grid size-9 place-items-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:bg-primary/10 group-hover:text-primary">
            <BlockIcon type={entry.icon} className="size-4" />
          </span>
          <span className="sr-only text-[11px] font-medium leading-tight @min-[1200px]/editor:not-sr-only">{entry.label}</span>
        </div>
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-56">
        <p className="font-medium">{entry.label}</p>
        <p className="opacity-80">{entry.hint}</p>
      </TooltipContent>
    </Tooltip>
  )
}

/** Left rail of blocks to click or drag in; `components` are the channel's saved blocks, added as a `ref` */
export function BlockPalette({
  components = [],
  onAdd,
  ...options
}: PaletteOptions & {
  components?: BlockComponent[]
  onAdd?: (type: string) => void
}) {
  return (
    // A slim rail of icons while the editor is narrow (a laptop with the sidebar open), so the email keeps its width
    <aside data-email-palette aria-label="Blocks" className={cn(FLOATING_PANEL, 'mr-0 w-16 @min-[1200px]/editor:w-56')}>
      <div className="hidden border-b px-4 py-3.5 @min-[1200px]/editor:block">
        <h2 className={PANEL_TITLE}>Blocks</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">{onAdd ? 'Click to add, or drag onto the canvas' : 'Drag onto the canvas'}</p>
      </div>
      <div className="flex-1 space-y-2 overflow-y-auto px-2 py-3 @min-[1200px]/editor:space-y-3 @min-[1200px]/editor:p-3">
        {paletteGroups(options, components).map((g, i) => (
          <div key={g.label} className="space-y-1.5">
            <div className="hidden px-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70 @min-[1200px]/editor:block">
              {g.label}
              {g.note ? <span className="font-normal normal-case tracking-normal"> · {g.note}</span> : null}
            </div>
            {i > 0 ? <div className="mx-auto h-px w-8 bg-border @min-[1200px]/editor:hidden" aria-hidden /> : null}
            <div className="grid grid-cols-1 justify-items-center gap-1 @min-[1200px]/editor:grid-cols-2 @min-[1200px]/editor:justify-items-stretch @min-[1200px]/editor:gap-2">
              {g.items.map((entry) => (
                <PaletteItem key={entry.type} entry={entry} onAdd={onAdd} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  )
}

export function BlockMenu({ components = [], onPick, ...options }: PaletteOptions & { components?: BlockComponent[]; onPick: (type: string) => void }) {
  return (
    <div className="grid gap-2">
      {paletteGroups(options, components).map((g) => (
        <div key={g.label} className="grid gap-0.5">
          <div className="px-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70">
            {g.label}
            {g.note ? <span className="font-normal normal-case tracking-normal"> · {g.note}</span> : null}
          </div>
          <div className="grid grid-cols-2 gap-0.5">
            {g.items.map((entry) => (
              <button
                key={entry.type}
                type="button"
                title={entry.hint}
                onClick={() => onPick(entry.type)}
                className="flex items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-xs hover:bg-accent hover:text-accent-foreground"
              >
                <BlockIcon type={entry.icon} className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="truncate">{entry.label}</span>
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
