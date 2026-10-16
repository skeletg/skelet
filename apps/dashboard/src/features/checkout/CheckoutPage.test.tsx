import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter } from 'react-router-dom'
import { PrefsContext } from '@/lib/admin-prefs'
import {
  CheckoutSettingsDocument,
  CreateCheckoutOptionDocument,
  DeleteCheckoutOptionDocument,
  SaveCheckoutDeliveryPointsDocument,
  UpdateCheckoutOptionDocument,
} from './Checkout.gql'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { ShippingCarriersDocument, SetShippingCarrierEnabledDocument } from '@/features/shipping/Shipping.gql'
import { PaymentMethodsDocument, SetPaymentMethodEnabledDocument, UpdatePaymentMethodPresentationDocument } from '@/features/payments/Payments.gql'
import { pointsDiff } from './checkoutDiff'
import { CheckoutPage } from './CheckoutPage'
import type { Configuration, Point } from './types'

// Radix's Switch measures itself inside a <form>, and jsdom has no ResizeObserver
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver
// Radix's Select captures the pointer and scrolls its selected item into view - neither exists in jsdom
Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}

const scopeInput = { kind: 'CHANNEL' as const, channelId: 'channel-1' }
vi.mock('@/lib/config-scope', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/config-scope')>()),
  useConfigScope: () => ({
    effectiveChannelId: 'channel-1',
    scopeInput,
    isChannel: true,
    scope: { mode: 'CHANNEL', channelId: 'channel-1', locale: '' },
    selectedChannel: { id: 'channel-1', name: 'Main' },
  }),
}))
const { canAccess } = vi.hoisted(() => ({ canAccess: vi.fn(() => true) }))
vi.mock('@/shared/auth/usePermissions', () => ({ usePermissions: () => ({ canAccessChannelScope: canAccess, has: () => true }) }))
beforeEach(() => canAccess.mockImplementation(() => true))

const guestField = {
  __typename: 'ConfigField' as const,
  path: 'sales/checkout/allow_guest',
  section: 'sales',
  group: 'checkout',
  field: 'allow_guest',
  label: 'Guest checkout',
  description: '',
  type: 'BOOL',
  value: 'true',
  isSet: false,
  options: [],
  isDefault: true,
  public: true,
  scoping: 'CHANNEL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
}

const configSections = {
  request: { query: ConfigSectionsDocument, variables: () => true },
  result: {
    data: {
      configSections: [
        {
          __typename: 'ConfigSection',
          slug: 'sales',
          label: 'Sales',
          groups: [{ __typename: 'ConfigGroup', slug: 'checkout', label: 'Checkout', fields: [guestField] }],
        },
      ],
    },
  },
  maxUsageCount: 20,
}

const poBox: Configuration['deliveryPoints'][number] = {
  id: 'point-1',
  name: 'Main post office',
  street1: 'Takovska 2',
  street2: '',
  city: 'Belgrade',
  region: '',
  postalCode: '11000',
  countryCode: 'RS',
  instructions: '',
  hours: '',
  isActive: true,
}

describe('pickup point change confirmation', () => {
  const point: Point = { ...poBox, key: poBox.id }

  it('names a changed pickup point field', () => {
    const diff = pointsDiff([point], [{ ...point, city: 'Novi Sad' }])
    expect(diff.some((d) => d.label === 'Pickup point · Main post office · Address' && d.after.includes('Novi Sad'))).toBe(true)
  })

  it('does not mark an unchanged list as dirty', () => {
    expect(pointsDiff([point], [structuredClone(point)])).toEqual([])
  })
})

function configuration(overrides: { ready?: boolean; enabled?: boolean; readiness?: Configuration['readiness'] } = {}): Configuration {
  const ready = overrides.ready ?? true
  return {
    channelId: 'channel-1',
    usingDefaults: false,
    digitalBlocks: [],
    kinds: [
      {
        slug: 'store_pickup',
        label: 'Store pickup',
        description: 'The shopper collects the order.',
        shipTo: 'point',
        pointLabel: 'Store',
        owner: 'core',
        blocks: [{ type: 'phone', label: 'Phone', floor: 'CONFIGURABLE', default: 'OPTIONAL', reason: 'To say the order is ready' }],
      },
    ],
    options: [
      {
        id: 'option-1',
        kind: 'store_pickup',
        kindLabel: 'Store pickup',
        name: 'Main store',
        description: '',
        enabled: overrides.enabled ?? false,
        sortOrder: 0,
        blockModes: {},
        pointSource: '',
        carrierSlug: '',
        implicit: false,
        ready,
        readiness: [
          {
            code: 'point_source',
            label: 'Point source is active',
            ok: ready,
            detail: ready ? 'Pickup-enabled inventory locations' : 'Choose where stores come from',
            tab: 'delivery',
            advisory: false,
          },
        ],
      },
    ],
    pointSources: [],
    carrierKinds: [],
    deliveryPoints: [poBox],
    readiness: overrides.readiness ?? [],
  }
}

