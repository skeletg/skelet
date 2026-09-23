import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { CurrencyCreatePage, CurrencyEditPage } from './CurrencyForm'
import {
  CreateCurrencyDocument,
  CurrenciesAdminListDocument,
  CurrencyDocument,
  UpdateCurrencyDocument,
} from './Currencies.gql'

// Through the real page: an edit lights the header Save (it never did), confirming sends the record
// without echoing the fetched exchange rate back as an override, and a bad code stops a create

const stored = {
  __typename: 'Currency' as const,
  code: 'EUR',
  name: 'Euro',
  symbol: '€',
  decimals: 2,
  symbolPosition: 'suffix',
  isActive: true,
  sortOrder: 1,
  exchangeRate: 0.92,
  rateUpdatedAt: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  metadata: [],
}

function renderAt(path: string, onWrite = vi.fn()) {
  const capture = (vars: { input: Record<string, unknown> }) => {
    onWrite(vars.input)
    return true
  }
  const mocks = [
    { request: { query: CurrencyDocument, variables: { code: 'EUR' } }, result: { data: { currency: stored } } },
    { request: { query: UpdateCurrencyDocument, variables: capture }, result: { data: { updateCurrency: stored } } },
    { request: { query: CreateCurrencyDocument, variables: capture }, result: { data: { createCurrency: stored } } },
    { request: { query: CurrenciesAdminListDocument }, result: { data: { currencies: [] } }, maxUsageCount: 5 },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/currencies/new" element={<CurrencyCreatePage />} />
          <Route path="/currencies/:code" element={<CurrencyEditPage />} />
          <Route path="/currencies" element={<p>currencies list</p>} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return onWrite
}

describe('CurrencyEditPage', () => {
  it('reviews and saves an edit without pinning the fetched rate', async () => {
    const onWrite = renderAt('/currencies/EUR')
    await screen.findByDisplayValue('Euro')
    const save = screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement
    expect(save.disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Decimals'), { target: { value: '0' } })
    expect(save.disabled).toBe(false)
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Decimals')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('currencies list')
    const input = onWrite.mock.calls[0][0]
    expect(input).toMatchObject({ code: 'EUR', decimals: 0, symbolPosition: 'suffix' })
    expect(input).not.toHaveProperty('exchangeRate')
  })
})

describe('CurrencyCreatePage', () => {
  it('stops a bad code at the field', async () => {
    const onWrite = renderAt('/currencies/new')
    fireEvent.change(await screen.findByLabelText('Code'), { target: { value: 'eu' } })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Euro' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create currency' }))
    expect(await screen.findByText(/Fix the highlighted field: Use ISO 4217/)).toBeDefined()
    expect(onWrite).not.toHaveBeenCalled()
  })
})
