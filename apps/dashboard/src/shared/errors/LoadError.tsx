import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeftIcon, HomeIcon, RotateCcwIcon, SkullIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'

// Full-page failure state for load errors - mirrors the NotFound and Forbidden hero layout but in the destructive palette, so it reads as
// "something is broken" rather than "wrong turn" Pass `onRetry` to expose a Retry button wired to the query's refetch

export function LoadError({
  title = "Couldn't load this page",
  description,
  message,
  onRetry,
  retrying,
}: {
  title?: string
  description?: string
  /** Raw error message from the failed request - surfaced in a mono block */
  message?: string
  /** When provided, renders a Retry button that calls it */
  onRetry?: () => void
  retrying?: boolean
}) {
  const navigate = useNavigate()
  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-5 overflow-hidden px-6 text-center">
      <div className="relative z-10 grid size-16 place-items-center rounded-full bg-background ring-1 ring-destructive/30 shadow-sm">
        <SkullIcon className="size-8 text-destructive" />
      </div>
      <div className="relative z-10 grid gap-1.5">
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {description ? (
          <p className="max-w-md text-sm text-muted-foreground">{description}</p>
        ) : (
          <p className="max-w-md text-sm text-muted-foreground">
            The server returned an error. This is usually transient - try again
            in a moment.
          </p>
        )}
        {message ? (
          <pre className="mx-auto mt-2 max-w-xl overflow-x-auto rounded-md border bg-muted/40 px-3 py-2 text-left font-mono text-xs text-muted-foreground">
            {message}
          </pre>
        ) : null}
      </div>
      <div className="relative z-10 flex flex-wrap items-center justify-center gap-2">
        {onRetry ? (
          <Button onClick={() => onRetry()} disabled={retrying}>
            <RotateCcwIcon className="size-4" />
            {retrying ? 'Retrying…' : 'Retry'}
          </Button>
        ) : null}
        <Button variant="outline" onClick={() => navigate(-1)}>
          <ArrowLeftIcon className="size-4" />
          Go back
        </Button>
        <Button asChild variant="outline">
          <Link to="/">
            <HomeIcon className="size-4" />
            Dashboard
          </Link>
        </Button>
      </div>
    </div>
  )
}
