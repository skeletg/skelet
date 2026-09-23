import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { FieldError } from '@/components/form/fields'
import { duringCommit } from '@/lib/nav-guard'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import type { WebhookKind } from '@/graphql/schema.types'
import { NewSecretDialog } from './NewSecretDialog'
import { WebhookDeliverySettings, WebhookEventPicker, WebhookKindPicker } from './WebhookPickers'
import { CreateWebhookDocument, WebhookEventTypesDocument, WebhooksDocument } from './Webhooks.gql'
import {
  WEBHOOK_LABELS,
  keepEventsOfKind,
  toWebhookCreateInput,
  toWebhookValues,
  webhookDiff,
  webhookSchema,
} from './schemas/webhook'

// The standalone create page at /webhooks/new - once created, the one-time signing secret is shown
// and dismissing it returns to the list
export function WebhookCreate() {
  const navigate = useNavigate()
  const [createdSecret, setCreatedSecret] = useState<{ secret: string; name: string } | null>(null)

  const { data: eventTypesData, loading: loadingEvents } = useQuery(WebhookEventTypesDocument)
  const events = useMemo(() => eventTypesData?.webhookEventTypes ?? [], [eventTypesData])
  const channelsQuery = useQuery(ChannelsDocument)
  const channels = channelsQuery.data?.channels ?? []
  const [createWebhook] = useMutation(CreateWebhookDocument, { refetchQueries: [{ query: WebhooksDocument }] })

  const baseline = useMemo(() => toWebhookValues(), [])
  const record = useRecordForm({
    id: 'webhook',
    title: 'Webhook',
    label: 'New webhook',
    isEdit: false,
    baseline,
    schema: webhookSchema,
    diff: webhookDiff,
    // Custom headers usually carry auth tokens - they never go into the stored draft
    persist: { routeKey: 'webhook:new', exclude: ['headers'] },
    submit: async (v) => {
      const { data } = await createWebhook({ variables: { input: toWebhookCreateInput(v) } })
      if (data) setCreatedSecret({ secret: data.createWebhook.rawSecret, name: data.createWebhook.webhook.name })
    },
  })
  const { form } = record
  const kind = useStore(form.store, (s) => s.values.kind)

  function onKindChange(next: WebhookKind) {
    form.setFieldValue('kind', next)
    const allowed = new Set(events.filter((et) => et.kind === next).map((et) => et.type))
    form.setFieldValue('eventTypes', keepEventsOfKind(form.state.values.eventTypes, allowed))
  }

  return (
    <>
      <RecordFormPage
        record={record}
        backTo="/webhooks"
        backLabel="Back to webhooks"
        title="New webhook"
        meta={
          <span>
            Configure an endpoint to receive domain events. Skelet currently registers{' '}
            <span className="font-medium text-foreground">{events.length}</span> event{events.length === 1 ? '' : 's'}.
          </span>
        }
        createLabel="Create webhook"
        labels={WEBHOOK_LABELS}
      >
        <DetailPageSection
          title="Delivery mode"
          description="Async webhooks fire after an event commits. Sync webhooks run during the request and can reject or enrich the operation."
        >
          <WebhookKindPicker value={kind} onChange={onKindChange} />
        </DetailPageSection>

        <DetailPageSection title="Endpoint" description="Where Skelet sends the signed payload.">
          <div className="grid items-start gap-4 md:grid-cols-2">
            <form.AppField name="name">
              {(f) => <f.TextField id="webhook-name" label="Name" placeholder="My application webhook" />}
            </form.AppField>
            <form.AppField name="channelId">
              {(f) => (
                <f.SelectField
                  id="webhook-store"
                  label="Channel scope"
                  noneLabel="All stores (global)"
                  disabled={channelsQuery.loading}
                  options={channels.map((c) => ({ value: c.id, label: c.name }))}
                />
              )}
            </form.AppField>
          </div>
          <form.AppField name="targetUrl">
            {(f) => (
              <f.TextField
                id="webhook-url"
                label="Target URL"
                type="url"
                placeholder="https://api.example.com/webhooks"
                className="font-mono"
                hint="Skelet POSTs the JSON payload here. Each delivery is signed with an HMAC-SHA256 header so you can verify authenticity."
              />
            )}
          </form.AppField>
        </DetailPageSection>

        <DetailPageSection
          title="Delivery settings"
          description="Failure handling, timeout and extra request headers for this endpoint."
        >
          <WebhookDeliverySettings form={form} />
        </DetailPageSection>

        <DetailPageSection
          title="Event types"
          description={
            <>
              Pick which events trigger this webhook. Only <span className="font-medium text-foreground">{kind.toLowerCase()}</span>{' '}
              events are listed below.
            </>
          }
        >
          <form.Field name="eventTypes">
            {(f) => (
              <WebhookEventPicker
                events={events}
                kind={kind}
                loading={loadingEvents}
                selected={f.state.value}
                onChange={f.handleChange}
                error={<FieldError field={f} />}
              />
            )}
          </form.Field>
        </DetailPageSection>
      </RecordFormPage>

      <NewSecretDialog
        secret={createdSecret?.secret ?? null}
        webhookName={createdSecret?.name ?? null}
        open={!!createdSecret}
        onClose={() => {
          setCreatedSecret(null)
          // The webhook exists already - this only dismisses its one-time secret
          void duringCommit(() => navigate('/webhooks'))
        }}
      />
    </>
  )
}

