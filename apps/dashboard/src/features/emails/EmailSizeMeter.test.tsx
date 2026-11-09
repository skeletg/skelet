import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, renderPage } from '@/test/page-harness'
import { GMAIL_CLIP_BYTES, sizeLevel } from '@/features/email/previewDoc'
import { RenderLibraryPreviewDocument } from './Emails.gql'
import { EmailSizeMeter } from './EmailSizeMeter'

// What this pins: the bar reads the server's real render against Gmail's clipping limit, and warns before the limit
// rather than only once it's past
describe('the email size bar', () => {
  it('turns amber near the limit and red past it', () => {
    expect(sizeLevel(GMAIL_CLIP_BYTES * 0.5)).toBe('ok')
    expect(sizeLevel(GMAIL_CLIP_BYTES * 0.8)).toBe('warn')
    expect(sizeLevel(GMAIL_CLIP_BYTES)).toBe('warn')
    expect(sizeLevel(GMAIL_CLIP_BYTES + 1)).toBe('over')
  })

  it('measures the rendered email', async () => {
    const html = `<html><body>${'x'.repeat(110 * 1024)}</body></html>`
    const mocks: MockLink.MockedResponse[] = [
      {
        request: { query: RenderLibraryPreviewDocument, variables: { channelId: 'ch1', customerId: null, subject: 'Hi', preheader: '', blocks: [], styles: {} } },
        result: { data: { renderLibraryPreview: { subject: 'Hi', html, text: '', preheader: null, fromName: null, fromAddress: null } } },
      },
    ]
    renderPage({ path: '/', routes: { '/': <EmailSizeMeter channelId="ch1" subject="Hi" preheader="" blocks={[]} styles={{}} /> }, mocks })
    const meter = await screen.findByRole('meter', { name: 'Email size' }, SLOW)
    expect(meter.getAttribute('aria-valuetext')).toBe('110 KB of 102 KB')
    expect(meter.textContent).toContain('110 KB')
  })
})
