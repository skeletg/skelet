import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { GiftCardsList } from './GiftCardsList'
import { GiftCardsAdminListDocument } from './GiftCards.gql'

// What this pins: gift cards page and search on the server. The list used to load the first 100 cards and search
// those in the browser, so a card issued after them could never be found

describe('GiftCardsList', () => {
  it('asks the server for the searched page in the URL and pages by its total', async () => {
    const asked = vi.fn()
    const card = {
      __typename: 'GiftCard', id: 'gc1', code: 'GIFT-AAAA-BBBB', currencyCode: 'EUR', initialBalance: 5000, balance: 2500,
      status: 'active', recipientEmail: 'alice@example.test', channelId: null, expiresAt: null, createdAt: '2026-10-01T10:00:00Z',
    }
    renderPage({
      path: '/gift-cards?q=alice&page=2&size=10',
      adminId: 'admin-1',
      routes: { '/gift-cards': <GiftCardsList /> },
      mocks: [
        meMock(),
        {
          request: { query: GiftCardsAdminListDocument, variables: captureInput(asked) },
          result: { data: { giftCardsPaged: { __typename: 'GiftCardPage', totalCount: 25, items: [card] } } },
          maxUsageCount: 3,
        },
      ],
    })
    expect(await screen.findByText('GIFT-AAAA-BBBB', {}, SLOW)).toBeDefined()
    expect(asked).toHaveBeenCalledWith({ search: 'alice', limit: 10, offset: 10 })
    expect(screen.getByText('25 rows')).toBeDefined()
  })
})
