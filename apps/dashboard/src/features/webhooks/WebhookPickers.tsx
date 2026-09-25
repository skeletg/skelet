import { useMemo, useState, type ReactNode } from 'react'
import { AlertTriangleIcon, BellIcon, ShieldCheckIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import type { WebhookKind } from '@/graphql/schema.types'
import { useStore, type AnyFormApi } from '@tanstack/react-form'
import { DeliverySettingsFields, WildcardEditor } from './DeliverySettings'
import type { WebhookValues } from './schemas/webhook'
import { isWildcardEntry } from './deliverySettings.utils'

// The two webhook pickers the create page and the edit dialog share - delivery kind and event types

export type WebhookEventType = { type: string; name: string; description: string; kind: string }

export function WebhookKindPicker({ value, onChange }: { value: WebhookKind; onChange: (kind: WebhookKind) => void }) {
  return (
    <div className="grid gap-2">
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <KindOption
          selected={value === 'ASYNC'}
          kind="ASYNC"
          title="Async"
          subtitle="Notify"
          description="Delivered after the event. Retried on failure. Cannot block the operation."
          icon={<BellIcon className="size-4" />}
          onSelect={() => onChange('ASYNC')}
        />
        <KindOption
          selected={value === 'SYNC'}
          kind="SYNC"
          title="Sync"
          subtitle="Block & validate"
          description="Runs during the request. Can reject the operation. No retries."
          icon={<ShieldCheckIcon className="size-4" />}
          onSelect={() => onChange('SYNC')}
        />
      </div>
      {value === 'SYNC' && (
        <Alert variant="destructive">
          <AlertTriangleIcon className="size-4" />
          <AlertTitle>Sync webhooks run during user-facing requests</AlertTitle>
          <AlertDescription>
            Keep endpoints fast (&lt;5s) and reliable - failures abort the operation and are visible to users. No retries.
          </AlertDescription>
        </Alert>
      )}
    </div>
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
            : 'border-info/50 bg-info-soft ring-1 ring-info/30'
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

// Events of one kind, grouped by the domain before the first dot ("customer.*" under "customer"),
// with a free-text filter - what makes 100+ registered events navigable - and wildcard patterns
export function WebhookEventPicker({
  events,
  kind,
  loading,
  selected,
  onChange,
  error,
}: {
  events: WebhookEventType[]
  kind: WebhookKind
  loading: boolean
  selected: string[]
  onChange: (next: string[]) => void
  error?: ReactNode
}) {
  const [filter, setFilter] = useState('')

  const groups = useMemo(() => {
    const byDomain = new Map<string, WebhookEventType[]>()
    const q = filter.trim().toLowerCase()
    for (const et of events) {
      if (et.kind !== kind) continue
      if (q && !et.type.toLowerCase().includes(q) && !et.name.toLowerCase().includes(q)) continue
      const domain = et.type.split('.')[0] || 'other'
      byDomain.set(domain, [...(byDomain.get(domain) ?? []), et])
    }
    return [...byDomain.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([domain, list]) => ({ domain, events: list.sort((a, b) => a.type.localeCompare(b.type)) }))
  }, [events, kind, filter])
  const ofKind = events.some((et) => et.kind === kind)

  const toggle = (type: string) => onChange(selected.includes(type) ? selected.filter((t) => t !== type) : [...selected, type])
  const toggleGroup = (types: string[], select: boolean) =>
    onChange(select ? [...new Set([...selected, ...types])] : selected.filter((t) => !types.includes(t)))

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between gap-2">
        <Label>Event types</Label>
        <span className="text-xs text-muted-foreground">{selected.length} selected</span>
      </div>
      {!loading && ofKind && (
        <Input aria-label="Filter events" placeholder="Filter events…" value={filter} onChange={(e) => setFilter(e.target.value)} className="h-8" />
      )}
      <div className="grid gap-3 rounded-md border p-4">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading events…</p>
        ) : !ofKind ? (
          <p className="text-sm text-muted-foreground">No {kind.toLowerCase()} events are registered.</p>
        ) : groups.length === 0 ? (
          <p className="text-sm text-muted-foreground">No events match “{filter}”.</p>
        ) : (
          groups.map(({ domain, events: list }) => {
            const allSelected = list.every((et) => selected.includes(et.type))
            return (
              <div key={domain} className="grid gap-2">
                <div className="flex items-center justify-between gap-2">
                  <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{domain}</h4>
                  <button
                    type="button"
                    className="text-xs font-medium underline-offset-2 hover:underline"
                    onClick={() => toggleGroup(list.map((et) => et.type), !allSelected)}
                  >
                    {allSelected ? 'Clear' : 'Select all'}
                  </button>
                </div>
                <div className="grid gap-3">
                  {list.map((et) => (
                    <div key={et.type} className="flex items-start gap-3">
                      <Checkbox id={`event-${et.type}`} checked={selected.includes(et.type)} onCheckedChange={() => toggle(et.type)} />
                      <div className="grid gap-1.5 leading-none">
                        <div className="flex flex-wrap items-center gap-2">
                          <Label htmlFor={`event-${et.type}`} className="text-sm font-medium leading-none">
                            {et.name}
                          </Label>
                          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">{et.type}</code>
                        </div>
                        <p className="line-clamp-2 whitespace-pre-line text-xs text-muted-foreground">{et.description}</p>
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
        wildcards={selected.filter(isWildcardEntry)}
        onAdd={(p) => onChange([...selected, p])}
        onRemove={(p) => onChange(selected.filter((t) => t !== p))}
      />
      {error}
    </div>
  )
}

// DeliverySettingsFields edits failure policy, timeout and headers together - bound here to any form
// holding webhook values (the create page's record form, the edit dialog's)
export function WebhookDeliverySettings({ form }: { form: AnyFormApi }) {
  const values = useStore(form.store, (s) => s.values as WebhookValues)
  // DeliverySettingsFields owns the timeout input, so its schema error is read off the field meta here
  const timeoutMeta = useStore(form.store, (s) => s.fieldMeta.timeoutMs as { isTouched?: boolean; errors?: { message?: string }[] } | undefined)
  const timeoutError = timeoutMeta?.isTouched ? timeoutMeta.errors?.[0]?.message : undefined
  return (
    <div className="grid gap-2">
      <DeliverySettingsFields
        kind={values.kind}
        failurePolicy={values.failurePolicy}
        onFailurePolicyChange={(v) => form.setFieldValue('failurePolicy', v)}
        timeoutMs={values.timeoutMs}
        onTimeoutMsChange={(v) => form.setFieldValue('timeoutMs', v)}
        headers={values.headers}
        onHeadersChange={(v) => form.setFieldValue('headers', v)}
      />
      {timeoutError && <p className="text-xs text-destructive">{timeoutError}</p>}
    </div>
  )
}
