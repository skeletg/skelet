import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { ScheduledJobsList } from './ScheduledJobsList'
import { DeleteScheduledJobDocument, ScheduledJobsDocument } from './Scheduler.gql'

// What this pins: deleting a webhook job confirms through the shared ConfirmDialog without waiting on the server -
// it closes as soon as it is confirmed, and a refused delete still reports on the page

const job = {
  __typename: 'ScheduledJob', id: 'j1', name: 'Nightly export', kind: 'webhook', schedule: '0 2 * * *', enabled: true, description: '',
  timezone: 'UTC', maxAttempts: 3, retryBackoffSeconds: 60, currentAttempt: 0, alertAfterFailures: 3, consecutiveFailures: 0,
  timeoutSeconds: 30, webhook: { __typename: 'ScheduledJobWebhook', host: 'example.com', method: 'POST', hasSecret: false },
  nextRunAt: null, lastRunAt: null, lastStatus: null, lastError: null, lastDurationMs: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
}

describe('ScheduledJobsList delete', () => {
  it('closes at once and reports a refused delete on the page', async () => {
    renderPage({
      path: '/scheduler',
      adminId: 'admin-1',
      routes: { '/scheduler': <ScheduledJobsList /> },
      mocks: [
        meMock(),
        { request: { query: ScheduledJobsDocument }, result: { data: { scheduledJobs: [job] } }, maxUsageCount: 5 },
        { request: { query: DeleteScheduledJobDocument, variables: { id: 'j1' } }, result: { errors: [{ message: 'job is running' }] }, delay: 500 },
      ],
    })
    // Delete sits behind scheduler.manage - findBy waits for the grants
    fireEvent.click(await screen.findByTitle('Delete', {}, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
    // Well inside the server's 500ms - an awaited delete would still be open
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), { timeout: 200 })
    expect(await screen.findByText('job is running', {}, SLOW)).toBeDefined()
  })
})
