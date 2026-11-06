// Shared fixtures for the tracking page tests
export const ga4Platform = {
  __typename: 'TrackingPlatform' as const,
  kind: 'ga4',
  label: 'Google Analytics 4',
  pluginSlug: 'tracking-ga4',
  installed: true,
  availableOnChannel: true,
  purposes: ['analytics'],
  defaultEvents: ['page_view', 'purchase'],
  necessarySetting: null,
  fields: [
    { __typename: 'TrackingField' as const, key: 'measurement_id', label: 'Measurement ID', help: 'G-XXXX, from the web data stream', required: true, secret: false, testMode: false },
    { __typename: 'TrackingField' as const, key: 'api_secret', label: 'Measurement Protocol API secret', help: '', required: true, secret: true, testMode: false },
    { __typename: 'TrackingField' as const, key: 'endpoint', label: 'Endpoint', help: '', required: false, secret: false, testMode: false },
    { __typename: 'TrackingField' as const, key: 'validate', label: 'Validate only', help: '', required: false, secret: false, testMode: true },
  ],
}

export const metaNotInstalled = {
  __typename: 'TrackingPlatform' as const,
  kind: 'meta_capi',
  label: 'Meta (Facebook, Instagram)',
  pluginSlug: 'tracking-meta',
  installed: false,
  availableOnChannel: false,
  purposes: [],
  defaultEvents: [],
  necessarySetting: null,
  fields: [],
}

export function ga4Destination(over: Record<string, unknown> = {}) {
  return {
    __typename: 'TrackingDestination' as const,
    id: 'd1',
    channelId: 'ch1',
    kind: 'ga4',
    name: 'GA4 main',
    enabled: false,
    events: [],
    effectiveEvents: ['page_view', 'purchase'],
    config: [
      { __typename: 'TrackingConfigValue' as const, key: 'measurement_id', value: 'G-OLD' },
      { __typename: 'TrackingConfigValue' as const, key: 'validate', value: 'true' },
    ],
    secretSet: true,
    needsSecret: false,
    inTestMode: true,
    importedFrom: null,
    health: { __typename: 'TrackingHealth' as const, lastSentAt: null, lastError: '', dead24h: 0 },
    createdAt: '2026-11-01T00:00:00Z',
    updatedAt: '2026-11-01T00:00:00Z',
    ...over,
  }
}
