import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { InventoryLocationEditPage } from './InventoryLocationForm'
import {
  InventoryLocationDocument,
  InventoryLocationsDocument,
  InventoryModeDocument,
  UpdateInventoryLocationDocument,
} from './Inventory.gql'

// Save -> mutate -> navigate is several async hops - past testing-library's 1s default when the whole
// suite is loading the machine, which made these flaky
const SLOW = { timeout: 5000 }
const nativeScrollIntoView = HTMLElement.prototype.scrollIntoView

// What these pin, end to end through the real page: the header Save lights up for an edit (it
// didn't - the section registered into a context that wasn't there yet), the dialog lists what
// changed including the nested fields, confirming sends the edited record, and a schema error keeps
// the dialog open instead of closing as if it had saved

const stored = {
  __typename: 'InventoryLocation' as const,
  id: 'loc-1',
  slug: 'main',
  name: 'Main store',
  isActive: true,
  storefrontVisible: false,
  type: 'RETAIL_STORE' as const,
  line1: 'Street 1',
  line2: '',
  city: 'Berlin',
  region: '',
  postalCode: '10115',
  countryCode: 'DE',
  latitude: null,
  longitude: null,
  timezone: 'Europe/Berlin',
  hours: {},
  email: '',
  phone: '',
  contactName: '',
  externalReference: '',
  pickupEnabled: false,
  pickupInstructions: '',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  metadata: [{ __typename: 'MetadataItem' as const, key: 'carrier', value: 'dhl' }],
}

function renderPage(onUpdate = vi.fn(), overrides: Partial<Omit<typeof stored, 'type'>> = {}) {
  const record = { ...stored, ...overrides }
  const mocks = [
    { request: { query: InventoryLocationDocument, variables: { id: 'loc-1' } }, result: { data: { inventoryLocation: record } } },
    { request: { query: CountriesPickerDocument }, result: { data: { countries: [] } } },
    { request: { query: InventoryModeDocument }, result: { data: { inventoryMode: 'MULTI' } } },
    {
      request: {
        query: UpdateInventoryLocationDocument,
        variables: (vars: { input: Record<string, unknown> }) => {
          onUpdate(vars.input)
          return true
        },
      },
      result: { data: { updateInventoryLocation: { ...record, city: 'Hamburg' } } },
    },
    { request: { query: InventoryLocationsDocument }, result: { data: { inventoryLocations: [] } }, maxUsageCount: 5 },
    {
      request: { query: InventoryLocationDocument, variables: { id: 'loc-1' } },
      result: { data: { inventoryLocation: { ...record, city: 'Hamburg' } } },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={['/inventory/locations/loc-1']}>
        <Routes>
          <Route path="/inventory/locations/:id" element={<InventoryLocationEditPage />} />
          <Route path="/inventory/locations" element={<p>locations list</p>} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return onUpdate
}

const saveButton = () => screen.getByRole('button', { name: 'Save changes' })

afterEach(() => {
  vi.restoreAllMocks()
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
})

describe('InventoryLocationEditPage', () => {
  it('stays clean until something is edited', async () => {
    renderPage()
    await screen.findByDisplayValue('Main store')
    expect((saveButton() as HTMLButtonElement).disabled).toBe(true)
  })

  it('reviews and saves an edit, metadata included', async () => {
    const onUpdate = renderPage()
    fireEvent.change(await screen.findByLabelText('City'), { target: { value: 'Hamburg' } })
    fireEvent.change(screen.getByLabelText('Metadata 1 value'), { target: { value: 'ups' } })
    expect((saveButton() as HTMLButtonElement).disabled).toBe(false)

    fireEvent.click(saveButton())
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('City')).toBeDefined()
    expect(within(dialog).getByText('Hamburg')).toBeDefined()
    expect(within(dialog).getByText('Metadata · carrier')).toBeDefined()

    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await screen.findByText('locations list', {}, SLOW)
    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'loc-1',
        city: 'Hamburg',
        metadata: [{ key: 'carrier', value: 'ups' }],
        clearCoords: true,
      }),
    )
    // Stored with no hours - the form shows that as closed every day, which must not be written back
    expect(onUpdate.mock.calls[0][0].hours).toBeUndefined()
  })

  it('stages public visibility until the page save is confirmed', async () => {
    const onUpdate = renderPage()
    fireEvent.click(await screen.findByRole('switch', { name: 'List in store finder' }))
    expect(onUpdate).not.toHaveBeenCalled()
    fireEvent.click(saveButton())
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('List in store finder')).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await screen.findByText('locations list', {}, SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ storefrontVisible: true }))
  })

  it('reviews customer-service removal when changing a store to a warehouse', async () => {
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, writable: true, value: vi.fn() })
    const onUpdate = renderPage(vi.fn(), { storefrontVisible: true, pickupEnabled: true })
    fireEvent.click(await screen.findByRole('combobox', { name: 'Location type' }))
    fireEvent.click(await screen.findByRole('option', { name: 'Warehouse' }))
    expect(screen.queryByRole('switch', { name: 'List in store finder' })).toBeNull()
    expect(onUpdate).not.toHaveBeenCalled()
    fireEvent.click(saveButton())
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('List in store finder')).toBeDefined()
    expect(within(dialog).getByText('Accept online orders for pickup')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 3 changes/ }))
    await screen.findByText('locations list', {}, SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ type: 'WAREHOUSE', storefrontVisible: false, pickupEnabled: false }))
  })

  it('keeps the dialog open with the reason when the schema rejects the edit', async () => {
    const onUpdate = renderPage()
    fireEvent.change(await screen.findByLabelText('Latitude'), { target: { value: '120' } })
    await waitFor(() => expect(screen.getByText('Between -90 and 90')).toBeDefined())

    fireEvent.click(saveButton())
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    expect(await within(dialog).findByText(/Between -90 and 90/)).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()
  })
})
