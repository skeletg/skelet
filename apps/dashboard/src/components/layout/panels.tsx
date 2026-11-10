import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRightIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'

// The stat tile, header-strip panel and empty state the home dashboard and the analytics pages are built from

export function Tile({
  label,
  value,
  sub,
  tone,
  loading,
  to,
}: {
  label: string
  value: string
  sub?: string
  tone?: 'warn'
  loading?: boolean
  /** Makes the whole tile a link to where its number comes from */
  to?: string
}) {
  const body = (
    <>
      <p className="flex items-center justify-between gap-2 text-xs uppercase tracking-wider text-muted-foreground">
        {label}
        {to ? <ArrowRightIcon className="size-3.5 shrink-0 transition-transform group-hover:translate-x-0.5" aria-hidden /> : null}
      </p>
      {loading ? (
        <Skeleton className="mt-1 h-8 w-24" />
      ) : (
        <p
          className={cn('mt-1 text-2xl font-semibold tabular-nums', tone === 'warn' && 'text-warning')}
          data-arrive={loading === false ? '' : undefined}
        >
          {value}
        </p>
      )}
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </>
  )
  return to ? (
    <Link to={to} className="group block rounded border p-4 transition-colors hover:bg-muted/40">
      {body}
    </Link>
  ) : (
    <div className="rounded border p-4">{body}</div>
  )
}

// A bordered panel with a header strip - the flat skelet alternative to a Card
export function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </div>
      <div className="p-3">{children}</div>
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}

export function PickStore({ title = 'Pick a store', why }: { title?: string; why: string }) {
  return (
    <Alert>
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{why} - select one from the channel switcher.</AlertDescription>
    </Alert>
  )
}
