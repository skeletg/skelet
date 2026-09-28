import { useState } from 'react'
import { PlusIcon, ShieldAlertIcon, ShieldCheckIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import type { WebhookFailurePolicy, WebhookKind } from '@/graphql/schema.types'
import { isWildcardEntry, type HeaderRow } from './deliverySettings.utils'

// DeliverySettingsFields - the webhook contract knobs shared by the create page and the edit dialog: failure policy
// (sync only), per-attempt timeout and custom request headers
export function DeliverySettingsFields({
  kind,
  failurePolicy,
  onFailurePolicyChange,
  timeoutMs,
  onTimeoutMsChange,
  headers,
  onHeadersChange,
}: {
  kind: WebhookKind
  failurePolicy: WebhookFailurePolicy
  onFailurePolicyChange: (p: WebhookFailurePolicy) => void
  timeoutMs: string
  onTimeoutMsChange: (v: string) => void
  headers: HeaderRow[]
  onHeadersChange: (rows: HeaderRow[]) => void
}) {
  return (
    <div className="grid gap-4">
      {kind === 'SYNC' && (
        <div className="grid gap-2">
          <Label>Failure policy</Label>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <PolicyOption
              selected={failurePolicy === 'FAIL_CLOSED'}
              title="Fail closed"
              subtitle="gate"
              description="A failed delivery aborts the operation. Use when the endpoint must approve every event."
              icon={<ShieldCheckIcon className="size-4" />}
              onSelect={() => onFailurePolicyChange('FAIL_CLOSED')}
            />
            <PolicyOption
              selected={failurePolicy === 'FAIL_OPEN'}
              title="Fail open"
              subtitle="advisory"
              description="Failures are logged and the operation continues. An explicit reject response still blocks it."
              icon={<ShieldAlertIcon className="size-4" />}
              onSelect={() => onFailurePolicyChange('FAIL_OPEN')}
            />
          </div>
        </div>
      )}

      <div className="grid gap-2">
        <Label htmlFor="webhook-timeout">Timeout (ms)</Label>
        <Input
          id="webhook-timeout"
          type="number"
          min={0}
          max={30000}
          step={100}
          placeholder={kind === 'SYNC' ? 'Default: 5000' : 'Default: 10000'}
          value={timeoutMs}
          onChange={(e) => onTimeoutMsChange(e.target.value)}
          className="w-40"
        />
        <p className="text-xs text-muted-foreground">
          Per-attempt HTTP timeout. Leave empty for the default. Max 30000.
          {kind === 'SYNC' && ' Sync webhooks block a user-facing request - keep this low.'}
        </p>
      </div>

      <div className="grid gap-2">
        <Label>Custom headers</Label>
        {headers.length > 0 && (
          <div className="grid gap-2">
            {headers.map((row, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  placeholder="Header name (e.g. Authorization)"
                  value={row.key}
                  onChange={(e) =>
                    onHeadersChange(headers.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))
                  }
                  className="h-8 font-mono text-xs"
                />
                <Input
                  placeholder="Value"
                  value={row.value}
                  onChange={(e) =>
                    onHeadersChange(headers.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))
                  }
                  className="h-8 font-mono text-xs"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8 shrink-0"
                  onClick={() => onHeadersChange(headers.filter((_, j) => j !== i))}
                >
                  <XIcon className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-fit"
          onClick={() => onHeadersChange([...headers, { key: '', value: '' }])}
        >
          <PlusIcon className="size-4" />
          Add header
        </Button>
        <p className="text-xs text-muted-foreground">
          Sent with every delivery - typically an Authorization token for your receiver. Reserved
          names (Content-Type, X-Webhook-*) are not allowed.
        </p>
      </div>
    </div>
  )
}

function PolicyOption({
  selected,
  title,
  subtitle,
  description,
  icon,
  onSelect,
}: {
  selected: boolean
  title: string
  subtitle: string
  description: string
  icon: React.ReactNode
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'grid gap-1 rounded-md border p-3 text-left transition',
        selected ? 'border-primary/50 bg-primary/5 ring-1 ring-primary/30' : 'hover:bg-muted/40',
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

// WildcardEditor manages the pattern entries ("order.*", "*") living alongside exact types in the same eventTypes list
// - Wildcards cover future events too - a webhook on "order.*" picks up new order events the day they ship
export function WildcardEditor({
  wildcards,
  onAdd,
  onRemove,
}: {
  wildcards: string[]
  onAdd: (pattern: string) => void
  onRemove: (pattern: string) => void
}) {
  const [draft, setDraft] = useState('')

  function add() {
    const pattern = draft.trim()
    if (!pattern || !isWildcardEntry(pattern) || wildcards.includes(pattern)) return
    onAdd(pattern)
    setDraft('')
  }

  return (
    <div className="grid gap-2">
      <div className="flex items-center gap-2">
        <Input
          placeholder="Wildcard pattern, e.g. order.* or *"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
          className="h-8 font-mono text-xs"
        />
        <Button
          className="h-10"
          type="button"
          variant="outline"
          size="sm"
          onClick={add}
          disabled={!isWildcardEntry(draft.trim())}
        >
          Add
        </Button>
      </div>
      {wildcards.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {wildcards.map((w) => (
            <Badge key={w} variant="secondary" className="gap-1 font-mono text-[11px]">
              {w}
              <button type="button" onClick={() => onRemove(w)} aria-label={`Remove ${w}`}>
                <XIcon className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
      <p className="text-xs text-muted-foreground">
        Wildcards subscribe to every current and future event of this webhook's kind under the
        prefix - "order.*" covers new order events automatically.
      </p>
    </div>
  )
}
