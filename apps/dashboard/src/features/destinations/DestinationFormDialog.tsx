import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { DESTINATION_KINDS } from './kinds'
import { CreateEventDestinationDocument, UpdateEventDestinationDocument } from './Destinations.gql'
import {
  EVENT_SOURCES,
  destinationSchema,
  toDestinationInput,
  toDestinationValues,
  type ExistingDestination,
} from './schemas/destination'

export type { ExistingDestination }

const URL_HINTS: Record<string, string> = {
  webhook:
    'Receives POST batches signed with the secret (X-Destination-Signature over "<ts>.<body>", timestamp in X-Destination-Timestamp).',
  ga4_mp:
    'The Measurement Protocol collect URL with credentials as query params, e.g. https://www.google-analytics.com/mp/collect?measurement_id=G-XXXX&api_secret=…',
  meta_capi:
    'The pixel events URL with the token as a query param, e.g. https://graph.facebook.com/v21.0/<pixel-id>/events?access_token=…',
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string
  existing?: ExistingDestination | null
  onSaved: () => void
}

export function DestinationFormDialog({ open, onOpenChange, ...props }: Props) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="flex max-h-[90vh] flex-col sm:max-w-lg">
      {(close) => <DestinationForm {...props} onClose={close} />}
    </FormDialog>
  )
}

function DestinationForm({ channelId, existing, onSaved, onClose }: Omit<Props, 'open' | 'onOpenChange'> & { onClose: () => void }) {
  const [create] = useMutation(CreateEventDestinationDocument)
  const [update] = useMutation(UpdateEventDestinationDocument)

  const dialog = useDialogForm({
    baseline: toDestinationValues(existing),
    schema: destinationSchema({ isEdit: !!existing }),
    submit: async (v) => {
      const input = toDestinationInput(v, channelId)
      if (existing) await update({ variables: { id: existing.id, input } })
      else await create({ variables: { input } })
    },
    onSaved: () => {
      onClose()
      onSaved()
    },
  })
  const { form } = dialog
  const kind = useStore(form.store, (s) => s.values.kind)

  return (
    <FormDialogBody
      dialog={dialog}
      scrollable
      title={existing ? 'Edit destination' : 'New destination'}
      description={
        existing
          ? 'Update where and which analytics events are forwarded.'
          : 'Forward analytics events to an external system. Delivery starts from creation - history is not replayed.'
      }
      submitLabel={existing ? 'Save changes' : 'Create destination'}
      onCancel={onClose}
    >
      <form.Field name="kind">
        {(f) => (
          <div className="grid gap-2">
            <Label>Kind</Label>
            <div className="grid grid-cols-3 gap-2">
              {DESTINATION_KINDS.map((k) => (
                <button
                  key={k.value}
                  type="button"
                  onClick={() => f.handleChange(k.value)}
                  aria-pressed={f.state.value === k.value}
                  className={cn(
                    'grid gap-1 rounded-md border p-3 text-left transition',
                    f.state.value === k.value ? 'border-primary/50 bg-primary/5 ring-1 ring-primary/30' : 'hover:bg-muted/40',
                  )}
                >
                  <span className="text-sm font-semibold">{k.label}</span>
                  <span className="text-xs text-muted-foreground">{k.description}</span>
                </button>
              ))}
            </div>
          </div>
        )}
      </form.Field>

      <form.AppField name="name">
        {(f) => <f.TextField id="destination-name" label="Name" placeholder="e.g. Production GA4" />}
      </form.AppField>

      <form.AppField name="targetUrl">
        {(f) => <f.TextField id="destination-url" label="Target URL" type="url" placeholder="https://…" hint={URL_HINTS[kind]} />}
      </form.AppField>

      {kind === 'webhook' && (
        <form.AppField name="secret">
          {(f) => (
            <f.TextField
              id="destination-secret"
              label="Signing secret"
              placeholder={existing ? 'Leave blank to keep the current secret' : 'HMAC key'}
              hint="Write-only - it is never shown again after saving."
            />
          )}
        </form.AppField>
      )}

      <form.Field name="eventSources">
        {(f) => (
          <div className="grid gap-2">
            <Label>Event sources</Label>
            <div className="flex flex-wrap gap-4 rounded-md border p-3">
              {EVENT_SOURCES.map((source) => (
                <div key={source} className="flex items-center gap-2">
                  <Checkbox
                    id={`source-${source}`}
                    checked={f.state.value.includes(source)}
                    onCheckedChange={() =>
                      f.handleChange(
                        f.state.value.includes(source) ? f.state.value.filter((s) => s !== source) : [...f.state.value, source],
                      )
                    }
                  />
                  <Label htmlFor={`source-${source}`} className="text-sm font-normal">
                    {source}
                  </Label>
                </div>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">Leave everything unchecked to forward all sources.</p>
          </div>
        )}
      </form.Field>

      <form.AppField name="eventNames">
        {(f) => (
          <f.TextField
            id="destination-events"
            label="Event names"
            placeholder="order_placed, page_view (blank = all events)"
            hint="Comma-separated exact event names. Blank forwards every event."
          />
        )}
      </form.AppField>

      <form.Field name="isActive">
        {(f) => (
          <div className="flex items-center gap-3">
            <Switch id="destination-active" checked={f.state.value} onCheckedChange={f.handleChange} />
            <Label htmlFor="destination-active" className="font-normal">
              Active - included in the every-minute forwarding sweep
            </Label>
          </div>
        )}
      </form.Field>
    </FormDialogBody>
  )
}
