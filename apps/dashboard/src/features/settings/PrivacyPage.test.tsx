import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { renderPage, SLOW } from '@/test/page-harness'
import { TrackingOverviewDocument } from '@/features/tracking/Tracking.gql'
import { ga4Destination, ga4Platform } from '@/features/tracking/fixtures.test-data'
import { ConfigSectionsDocument, SetConfigValueDocument } from './Settings.gql'
import { PrivacyPage } from './PrivacyPage'

const scopeInput = { kind: 'CHANNEL' as const, channelId: 'ch1' }
vi.mock('@/lib/config-scope', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/config-scope')>()),
  useConfigScope: () => ({
    effectiveChannelId: 'ch1',
    scopeInput,
    isChannel: true,
    scope: { mode: 'CHANNEL', channelId: 'ch1', locale: '' },
    selectedChannel: { id: 'ch1', name: 'Main' },
  }),
}))
vi.mock('@/shared/auth/usePermissions', () => ({ usePermissions: () => ({ canAccessChannelScope: () => true, has: () => true }) }))

const countsField = {
  __typename: 'ConfigField' as const,
  path: 'general/privacy/aggregate_counts',
  section: 'general',
  group: 'privacy',
  field: 'aggregate_counts',
  label: 'Count visitors who decline analytics, as nobody',
  description: '',
  type: 'BOOL',
  value: 'true',
  isSet: false,
  options: [],
  isDefault: true,
  public: false,
  scoping: 'CHANNEL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
}

// What this pins: the consent settings stage into the page's one Save, and each banner category lists the destinations
// it feeds
describe('PrivacyPage', () => {
  it('maps each consent choice to its destinations and saves a staged setting', async () => {
    const onSet = vi.fn()
    renderPage({
      path: '/privacy',
      routes: { '/privacy': <PrivacyPage /> },
      mocks: [
        {
          request: { query: ConfigSectionsDocument, variables: () => true },
          result: { data: { configSections: [{ __typename: 'ConfigSection', slug: 'general', label: 'General', groups: [{ __typename: 'ConfigGroup', slug: 'privacy', label: 'Privacy', fields: [countsField] }] }] } },
          maxUsageCount: 20,
        },
        {
          request: { query: TrackingOverviewDocument, variables: { channelId: 'ch1' } },
          result: { data: { trackingPlatforms: [ga4Platform], trackingDestinations: [ga4Destination({ enabled: true })] } },
          maxUsageCount: 5,
        },
        {
          request: { query: SetConfigValueDocument, variables: (v: Record<string, unknown>) => (onSet(v), true) },
          result: {
            data: {
              setConfigValue: {
                __typename: 'ConfigField',
                path: 'general/privacy/aggregate_counts',
                value: 'false',
                isDefault: false,
                scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'ch1', locale: null },
              },
            },
          },
        },
      ],
    })
    const analytics = (await screen.findByText('Analytics', {}, SLOW)).closest('li')!
    expect(await within(analytics).findByText('GA4 main', {}, SLOW)).toBeDefined()
    expect(within(screen.getByText('Advertising').closest('li')!).getByText('No destination')).toBeDefined()

    fireEvent.click(await screen.findByRole('switch', { name: /Count visitors/ }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await waitFor(() => expect(onSet).toHaveBeenCalledWith(expect.objectContaining({ path: 'general/privacy/aggregate_counts', value: 'false' })), SLOW)
  })
})
