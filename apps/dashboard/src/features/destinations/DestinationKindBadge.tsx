import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { DESTINATION_KINDS } from './kinds'

const BADGE_STYLES: Record<string, string> = {
  webhook: 'border-blue-500/40 bg-blue-500/10 text-blue-600 dark:text-blue-400',
  ga4_mp: 'border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  meta_capi: 'border-violet-500/40 bg-violet-500/10 text-violet-600 dark:text-violet-400',
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
