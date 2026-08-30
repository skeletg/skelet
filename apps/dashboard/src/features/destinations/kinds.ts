// Kind values mirror the backend CHECK constraint on event_destinations
export const DESTINATION_KINDS = [
  {
    value: 'webhook',
    label: 'Webhook',
    description: 'Signed JSON batches to any HTTPS endpoint.',
  },
  {
    value: 'ga4_mp',
    label: 'GA4',
    description: 'Google Analytics 4 Measurement Protocol.',
  },
  {
    value: 'meta_capi',
    label: 'Meta CAPI',
    description: 'Meta (Facebook) Conversions API.',
  },
] as const
