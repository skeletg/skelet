import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { DESTINATION_KINDS } from './kinds'

const BADGE_STYLES: Record<string, string> = {
  webhook: 'border-info/40 bg-info-soft text-info',
  ga4_mp: 'border-border bg-muted text-muted-foreground',
  meta_capi: 'border-border bg-muted text-muted-foreground',
}

export function DestinationKindBadge({
  kind,
  className,
}: {
  kind: string
  className?: string
}) {
  const label = DESTINATION_KINDS.find((k) => k.value === kind)?.label ?? kind
  return (
    <Badge
      variant="outline"
      className={cn(
        'font-mono text-[10px] uppercase tracking-wider',
        BADGE_STYLES[kind] ?? 'text-muted-foreground',
        className,
      )}
    >
      {label}
    </Badge>
  )
}
