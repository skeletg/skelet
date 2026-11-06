import { describe, expect, it } from 'vitest'
import { destinationStatus, platformStatus, type Destination, type Platform } from './status'

const platform = (over: Partial<Platform> = {}): Platform => ({
  kind: 'ga4', label: 'Google Analytics 4', pluginSlug: 'tracking-ga4', installed: true, availableOnChannel: true,
  purposes: ['analytics'], defaultEvents: [], necessarySetting: null, fields: [], ...over,
})
const dest = (over: Partial<Destination> = {}): Destination => ({
  id: 'd1', channelId: 'c1', kind: 'ga4', name: 'GA4', enabled: true, events: [], effectiveEvents: [], config: [],
  secretSet: true, needsSecret: false, inTestMode: false, importedFrom: null, createdAt: '', updatedAt: '',
  health: { lastSentAt: null, lastError: '', dead24h: 0 }, ...over,
})

describe('tracking status', () => {
  it('says why a platform can not be used before anything else', () => {
    expect(platformStatus(platform({ installed: false }), [dest()]).kind).toBe('not-installed')
    expect(platformStatus(platform({ availableOnChannel: false }), []).kind).toBe('off-on-channel')
    expect(platformStatus(platform(), []).kind).toBe('not-set-up')
  })

  it('surfaces the most pressing destination', () => {
    expect(platformStatus(platform(), [dest(), dest({ needsSecret: true })]).kind).toBe('needs-secret')
    expect(platformStatus(platform(), [dest({ enabled: false }), dest({ health: { lastSentAt: null, lastError: 'x', dead24h: 2 } })]).kind).toBe('errors')
    expect(platformStatus(platform(), [dest({ enabled: false }), dest()]).kind).toBe('enabled')
  })

  it('only counts failures on a destination that is on', () => {
    expect(destinationStatus(dest({ enabled: false, health: { lastSentAt: null, lastError: 'x', dead24h: 3 } })).kind).toBe('disabled')
  })
})
