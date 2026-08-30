import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { WebhookKind } from '@/graphql/schema.types'

// Blue for async fire-and-forget, red for sync validators that can abort the calling operation -
// same palette in list, detail and picker so the distinction reads consistently
export function WebhookKindBadge({
  kind,
  className,
}: {
  kind: WebhookKind
  className?: string
}) {
  return (
    <Badge
      variant="outline"
      className={cn(
        'font-mono text-[10px] uppercase tracking-wider',
        kind === 'SYNC'
          ? 'border-destructive/40 bg-destructive/10 text-destructive'
          : 'border-blue-500/40 bg-blue-500/10 text-blue-600 dark:text-blue-400',
        className,
      )}
    >
      {kind}
    </Badge>
  )
}
