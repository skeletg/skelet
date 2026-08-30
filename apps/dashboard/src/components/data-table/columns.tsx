import { Fragment, type ComponentType, type ReactNode } from 'react'
import type {
  RowData, ColumnDef, HeaderContext, Row } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Badge } from '@/components/ui/badge'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import { DataTableColumnHeader } from './DataTableColumnHeader'

// Shared column builders, so a list declares what it shows rather than how to render it The row-action menu was hand-rolled in 41 list
// files, each with its own aria-label and separator rules - registered actions can only merge into one implementation

export type RowAction<TData extends RowData> = {
  id?: string
  label: ReactNode
  icon?: ComponentType<{ className?: string }>
  to?: (row: TData) => string // renders a Link, so middle-click and copy-link behave
  onSelect?: (row: TData) => void
  hidden?: (row: TData) => boolean
  destructive?: boolean
  separatorBefore?: boolean
}

export type ImageRef = { url: string; altText?: string | null; status?: string | null } | null | undefined

// status is the media pipeline's - a row whose image is still processing has a url that 404s
function ready(image: ImageRef): boolean {
  return !!image?.url && (image.status === undefined || image.status === null || image.status === 'READY')
}

// A thumbnail cell - products get one by default, and any list with a media field can opt in
export function imageColumn<TData extends RowData>({
  id = 'image',
  image,
  alt,
  size = 40,
  rounded = 'rounded',
  header = '',
}: {
  id?: string
  image: (row: TData) => ImageRef
  alt?: (row: TData) => string
  size?: number
  rounded?: string
  header?: ReactNode
}): ColumnDef<TData, unknown> {
  return {
    id,
    header: () => header,
    enableSorting: false,
    enableHiding: true,
    size: size + 16,
    cell: ({ row }) => {
      const img = image(row.original)
      const label = alt?.(row.original) ?? ''
      if (!ready(img)) {
        // A fixed placeholder rather than nothing - a column that collapses to zero width makes every row jump
        return (
          <div
            className={cn('shrink-0 border bg-muted', rounded)}
            style={{ width: size, height: size }}
            aria-hidden
          />
        )
      }
      return (
        <img
          src={img!.url}
          alt={img!.altText || label}
          loading="lazy"
          className={cn('shrink-0 border object-cover', rounded)}
          style={{ width: size, height: size }}
        />
      )
    },
  }
}

// A plain render call, not a component - a component export here would split this file for fast refresh
function renderAction<TData extends RowData>(action: RowAction<TData>, row: TData): ReactNode {
  const Icon = action.icon
  const body = (
    <>
      {Icon && <Icon className="size-4" />}
      {action.label}
    </>
  )
  if (action.to) {
    return (
      <DropdownMenuItem asChild variant={action.destructive ? 'destructive' : undefined}>
        <Link to={action.to(row)}>{body}</Link>
      </DropdownMenuItem>
    )
  }
  return (
    <DropdownMenuItem
      variant={action.destructive ? 'destructive' : undefined}
      onSelect={() => action.onSelect?.(row)}
    >
      {body}
    </DropdownMenuItem>
  )
}

