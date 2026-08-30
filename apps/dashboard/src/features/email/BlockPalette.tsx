import { useDraggable } from '@dnd-kit/core'
import {
  GripVerticalIcon,
  HeadingIcon,
  TypeIcon,
  MousePointerClickIcon,
  ImageIcon,
  MinusIcon,
  MoveVerticalIcon,
  Columns2Icon,
  ShoppingBagIcon,
  ReceiptIcon,
  MapPinIcon,
  TruckIcon,
  BanknoteIcon,
  PuzzleIcon,
  LayoutGridIcon,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { BLOCK_TYPES, type BlockComponent } from './blocks'

const ICONS: Record<string, LucideIcon> = {
  heading: HeadingIcon,
  text: TypeIcon,
  button: MousePointerClickIcon,
  image: ImageIcon,
  divider: MinusIcon,
  spacer: MoveVerticalIcon,
  products: LayoutGridIcon,
  row: Columns2Icon,
  order_items: ShoppingBagIcon,
  order_totals: ReceiptIcon,
  shipping_address: MapPinIcon,
  tracking: TruckIcon,
  refund_amount: BanknoteIcon,
  ref: PuzzleIcon,
}

// Saved reusable blocks are listed dynamically (the `components` prop); the
// built-in palette carries no static "Reusable" group
const GROUPS: Array<{ label: string; group: 'content' | 'layout' | 'data' }> = [
  { label: 'Content', group: 'content' },
  { label: 'Layout', group: 'layout' },
  { label: 'Order data', group: 'data' },
]

function PaletteItem({
  type,
  label,
  icon,
}: {
  type: string
  label: string
  icon?: LucideIcon
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `palette:${type}`,
    data: { fromPalette: true, blockType: type },
  })
  const Icon = icon ?? ICONS[type] ?? GripVerticalIcon
  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      role="button"
      tabIndex={0}
      className={cn(
        'group flex cursor-grab touch-none select-none flex-col items-center gap-1.5 rounded-lg border bg-background p-2.5 text-center transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-sm active:cursor-grabbing',
        isDragging && 'opacity-40',
      )}
      title="Drag onto the canvas to add"
    >
      <span className="grid size-9 place-items-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:bg-primary/10 group-hover:text-primary">
        <Icon className="size-4" />
      </span>
      <span className="line-clamp-2 text-[11px] font-medium leading-tight">{label}</span>
    </div>
  )
}

/** Left rail of draggable blocks - drag one onto the canvas to place it at a
 *  position. `allowData`/`allowLayout` hide built-in groups that don't apply
 *  `components` are the channel's saved reusable blocks; dropping one adds a
 *  `ref` to it (the host maps the slug → ref). `allowRef` hides the saved-blocks
 *  group (e.g. when editing a block that shouldn't nest) */
export function BlockPalette({
  allowRef = true,
  allowData = true,
  allowLayout = true,
  components = [],
}: {
  allowRef?: boolean
  allowData?: boolean
  allowLayout?: boolean
  components?: BlockComponent[]
}) {
  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-card">
      <div className="border-b px-4 py-3">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Blocks</h2>
        <p className="mt-0.5 text-[11px] text-muted-foreground">Drag onto the canvas</p>
      </div>
      <div className="flex-1 space-y-3 overflow-y-auto p-3">
        {GROUPS.map((g) => {
          if (g.group === 'data' && !allowData) return null
          if (g.group === 'layout' && !allowLayout) return null
          const items = BLOCK_TYPES.filter((b) => b.group === g.group)
          if (items.length === 0) return null
          return (
            <div key={g.group} className="space-y-1.5">
              <div className="px-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                {g.label}
              </div>
              <div className="grid grid-cols-2 gap-2">
                {items.map((b) => (
                  <PaletteItem key={b.type} type={b.type} label={b.label} />
                ))}
              </div>
            </div>
          )
        })}
        {allowRef && components.length > 0 && (
          <div className="space-y-1.5">
            <div className="px-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70">
              Saved blocks
            </div>
            <div className="grid grid-cols-2 gap-2">
              {components.map((c) => (
                <PaletteItem key={c.slug} type={c.slug} label={c.name} icon={PuzzleIcon} />
              ))}
            </div>
          </div>
        )}
      </div>
    </aside>
  )
}
