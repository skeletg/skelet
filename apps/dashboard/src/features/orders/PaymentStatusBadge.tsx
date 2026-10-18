import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { PaymentStatus } from '@/graphql/schema.types'

const styles: Record<PaymentStatus, { label: string; className: string }> = {
  UNPAID:     { label: 'Unpaid',     className: 'bg-muted text-muted-foreground' },
  AUTHORIZED: { label: 'Authorized', className: 'bg-warning-soft text-warning border-warning/30' },
  PARTIALLY_CAPTURED: { label: 'Partially captured', className: 'bg-warning-soft text-warning border-warning/30' },
  PAID:       { label: 'Paid',       className: 'bg-success-soft text-success border-success/30' },
  PARTIALLY_REFUNDED: { label: 'Partially refunded', className: 'bg-muted text-muted-foreground border-border' },
  REFUNDED:   { label: 'Refunded',   className: 'bg-muted text-muted-foreground border-border' },
  VOIDED:     { label: 'Voided',     className: 'bg-muted text-muted-foreground border-border' },
}

export function PaymentStatusBadge({ status, className }: { status: PaymentStatus; className?: string }) {
  // Defensive fallback: an unrecognised status (schema drift) renders a
  // neutral badge instead of crashing the order screen
  const s = styles[status] ?? { label: String(status), className: 'bg-muted text-muted-foreground' }
  return (
    <Badge variant="outline" className={cn('font-medium', s.className, className)}>
      {s.label}
    </Badge>
  )
}
