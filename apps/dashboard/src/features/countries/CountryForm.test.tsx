import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { __resetDetailFieldsForTests, registerDetailField } from '@/plugins/field-registry'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { CountryCreatePage, CountryEditPage } from './CountryForm'
import {
  CountriesAdminListDocument,
  CountryDocument,
  CreateCountryDocument,
  UpdateCountryDocument,
} from './Countries.gql'

// Save -> mutate -> navigate is several async hops - past testing-library's 1s default when the whole
// suite is loading the machine, which made these flaky
const SLOW = { timeout: 5000 }

// Through the real page: an edit lights the header Save (it never did - the section registered
// into a context that wasn't there yet), a metadata-only edit counts, confirming sends the record,
// and a create with a bad code is stopped at the field

const stored = {
  __typename: 'Country' as const,
  code: 'DE',
  name: 'Germany',
  nativeName: 'Deutschland',
  emojiFlag: '🇩🇪',
  phoneCode: '+49',
  iso3Code: 'DEU',
  numericCode: '276',
  defaultCurrencyCode: 'EUR',
  isActive: true,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  defaultCurrency: { __typename: 'Currency' as const, code: 'EUR', symbol: '€' },
  metadata: [{ __typename: 'MetadataItem' as const, key: 'vat', value: 'DE123' }],
  businessIdFields: [],
  regions: [],
}

function renderAt(path: string, onWrite = vi.fn()) {
  const capture = (vars: { input: Record<string, unknown> }) => {
    onWrite(vars.input)
    return true
  }
  const mocks = [
    { request: { query: CountryDocument, variables: { code: 'DE' } }, result: { data: { country: stored } } },
    { request: { query: CurrenciesPickerDocument }, result: { data: { currencies: [] } } },
    {
      request: { query: UpdateCountryDocument, variables: capture },
      result: { data: { updateCountry: { ...stored } } },
    },
    {
      request: { query: CreateCountryDocument, variables: capture },
      result: { data: { createCountry: { ...stored } } },
    },
    { request: { query: CountriesAdminListDocument }, result: { data: { countries: [] } }, maxUsageCount: 5 },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/countries/new" element={<CountryCreatePage />} />
          <Route path="/countries/:code" element={<CountryEditPage />} />
          <Route path="/countries" element={<p>countries list</p>} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return onWrite
}

describe('CountryEditPage', () => {
  it('reviews and saves a metadata-only edit', async () => {
    const onWrite = renderAt('/countries/DE')
    await screen.findByDisplayValue('Germany')
    const save = screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement
    expect(save.disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Metadata 1 value'), { target: { value: 'DE999' } })
    expect(save.disabled).toBe(false)
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Metadata · vat')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('countries list', {}, SLOW)
    expect(onWrite).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'DE', name: 'Germany', metadata: [{ key: 'vat', value: 'DE999' }] }),
    )
  })
})

describe('plugin fields on the country page', () => {
  afterEach(() => __resetDetailFieldsForTests())

  it('edit their metadata keys through the page Save, under their own labels', async () => {
    registerDetailField({ id: 'acme:vat', target: 'country', key: 'vat', label: 'VAT prefix', hint: 'Two letters' })
    registerDetailField({ id: 'acme:eu', target: 'country', key: 'eu_member', label: 'EU member', type: 'checkbox' })
    const onWrite = renderAt('/countries/DE')
    const vat = (await screen.findByLabelText('VAT prefix')) as HTMLInputElement
    expect(vat.value).toBe('DE123')
    expect(document.getElementById(vat.getAttribute('aria-describedby') ?? '')?.textContent).toBe('Two letters')
    expect(screen.queryByLabelText('Metadata 1 value')).toBeNull() // its key left the free-form rows

    fireEvent.change(vat, { target: { value: 'DE999' } })
    fireEvent.click(screen.getByLabelText('EU member'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('VAT prefix')).toBeDefined()
    expect(within(dialog).getByText('EU member')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))

    await screen.findByText('countries list', {}, SLOW)
    expect(onWrite).toHaveBeenCalledWith(
      expect.objectContaining({ metadata: [{ key: 'vat', value: 'DE999' }, { key: 'eu_member', value: 'true' }] }),
    )
  })

  it('keeps a free row typed into a plugin field’s key, flagged rather than gone', async () => {
    registerDetailField({ id: 'acme:vat', target: 'country', key: 'vat', label: 'VAT prefix' })
    renderAt('/countries/DE')
    const vat = (await screen.findByLabelText('VAT prefix')) as HTMLInputElement
    fireEvent.click(screen.getByRole('button', { name: /Add row/ }))
    const key = screen.getByLabelText('Metadata 1 key') as HTMLInputElement
    fireEvent.change(key, { target: { value: 'vat' } })
    expect((screen.getByLabelText('Metadata 1 key') as HTMLInputElement).value).toBe('vat')
    expect(screen.getByText(/VAT prefix has its own field above/)).toBeDefined()
    expect(vat.value).toBe('DE123')
  })
})

describe('CountryCreatePage', () => {
  it('stops a bad code at the field and creates once it is fixed', async () => {
    const onWrite = renderAt('/countries/new')
    const code = await screen.findByLabelText('Code')
    fireEvent.change(code, { target: { value: 'deu' } })
    expect((code as HTMLInputElement).value).toBe('DEU') // uppercased as typed
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Germany' } })

    fireEvent.click(screen.getByRole('button', { name: 'Create country' }))
    expect(await screen.findByText(/Fix the highlighted field: A 2-letter ISO 3166-1 code/)).toBeDefined()
    expect(onWrite).not.toHaveBeenCalled()

    fireEvent.change(code, { target: { value: 'de' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create country' }))
    await screen.findByText('countries list', {}, SLOW)
    expect(onWrite).toHaveBeenCalledWith(expect.objectContaining({ code: 'DE', name: 'Germany' }))
  })
})
