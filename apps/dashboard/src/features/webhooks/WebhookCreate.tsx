import { useMemo, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  AlertTriangleIcon,
  BellIcon,
  ShieldCheckIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import type { WebhookFailurePolicy, WebhookKind } from '@/graphql/schema.types'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { NewSecretDialog } from './NewSecretDialog'
import { DeliverySettingsFields, WildcardEditor } from './DeliverySettings'
import {
  headerRowsToObject,
  isWildcardEntry,
  type HeaderRow,
} from './deliverySettings.utils'
import {
  CreateWebhookDocument,
  WebhookEventTypesDocument,
  WebhooksDocument,
} from './Webhooks.gql'

const ALL_STORES_VALUE = '__all__'

// WebhookCreate is a standalone PDP-style page reached from /webhooks/new Header carries the "Back to webhooks"
// affordance and the Create button (form submission is bound via the `form` attribute on the header button)
export function WebhookCreate() {
  const navigate = useNavigate()

  const draft = useDraft({
    name: '',
    targetUrl: '',
    kind: 'ASYNC' as WebhookKind,
    selectedEvents: [] as string[],
    channelId: ALL_STORES_VALUE as string,
    failurePolicy: 'FAIL_CLOSED' as WebhookFailurePolicy,
    timeoutMs: '',
    headers: [] as HeaderRow[],
  }, 'New webhook')
  const { name, targetUrl, kind, selectedEvents, channelId, failurePolicy, timeoutMs, headers } = draft.values
  const { setName, setTargetUrl, setKind, setSelectedEvents, setChannelId, setFailurePolicy, setTimeoutMs, setHeaders } = draft.setters
  const [error, setError] = useState<string | null>(null)
  const [createdSecret, setCreatedSecret] = useState<{ secret: string; name: string } | null>(null)

  const { data: eventTypesData, loading: loadingEvents } = useQuery(WebhookEventTypesDocument)
  const channelsQuery = useQuery(ChannelsDocument)
  const channels = channelsQuery.data?.channels ?? []
  const [createWebhook, { loading: creating }] = useMutation(CreateWebhookDocument, {
    refetchQueries: [{ query: WebhooksDocument }],
  })

  const eventsByKind = useMemo(() => {
    const all = eventTypesData?.webhookEventTypes ?? []
    return {
      ASYNC: all.filter((et) => et.kind === 'ASYNC'),
      SYNC: all.filter((et) => et.kind === 'SYNC'),
    }
  }, [eventTypesData])

  const filteredEventTypes = eventsByKind[kind]
  const totalEventCount = (eventTypesData?.webhookEventTypes ?? []).length

  // Group events by the domain prefix before the first dot so subscribers see "customer.*" rows under a "customer"
  // heading, "order.*" under "order", etc - Domains and events are sorted alphabetically
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

  // When the kind (or the loaded event-type list) changes, drop any selected events that don't belong to it (state
  // adjustment during render, keyed on the kind and its event list)
  const [prevEventFilter, setPrevEventFilter] = useState({ kind, eventsByKind })
  if (prevEventFilter.kind !== kind || prevEventFilter.eventsByKind !== eventsByKind) {
    setPrevEventFilter({ kind, eventsByKind })
    const allowed = new Set(eventsByKind[kind].map((et) => et.type))
    // wildcards are kind-agnostic patterns - they survive a kind switch
    setSelectedEvents((prev) => prev.filter((t) => isWildcardEntry(t) || allowed.has(t)))
  }

  function toggleEvent(type: string) {
    setSelectedEvents((prev) =>
      prev.includes(type) ? prev.filter((t) => t !== type) : [...prev, type],
    )
  }

  function selectAllEvents() {
    setSelectedEvents(filteredEventTypes.map((et) => et.type))
  }

  function clearEvents() {
    setSelectedEvents([])
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (selectedEvents.length === 0) {
      setError('Select at least one event type.')
      return
    }
    try {
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
      if (data) {
        setCreatedSecret({
          secret: data.createWebhook.rawSecret,
          name: data.createWebhook.webhook.name,
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed.')
    }
  }

  function onSecretClose() {
    setCreatedSecret(null)
    // The webhook exists already - this is just dismissing its one-time secret
    void duringCommit(() => navigate('/webhooks'))
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/webhooks"
        backLabel="Back to webhooks"
        title="New webhook"
        meta={
          <span>
            Configure an endpoint to receive domain events. Skelet currently registers{' '}
            <span className="font-medium text-foreground">{totalEventCount}</span>{' '}
            event{totalEventCount === 1 ? '' : 's'} across customer and order domains.
          </span>
        }
        actions={
          <Button
            type="submit"
            form="webhook-create-form"
            size="sm"
            disabled={creating || loadingEvents}
          >
            {creating ? 'Creating…' : 'Create webhook'}
          </Button>
        }
      />

      <form id="webhook-create-form" onSubmit={onSubmit}>
        <DetailPageBody>
        {error && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Couldn&apos;t create webhook</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Delivery mode"
          description="Async webhooks fire after an event commits. Sync webhooks run during the request and can reject or enrich the operation."
        >
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <KindOption
              selected={kind === 'ASYNC'}
              kind="ASYNC"
              title="Async"
              subtitle="Notify"
              description="Delivered after the event. Retried on failure. Cannot block the operation."
              icon={<BellIcon className="size-4" />}
              onSelect={() => setKind('ASYNC')}
            />
            <KindOption
              selected={kind === 'SYNC'}
              kind="SYNC"
              title="Sync"
              subtitle="Block & validate"
              description="Runs during the request. Can reject the operation. No retries."
              icon={<ShieldCheckIcon className="size-4" />}
              onSelect={() => setKind('SYNC')}
            />
          </div>
          {kind === 'SYNC' && (
            <Alert variant="destructive">
              <AlertTriangleIcon className="size-4" />
              <AlertTitle>Sync webhooks run during user-facing requests</AlertTitle>
              <AlertDescription>
                Keep endpoints fast (&lt;5s) and reliable - failures abort the operation and are
                visible to users. No retries.
              </AlertDescription>
            </Alert>
          )}
        </DetailPageSection>

        <DetailPageSection
          title="Endpoint"
          description="Where Skelet sends the signed payload."
        >
          <div className="grid gap-4 md:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="webhook-name">Name</Label>
              <Input
                id="webhook-name"
                placeholder="My application webhook"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="webhook-store">Channel scope</Label>
              <Select
                value={channelId}
                onValueChange={setChannelId}
                disabled={channelsQuery.loading}
              >
                <SelectTrigger className="w-full" id="webhook-store">
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
            </div>
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
              className="font-mono"
            />
            <p className="text-xs text-muted-foreground">
              Skelet POSTs the JSON payload here. Each delivery is signed with an HMAC-SHA256
              header so you can verify authenticity.
            </p>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Delivery settings"
          description="Failure handling, timeout and extra request headers for this endpoint."
        >
          <DeliverySettingsFields
            kind={kind}
            failurePolicy={failurePolicy}
            onFailurePolicyChange={setFailurePolicy}
            timeoutMs={timeoutMs}
            onTimeoutMsChange={setTimeoutMs}
            headers={headers}
            onHeadersChange={setHeaders}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Event types"
          description={
            <>
              Pick which events trigger this webhook. Only{' '}
              <span className="font-medium text-foreground">{kind.toLowerCase()}</span> events
              are listed below.
            </>
          }
          trailing={
            filteredEventTypes.length > 0 ? (
              <div className="flex items-center gap-1">
                <Button type="button" variant="ghost" size="sm" onClick={selectAllEvents}>
                  Select all
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={clearEvents}>
                  Clear
                </Button>
              </div>
            ) : undefined
          }
        >
          {loadingEvents ? (
            <p className="text-sm text-muted-foreground">Loading events…</p>
          ) : filteredEventTypes.length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              No {kind.toLowerCase()} events are registered.
            </div>
          ) : (
            <div className="grid gap-5">
              {groupedEventTypes.map(({ domain, events }) => (
                <div key={domain} className="grid gap-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {domain}
                  </h3>
                  <ul className="grid gap-2">
                    {events.map((et) => {
                      const checked = selectedEvents.includes(et.type)
                      return (
                        <li
                          key={et.type}
                          className={cn(
                            'flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors',
                            checked
                              ? 'border-primary bg-primary/5'
                              : 'hover:bg-muted/50',
                          )}
                          onClick={() => toggleEvent(et.type)}
                        >
                          <Checkbox
                            id={`event-${et.type}`}
                            checked={checked}
                            onCheckedChange={() => toggleEvent(et.type)}
                            onClick={(e) => e.stopPropagation()}
                            className="mt-0.5"
                          />
                          <div className="grid min-w-0 gap-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <Label
                                htmlFor={`event-${et.type}`}
                                className="cursor-pointer text-sm font-medium leading-none"
                              >
                                {et.name}
                              </Label>
                              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                                {et.type}
                              </code>
                            </div>
                            <p className="whitespace-pre-line text-xs text-muted-foreground">
                              {et.description}
                            </p>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                </div>
              ))}
            </div>
          )}
          <WildcardEditor
            wildcards={selectedEvents.filter(isWildcardEntry)}
            onAdd={(p) => setSelectedEvents((prev) => [...prev, p])}
            onRemove={(p) => setSelectedEvents((prev) => prev.filter((t) => t !== p))}
          />
          {selectedEvents.length > 0 && (
            <p className="text-xs text-muted-foreground">
              {selectedEvents.length} event{selectedEvents.length === 1 ? '' : 's'} selected.
            </p>
          )}
        </DetailPageSection>
        </DetailPageBody>
      </form>

      <NewSecretDialog
        secret={createdSecret?.secret ?? null}
        webhookName={createdSecret?.name ?? null}
        open={!!createdSecret}
        onClose={onSecretClose}
      />
    </DetailPageLayout>
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
            : 'border-primary/50 bg-primary/5 ring-1 ring-primary/30'
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
