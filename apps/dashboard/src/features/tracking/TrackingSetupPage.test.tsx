import { fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import { TrackingSetupPage } from './TrackingSetupPage'
import { CreateTrackingDestinationDocument, TrackingPlatformsDocument } from './Tracking.gql'
import { ga4Destination, ga4Platform, metaNotInstalled } from './fixtures.test-data'

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'ch1',
  selectedChannel: { id: 'ch1', name: 'Main shop' } as typeof globalScope.selectedChannel,
}

// What these pin: a new destination is created switched off, in the platform's test mode, with its secret - then the
// destination page takes over; a platform missing from the build says how to add it instead of a form

function renderSetup(kind: string, create = vi.fn()) {
  renderPage({
    path: `/tracking/new/${kind}`,
    adminId: 'admin-1',
    scope: channelScope,
    routes: { '/tracking/new/:kind': <TrackingSetupPage />, '/tracking/:id': <p>destination page</p> },
    mocks: [
      meMock(),
      { request: { query: TrackingPlatformsDocument, variables: { channelId: 'ch1' } }, result: { data: { trackingPlatforms: [ga4Platform, metaNotInstalled] } }, maxUsageCount: 5 },
      {
        request: { query: CreateTrackingDestinationDocument, variables: captureInput(create) },
        result: { data: { createTrackingDestination: ga4Destination() } },
      },
    ],
  })
  return create
}

afterEach(() => localStorage.clear())

describe('TrackingSetupPage', () => {
  it('creates the destination off and in test mode, then opens it', async () => {
    const create = renderSetup('ga4')
    fireEvent.change(await screen.findByLabelText('Measurement ID', {}, SLOW), { target: { value: 'G-123' } })
    fireEvent.change(screen.getByLabelText('Measurement Protocol API secret'), { target: { value: 'secret-1' } })
    // The stored draft keeps the settings but never the secret
    await vi.waitFor(() => expect(JSON.stringify({ ...localStorage })).toContain('G-123'), SLOW)
    expect(JSON.stringify({ ...localStorage })).not.toContain('secret-1')
    fireEvent.click(screen.getByRole('button', { name: 'Save and continue' }))
    expect(await screen.findByText('destination page', {}, SLOW)).toBeDefined()
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: false,
        events: [],
        config: expect.arrayContaining([
          { key: 'measurement_id', value: 'G-123' },
          { key: 'api_secret', value: 'secret-1' },
          { key: 'validate', value: 'true' },
        ]),
      }),
    )
  })

  it('says how to add a platform that is not in this build', async () => {
    renderSetup('meta_capi')
    expect(await screen.findByText("Meta (Facebook, Instagram) isn't in this build", {}, SLOW)).toBeDefined()
    expect(screen.getByText('skelet cli plugin enable tracking-meta')).toBeDefined()
  })
})
