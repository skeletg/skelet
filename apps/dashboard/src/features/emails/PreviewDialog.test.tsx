import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, renderPage } from '@/test/page-harness'
import { PreviewCustomersDocument, RenderLibraryPreviewDocument } from './Emails.gql'
import { PreviewDialog } from './PreviewDialog'

// What this pins: the preview shows the email as an inbox lists it - sender, subject, preheader - plus the plain-text
// part and the checks, and says so when there's no preheader rather than leaving the inbox to guess
const blocks = [{ type: 'heading', text: 'Hello' }]

function mocks(preheader: string | null): MockLink.MockedResponse[] {
  return [
    { request: { query: PreviewCustomersDocument, variables: { channelId: 'ch1' } }, result: { data: { customersPaged: { items: [] } } }, maxUsageCount: 5 },
    {
      request: { query: RenderLibraryPreviewDocument, variables: { channelId: 'ch1', customerId: null, subject: 'Hi {{customer.firstName}}', preheader: preheader ?? '', blocks, styles: {} } },
      result: {
        data: {
          renderLibraryPreview: {
            subject: 'Hi Alex',
            html: '<!DOCTYPE html><html><head></head><body><h2>Hello</h2><a href="#">Dead</a></body></html>',
            text: 'Hello\nDead: #',
            preheader,
            fromName: 'Vetrina',
            fromAddress: 'hello@vetrina.test',
          },
        },
      },
      maxUsageCount: 5,
    },
  ]
}

function page(preheader: string | null) {
  renderPage({
    path: '/',
    routes: {
      '/': <PreviewDialog open onOpenChange={() => {}} channelId="ch1" subject="Hi {{customer.firstName}}" preheader={preheader ?? ''} blocks={blocks} styles={{}} />,
    },
    mocks: mocks(preheader),
  })
}

describe('PreviewDialog', () => {
  it('shows the email as the inbox lists it, its plain text and its checks', async () => {
    page('Your account is ready')
    const inbox = await screen.findByRole('region', { name: 'In the inbox' }, SLOW)
    expect(within(inbox).getByText('Vetrina')).toBeDefined()
    expect(within(inbox).getByText('Hi Alex')).toBeDefined()
    expect(within(inbox).getByText(/Your account is ready/)).toBeDefined()
    expect(screen.getByText(/Goes nowhere real: #/)).toBeDefined()
    fireEvent.click(screen.getByRole('radio', { name: 'Plain text' }))
    expect(screen.getByText(/Dead: #/)).toBeDefined()
  })

  it('warns when there is no preheader', async () => {
    page(null)
    expect(await screen.findByText(/inboxes show the email's first words/, {}, SLOW)).toBeDefined()
  })
})
