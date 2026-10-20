import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CurrenciesList } from './CurrenciesList'
import { CurrenciesAdminListDocument, DeleteCurrencyDocument } from './Currencies.gql'

// What this pins: the delete confirm runs through the shared ConfirmDialog - a refused delete keeps it open with the
// page's own explanation, and a successful one closes it

const eur = {
  __typename: 'Currency', code: 'EUR', name: 'Euro', symbol: '€', decimals: 2, symbolPosition: 'BEFORE', isActive: true,
  sortOrder: 0, metadata: [], exchangeRate: 1, rateUpdatedAt: null, updatedAt: '2026-10-01T00:00:00Z',
}
const list = { request: { query: CurrenciesAdminListDocument }, result: { data: { currencies: [eur] } }, maxUsageCount: 5 }

async function confirmDelete(deleteMock: MockLink.MockedResponse) {
  renderPage({ path: '/currencies', adminId: 'admin-1', routes: { '/currencies': <CurrenciesList /> }, mocks: [meMock(), list, deleteMock] })
  // Delete is behind currencies.manage, like New currency - wait for the grants before opening the row menu
  await screen.findByText('New currency', {}, SLOW)
  fireEvent.keyDown(await screen.findByRole('button', { name: 'Actions for Euro' }, SLOW), { key: 'Enter' })
  fireEvent.click(await screen.findByRole('menuitem', { name: /Delete/ }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete currency' }))
  return dialog
}

describe('CurrenciesList delete', () => {
  it('keeps the dialog open and explains a refused delete', async () => {
    const dialog = await confirmDelete({
      request: { query: DeleteCurrencyDocument, variables: { code: 'EUR' } },
      result: { errors: [{ message: 'CURRENCY_IN_USE: 3 references' }] },
    })
    expect(await within(dialog).findByText(/3 references; delete blocked/)).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteCurrencyDocument, variables: { code: 'EUR' } }, result: { data: { deleteCurrency: true } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
