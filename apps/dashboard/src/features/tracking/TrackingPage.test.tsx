import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { ServerFeaturesDocument } from '@/shared/features/Features.gql'
import { TrackingPage } from './TrackingPage'
import { TrackingOverviewDocument } from './Tracking.gql'
import { ga4Destination, ga4Platform, metaNotInstalled } from './fixtures.test-data'

const channelScope = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1' }

// What this pins: every platform is listed with where it stands - a missing one with the command that adds it - and
// a destination that needs its secret says so
describe('TrackingPage', () => {
  it('lists the platforms with their status and the destinations set up', async () => {
    renderPage({
      path: '/tracking',
      adminId: 'admin-1',
      scope: channelScope,
      routes: { '/tracking': <TrackingPage /> },
      mocks: [
        meMock(),
        { request: { query: ServerFeaturesDocument }, result: { data: { serverFeatures: { __typename: 'ServerFeatures', signals: false, ai: false } } }, maxUsageCount: 5 },
        {
          request: { query: TrackingOverviewDocument, variables: { channelId: 'ch1' } },
          result: { data: { trackingPlatforms: [ga4Platform, metaNotInstalled], trackingDestinations: [ga4Destination({ needsSecret: true, secretSet: false })] } },
          maxUsageCount: 5,
        },
      ],
    })
    expect(await screen.findByText('GA4 main', {}, SLOW)).toBeDefined()
    expect(screen.getAllByText('Needs its secret').length).toBeGreaterThan(0)
    expect(screen.getByText('Not in this build')).toBeDefined()
    expect(screen.getByRole('button', { name: /skelet cli plugin enable tracking-meta/ })).toBeDefined()
    expect(screen.getByRole('link', { name: 'Add another' }).getAttribute('href')).toBe('/tracking/new/ga4')
  })
})
