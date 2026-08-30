import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeftIcon, ShieldAlertIcon, StoreIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'

// Full-page states for a channel-scoped entity opened outside its channel - the 404 siblings for
// "right URL, wrong scope" - Either the admin CAN reach the entity's channel and the header points
// elsewhere, so EntityWrongChannel offers one-click switching through setScope, or the channel is
// outside their role scopes and EntityChannelForbidden explains it - EntityWrongChannel picks
// between them itself: `channels` is permission-filtered, so a missing entity channel is the latter

export function EntityWrongChannel({
  entityLabel,
  entityChannelId,
  permission,
  backTo,
  backLabel,
}: {
  entityLabel: string
  entityChannelId: string
  permission?: string
  backTo: string
  backLabel: string
}) {
  const { channels, setScope } = useConfigScope()
  const target = channels.find((c) => c.id === entityChannelId)

  if (!target) {
    // Not a scope-switching problem but a permissions one
    return (
      <EntityChannelForbidden
        entityLabel={entityLabel}
        permission={permission}
        backTo={backTo}
        backLabel={backLabel}
      />
    )
  }

  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-5 overflow-hidden px-6 text-center">
      <div className="relative z-10 grid size-16 place-items-center rounded-full bg-background ring-1 ring-border shadow-sm">
        <StoreIcon className="size-8 text-muted-foreground" />
      </div>
      <div className="relative z-10 grid gap-1.5">
        <div className="font-mono text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          Wrong channel
        </div>
        <h1 className="text-2xl font-bold tracking-tight">
          This {entityLabel} belongs to {target.name}
        </h1>
        <p className="max-w-md text-sm text-muted-foreground">
          Your header scope is set to a different channel. Entities are
          per-channel - switch the scope to{' '}
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
            {target.name}
          </code>{' '}
          (or to Global) to view and manage this {entityLabel}.
        </p>
      </div>
      <div className="relative z-10 flex flex-wrap items-center justify-center gap-2">
        <Button
          onClick={() => setScope({ mode: 'CHANNEL', channelId: target.id, locale: '' })}
        >
          <StoreIcon className="size-4" />
          Switch to {target.name}
        </Button>
        <Button
          variant="outline"
          onClick={() => setScope({ mode: 'GLOBAL', channelId: '', locale: '' })}
        >
          View in Global scope
        </Button>
        <Button variant="ghost" asChild>
          <Link to={backTo}>
            <ArrowLeftIcon className="size-4" />
            {backLabel}
          </Link>
        </Button>
      </div>
    </div>
  )
}

export function EntityChannelForbidden({
  entityLabel,
  permission,
  backTo,
  backLabel,
}: {
  entityLabel: string
  permission?: string
  backTo: string
  backLabel: string
}) {
  const navigate = useNavigate()
  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-5 overflow-hidden px-6 text-center">
      <div className="relative z-10 grid size-16 place-items-center rounded-full bg-background ring-1 ring-destructive/30 shadow-sm">
        <ShieldAlertIcon className="size-8 text-destructive" />
      </div>
      <div className="relative z-10 grid gap-1.5">
        <div className="font-mono text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          Access restricted
        </div>
        <h1 className="text-2xl font-bold tracking-tight">
          Not allowed for this {entityLabel}&apos;s channel
        </h1>
        <p className="max-w-md text-sm text-muted-foreground">
          This {entityLabel} belongs to a channel your role doesn&apos;t
          cover
          {permission ? (
            <>
              {' '}
              (requires{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {permission}
              </code>{' '}
              on that channel)
            </>
          ) : null}
          . Talk to an administrator if you think you should have access.
        </p>
      </div>
      <div className="relative z-10 flex flex-wrap items-center justify-center gap-2">
        <Button variant="outline" onClick={() => navigate(-1)}>
          <ArrowLeftIcon className="size-4" />
          Go back
        </Button>
        <Button asChild>
          <Link to={backTo}>{backLabel}</Link>
        </Button>
      </div>
    </div>
  )
}
