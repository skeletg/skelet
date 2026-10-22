import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CampaignComposer } from './CampaignComposer'
import { CancelEmailCampaignDocument, EmailCampaignDocument } from './Campaigns.gql'

// What this pins: cancelling a scheduled send runs through the shared ConfirmDialog without waiting on the server -
// it closes as soon as it is confirmed, then sends the cancel

const campaign = {
  __typename: 'EmailCampaign', id: 'cmp1', channelId: 'ch1', name: 'Autumn sale', emailId: null, subject: 'Sale', blocks: [], styles: {},
  audienceKind: 'all', audienceId: null, status: 'scheduled', scheduledFor: '2026-11-01T09:00:00Z', sentAt: null,
  totalRecipients: 0, sentCount: 0, failedCount: 0, skippedCount: 0, openedCount: 0, clickedCount: 0,
  abEnabled: false, abSubjectB: '', abTestPercent: 20, abWinnerMetric: 'open', abTestMinutes: 240, abWinner: '', abTestStartedAt: null,
  createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', deletedAt: null,
}

describe('CampaignComposer cancel send', () => {
  it('closes at once, then sends the cancel', async () => {
    const cancelled = vi.fn(() => ({ data: { cancelEmailCampaign: { ...campaign, status: 'canceled' } } }))
    renderPage({
      path: '/campaigns/cmp1',
      routes: { '/campaigns/:id': <CampaignComposer /> },
      mocks: [
        meMock(),
        { request: { query: EmailCampaignDocument, variables: { id: 'cmp1' } }, result: { data: { emailCampaign: campaign } }, maxUsageCount: 10 },
        { request: { query: CancelEmailCampaignDocument, variables: { id: 'cmp1' } }, result: cancelled, delay: 500 },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel send' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByRole('button', { name: 'Keep' })).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel send' }))
    // Well inside the server's 500ms - an awaited cancel would still be open
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), { timeout: 200 })
    await waitFor(() => expect(cancelled).toHaveBeenCalled(), SLOW)
  })
})
