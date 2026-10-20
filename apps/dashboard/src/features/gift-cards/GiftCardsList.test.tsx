import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { GiftCardsList } from './GiftCardsList'
import { DisableGiftCardDocument, GiftCardsAdminListDocument } from './GiftCards.gql'

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
    mocks: [meMock(), { request: { query: GiftCardsAdminListDocument, variables: { limit: 100, offset: 0 } }, result: { data: { giftCards: [card] } }, maxUsageCount: 5 }, mock],
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
