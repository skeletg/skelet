import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ConfigScopeContext, type ConfigScopeContextValue } from '@/lib/config-scope'
import { globalScope } from '@/test/page-harness'
import { ShippingZoneEditPage } from './ShippingZoneForm'
import { ShippingZoneDocument, UpdateShippingMethodDocument, UpdateShippingZoneDocument } from './Shipping.gql'

// Save -> mutate -> navigate is several async hops - past testing-library's 1s default when the whole
// suite is loading the machine, which made these flaky
const SLOW = { timeout: 5000 }

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

const scope: ConfigScopeContextValue = {
  ...globalScope,
  channels: [{ id: 'c1', name: 'Shop', defaultLocale: 'en', locales: [{ code: 'en' }], isActive: true, currencyCode: 'EUR' }],
}

function renderPage(writes = { zone: vi.fn(), method: vi.fn() }, withMethod: Record<string, unknown> = method) {
  const capture = (fn: (input: unknown) => void) => (vars: { input: unknown }) => {
    fn(vars.input)
    return true
  }
  const mocks = [
    {
      request: { query: ShippingZoneDocument, variables: { id: 'z1' } },
      result: { data: { shippingZone: { ...zone, methods: [withMethod] } } },
      maxUsageCount: 3,
    },
    {
      request: { query: UpdateShippingZoneDocument, variables: capture(writes.zone) },
      result: { data: { updateShippingZone: { ...zone, countries: ['US', 'CA'] } } },
    },
    {
      request: { query: UpdateShippingMethodDocument, variables: capture(writes.method) },
      result: { data: { updateShippingMethod: withMethod } },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <ConfigScopeContext.Provider value={scope}>
        <MemoryRouter initialEntries={['/shipping/z1']}>
          <Routes>
            <Route path="/shipping/:id" element={<ShippingZoneEditPage />} />
          </Routes>
        </MemoryRouter>
      </ConfigScopeContext.Provider>
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
      SLOW,
    )
  })

  it('shows a method without an estimate as 0 days on weekdays', async () => {
    renderPage()
    const days = (await screen.findByLabelText('Delivery days')) as HTMLInputElement
    expect(days.value).toBe('0')
    expect(screen.getByRole('checkbox', { name: /Delivers on weekends/ }).getAttribute('aria-checked')).toBe('false')
  })

  it("sends a method's delivery days and weekend rule with its own Save", async () => {
    const writes = renderPage()
    fireEvent.change(await screen.findByLabelText('Delivery days'), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /Delivers on weekends/ }))
    expect(writes.method).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(
      () => expect(writes.method).toHaveBeenCalledWith(expect.objectContaining({ id: 'm1', estimatedDays: 2, deliversOnWeekends: true })),
      SLOW,
    )
  })

  it('refuses negative delivery days before sending anything', async () => {
    const writes = renderPage()
    fireEvent.change(await screen.findByLabelText('Delivery days'), { target: { value: '-1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Zero or more/)).toBeDefined()
    expect(writes.method).not.toHaveBeenCalled()
  })

  it('validates a method row before its own Save sends anything', async () => {
    const writes = renderPage()
    fireEvent.change(await screen.findByLabelText('Flat price (EUR)'), { target: { value: '-3' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Fix the highlighted field: Zero or more/)).toBeDefined()
    expect(writes.method).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Flat price (EUR)'), { target: { value: '7.5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(() =>
      expect(writes.method).toHaveBeenCalledWith(expect.objectContaining({ id: 'm1', flatCents: 750 })),
      SLOW,
    )
  })

  it('stops a table at a not-offered tier', async () => {
    const writes = { zone: vi.fn(), method: vi.fn() }
    renderPage(writes, {
      ...method,
      calcType: 'TABLE',
      tableBasis: 'WEIGHT',
      rateTiers: [
        { __typename: 'ShippingRateTier', from: 0, priceCents: 490, notOffered: false },
        { __typename: 'ShippingRateTier', from: 30000, priceCents: 1200, notOffered: false },
      ],
    })
    expect(await screen.findAllByText('From (kg)', {}, SLOW)).toHaveLength(2)
    fireEvent.click(screen.getAllByRole('checkbox', { name: 'Not offered' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(
      () =>
        expect(writes.method).toHaveBeenCalledWith(
          expect.objectContaining({
            rateTiers: [
              { from: 0, priceCents: 490, notOffered: false },
              { from: 30000, priceCents: 0, notOffered: true },
            ],
          }),
        ),
      SLOW,
    )
  })

  it('retypes the tier starts when the basis changes, keeping a zero start', async () => {
    renderPage(undefined, {
      ...method,
      calcType: 'TABLE',
      tableBasis: 'WEIGHT',
      rateTiers: [
        { __typename: 'ShippingRateTier', from: 0, priceCents: 490, notOffered: false },
        { __typename: 'ShippingRateTier', from: 2500, priceCents: 890, notOffered: false },
      ],
    })
    const second = (await screen.findByLabelText('From (kg)', { selector: '#m1-from-1' }, SLOW)) as HTMLInputElement
    expect(second.value).toBe('2.5')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, writable: true, value: vi.fn() })
    fireEvent.click(screen.getByRole('combobox', { name: 'Priced by' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Order subtotal' }))
    expect(((await screen.findByLabelText('From (EUR)', { selector: '#m1-from-1' })) as HTMLInputElement).value).toBe('')
    expect((screen.getByLabelText('From (EUR)', { selector: '#m1-from-0' }) as HTMLInputElement).value).toBe('0')
  })
})