function renderPage({ url, config = configuration(), mocks = [], onRead }: { url: string; config?: Configuration; mocks?: MockLink.MockedResponse[]; onRead?: () => void }) {
  const settings = {
    request: { query: CheckoutSettingsDocument, variables: { channelId: 'channel-1' } },
    result: () => { onRead?.(); return { data: { checkoutConfiguration: config } } },
    maxUsageCount: 5,
  }
  return render(
    <MockedProvider mocks={[settings, configSections, {
      request: { query: CountriesPickerDocument }, result: { data: { countries: [{ code: 'RS', name: 'Serbia', emojiFlag: '' }] } }, maxUsageCount: 20,
    }, ...mocks, {
      request: { query: ShippingCarriersDocument, variables: { channelId: 'channel-1' } },
      result: { data: { shippingCarriers: [{ carrierSlug: 'builtin', displayName: 'Shipping', isEnabled: false, sortOrder: 0, countryMode: 'all', allowedCountries: [], supportsTracking: false, supportsLabels: false, deliversOnWeekends: false }] } }, maxUsageCount: 20,
    }, {
      request: { query: PaymentMethodsDocument, variables: { channelId: 'channel-1' } },
      result: { data: { paymentMethods: [{ providerSlug: 'cash', displayName: 'Cash', isOffline: true, isAvailable: true, isEnabled: false, sortOrder: 0, countryMode: 'all', allowedCountries: [] }] } }, maxUsageCount: 20,
    }]}>
      <MemoryRouter initialEntries={[url]}>
        <PrefsContext.Provider value="checkout-test">
          <CheckoutPage />
        </PrefsContext.Provider>
      </MemoryRouter>
    </MockedProvider>,
  )
}

async function openSaveDialog() {
  const save = screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement
  await waitFor(() => expect(save.disabled).toBe(false))
  fireEvent.click(save)
  return screen.findByRole('alertdialog')
}

it('stages edits and sends mutations only after the header confirmation', async () => {
  const onUpdate = vi.fn()
  renderPage({
    url: '/checkout-settings?tab=delivery&option=option-1',
    mocks: [
      {
        request: { query: UpdateCheckoutOptionDocument, variables: (v: { input: { name: string } }) => (onUpdate(v.input), true) },
        result: { data: { updateDeliveryOption: { id: 'option-1' } } },
      },
    ],
  })
  const name = await screen.findByLabelText('Name')
  expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.change(name, { target: { value: 'Downtown store' } })
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Downtown store')).toBeDefined()
  expect(onUpdate).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ name: 'Downtown store' })), { timeout: 5000 })
})

it('links a failing go-live check to the tab that fixes it', async () => {
  renderPage({
    url: '/checkout-settings',
    config: configuration({
      readiness: [{ code: 'option:option-1', label: 'PO box is ready', ok: false, detail: 'Add PO boxes', tab: 'points', advisory: false }],
    }),
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Fix in Pickup points' }))
  expect(await screen.findByText('Pickup points', { selector: 'h2, h3' })).toBeDefined()
})

it('lets activation be staged with setup changes', async () => {
  renderPage({ url: '/checkout-settings?tab=delivery&option=option-1', config: configuration({ ready: false }) })
  const toggle = await screen.findByRole('switch', { name: 'Offer at checkout' })
  expect((toggle as HTMLButtonElement).disabled).toBe(false)
  fireEvent.click(toggle)
  expect(screen.getAllByText('Will be offered after saving').length).toBeGreaterThan(0)
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Delivery · Main store · Enabled')).toBeDefined()
})

it('deletes a removed option only after confirmation', async () => {
  const onDelete = vi.fn()
  renderPage({
    url: '/checkout-settings?tab=delivery&option=option-1',
    mocks: [
      {
        request: { query: DeleteCheckoutOptionDocument, variables: (v: { id: string }) => (onDelete(v.id), true) },
        result: { data: { deleteDeliveryOption: true } },
      },
    ],
  })
  fireEvent.click(await screen.findByRole('button', { name: /Remove option/ }))
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Delivery · Main store')).toBeDefined()
  expect(onDelete).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onDelete).toHaveBeenCalledWith('option-1'), { timeout: 5000 })
})