// The trailing per-row menu - `label` names the row so the trigger gets a distinguishable aria-label
export function rowActionsColumn<TData extends RowData>({
  actions,
  label,
  id = 'actions',
}: {
  actions: RowAction<TData>[]
  label?: (row: TData) => string
  id?: string
}): ColumnDef<TData, unknown> {
  return {
    id,
    size: 48,
    enableSorting: false,
    enableHiding: false,
    cell: ({ row }: { row: Row<TData> }) => {
      const visible = actions.filter((a) => !a.hidden?.(row.original))
      if (visible.length === 0) return null
      const name = label?.(row.original)
      return (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={name ? `Actions for ${name}` : 'Row actions'}
            >
              <MoreHorizontalIcon className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {visible.map((action, i) => (
              <Fragment key={action.id ?? i}>
                {action.separatorBefore && i > 0 && <DropdownMenuSeparator />}
                {renderAction(action, row.original)}
              </Fragment>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )
    },
  }
}

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline' | 'ghost' | 'link'

// Header helper - a sortable header was three lines of JSX per column, repeated per list
function head<TData extends RowData>(
  title: ReactNode,
  sortable: boolean,
): ColumnDef<TData, unknown>['header'] {
  if (!sortable) return () => title
  return ({ column }: HeaderContext<TData, unknown>) => (
    <DataTableColumnHeader column={column} title={String(title)} />
  )
}

// Dates rendered per list with a local toLocaleDateString call, so the same column read
// differently across pages - one builder keeps them consistent and hides the null branch
export function dateColumn<TData extends RowData>({
  id,
  title,
  value,
  withTime = false,
  sortable = true,
  empty = '-',
}: {
  id: string
  title: ReactNode
  value: (row: TData) => string | number | Date | null | undefined
  withTime?: boolean
  sortable?: boolean
  empty?: ReactNode
}): ColumnDef<TData, unknown> {
  return {
    id,
    accessorFn: (row) => value(row) ?? null,
    header: head<TData>(title, sortable),
    cell: ({ row }) => {
      const raw = value(row.original)
      if (raw === null || raw === undefined || raw === '') return empty
      const d = new Date(raw)
      if (Number.isNaN(d.getTime())) return empty
      return (
        <span className="whitespace-nowrap tabular-nums">
          {withTime ? d.toLocaleString() : d.toLocaleDateString()}
        </span>
      )
    },
  }
}

// Money is integer cents everywhere on the wire - a column taking a float is the bug this prevents
export function moneyColumn<TData extends RowData>({
  id,
  title,
  cents,
  currency,
  sortable = true,
  empty = '-',
}: {
  id: string
  title: ReactNode
  cents: (row: TData) => number | null | undefined
  currency: ((row: TData) => string) | string
  sortable?: boolean
  empty?: ReactNode
}): ColumnDef<TData, unknown> {
  return {
    id,
    accessorFn: (row) => cents(row) ?? null,
    header: head<TData>(title, sortable),
    cell: ({ row }) => {
      const amount = cents(row.original)
      if (amount === null || amount === undefined) return empty
      const code = typeof currency === 'string' ? currency : currency(row.original)
      return <span className="whitespace-nowrap tabular-nums">{formatCents(amount, code)}</span>
    },
  }
}

// 33 list files render their own status Badge - a map from value to variant covers nearly all of them
export function badgeColumn<TData extends RowData>({
  id,
  title,
  value,
  variants,
  labels,
  sortable = true,
  empty = '-',
}: {
  id: string
  title: ReactNode
  value: (row: TData) => string | null | undefined
  variants?: Record<string, BadgeVariant>
  labels?: Record<string, string>
  sortable?: boolean
  empty?: ReactNode
}): ColumnDef<TData, unknown> {
  return {
    id,
    accessorFn: (row) => value(row) ?? null,
    header: head<TData>(title, sortable),
    cell: ({ row }) => {
      const raw = value(row.original)
      if (!raw) return empty
      return <Badge variant={variants?.[raw] ?? 'secondary'}>{labels?.[raw] ?? raw}</Badge>
    },
  }
}

// The primary column of most lists - the row's name linking to its detail page
export function linkColumn<TData extends RowData>({
  id,
  title,
  label,
  to,
  subtitle,
  sortable = true,
}: {
  id: string
  title: ReactNode
  label: (row: TData) => string
  to: (row: TData) => string
  subtitle?: (row: TData) => ReactNode
  sortable?: boolean
}): ColumnDef<TData, unknown> {
  return {
    id,
    accessorFn: (row) => label(row),
    header: head<TData>(title, sortable),
    cell: ({ row }) => {
      const sub = subtitle?.(row.original)
      return (
        <div className="grid min-w-0 gap-0.5">
          <Link to={to(row.original)} className="truncate font-medium hover:underline">
            {label(row.original)}
          </Link>
          {sub && <span className="line-clamp-1 text-xs text-muted-foreground">{sub}</span>}
        </div>
      )
    },
  }
}
