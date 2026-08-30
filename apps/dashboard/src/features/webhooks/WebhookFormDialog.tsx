import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { AlertTriangleIcon, BellIcon, ShieldCheckIcon } from 'lucide-react'
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
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type { WebhookFailurePolicy, WebhookKind } from '@/graphql/schema.types'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { WebhookKindBadge } from './WebhookKindBadge'
import { DeliverySettingsFields, WildcardEditor } from './DeliverySettings'
import {
  headerRowsToObject,
  isWildcardEntry,
  objectToHeaderRows,
  type HeaderRow,
} from './deliverySettings.utils'
import {
  CreateWebhookDocument,
  UpdateWebhookDocument,
  WebhookEventTypesDocument,
  WebhooksDocument,
  type UpdateWebhookMutationVariables,
} from './Webhooks.gql'

export type WebhookFormData = {
  name: string
  targetUrl: string
  eventTypes: string[]
  kind: WebhookKind
}

export type ExistingWebhook = {
  id: string
  name: string
  targetUrl: string
  eventTypes: string[]
  kind: WebhookKind
  channel?: { id: string; slug: string; name: string } | null
  failurePolicy?: WebhookFailurePolicy | null
  timeoutMs?: number | null
  customHeaders?: unknown
}

const ALL_STORES_VALUE = '__all__'

