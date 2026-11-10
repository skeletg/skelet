import {
  BanknoteIcon,
  CodeIcon,
  Columns2Icon,
  HeadingIcon,
  ImageIcon,
  LayoutGridIcon,
  LayoutPanelLeftIcon,
  MapPinIcon,
  MenuIcon,
  MinusIcon,
  MousePointerClickIcon,
  MoveVerticalIcon,
  PanelBottomIcon,
  PlayIcon,
  PuzzleIcon,
  ReceiptIcon,
  Share2Icon,
  ShoppingBagIcon,
  TicketPercentIcon,
  TimerIcon,
  TruckIcon,
  TypeIcon,
  type LucideIcon,
} from 'lucide-react'

const ICONS: Record<string, LucideIcon> = {
  heading: HeadingIcon,
  text: TypeIcon,
  button: MousePointerClickIcon,
  image: ImageIcon,
  products: LayoutGridIcon,
  social: Share2Icon,
  video: PlayIcon,
  countdown: TimerIcon,
  html: CodeIcon,
  coupon: TicketPercentIcon,
  image_text: LayoutPanelLeftIcon,
  menu: MenuIcon,
  store_footer: PanelBottomIcon,
  divider: MinusIcon,
  spacer: MoveVerticalIcon,
  row: Columns2Icon,
  order_items: ShoppingBagIcon,
  order_totals: ReceiptIcon,
  shipping_address: MapPinIcon,
  tracking: TruckIcon,
  refund_amount: BanknoteIcon,
  ref: PuzzleIcon,
}

// A block type's icon - a saved reusable block, or anything else without its own, is a puzzle piece
export function BlockIcon({ type, className }: { type: string; className?: string }) {
  const Icon = ICONS[type] ?? PuzzleIcon
  return <Icon className={className} />
}