it('stages a pickup point edit and saves the whole list after confirmation', async () => {
  const onSave = vi.fn()
  renderPage({
    url: '/checkout-settings?tab=points&point=point-1',
    mocks: [
      {
        request: { query: SaveCheckoutDeliveryPointsDocument, variables: (v: { points: unknown[] }) => (onSave(v.points), true) },
        result: { data: { saveDeliveryPoints: [{ id: 'point-1' }] } },
      },
    ],
  })
  fireEvent.change(await screen.findByLabelText('City'), { target: { value: 'Novi Sad' } })
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Pickup point · Main post office · Address')).toBeDefined()
  expect(onSave).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith([expect.objectContaining({ id: 'point-1', city: 'Novi Sad' })]), { timeout: 5000 })
})

it('stages a details setting into the same header Save as the rest of checkout', async () => {
  const onSet = vi.fn()
  renderPage({
    url: '/checkout-settings?tab=details',
    mocks: [
      {
        request: { query: SetConfigValueDocument, variables: (v: Record<string, unknown>) => (onSet(v), true) },
        result: {
          data: {
            setConfigValue: {
              __typename: 'ConfigField',
              path: 'sales/checkout/allow_guest',
              value: 'false',
              isDefault: false,
              scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'channel-1', locale: null },
            },
          },
        },
      },
    ],
  })
  fireEvent.click(await screen.findByRole('switch', { name: /Guest checkout/ }, { timeout: 3000 }))
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Sales · Guest checkout')).toBeDefined()
  expect(onSet).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onSet).toHaveBeenCalledWith(expect.objectContaining({ path: 'sales/checkout/allow_guest', value: 'false' })), {
    timeout: 5000,
  })
})

it('sends a failing payment check to the Payment tab', async () => {
  renderPage({
    url: '/checkout-settings',
    config: configuration({
      readiness: [{ code: 'payment', label: 'At least one payment method is enabled', ok: false, detail: 'Enable a payment method on the Payment tab', tab: 'payment', advisory: false }],
    }),
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Fix in Payment' }))
  expect(await screen.findByText('Payment methods', { selector: 'h2, h3' })).toBeDefined()
})


it('keeps carrier and payment edits registered after switching tabs', async () => {
  renderPage({ url: '/checkout-settings?tab=payment' })
  fireEvent.click(await screen.findByRole('switch', { name: 'Disabled' }))
  fireEvent.mouseDown(screen.getByRole('tab', { name: 'Delivery' }), { button: 0, ctrlKey: false })
  fireEvent.click(await screen.findByRole('switch', { name: 'Disabled' }))
  fireEvent.mouseDown(screen.getByRole('tab', { name: 'Overview' }), { button: 0, ctrlKey: false })
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Cash · Enabled')).toBeDefined()
  expect(within(dialog).getByText('Shipping · Enabled')).toBeDefined()
})

it('creates and enables an option in one confirmed save, after its carrier', async () => {
  const order: string[] = []
  const onCreate = vi.fn()
  const onUpdate = vi.fn()
  renderPage({ url: '/checkout-settings?tab=delivery', mocks: [
    { request: { query: SetShippingCarrierEnabledDocument, variables: () => (order.push('carrier'), true) }, result: { data: { setShippingCarrierEnabled: { carrierSlug: 'builtin', isEnabled: true } } } },
    { request: { query: CreateCheckoutOptionDocument, variables: (v: { input: { enabled: boolean } }) => (onCreate(v.input), order.push('create'), true) }, result: { data: { createDeliveryOption: { id: 'created-1' } } } },
    { request: { query: UpdateCheckoutOptionDocument, variables: (v: { id: string; input: { enabled: boolean } }) => (onUpdate(v), order.push('enable'), true) }, result: { data: { updateDeliveryOption: { id: 'created-1' } } } },
  ] })
  fireEvent.click(await screen.findByRole('button', { name: /Add delivery option/ }))
  fireEvent.click(screen.getByRole('button', { name: /^Store pickup/ }))
  fireEvent.click(screen.getByRole('switch', { name: 'Offer at checkout' }))
  fireEvent.click(await screen.findByRole('switch', { name: 'Disabled' }))
  const dialog = await openSaveDialog()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'created-1', input: expect.objectContaining({ enabled: true }) })))
  expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }))
  expect(order).toEqual(['carrier', 'create', 'enable'])
})


