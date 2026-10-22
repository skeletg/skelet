import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { NotificationBell } from './NotificationBell'
import { NotificationAddedDocument, NotificationUnreadCountDocument, NotificationsDocument } from './Notifications.gql'

// What this pins: a pushed notification updates the badge from the push itself - each push used to refetch both the
// count and the list, so a bulk job's burst became a burst of requests

const recorder = (fn: () => void) => () => {
  fn()
  return true
}

describe('NotificationBell push', () => {
  it('bumps the unread badge from the pushed row without refetching', async () => {
    const counts = vi.fn()
    const lists = vi.fn()
    const pushed = {
      __typename: 'Notification',
      id: 'n2',
      category: 'system',
      severity: 'info',
      title: 'Import finished',
      body: '',
      entityType: '',
      entityId: '',
      actionUrl: '',
      channelId: null,
      createdAt: '2026-01-02T00:00:00Z',
      read: false,
    }
    renderPage({
      path: '/',
      adminId: 'admin-1',
      routes: { '/': <NotificationBell /> },
      mocks: [
        meMock(),
        { request: { query: NotificationUnreadCountDocument, variables: recorder(counts) }, result: { data: { notificationUnreadCount: 1 } }, maxUsageCount: 5 },
        { request: { query: NotificationsDocument, variables: recorder(lists) }, result: { data: { notifications: [] } }, maxUsageCount: 5 },
        { request: { query: NotificationAddedDocument }, result: { data: { notificationAdded: pushed } }, delay: 50 },
      ],
    })
    expect(await screen.findByText('1', {}, SLOW)).toBeTruthy()
    expect(await screen.findByText('2', {}, SLOW)).toBeTruthy()
    expect(counts).toHaveBeenCalledTimes(1)
    expect(lists).toHaveBeenCalledTimes(1)
  })
})