export function WebhookFormDialog({
  trigger,
  existing,
  onCreated,
  onUpdated,
}: {
  trigger: ReactNode
  existing?: ExistingWebhook
  onCreated?: (data: { secret: string; name: string }) => void
  onUpdated?: () => void
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [targetUrl, setTargetUrl] = useState('')
  const [kind, setKind] = useState<WebhookKind>('ASYNC')
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])
  const [channelId, setChannelId] = useState<string>(ALL_STORES_VALUE)
  const [eventFilter, setEventFilter] = useState('')
  const [failurePolicy, setFailurePolicy] = useState<WebhookFailurePolicy>('FAIL_CLOSED')
  const [timeoutMs, setTimeoutMs] = useState('')
  const [headers, setHeaders] = useState<HeaderRow[]>([])
  const [error, setError] = useState<string | null>(null)

  const { data: eventTypesData, loading: loadingEvents } = useQuery(WebhookEventTypesDocument)
  const channelsQuery = useQuery(ChannelsDocument, { skip: !open })
  const channels = channelsQuery.data?.channels ?? []
  const [createWebhook, { loading: creating }] = useMutation(CreateWebhookDocument, {
    refetchQueries: [{ query: WebhooksDocument }],
  })
  const [updateWebhook, { loading: updating }] = useMutation(UpdateWebhookDocument)

  // Initialize state when dialog opens or existing changes - On create we
  // reset to ASYNC so the safer default wins for anyone cancelling midway;
  // on edit we pin the existing kind (it's read-only below) - Applied as a
  // state adjustment during render, keyed on the same inputs the old
  // effect watched
  const [prevHydrateKey, setPrevHydrateKey] = useState<{
    open: boolean
    existing?: ExistingWebhook
  }>({ open, existing })
  if (prevHydrateKey.open !== open || prevHydrateKey.existing !== existing) {
    setPrevHydrateKey({ open, existing })
    if (open) {
      setName(existing?.name ?? '')
      setTargetUrl(existing?.targetUrl ?? '')
      setKind(existing?.kind ?? 'ASYNC')
      setSelectedEvents(existing?.eventTypes ?? [])
      setChannelId(existing?.channel?.id ?? ALL_STORES_VALUE)
      setEventFilter('')
      setFailurePolicy(existing?.failurePolicy ?? 'FAIL_CLOSED')
      setTimeoutMs(existing?.timeoutMs ? String(existing.timeoutMs) : '')
      setHeaders(objectToHeaderRows(existing?.customHeaders))
      setError(null)
    }
  }

  // Only events of the current kind can be subscribed - the backend enforces this with a clear error, but we filter the
  // picker too so the admin sees the correct set upfront instead of relying on the server error
  const filteredEventTypes = useMemo(() => {
    return (eventTypesData?.webhookEventTypes ?? []).filter((et) => et.kind === kind)
  }, [eventTypesData, kind])

  // Group events by the domain prefix before the first dot so subscribers see
  // "customer.*" rows under a "customer" heading, etc.
  const groupedEventTypes = useMemo(() => {
    const groups = new Map<string, typeof filteredEventTypes>()
    for (const et of filteredEventTypes) {
      const domain = et.type.split('.')[0] || 'other'
      const list = groups.get(domain) ?? []
      list.push(et)
      groups.set(domain, list)
    }
    return Array.from(groups.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([domain, events]) => ({
        domain,
        events: events.slice().sort((a, b) => a.type.localeCompare(b.type)),
      }))
  }, [filteredEventTypes])

  // Narrow the grouped list by the free-text filter (matches the event type or its display name) - Empty groups drop
  // out, so the picker only shows domains with a hit - the way to make 100+ registered events navigable
  const visibleGroups = useMemo(() => {
    const q = eventFilter.trim().toLowerCase()
    if (!q) return groupedEventTypes
    return groupedEventTypes
      .map(({ domain, events }) => ({
        domain,
        events: events.filter(
          (et) =>
            et.type.toLowerCase().includes(q) || et.name.toLowerCase().includes(q),
        ),
      }))
      .filter((g) => g.events.length > 0)
  }, [groupedEventTypes, eventFilter])

  // Switching kind on create should drop any events that no longer belong to
  // the selected kind - (On edit, kind is read-only so this is a no-op.)
  function onKindChange(next: WebhookKind) {
    if (existing) return
    setKind(next)
    const allowed = new Set(
      (eventTypesData?.webhookEventTypes ?? [])
        .filter((et) => et.kind === next)
        .map((et) => et.type),
    )
    // wildcards are kind-agnostic patterns - they survive a kind switch
    setSelectedEvents((prev) => prev.filter((t) => isWildcardEntry(t) || allowed.has(t)))
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (selectedEvents.length === 0) {
      setError('Select at least one event type.')
      return
    }

    try {
      if (existing) {
        const input: UpdateWebhookMutationVariables['input'] = { id: existing.id }
        if (name !== existing.name) input.name = name
        if (targetUrl !== existing.targetUrl) input.targetUrl = targetUrl
        if (JSON.stringify(selectedEvents) !== JSON.stringify(existing.eventTypes)) {
          input.eventTypes = selectedEvents
        }
        if (failurePolicy !== (existing.failurePolicy ?? 'FAIL_CLOSED')) {
          input.failurePolicy = failurePolicy
        }
        const parsedTimeout = timeoutMs.trim() === '' ? 0 : Number(timeoutMs)
        if (parsedTimeout !== (existing.timeoutMs ?? 0)) input.timeoutMs = parsedTimeout
        const newHeaders = headerRowsToObject(headers) ?? {}
        const oldHeaders = headerRowsToObject(objectToHeaderRows(existing.customHeaders)) ?? {}
        if (JSON.stringify(newHeaders) !== JSON.stringify(oldHeaders)) {
          input.customHeaders = newHeaders // {} clears server-side
        }
        // NOTE: channel reassignment on update isn't supported by the API
        // (UpdateWebhookInput has no channelId, and the service/repo don't
        // change it) - a webhook's channel is fixed at creation - The picker
        // is disabled in edit mode below; sending channelId here was a no-op
        // that also broke the type check, so it's intentionally omitted

        if (Object.keys(input).length > 1) {
          await updateWebhook({ variables: { input } })
        }
        setOpen(false)
        onUpdated?.()
      } else {
        const { data } = await createWebhook({
          variables: {
            input: {
              name,
              targetUrl,
              kind,
              eventTypes: selectedEvents,
              channelId: channelId === ALL_STORES_VALUE ? undefined : channelId,
              failurePolicy,
              timeoutMs: timeoutMs.trim() === '' ? undefined : Number(timeoutMs),
              customHeaders: headerRowsToObject(headers),
            },
          },
        })
        setOpen(false)
        if (data) {
          onCreated?.({
            secret: data.createWebhook.rawSecret,
            name: data.createWebhook.webhook.name,
          })
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed.')
    }
  }

  function toggleEvent(type: string) {
    setSelectedEvents((prev) =>
      prev.includes(type) ? prev.filter((t) => t !== type) : [...prev, type],
    )
  }

  // Select or clear every event in a domain group at once
  function toggleGroup(types: string[], select: boolean) {
    setSelectedEvents((prev) => {
      if (select) return Array.from(new Set([...prev, ...types]))
      const drop = new Set(types)
      return prev.filter((t) => !drop.has(t))
    })
  }

  const isLoading = creating || updating

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-lg">
        <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{existing ? 'Edit Webhook' : 'Create Webhook'}</DialogTitle>
            <DialogDescription>
              {existing
                ? 'Update the configuration for this webhook.'
                : 'Configure a new endpoint to receive domain events.'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto py-1 pr-1">
            {/* Kind - choose on create, read-only on edit */}
            <div className="grid gap-2">
              <Label>Kind</Label>
              {existing ? (
                <div className="grid gap-1.5 rounded-md border bg-muted/40 p-3">
                  <div className="flex items-center gap-2">
                    <WebhookKindBadge kind={existing.kind} />
                    <span className="text-sm text-muted-foreground">
                      {existing.kind === 'SYNC'
                        ? 'Runs during user-facing requests and can abort them.'
                        : 'Runs after the event; failures do not affect the original request.'}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Kind is fixed at creation. Delete and recreate to change.
                  </p>
                </div>
              ) : (
                <div className="grid grid-cols-2 gap-2">
                  <KindOption
                    selected={kind === 'ASYNC'}
                    kind="ASYNC"
                    title="Async"
                    subtitle="Notify"
                    description="Delivered after the event. Retried on failure. Cannot block the operation."
                    icon={<BellIcon className="size-4" />}
                    onSelect={() => onKindChange('ASYNC')}
                  />
                  <KindOption
                    selected={kind === 'SYNC'}
                    kind="SYNC"
                    title="Sync"
                    subtitle="Block & validate"
                    description="Runs during the request. Can reject the operation. No retries."
                    icon={<ShieldCheckIcon className="size-4" />}
                    onSelect={() => onKindChange('SYNC')}
                  />
                </div>
              )}
              {!existing && kind === 'SYNC' && (
                <Alert variant="destructive" className="mt-1">
                  <AlertTriangleIcon className="size-4" />
                  <AlertTitle>Sync webhooks run during user-facing requests</AlertTitle>
                  <AlertDescription>
                    Keep endpoints fast (&lt;5s) and reliable - failures abort the operation and
                    are visible to users. No retries.
                  </AlertDescription>
                </Alert>
              )}
            </div>

            <div className="grid gap-2">
              <Label htmlFor="webhook-name">Name</Label>
              <Input
                id="webhook-name"
                placeholder="e.g. My Application Webhook"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="webhook-url">Target URL</Label>
              <Input
                id="webhook-url"
                type="url"
                placeholder="https://api.example.com/webhooks"
                value={targetUrl}
                onChange={(e) => setTargetUrl(e.target.value)}
                required
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="webhook-store">Channel scope</Label>
              <Select
                value={channelId}
                onValueChange={setChannelId}
                disabled={channelsQuery.loading || !!existing}
              >
                <SelectTrigger id="webhook-store">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_STORES_VALUE}>All stores (global)</SelectItem>
                  {channels.map((sv) => (
                    <SelectItem key={sv.id} value={sv.id}>
                      {sv.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Global subscribers receive events from every store. Scoped subscribers only fire
                for events on the selected store.
              </p>
            </div>

            <DeliverySettingsFields
              kind={kind}
              failurePolicy={failurePolicy}
              onFailurePolicyChange={setFailurePolicy}
              timeoutMs={timeoutMs}
              onTimeoutMsChange={setTimeoutMs}
              headers={headers}
              onHeadersChange={setHeaders}
            />

            <div className="grid gap-2">
              <div className="flex items-center justify-between gap-2">
                <Label>Event types</Label>
                <span className="text-xs text-muted-foreground">
                  {selectedEvents.length} selected
                </span>
              </div>
              {!loadingEvents && filteredEventTypes.length > 0 && (
                <Input
                  placeholder="Filter events…"
                  value={eventFilter}
                  onChange={(e) => setEventFilter(e.target.value)}
                  className="h-8"
                />
              )}
              <div className="grid gap-3 rounded-md border p-4">
                {loadingEvents ? (
                  <p className="text-sm text-muted-foreground">Loading events…</p>
                ) : filteredEventTypes.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No {kind.toLowerCase()} events are registered.
                  </p>
                ) : visibleGroups.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No events match “{eventFilter}”.
                  </p>
                ) : (
                  visibleGroups.map(({ domain, events }) => {
                    const allSelected = events.every((et) =>
                      selectedEvents.includes(et.type),
                    )
                    return (
                      <div key={domain} className="grid gap-2">
                        <div className="flex items-center justify-between gap-2">
                          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                            {domain}
                          </h4>
                          <button
                            type="button"
                            className="text-xs font-medium text-primary hover:underline"
                            onClick={() =>
                              toggleGroup(
                                events.map((et) => et.type),
                                !allSelected,
                              )
                            }
                          >
                            {allSelected ? 'Clear' : 'Select all'}
                          </button>
                        </div>
                        <div className="grid gap-3">
                          {events.map((et) => (
                            <div key={et.type} className="flex items-start gap-3">
                              <Checkbox
                                id={`event-${et.type}`}
                                checked={selectedEvents.includes(et.type)}
                                onCheckedChange={() => toggleEvent(et.type)}
                              />
                              <div className="grid gap-1.5 leading-none">
                                <Label
                                  htmlFor={`event-${et.type}`}
                                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                                >
                                  {et.name}
                                </Label>
                                <p className="line-clamp-2 whitespace-pre-line text-xs text-muted-foreground">
                                  {et.description}
                                </p>
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )
                  })
                )}
              </div>
              <WildcardEditor
                wildcards={selectedEvents.filter(isWildcardEntry)}
                onAdd={(p) => setSelectedEvents((prev) => [...prev, p])}
                onRemove={(p) => setSelectedEvents((prev) => prev.filter((t) => t !== p))}
              />
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading || loadingEvents}>
              {isLoading ? 'Saving…' : existing ? 'Save changes' : 'Create webhook'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function KindOption({
  selected,
  title,
  subtitle,
  description,
  icon,
  kind,
  onSelect,
}: {
  selected: boolean
  title: string
  subtitle: string
  description: string
  icon: ReactNode
  kind: WebhookKind
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'grid gap-1 rounded-md border p-3 text-left transition',
        selected
          ? kind === 'SYNC'
            ? 'border-destructive/50 bg-destructive/5 ring-1 ring-destructive/30'
            : 'border-blue-500/50 bg-blue-500/5 ring-1 ring-blue-500/30'
          : 'hover:bg-muted/40',
      )}
    >
      <div className="flex items-center gap-2">
        {icon}
        <span className="text-sm font-semibold">{title}</span>
        <span className="text-xs text-muted-foreground">({subtitle})</span>
      </div>
      <p className="text-xs text-muted-foreground">{description}</p>
    </button>
  )
}
