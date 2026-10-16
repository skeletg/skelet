import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MockedProvider } from '@apollo/client/testing/react'
import type { MockLink } from '@apollo/client/testing'
import { MemoryRouter } from 'react-router-dom'
import { PrefsContext } from '@/lib/admin-prefs'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { PaymentProvidersDocument } from './Payments.gql'
import { PaymentProvidersPage } from './PaymentProvidersPage'

const scopeInput = { kind: 'CHANNEL' as const, channelId: 'channel-1' }
vi.mock('@/lib/config-scope', async (original) => ({
  ...(await original<typeof import('@/lib/config-scope')>()),
  useConfigScope: () => ({ effectiveChannelId: 'channel-1', scopeInput, scope: { mode: 'CHANNEL', channelId: 'channel-1', locale: '' }, selectedChannel: { id: 'channel-1', name: 'Main' }, isChannel: true }),
}))
vi.mock('@/shared/auth/usePermissions', () => ({ usePermissions: () => ({ has: () => true, canAccessChannelScope: () => true, allowedChannelScopes: () => ({ allChannels: true }) }) }))
afterEach(() => localStorage.clear())

const provider = (slug: string) => ({ slug, displayName: slug === 'cards' ? 'Card gateway' : 'Second gateway', pluginSlug: `payment-${slug}`, pluginName: `${slug} plugin`, pluginVersion: '1.0.0', isOffline: false, isAvailable: true, configurationPaths: [`payment/${slug}/api_key`], webhookPath: `/api/v1/payments/webhook/${slug}?channel=channel-1` })
const field = (slug: string) => ({ path: `payment/${slug}/api_key`, section: 'payment', group: slug, field: 'api_key', label: `${slug} API key`, description: '', type: 'SECRET' as const, value: '', isSet: true, isDefault: false, public: false, scoping: 'CHANNEL' as const, updatedAt: null, options: [], scope: { kind: 'CHANNEL' as const, channelId: 'channel-1', locale: null } })

function mount(providers = [provider('cards'), provider('second')], extra: MockLink.MockedResponse[] = []) {
  return render(<MockedProvider mocks={[
    { request: { query: PaymentProvidersDocument, variables: { channelId: 'channel-1' } }, result: { data: { paymentProviders: providers } }, maxUsageCount: 10 },
    { request: { query: ConfigSectionsDocument, variables: { scope: scopeInput } }, result: { data: { configSections: [{ slug: 'payment', label: 'Payment', groups: ['cards', 'second'].map((slug) => ({ slug, label: slug, fields: [field(slug)] })) }] } }, maxUsageCount: 10 },
    ...extra,
  ]}><MemoryRouter><PrefsContext.Provider value="payment-provider-test"><PaymentProvidersPage /></PrefsContext.Provider></MemoryRouter></MockedProvider>)
}

it('discovers installed plugins and exposes their channel webhook path', async () => {
  mount()
  expect(await screen.findByText('cards plugin · 1.0.0')).toBeDefined()
  expect(screen.getByText('/api/v1/payments/webhook/cards?channel=channel-1')).toBeDefined()
  expect(screen.getByRole('link', { name: 'Checkout payment methods' }).getAttribute('href')).toBe('/checkout-settings?tab=payment')
})

it('retains a secret draft across provider selections and masks it in the Save dialog', async () => {
  const onSave = vi.fn()
  mount(undefined, [{ request: { query: SetConfigValueDocument, variables: (v: Record<string, unknown>) => (onSave(v), true) }, result: { data: { setConfigValue: { path: 'payment/cards/api_key', value: '', isDefault: false, scope: { kind: 'CHANNEL', channelId: 'channel-1', locale: null } } } } }])
  const keyField = await screen.findByLabelText('cards API key')
  keyField.focus()
  fireEvent.change(keyField, { target: { value: 'secret-test-value' } })
  fireEvent.click(screen.getByText('Second gateway'))
  expect(await screen.findByLabelText('second API key')).toBeDefined()
  await waitFor(() => expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('Payment · cards API key')).toBeDefined()
  expect(within(dialog).queryByText('secret-test-value')).toBeNull()
  expect(onSave).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ path: 'payment/cards/api_key', value: 'secret-test-value', scope: scopeInput })))
  expect(Array.from({ length: localStorage.length }, (_, i) => localStorage.getItem(localStorage.key(i)!)).join('')).not.toContain('secret-test-value')
})

it('shows installation guidance when no gateway is installed', async () => {
  mount([])
  expect(await screen.findByText(/No payment gateway plugins are installed/)).toBeDefined()
  expect(screen.getByText('./skelet cli plugin add <module>@<version>')).toBeDefined()
})
