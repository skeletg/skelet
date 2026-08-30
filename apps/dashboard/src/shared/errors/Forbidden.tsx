import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeftIcon, HomeIcon, ShieldAlertIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function Forbidden({ permission }: { permission?: string }) {
  const navigate = useNavigate()
  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-5 overflow-hidden px-6 text-center">
      <div className="relative z-10 grid size-16 place-items-center rounded-full bg-background ring-1 ring-destructive/30 shadow-sm">
        <ShieldAlertIcon className="size-8 text-destructive" />
      </div>
      <div className="relative z-10 grid gap-1.5">
        <h1 className="text-2xl font-bold tracking-tight">Access denied</h1>
        <p className="max-w-md text-sm text-muted-foreground">
          You don&apos;t have permission to view this page.
          {permission ? (
            <>
              {' '}It requires the{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {permission}
              </code>{' '}
              permission.
            </>
          ) : null}{' '}
          Talk to an administrator if you think you should have access.
        </p>
      </div>
      <div className="relative z-10 flex flex-wrap items-center justify-center gap-2">
        <Button variant="outline" onClick={() => navigate(-1)}>
          <ArrowLeftIcon className="size-4" />
          Go back
        </Button>
        <Button asChild>
          <Link to="/">
            <HomeIcon className="size-4" />
            Dashboard
          </Link>
        </Button>
      </div>
    </div>
  )
}
