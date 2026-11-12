import { useDraggable } from '@dnd-kit/core'
import { Rail, RailGroup, RailItem } from '@/components/layout/workbench'
import { BLOCK_TYPES, type BlockComponent } from './blocks'
import { BlockIcon } from './BlockIcon'

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
    <RailItem
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      label={entry.label}
      hint={entry.hint}
      icon={<BlockIcon type={entry.icon} className="size-4" />}
      dragging={isDragging}
      aria-label={onAdd ? `Add ${entry.label}` : entry.label}
      // The pointer sensor only starts a drag after a few pixels, so a plain click lands here
      onClick={() => onAdd?.(entry.type)}
      onKeyDown={(e) => {
        if (onAdd && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          onAdd(entry.type)
        }
      }}
    />
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
    <Rail data-email-palette label="Blocks" title="Blocks" hint={onAdd ? 'Click to add, or drag onto the canvas' : 'Drag onto the canvas'}>
      {paletteGroups(options, components).map((g, i) => (
        <RailGroup key={g.label} label={g.label} note={g.note} first={i === 0}>
          {g.items.map((entry) => (
            <PaletteItem key={entry.type} entry={entry} onAdd={onAdd} />
          ))}
        </RailGroup>
      ))}
    </Rail>
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
