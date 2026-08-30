import { SkullIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

// Centered loading placeholder for detail pages while their primary query is in flight -
// matches the 404 page's Skull and badge, so loading, empty and not-found read as one family
// `default` is a full-pane block for top-level views, `compact` an inline one for in-card spots
export function LoadingState({
  label = 'Loading…',
  className,
  compact = false,
}: {
  label?: string
  className?: string
  compact?: boolean
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-3 text-center',
        compact ? 'py-8' : 'min-h-[40vh] py-12',
        className,
      )}
    >
      <div
        className={cn(
          'grid place-items-center rounded-full bg-background ring-1 ring-border shadow-sm',
          compact ? 'size-10' : 'size-14',
        )}
      >
        <SkullIcon
          className={cn(
            'animate-pulse text-muted-foreground',
            compact ? 'size-5' : 'size-7',
          )}
        />
      </div>
      <p className="text-sm text-muted-foreground">{label}</p>
    </div>
  )
}
