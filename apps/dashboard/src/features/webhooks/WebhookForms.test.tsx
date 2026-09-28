import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { WebhookCreate } from './WebhookCreate'
import { WebhookFormDialog } from './WebhookFormDialog'
import { CreateWebhookDocument, UpdateWebhookDocument, WebhookEventTypesDocument, WebhooksDocument } from './Webhooks.gql'
import { toWebhookUpdateInput, toWebhookValues } from './schemas/webhook'

// The create page and the edit dialog share one schema and one event picker. What these pin: a
// create needs at least one event and a sane timeout, the secret is shown once after it, switching
// kind drops the other kind's events, and an edit sends only what moved

const eventTypes = {
  webhookEventTypes: [
    { __typename: 'WebhookEventType', type: 'order.placed', name: 'Order placed', description: '', kind: 'ASYNC' },
    { __typename: 'WebhookEventType', type: 'order.validate', name: 'Validate order', description: '', kind: 'SYNC' },
  ],
}
const eventsMock = { request: { query: WebhookEventTypesDocument }, result: { data: eventTypes }, maxUsageCount: 5 }

const existing = {
  id: 'w1',
  name: 'Orders',
  targetUrl: 'https://example.com/hook',
  eventTypes: ['order.placed'],
  kind: 'ASYNC' as const,
  channel: null,
  failurePolicy: 'FAIL_CLOSED' as const,
  timeoutMs: null,
  customHeaders: { Authorization: 'Bearer t' },
}

describe('webhook schema', () => {
  it('sends only what moved', () => {
    const values = toWebhookValues(existing)
    expect(toWebhookUpdateInput(values, existing)).toBeNull()
    expect(toWebhookUpdateInput({ ...values, headers: [] }, existing)).toEqual({ id: 'w1', customHeaders: {} })
  })
})

describe('WebhookCreate', () => {
  it('needs an event and a sane timeout, then shows the one-time secret', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/webhooks/new',
      routes: { '/webhooks/new': <WebhookCreate /> },
      mocks: [
        eventsMock,
        { request: { query: ChannelsDocument }, result: { data: { channels: [] } } },
        {
          request: { query: CreateWebhookDocument, variables: captureInput(onCreate) },
          result: {
            data: {
              createWebhook: {
                __typename: 'CreateWebhookPayload',
                rawSecret: 'whsec_once',
                webhook: {
                  __typename: 'Webhook',
                  id: 'w1',
                  name: 'Orders',
                  targetUrl: 'https://example.com/hook',
                  kind: 'ASYNC',
                  isActive: true,
                  eventTypes: ['order.placed'],
                  createdAt: '2026-01-01T00:00:00Z',
                  channel: null,
                },
              },
            },
          },
        },
        { request: { query: WebhooksDocument }, result: { data: { webhooks: [] } }, maxUsageCount: 3 },
      ],
    })

    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Orders' } })
    fireEvent.change(screen.getByLabelText('Target URL'), { target: { value: 'https://example.com/hook' } })
    fireEvent.change(screen.getByLabelText('Timeout (ms)'), { target: { value: '99999' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    expect(await screen.findByText('Select at least one event type')).toBeDefined()
    expect(screen.getByText('A whole number from 0 to 30000')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Timeout (ms)'), { target: { value: '4000' } })
    fireEvent.click(await screen.findByLabelText('Order placed'))
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    expect(await screen.findByText('whsec_once', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Orders', kind: 'ASYNC', eventTypes: ['order.placed'], timeoutMs: 4000 }),
    )
  })

  it('drops the other kind’s events when the kind switches', async () => {
    renderPage({
      path: '/webhooks/new',
      routes: { '/webhooks/new': <WebhookCreate /> },
      mocks: [eventsMock, { request: { query: ChannelsDocument }, result: { data: { channels: [] } } }],
    })
    fireEvent.click(await screen.findByLabelText('Order placed'))
    expect(screen.getByText('1 selected')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: /Sync/ }))
    expect(await screen.findByText('0 selected')).toBeDefined()
    expect(screen.getByLabelText('Validate order')).toBeDefined()
  })
})

describe('WebhookFormDialog', () => {
  it('edits and sends only the renamed field', async () => {
    const onInput = vi.fn()
    const onUpdated = vi.fn()
    renderDialog(<WebhookFormDialog trigger={<button type="button">Edit</button>} existing={existing} onUpdated={onUpdated} />, [
      eventsMock,
      {
        request: { query: UpdateWebhookDocument, variables: captureInput(onInput) },
        result: { data: { updateWebhook: { __typename: 'Webhook', id: 'w1' } } },
      },
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Order events' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await vi.waitFor(() => expect(onUpdated).toHaveBeenCalled(), SLOW)
    expect(onInput).toHaveBeenCalledWith({ id: 'w1', name: 'Order events' })
  })
})
