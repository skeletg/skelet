import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { InboxPage } from './InboxPage'
import { NotificationsDocument } from './Notifications.gql'

// What this pins: a failed load says so - it used to render the empty state, "You're all caught up"

describe('InboxPage', () => {
  it('shows the load error instead of the all-caught-up empty state', async () => {
    renderPage({
      path: '/inbox',
      routes: { '/inbox': <InboxPage /> },
      adminId: 'admin-1',
      mocks: [meMock(), { request: { query: NotificationsDocument, variables: { limit: 200 } }, error: new Error('notifications are down') }],
    })
    expect(await screen.findByText('notifications are down', {}, SLOW)).toBeTruthy()
    expect(screen.queryByText(/all caught up/)).toBeNull()
  })
})
