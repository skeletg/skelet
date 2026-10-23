import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRightIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { Empty } from '@/components/layout/panels'
import { Skeleton } from '@/components/ui/skeleton'

// The pieces every dashboard section is built from, so twenty sections don't each invent a panel,
// an empty state and a loading row - the tile, panel and empty state are shared with the analytics pages
export { Empty, Panel, Tile } from '@/components/layout/panels'

// One section of the dashboard: the heading and description feed the page's section index, `to` turns the section into an entry point
// rather than a dead end, and the three states below are handled here so a section body only ever renders the happy path
export function WidgetSection({
  title,
  description,
  to,
  toLabel = 'View all',
  loading,
  error,
  empty,
  children,
  trailing,
}: {
  title: string
  description?: string
  to?: string
  toLabel?: string
  loading?: boolean
  error?: { message: string } | null
  empty?: boolean
  children: ReactNode
  trailing?: ReactNode
}) {
  return (
    <DetailPageSection
      title={title}
      description={description}
      trailing={
        <>
          {trailing}
          {to && (
            <Link
              to={to}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-primary"
            >
              {toLabel}
              <ArrowRightIcon className="size-3.5" />
            </Link>
          )}
        </>
      }
    >
      {error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : loading ? (
        <div className="grid gap-2" aria-busy>
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-2/3" />
        </div>
      ) : (
        <div data-arrive="">{empty ? <Empty>Nothing here right now.</Empty> : children}</div>
      )}
    </DetailPageSection>
  )
}

// A queue row: what's waiting, how much of it, and a way in. The count leads because that's what
// makes a queue worth opening
export function QueueRow({
  label,
  count,
  hint,
  to,
  tone = 'default',
}: {
  label: string
  count: number
  hint?: string
  to: string
  tone?: 'default' | 'warn' | 'bad'
}) {
  return (
    <Link
      to={to}
      className="flex items-center gap-3 px-3 py-2 transition hover:bg-muted/40"
    >
      <span
        className={cn(
          'h-8 w-0.5 shrink-0 rounded-full',
          count === 0 ? 'bg-transparent' : tone === 'bad' ? 'bg-destructive' : tone === 'warn' ? 'bg-warning' : 'bg-primary/40',
        )}
      />
      <span className="min-w-0 flex-1">
        <span className="block text-sm">{label}</span>
        {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
      </span>
      <span
        className={cn(
          'text-lg font-semibold tabular-nums',
          count === 0 && 'text-muted-foreground/50',
        )}
      >
        {count.toLocaleString()}
      </span>
    </Link>
  )
}

export function RowList({ children }: { children: ReactNode }) {
  return <div className="divide-y rounded-md border">{children}</div>
}

// Every channel-scoped section says the same thing in GLOBAL scope, in the same words the Carts
// page uses - an empty chart would read as "no traffic" rather than "no channel picked"
export function NeedsChannel({ what }: { what: string }) {
  return <Empty>Pick a channel in the header scope picker to see {what}.</Empty>
}
