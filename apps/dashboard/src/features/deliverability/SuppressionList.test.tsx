import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { SuppressionList } from './SuppressionList'
import { EmailSuppressionsDocument, UnsuppressEmailDocument } from './Deliverability.gql'

// What this pins: un-suppressing lets marketing mail reach an address that opted out or bounced, so it asks first

const entry = {
  __typename: 'EmailSuppression' as const,
  id: 's1',
  email: 'gone@example.com',
  reason: 'unsubscribe',
  detail: '',
  softBounces: 0,
  createdAt: '2026-01-01T00:00:00Z',
}

describe('SuppressionList remove', () => {
  it('removes an address only after the admin confirms', async () => {
    const removed = vi.fn()
    renderPage({
      path: '/suppressions',
      routes: { '/suppressions': <SuppressionList /> },
      adminId: 'admin-1',
      mocks: [
        meMock(),
        {
          request: { query: EmailSuppressionsDocument, variables: { reason: null, limit: 100, offset: 0 } },
          result: { data: { emailSuppressions: { __typename: 'EmailSuppressionPage', totalCount: 1, items: [entry] } } },
          maxUsageCount: 5,
        },
        { request: { query: UnsuppressEmailDocument, variables: captureInput(removed) }, result: { data: { unsuppressEmail: true } } },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(removed).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove from list' }))
    await vi.waitFor(() => expect(removed).toHaveBeenCalledWith({ email: 'gone@example.com' }), SLOW)
  })
})
