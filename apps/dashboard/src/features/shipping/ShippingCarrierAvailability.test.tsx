import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, globalScope, renderSection } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { ShippingCarrierAvailability } from './ShippingCarrierAvailability'
import {
  SetShippingCarrierEnabledDocument,
  SetShippingCarrierWeekendDeliveryDocument,
  ShippingCarriersDocument,
} from './Shipping.gql'

// The carrier list on Checkout and Settings: a weekend switch for every carrier but the built-in one, staged into the
// page-header Save like enablement and countries

const channelScope: ConfigScopeContextValue = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'channel-1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'channel-1',
}

const carrier = (slug: string, name: string, deliversOnWeekends = false) => ({
  __typename: 'ShippingCarrierAdmin' as const,
  carrierSlug: slug,
  displayName: name,
  isEnabled: true,
  sortOrder: slug === 'builtin' ? 0 : 1,
  countryMode: 'all',
  allowedCountries: [],
  supportsTracking: false,
  supportsLabels: false,
  deliversOnWeekends,
})

function renderCarriers(writes = { weekends: vi.fn(), enabled: vi.fn() }) {
  renderSection(
    <ShippingCarrierAvailability />,
    [
      {
        request: { query: ShippingCarriersDocument, variables: { channelId: 'channel-1' } },
        result: { data: { shippingCarriers: [carrier('builtin', 'Store rates'), carrier('dhl', 'DHL')] } },
        maxUsageCount: 10,
      },
      {
        request: { query: SetShippingCarrierWeekendDeliveryDocument, variables: (v: unknown) => (writes.weekends(v), true) },
        result: { data: { setShippingCarrierWeekendDelivery: { __typename: 'ShippingCarrierAdmin', carrierSlug: 'dhl', deliversOnWeekends: true } } },
      },
      {
        request: { query: SetShippingCarrierEnabledDocument, variables: (v: unknown) => (writes.enabled(v), true) },
        result: { data: { setShippingCarrierEnabled: { __typename: 'ShippingCarrierAdmin', carrierSlug: 'dhl', isEnabled: false } } },
      },
    ],
    channelScope,
  )
  return writes
}

describe('ShippingCarrierAvailability', () => {
  it('offers the weekend switch for a carrier, never for the built-in one', async () => {
    renderCarriers()
    await screen.findByText('DHL')
    expect(screen.getAllByRole('switch', { name: 'Delivers on weekends' })).toHaveLength(1)
    expect(document.getElementById('carrier-availability-weekends-dhl')).not.toBeNull()
    expect(document.getElementById('carrier-availability-weekends-builtin')).toBeNull()
  })

  it('stages the weekend switch and sets it only after the header confirmation', async () => {
    const writes = renderCarriers()
    await screen.findByText('DHL')
    fireEvent.click(screen.getByRole('switch', { name: 'Delivers on weekends' }))
    expect(writes.weekends).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('DHL · Delivers on weekends')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await waitFor(
      () => expect(writes.weekends).toHaveBeenCalledWith({ channelId: 'channel-1', carrierSlug: 'dhl', deliversOnWeekends: true }),
      SLOW,
    )
    expect(writes.enabled).not.toHaveBeenCalled()
  })

  it('applies a weekend change and an enablement change in the same save', async () => {
    const writes = renderCarriers()
    await screen.findByText('DHL')
    const dhl = document.getElementById('carrier-availability-enabled-dhl') as HTMLElement
    fireEvent.click(dhl)
    fireEvent.click(screen.getByRole('switch', { name: 'Delivers on weekends' }))

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('DHL · Enabled')).toBeDefined()
    expect(within(dialog).getByText('DHL · Delivers on weekends')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await waitFor(() => expect(writes.weekends).toHaveBeenCalled(), SLOW)
    expect(writes.enabled).toHaveBeenCalledWith({ channelId: 'channel-1', carrierSlug: 'dhl', enabled: false })
  })

  it('treats switching the weekend rule back as no change', async () => {
    renderCarriers()
    await screen.findByText('DHL')
    const toggle = screen.getByRole('switch', { name: 'Delivers on weekends' })
    fireEvent.click(toggle)
    fireEvent.click(toggle)
    expect((screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement).disabled).toBe(true)
  })
})
