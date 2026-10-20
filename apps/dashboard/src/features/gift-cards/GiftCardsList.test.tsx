import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { GiftCardsList } from './GiftCardsList'
import { DisableGiftCardDocument, GiftCardsAdminListDocument } from './GiftCards.gql'

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

// What this pins: disabling a card confirms through the shared ConfirmDialog - a refused disable keeps it open with
// the server's message, a successful one closes it

const card = {
  __typename: 'GiftCard', id: 'gc1', code: 'GIFT-1234', currencyCode: 'EUR', initialBalance: 5000, balance: 5000, status: 'active',
  recipientEmail: null, channelId: null, expiresAt: null, createdAt: '2026-10-01T00:00:00Z',
}

async function confirmDisable(mock: MockLink.MockedResponse) {
  renderPage({
    path: '/gift-cards',
    adminId: 'admin-1',
    routes: { '/gift-cards': <GiftCardsList /> },
    mocks: [
      meMock(),
      {
        request: { query: GiftCardsAdminListDocument, variables: () => true },
        result: { data: { giftCardsPaged: { __typename: 'GiftCardPage', totalCount: 1, items: [card] } } },
        maxUsageCount: 5,
      },
      mock,
    ],
  })
  const row = (await screen.findByText('GIFT-1234', {}, SLOW)).closest('tr') as HTMLElement
  fireEvent.pointerDown(within(row).getAllByRole('button').at(-1)!, { button: 0, ctrlKey: false })
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Disable' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Disable card' }))
  return dialog
}

describe('GiftCardsList disable', () => {
  it('keeps the dialog open and shows a refused disable', async () => {
    const dialog = await confirmDisable({ request: { query: DisableGiftCardDocument, variables: { id: 'gc1' } }, result: { errors: [{ message: 'card has pending holds' }] } })
    expect(await within(dialog).findByText('card has pending holds')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the disable goes through', async () => {
    await confirmDisable({ request: { query: DisableGiftCardDocument, variables: { id: 'gc1' } }, result: { data: { disableGiftCard: { __typename: 'GiftCard', id: 'gc1', status: 'disabled' } } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
