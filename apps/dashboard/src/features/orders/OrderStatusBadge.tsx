import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { OrderStatus } from '@/graphql/schema.types'

// Status colors map to Tailwind palette classes rather than theme tokens so
// order statuses stay readable regardless of which palette the admin picks
const styles: Record<OrderStatus, { label: string; className: string }> = {
  DRAFT:      { label: 'Draft',      className: 'bg-muted text-muted-foreground border-border' },
  PENDING:    { label: 'Pending',    className: 'bg-warning-soft text-warning border-warning/30' },
  CONFIRMED:  { label: 'Confirmed',  className: 'bg-info-soft text-info border-info/30' },
  ON_HOLD:    { label: 'On hold',    className: 'bg-warning-soft text-warning border-warning/30' },
  PROCESSING: { label: 'Processing', className: 'bg-info-soft text-info border-info/30' },
  PARTIALLY_FULFILLED: { label: 'Partially fulfilled', className: 'bg-info-soft text-info border-info/30' },
  FULFILLED:  { label: 'Fulfilled',  className: 'bg-success-soft text-success border-success/30' },
  CANCELLED:  { label: 'Cancelled',  className: 'bg-destructive-soft text-destructive border-destructive/30' },
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
