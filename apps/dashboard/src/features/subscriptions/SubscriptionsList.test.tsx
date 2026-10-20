import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { SubscriptionsList } from './SubscriptionsList'
import { SubscriptionsDocument } from './Subscriptions.gql'

// What this pins: subscriptions page on the server. The list used to load the first 100, and the rest never showed

describe('SubscriptionsList', () => {
  it('asks the server for the page in the URL and pages by its total', async () => {
    const asked = vi.fn()
    const sub = {
      __typename: 'CustomerSubscription', id: 's1', customerId: null, productId: 'p1', orderId: null, status: 'ACTIVE',
      intervalUnit: 'MONTH', intervalCount: 1, price: 1500, currencyCode: 'EUR', quantity: 1, sku: 'BOX', title: 'Coffee box',
      currentPeriodStart: '2026-10-01T00:00:00Z', currentPeriodEnd: '2026-11-01T00:00:00Z', nextBillingAt: '2026-11-01T00:00:00Z',
      canceledAt: null, createdAt: '2026-10-01T00:00:00Z',
    }
    renderPage({
      path: '/subscriptions?page=3&size=10',
      adminId: 'admin-1',
      routes: { '/subscriptions': <SubscriptionsList /> },
      mocks: [
        meMock(),
        {
          request: { query: SubscriptionsDocument, variables: captureInput(asked) },
          result: { data: { subscriptionsPaged: { __typename: 'SubscriptionPage', totalCount: 30, items: [sub] } } },
          maxUsageCount: 3,
        },
      ],
    })
    expect(await screen.findByText('Coffee box', {}, SLOW)).toBeDefined()
    expect(asked).toHaveBeenCalledWith({ limit: 10, offset: 20 })
    expect(screen.getByText('30 rows')).toBeDefined()
  })
})
