import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { SubscriptionsList } from './SubscriptionsList'
import { CancelSubscriptionDocument, SubscriptionsDocument } from './Subscriptions.gql'

// What this pins: the cancel confirm runs through the shared ConfirmDialog without waiting on the server - it closes
// as soon as it is confirmed, and a refused cancel still reports on the page

const box = {
  __typename: 'CustomerSubscription', id: 'sub1', customerId: 'c1', productId: 'p1', orderId: 'o1', status: 'ACTIVE',
  intervalUnit: 'MONTH', intervalCount: 1, price: 1000, currencyCode: 'EUR', quantity: 1, sku: 'BOX', title: 'Monthly box',
  currentPeriodStart: '2026-10-01T00:00:00Z', currentPeriodEnd: '2026-11-01T00:00:00Z', nextBillingAt: '2026-11-01T00:00:00Z',
  canceledAt: null, createdAt: '2026-10-01T00:00:00Z',
}

describe('SubscriptionsList cancel', () => {
  it('closes at once and reports a refused cancel on the page', async () => {
    renderPage({
      path: '/subscriptions',
      adminId: 'admin-1',
      routes: { '/subscriptions': <SubscriptionsList /> },
      mocks: [
        meMock(),
        { request: { query: SubscriptionsDocument, variables: { limit: 100, offset: 0 } }, result: { data: { subscriptions: [box] } }, maxUsageCount: 5 },
        { request: { query: CancelSubscriptionDocument, variables: { id: 'sub1' } }, result: { errors: [{ message: 'gateway refused' }] }, delay: 500 },
      ],
    })
    const row = (await screen.findByText('Monthly box', {}, SLOW)).closest('tr') as HTMLElement
    fireEvent.pointerDown(within(row).getByRole('button'), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Cancel' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByRole('button', { name: 'Keep active' })).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel subscription' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(screen.queryByText('gateway refused')).toBeNull() // closed before the server answered
    expect(await screen.findByText('gateway refused', {}, SLOW)).toBeDefined()
  })
})