it('keeps a failed activation staged and retries without recreating the option', async () => {
  const onCreate = vi.fn()
  const onUpdate = vi.fn()
  renderPage({ url: '/checkout-settings?tab=delivery', mocks: [
    { request: { query: CreateCheckoutOptionDocument, variables: () => (onCreate(), true) }, result: { data: { createDeliveryOption: { id: 'created-1' } } } },
    { request: { query: UpdateCheckoutOptionDocument, variables: () => (onUpdate(), true) }, error: new Error('Delivery option is not ready: add a pickup point') },
    { request: { query: UpdateCheckoutOptionDocument, variables: () => (onUpdate(), true) }, result: { data: { updateDeliveryOption: { id: 'created-1' } } } },
  ] })
  fireEvent.click(await screen.findByRole('button', { name: /Add delivery option/ }))
  fireEvent.click(screen.getByRole('button', { name: /^Store pickup/ }))
  fireEvent.click(screen.getByRole('switch', { name: 'Offer at checkout' }))
  const dialog = await openSaveDialog()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  expect(await within(dialog).findByText(/Delivery option is not ready/)).toBeDefined()
  expect(onCreate).toHaveBeenCalledTimes(1)
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2))
  expect(onCreate).toHaveBeenCalledTimes(1)
})


it('refreshes readiness after a payment-only save', async () => {
  const onRead = vi.fn()
  renderPage({ url: '/checkout-settings?tab=payment', onRead, mocks: [
    { request: { query: SetPaymentMethodEnabledDocument, variables: () => true }, result: { data: { setPaymentMethodEnabled: { providerSlug: 'cash', isEnabled: true } } } },
  ] })
  fireEvent.click(await screen.findByRole('switch', { name: 'Disabled' }))
  const dialog = await openSaveDialog()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onRead).toHaveBeenCalledTimes(2))
})

it('blocks invalid pickup points with inline field errors before a mutation', async () => {
  const onSave = vi.fn()
  renderPage({ url: '/checkout-settings?tab=points', mocks: [
    { request: { query: SaveCheckoutDeliveryPointsDocument, variables: () => (onSave(), true) }, result: { data: { saveDeliveryPoints: [] } } },
  ] })
  fireEvent.click(await screen.findByRole('button', { name: /Add pickup point/ }))
  expect(screen.getByText('Name is required.')).toBeDefined()
  expect(screen.getByText('Country is required.')).toBeDefined()
  const dialog = await openSaveDialog()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  expect(await within(dialog).findByText(/Every pickup point needs/)).toBeDefined()
  expect(onSave).not.toHaveBeenCalled()
})


it('stages payment labels and order through the page-header Save', async () => {
  const onUpdate = vi.fn()
  renderPage({ url: '/checkout-settings?tab=payment', mocks: [
    { request: { query: UpdatePaymentMethodPresentationDocument, variables: (v: Record<string, unknown>) => (onUpdate(v), true) }, result: { data: { updatePaymentMethodPresentation: { providerSlug: 'cash', displayName: 'Pay in cash', sortOrder: 5 } } } },
  ] })
  fireEvent.change(await screen.findByLabelText('cash · Checkout label'), { target: { value: 'Pay in cash' } })
  fireEvent.change(screen.getByLabelText('Display order'), { target: { value: '5' } })
  const dialog = await openSaveDialog()
  expect(within(dialog).getByText('Cash · Checkout label')).toBeDefined()
  expect(within(dialog).getByText('Cash · Display order')).toBeDefined()
  expect(onUpdate).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ channelId: 'channel-1', providerSlug: 'cash', displayName: 'Pay in cash', sortOrder: 5 }))
})

