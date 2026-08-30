import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { PaymentStatus } from '@/graphql/schema.types'

const styles: Record<PaymentStatus, { label: string; className: string }> = {
  UNPAID:     { label: 'Unpaid',     className: 'bg-muted text-muted-foreground' },
  AUTHORIZED: { label: 'Authorized', className: 'bg-amber-100 text-amber-900 border-amber-200 dark:bg-amber-950/60 dark:text-amber-100 dark:border-amber-900' },
  PARTIALLY_CAPTURED: { label: 'Partially captured', className: 'bg-amber-100 text-amber-900 border-amber-200 dark:bg-amber-950/60 dark:text-amber-100 dark:border-amber-900' },
  PAID:       { label: 'Paid',       className: 'bg-emerald-100 text-emerald-900 border-emerald-200 dark:bg-emerald-950/60 dark:text-emerald-100 dark:border-emerald-900' },
  PARTIALLY_REFUNDED: { label: 'Partially refunded', className: 'bg-slate-100 text-slate-900 border-slate-200 dark:bg-slate-950/60 dark:text-slate-100 dark:border-slate-900' },
  REFUNDED:   { label: 'Refunded',   className: 'bg-slate-100 text-slate-900 border-slate-200 dark:bg-slate-950/60 dark:text-slate-100 dark:border-slate-900' },
  VOIDED:     { label: 'Voided',     className: 'bg-slate-100 text-slate-900 border-slate-200 dark:bg-slate-950/60 dark:text-slate-100 dark:border-slate-900' },
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
