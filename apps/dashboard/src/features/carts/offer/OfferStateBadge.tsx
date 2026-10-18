import { Badge } from '@/components/ui/badge'
import type { CartOfferState } from '@/graphql/schema.types'

const STATES: Record<CartOfferState, { label: string; variant: 'outline' | 'info' | 'success' | 'warning' | 'destructive' }> = {
  DRAFT: { label: 'Draft', variant: 'outline' },
  SENT: { label: 'Sent', variant: 'info' },
  OPENED: { label: 'Opened', variant: 'info' },
  COMPLETED: { label: 'Completed', variant: 'success' },
  EXPIRED: { label: 'Expired', variant: 'warning' },
  REVOKED: { label: 'Revoked', variant: 'destructive' },
}

export function OfferStateBadge({ state }: { state: CartOfferState }) {
  const s = STATES[state]
  return (
    <Badge variant={s.variant} className="text-[10px] uppercase tracking-wide">
      {s.label}
    </Badge>
  )
}
