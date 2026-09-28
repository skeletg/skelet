import type { ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { FieldError } from '@/components/form/fields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Label } from '@/components/ui/label'
import { WebhookKindBadge } from './WebhookKindBadge'
import { WebhookDeliverySettings, WebhookEventPicker } from './WebhookPickers'
import { UpdateWebhookDocument, WebhookEventTypesDocument } from './Webhooks.gql'
import { toWebhookUpdateInput, toWebhookValues, webhookSchema, type ExistingWebhook } from './schemas/webhook'

export type { ExistingWebhook }

// Edits an existing webhook - creating one is the /webhooks/new page. Kind and channel scope are
// fixed at creation, so they're shown, not edited
export function WebhookFormDialog({
  trigger,
  existing,
  onUpdated,
}: {
  trigger: ReactNode
  existing: ExistingWebhook
  onUpdated?: () => void
}) {
  return (
    <FormDialog trigger={trigger} className="flex max-h-[90vh] flex-col sm:max-w-lg">
      {(close) => <WebhookEditForm existing={existing} onUpdated={onUpdated} onClose={close} />}
    </FormDialog>
  )
}

function WebhookEditForm({
  existing,
  onUpdated,
  onClose,
}: {
  existing: ExistingWebhook
  onUpdated?: () => void
  onClose: () => void
}) {
  const { data: eventTypesData, loading: loadingEvents } = useQuery(WebhookEventTypesDocument)
  const [updateWebhook] = useMutation(UpdateWebhookDocument)

  const dialog = useDialogForm({
    baseline: toWebhookValues(existing),
    schema: webhookSchema,
    submit: async (v) => {
      const input = toWebhookUpdateInput(v, existing)
      if (input) await updateWebhook({ variables: { input } })
    },
    onSaved: () => {
      onClose()
      onUpdated?.()
    },
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      scrollable
      title="Edit webhook"
      description="Update the configuration for this webhook."
      submitLabel="Save changes"
      onCancel={onClose}
    >
      <div className="grid gap-2">
        <Label>Kind</Label>
        <div className="grid gap-1.5 rounded-md border bg-muted/40 p-3">
          <div className="flex items-center gap-2">
            <WebhookKindBadge kind={existing.kind} />
            <span className="text-sm text-muted-foreground">
              {existing.kind === 'SYNC'
                ? 'Runs during user-facing requests and can abort them.'
                : 'Runs after the event; failures do not affect the original request.'}
            </span>
          </div>
          <p className="text-xs text-muted-foreground">Kind is fixed at creation. Delete and recreate to change.</p>
        </div>
      </div>

      <form.AppField name="name">
        {(f) => <f.TextField id="webhook-name" label="Name" placeholder="e.g. My Application Webhook" />}
      </form.AppField>
      <form.AppField name="targetUrl">
        {(f) => <f.TextField id="webhook-url" label="Target URL" type="url" placeholder="https://api.example.com/webhooks" />}
      </form.AppField>

      <div className="grid gap-2">
        <Label>Channel scope</Label>
        <p className="rounded-md border bg-muted/40 px-3 py-2 text-sm">
          {existing.channel ? existing.channel.name : 'All stores (global)'}
          <span className="block text-xs text-muted-foreground">Fixed at creation.</span>
        </p>
      </div>

      <WebhookDeliverySettings form={form} />

      <form.Field name="eventTypes">
        {(f) => (
          <WebhookEventPicker
            events={eventTypesData?.webhookEventTypes ?? []}
            kind={existing.kind}
            loading={loadingEvents}
            selected={f.state.value}
            onChange={f.handleChange}
            error={<FieldError field={f} />}
          />
        )}
      </form.Field>
    </FormDialogBody>
  )
}
