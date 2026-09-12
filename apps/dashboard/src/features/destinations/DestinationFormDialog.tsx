import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { EventDestinationInput } from '@/graphql/schema.types'
import { DESTINATION_KINDS } from './kinds'
import {
  CreateEventDestinationDocument,
  UpdateEventDestinationDocument,
} from './Destinations.gql'

// The analytics spine's source column - filters match against these
const EVENT_SOURCES = ['campaign', 'flow', 'transactional', 'web', 'system']

const URL_HINTS: Record<string, string> = {
  webhook:
    'Receives POST batches signed with the secret (X-Destination-Signature over "<ts>.<body>", timestamp in X-Destination-Timestamp).',
  ga4_mp:
    'The Measurement Protocol collect URL with credentials as query params, e.g. https://www.google-analytics.com/mp/collect?measurement_id=G-XXXX&api_secret=…',
  meta_capi:
    'The pixel events URL with the token as a query param, e.g. https://graph.facebook.com/v21.0/<pixel-id>/events?access_token=…',
}

export type ExistingDestination = {
  id: string
  name: string
  kind: string
  targetUrl: string
  eventSources: string[]
  eventNames: string[]
  isActive: boolean
}

export function DestinationFormDialog({
  open,
  onOpenChange,
  channelId,
  existing,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string
  existing?: ExistingDestination | null
  onSaved: () => void
}) {
  const [name, setName] = useState('')
  const [kind, setKind] = useState('webhook')
  const [targetUrl, setTargetUrl] = useState('')
  const [secret, setSecret] = useState('')
  const [sources, setSources] = useState<string[]>([])
  const [names, setNames] = useState('')
  const [isActive, setIsActive] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [create, { loading: creating }] = useMutation(CreateEventDestinationDocument)
  const [update, { loading: updating }] = useMutation(UpdateEventDestinationDocument)

  // Hydrate form state when the dialog opens - a state adjustment during
  // render (same pattern as WebhookFormDialog) rather than an effect
  const [prevHydrateKey, setPrevHydrateKey] = useState<{
    open: boolean
    existing?: ExistingDestination | null
  }>({ open, existing })
  if (prevHydrateKey.open !== open || prevHydrateKey.existing !== existing) {
    setPrevHydrateKey({ open, existing })
    if (open) {
      setName(existing?.name ?? '')
      setKind(existing?.kind ?? 'webhook')
      setTargetUrl(existing?.targetUrl ?? '')
      setSecret('')
      setSources(existing?.eventSources ?? [])
      setNames(existing?.eventNames.join(', ') ?? '')
      setIsActive(existing?.isActive ?? true)
      setError(null)
    }
  }

  function toggleSource(source: string) {
    setSources((prev) =>
      prev.includes(source) ? prev.filter((s) => s !== source) : [...prev, source],
    )
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const input: EventDestinationInput = {
      channelId,
      name: name.trim(),
      kind,
      targetUrl: targetUrl.trim(),
      // Write-only: blank keeps the stored secret on edit
      secret: secret.trim() === '' ? undefined : secret.trim(),
      eventSources: sources,
      eventNames: names
        .split(',')
        .map((n) => n.trim())
        .filter(Boolean),
      isActive,
    }
    try {
      if (existing) {
        await update({ variables: { id: existing.id, input } })
      } else {
        await create({ variables: { input } })
      }
      onOpenChange(false)
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed.')
    }
  }

  const isLoading = creating || updating

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-lg">
        <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{existing ? 'Edit destination' : 'New destination'}</DialogTitle>
            <DialogDescription>
              {existing
                ? 'Update where and which analytics events are forwarded.'
                : 'Forward analytics events to an external system. Delivery starts from creation - history is not replayed.'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto py-1 pr-1">
            <div className="grid gap-2">
              <Label>Kind</Label>
              <div className="grid grid-cols-3 gap-2">
                {DESTINATION_KINDS.map((k) => (
                  <button
                    key={k.value}
                    type="button"
                    onClick={() => setKind(k.value)}
                    aria-pressed={kind === k.value}
                    className={cn(
                      'grid gap-1 rounded-md border p-3 text-left transition',
                      kind === k.value
                        ? 'border-primary/50 bg-primary/5 ring-1 ring-primary/30'
                        : 'hover:bg-muted/40',
                    )}
                  >
                    <span className="text-sm font-semibold">{k.label}</span>
                    <span className="text-xs text-muted-foreground">{k.description}</span>
                  </button>
                ))}
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="destination-name">Name</Label>
              <Input
                id="destination-name"
                placeholder="e.g. Production GA4"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="destination-url">Target URL</Label>
              <Input
                id="destination-url"
                type="url"
                placeholder="https://…"
                value={targetUrl}
                onChange={(e) => setTargetUrl(e.target.value)}
                required
              />
              <p className="text-xs text-muted-foreground">{URL_HINTS[kind]}</p>
            </div>

            {kind === 'webhook' && (
              <div className="grid gap-2">
                <Label htmlFor="destination-secret">Signing secret</Label>
                <Input
                  id="destination-secret"
                  placeholder={existing ? 'Leave blank to keep the current secret' : 'HMAC key'}
                  value={secret}
                  onChange={(e) => setSecret(e.target.value)}
                  required={!existing}
                />
                <p className="text-xs text-muted-foreground">
                  Write-only - it is never shown again after saving.
                </p>
              </div>
            )}

            <div className="grid gap-2">
              <Label>Event sources</Label>
              <div className="flex flex-wrap gap-4 rounded-md border p-3">
                {EVENT_SOURCES.map((source) => (
                  <div key={source} className="flex items-center gap-2">
                    <Checkbox
                      id={`source-${source}`}
                      checked={sources.includes(source)}
                      onCheckedChange={() => toggleSource(source)}
                    />
                    <Label htmlFor={`source-${source}`} className="text-sm font-normal">
                      {source}
                    </Label>
                  </div>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                Leave everything unchecked to forward all sources.
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="destination-events">Event names</Label>
              <Input
                id="destination-events"
                placeholder="order_placed, page_view (blank = all events)"
                value={names}
                onChange={(e) => setNames(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Comma-separated exact event names. Blank forwards every event.
              </p>
            </div>

            <div className="flex items-center gap-3">
              <Switch id="destination-active" checked={isActive} onCheckedChange={setIsActive} />
              <Label htmlFor="destination-active" className="font-normal">
                Active - included in the every-minute forwarding sweep
              </Label>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? 'Saving…' : existing ? 'Save changes' : 'Create destination'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
