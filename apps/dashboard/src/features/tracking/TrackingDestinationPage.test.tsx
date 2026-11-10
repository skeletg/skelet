import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { TrackingDestinationPage } from './TrackingDestinationPage'
import { SendTrackingTestDocument, TrackingDeliveriesDocument, TrackingDestinationDocument, TrackingPlatformsDocument, UpdateTrackingDestinationDocument } from './Tracking.gql'
import { ga4Destination, ga4Platform } from './fixtures.test-data'

// What these pin: edits go through the reviewed Save and never carry an untouched secret, going live drops the test
// setting in the same write, a test send outside test mode is confirmed first, and a refused one shows the platform's
// answer

function renderDestination(dest = ga4Destination(), writes = { update: vi.fn() }, extra: Parameters<typeof renderPage>[0]['mocks'] = []) {
  renderPage({
    path: '/tracking/d1',
    adminId: 'admin-1',
    routes: { '/tracking/:id': <TrackingDestinationPage /> },
    mocks: [
      meMock(),
      { request: { query: TrackingDestinationDocument, variables: { id: 'd1' } }, result: { data: { trackingDestination: dest } }, maxUsageCount: 5 },
      { request: { query: TrackingPlatformsDocument, variables: { channelId: 'ch1' } }, result: { data: { trackingPlatforms: [ga4Platform] } }, maxUsageCount: 5 },
      { request: { query: TrackingDeliveriesDocument, variables: { destinationId: 'd1', status: null, limit: 100 } }, result: { data: { trackingDeliveries: [] } }, maxUsageCount: 5 },
      {
        request: { query: UpdateTrackingDestinationDocument, variables: captureInput(writes.update) },
        result: { data: { updateTrackingDestination: dest } },
        maxUsageCount: 2,
      },
      ...extra,
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('TrackingDestinationPage', () => {
  it('reviews a settings change and saves it without the untouched secret', async () => {
    const writes = renderDestination()
    fireEvent.change(await screen.findByLabelText('Measurement ID', {}, SLOW), { target: { value: 'G-NEW' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('G-NEW')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    const input = writes.update.mock.calls[0][0] as { config: { key: string; value: string }[]; enabled?: boolean }
    expect(input.config).toContainEqual({ key: 'measurement_id', value: 'G-NEW' })
    expect(input.config.some((c) => c.key === 'api_secret')).toBe(false)
    expect(input.enabled).toBeUndefined()
  })

  it('leaves test mode and turns on in one write', async () => {
    const writes = renderDestination()
    fireEvent.click(await screen.findByRole('button', { name: 'Leave test mode and turn on' }, SLOW))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    const input = writes.update.mock.calls[0][0] as { config: { key: string; value: string }[]; enabled?: boolean }
    expect(input.enabled).toBe(true)
    expect(input.config).toContainEqual({ key: 'validate', value: '' })
  })

  it("shows the platform's answer when it refuses the test", async () => {
    renderDestination(ga4Destination(), { update: vi.fn() }, [
      { request: { query: SendTrackingTestDocument, variables: { id: 'd1' } }, result: { data: { sendTrackingTest: { __typename: 'TrackingTestResult', ok: false, message: 'api_secret=[redacted] rejected' } } } },
    ])
    fireEvent.click(await screen.findByRole('button', { name: 'Send test' }, SLOW))
    expect(await screen.findByText('Google Analytics 4 refused the test', {}, SLOW)).toBeDefined()
    expect(screen.getByText('api_secret=[redacted] rejected')).toBeDefined()
  })

  it('asks before a test send that a live platform would record as a real purchase', async () => {
    const sent = vi.fn(() => ({ data: { sendTrackingTest: { __typename: 'TrackingTestResult', ok: true, message: '' } } }))
    renderDestination(ga4Destination({ enabled: true, inTestMode: false }), { update: vi.fn() }, [
      { request: { query: SendTrackingTestDocument, variables: { id: 'd1' } }, result: sent },
    ])
    fireEvent.click(await screen.findByRole('button', { name: 'Send test' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Send a real test purchase?')).toBeDefined()
    expect(sent).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Send test' }))
    await vi.waitFor(() => expect(sent).toHaveBeenCalled(), SLOW)
  })

  it('asks for the secret a destination is missing and keeps it switched off', async () => {
    renderDestination(ga4Destination({ needsSecret: true, secretSet: false }))
    expect(await screen.findByText(/Enter it below and save before turning this destination on/, {}, SLOW)).toBeDefined()
    expect((screen.getByRole('switch', { name: /Off/ }) as HTMLButtonElement).disabled).toBe(true)
  })
})
