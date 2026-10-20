import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { WebhookDetail } from './WebhookDetail'
import { ClearWebhookDeliveriesDocument, DeleteWebhookDocument, WebhookDocument } from './Webhooks.gql'

// What this pins: delete runs through the shared ConfirmDialog - a refused delete keeps it open with the
// server's message, a successful one goes back to the list - while a failed clear still closes and reports on the page

const webhook = {
  __typename: 'Webhook', id: 'w1', name: 'Orders', targetUrl: 'https://example.com/hook', kind: 'ASYNC', isActive: true,
  eventTypes: ['order.placed'], failurePolicy: 'FAIL_CLOSED', timeoutMs: null, customHeaders: {}, payloadVersion: 1,
  circuit: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', channel: null,
}
const detail = { request: { query: WebhookDocument, variables: { id: 'w1' } }, result: { data: { webhook } }, maxUsageCount: 5 }

function render(mock: MockLink.MockedResponse) {
  renderPage({
    path: '/webhooks/w1',
    adminId: 'admin-1',
    routes: { '/webhooks/:id': <WebhookDetail />, '/webhooks': <p>Webhooks list</p> },
    mocks: [meMock(), detail, mock],
  })
}

async function confirmDelete(mock: MockLink.MockedResponse) {
  render(mock)
  // Delete sits behind webhooks.manage - wait for the grants before clicking it
  fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete webhook' }))
  return dialog
}

describe('WebhookDetail confirms', () => {
  it('keeps the delete dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({
      request: { query: DeleteWebhookDocument, variables: { id: 'w1' } },
      result: { errors: [{ message: 'webhook is locked' }] },
    })
    expect(await within(dialog).findByText('webhook is locked')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteWebhookDocument, variables: { id: 'w1' } }, result: { data: { deleteWebhook: true } } })
    expect(await screen.findByText('Webhooks list', {}, SLOW)).toBeDefined()
  })

  it('closes a failed clear and reports it on the page', async () => {
    render({
      request: { query: ClearWebhookDeliveriesDocument, variables: { webhookId: 'w1' } },
      result: { errors: [{ message: 'clear refused' }] },
    })
    await screen.findByRole('button', { name: 'Delete' }, SLOW)
    fireEvent.mouseDown(screen.getByRole('tab', { name: /Deliveries/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Clear history/ }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear history' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(await screen.findByText('clear refused')).toBeDefined()
  })
})