it('prevents offering a disabled plugin and links to provider setup', async () => {
  renderPage({ url: '/checkout-settings?tab=payment', mocks: [
    { request: { query: PaymentMethodsDocument, variables: { channelId: 'channel-1' } }, result: { data: { paymentMethods: [{ providerSlug: 'cards', displayName: 'Cards', isOffline: false, isAvailable: false, isEnabled: false, sortOrder: 0, countryMode: 'all', allowedCountries: [] }] } }, maxUsageCount: 20 },
  ] })
  const toggle = await screen.findByRole('switch', { name: 'Disabled' })
  expect((toggle as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByText(/The provider plugin is disabled/)).toBeDefined()
  expect(screen.getByRole('link', { name: 'Payment providers' }).getAttribute('href')).toBe('/payment-providers')
})

it('lets a payment manager save methods without checkout-management permission', async () => {
  canAccess.mockImplementation((...args: unknown[]) => args[0] === 'payments.manage')
  renderPage({ url: '/checkout-settings?tab=payment' })
  fireEvent.change(await screen.findByLabelText('cash · Checkout label'), { target: { value: 'Pay in cash' } })
  expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(false)
})

// A door-delivery channel: one option, two carriers quoting it, and an overlap warning on the option when asked for
function doorConfiguration({ overlap = false, carrierSlug = '' } = {}): Configuration {
  const base = configuration({ ready: true, enabled: true })
  return {
    ...base,
    kinds: [
      {
        slug: 'ship_to_address',
        label: 'Ship to address',
        description: 'A carrier delivers to the address the shopper enters.',
        shipTo: 'customer',
        pointLabel: null,
        owner: 'core',
        blocks: [],
      },
    ],
    options: [
      {
        ...base.options[0],
        kind: 'ship_to_address',
        kindLabel: 'Ship to address',
        name: 'Delivery',
        carrierSlug,
        readiness: [
          { code: 'carrier', label: 'A carrier quotes "Ship to address" for this channel', ok: true, detail: 'Store rates, DHL', tab: 'delivery', advisory: false },
          {
            code: 'overlap',
            label: 'No other option offers the same methods',
            ok: !overlap,
            detail: overlap ? 'Shoppers see these methods once under each of Home delivery too - limit each option to a different carrier' : '',
            tab: 'delivery',
            advisory: true,
          },
        ],
      },
    ],
    carrierKinds: [
      {
        kind: 'ship_to_address',
        carriers: ['Store rates', 'DHL'],
        carrierRefs: [
          { slug: 'builtin', name: 'Store rates' },
          { slug: 'dhl', name: 'DHL' },
        ],
      },
    ],
  } as Configuration
}

describe('overlapping delivery options', () => {
  it('shows an overlap as a warning that leaves the option on', async () => {
    renderPage({ url: '/checkout-settings?tab=delivery&option=option-1', config: doorConfiguration({ overlap: true }) })
    expect(await screen.findByText(/limit each option to a different carrier/)).toBeDefined()
    expect(screen.getByText("Warning - won't block going live")).toBeDefined()
    expect(screen.getByLabelText('Warning')).toBeDefined()
    expect(screen.queryByLabelText('To do')).toBeNull()
    const toggle = screen.getByRole('switch', { name: 'Offer at checkout' }) as HTMLButtonElement
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect(toggle.disabled).toBe(false)
    expect(screen.getAllByText('On').length).toBeGreaterThan(0)
  })

  it('shows no warning when the options are kept apart', async () => {
    renderPage({ url: '/checkout-settings?tab=delivery&option=option-1', config: doorConfiguration({ carrierSlug: 'dhl' }) })
    expect(await screen.findByText('No other option offers the same methods')).toBeDefined()
    expect(screen.queryByText("Warning - won't block going live")).toBeNull()
  })

  it('does not count an advisory check against going live', async () => {
    renderPage({
      url: '/checkout-settings',
      config: {
        ...doorConfiguration({ overlap: true }),
        readiness: [
          { code: 'payment', label: 'At least one payment method is enabled', ok: true, detail: 'Cash', tab: 'payment', advisory: false },
          { code: 'overlap', label: 'Delivery options overlap', ok: false, detail: 'Delivery and Home delivery', tab: 'delivery', advisory: true },
        ],
      },
    })
    expect(await screen.findByText('Shoppers can finish checking out on this channel.')).toBeDefined()
  })

  it("stages a carrier for the option and saves it after confirmation", async () => {
    const onUpdate = vi.fn()
    renderPage({
      url: '/checkout-settings?tab=delivery&option=option-1',
      config: doorConfiguration({ overlap: true }),
      mocks: [
        {
          request: { query: UpdateCheckoutOptionDocument, variables: (v: { input: { carrierSlug: string } }) => (onUpdate(v.input), true) },
          result: { data: { updateDeliveryOption: { id: 'option-1' } } },
        },
      ],
    })
    const carrier = await screen.findByRole('combobox', { name: 'Carrier' })
    expect(carrier.textContent).toContain('Any carrier')
    fireEvent.pointerDown(carrier, { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('option', { name: 'DHL' }))
    expect(onUpdate).not.toHaveBeenCalled()

    const dialog = await openSaveDialog()
    expect(within(dialog).getByText('Delivery · Delivery · Carrier')).toBeDefined()
    expect(within(dialog).getByText('Any carrier')).toBeDefined()
    expect(within(dialog).getByText('DHL')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ carrierSlug: 'dhl' })), { timeout: 5000 })
  })
})
