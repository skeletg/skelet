import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ShippingZoneEditPage } from './ShippingZoneForm'
import { ShippingZoneDocument, UpdateShippingMethodDocument, UpdateShippingZoneDocument } from './Shipping.gql'

// Through the real page: a zone edit is reviewed in the dialog before it's written (it used to
// write straight through), and a method row validates before its own Save sends anything

const method = {
  __typename: 'ShippingMethod' as const,
  id: 'm1',
  zoneId: 'z1',
  code: 'std',
  name: 'Standard',
  calcType: 'FLAT' as const,
  flatCents: 500,
  freeAboveCents: 0,
  sortOrder: 0,
  isActive: true,
  countryMode: 'all',
  allowedCountries: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const zone = {
  __typename: 'ShippingZone' as const,
  id: 'z1',
  channelId: 'c1',
  code: 'us',
  name: 'United States',
  sortOrder: 0,
  isActive: true,
  countries: ['US'],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  methods: [method],
}

function renderPage(writes = { zone: vi.fn(), method: vi.fn() }) {
  const capture = (fn: (input: unknown) => void) => (vars: { input: unknown }) => {
    fn(vars.input)
    return true
  }
  const mocks = [
    { request: { query: ShippingZoneDocument, variables: { id: 'z1' } }, result: { data: { shippingZone: zone } }, maxUsageCount: 3 },
    {
      request: { query: UpdateShippingZoneDocument, variables: capture(writes.zone) },
      result: { data: { updateShippingZone: { ...zone, countries: ['US', 'CA'] } } },
    },
    {
      request: { query: UpdateShippingMethodDocument, variables: capture(writes.method) },
      result: { data: { updateShippingMethod: method } },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={['/shipping/z1']}>
        <Routes>
          <Route path="/shipping/:id" element={<ShippingZoneEditPage />} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return writes
}

describe('ShippingZoneEditPage', () => {
  it('reviews a zone edit before writing it', async () => {
    const writes = renderPage()
    fireEvent.change(await screen.findByLabelText('Countries'), { target: { value: 'us, ca' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Countries')).toBeDefined()
    expect(writes.zone).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await vi.waitFor(() =>
      expect(writes.zone).toHaveBeenCalledWith(expect.objectContaining({ id: 'z1', countries: ['US', 'CA'] })),
    )
  })

  it('validates a method row before its own Save sends anything', async () => {
    const writes = renderPage()
    fireEvent.change(await screen.findByLabelText('Flat price ($)'), { target: { value: '-3' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Fix the highlighted field: Must be 0 or more/)).toBeDefined()
    expect(writes.method).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Flat price ($)'), { target: { value: '7.5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(() =>
      expect(writes.method).toHaveBeenCalledWith(expect.objectContaining({ id: 'm1', flatCents: 750 })),
    )
  })
})
