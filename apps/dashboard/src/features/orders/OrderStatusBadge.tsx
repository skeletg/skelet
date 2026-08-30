import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { OrderStatus } from '@/graphql/schema.types'

// Status colors map to Tailwind palette classes rather than theme tokens so
// order statuses stay readable regardless of which palette the admin picks
const styles: Record<OrderStatus, { label: string; className: string }> = {
  DRAFT:      { label: 'Draft',      className: 'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-800/60 dark:text-slate-200 dark:border-slate-700' },
  PENDING:    { label: 'Pending',    className: 'bg-amber-100 text-amber-900 border-amber-200 dark:bg-amber-950/60 dark:text-amber-100 dark:border-amber-900' },
  CONFIRMED:  { label: 'Confirmed',  className: 'bg-blue-100 text-blue-900 border-blue-200 dark:bg-blue-950/60 dark:text-blue-100 dark:border-blue-900' },
  ON_HOLD:    { label: 'On hold',    className: 'bg-orange-100 text-orange-900 border-orange-200 dark:bg-orange-950/60 dark:text-orange-100 dark:border-orange-900' },
  PROCESSING: { label: 'Processing', className: 'bg-violet-100 text-violet-900 border-violet-200 dark:bg-violet-950/60 dark:text-violet-100 dark:border-violet-900' },
  PARTIALLY_FULFILLED: { label: 'Partially fulfilled', className: 'bg-teal-100 text-teal-900 border-teal-200 dark:bg-teal-950/60 dark:text-teal-100 dark:border-teal-900' },
  FULFILLED:  { label: 'Fulfilled',  className: 'bg-emerald-100 text-emerald-900 border-emerald-200 dark:bg-emerald-950/60 dark:text-emerald-100 dark:border-emerald-900' },
  CANCELLED:  { label: 'Cancelled',  className: 'bg-rose-100 text-rose-900 border-rose-200 dark:bg-rose-950/60 dark:text-rose-100 dark:border-rose-900' },
}

// `status` is typed as the enum but reaches here from aggregate queries too, where it is a plain string (the reports
// breakdown returns the lowercase column value) - an unmapped one renders readably rather than taking the page down
export function OrderStatusBadge({ status, className }: { status: OrderStatus | string; className?: string }) {
  const s = styles[String(status).toUpperCase() as OrderStatus]
  return (
    <Badge variant="outline" className={cn('font-medium', s?.className, className)}>
      {s?.label ?? String(status).replaceAll('_', ' ')}
    </Badge>
  )
}
