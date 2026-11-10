import { describe, expect, it } from 'vitest'
import {
  toTrackingDestinationInput,
  toTrackingDestinationValues,
  trackingDestinationDiff,
  trackingDestinationSchema,
  type PlatformField,
} from './trackingDestination'

const fields: PlatformField[] = [
  { key: 'measurement_id', label: 'Measurement ID', help: '', required: true, secret: false },
  { key: 'api_secret', label: 'API secret', help: '', required: true, secret: true },
  { key: 'validate', label: 'Validate only', help: '', required: false, secret: false },
]
const existing = { name: 'GA4', events: [], config: [{ key: 'measurement_id', value: 'G-1' }, { key: 'validate', value: 'true' }] }

describe('tracking destination form', () => {
  it('never loads the secret back, and an edit keeps it when left empty', () => {
    const v = toTrackingDestinationValues(fields, existing)
    expect(v.secret).toBe('')
    expect('api_secret' in v.settings).toBe(false)
    expect(toTrackingDestinationInput(fields, v).config).toEqual([
      { key: 'measurement_id', value: 'G-1' },
      { key: 'validate', value: 'true' },
    ])
  })

  it('needs the secret on a new destination only', () => {
    const blank = toTrackingDestinationValues(fields, null, { validate: 'true' })
    blank.name = 'GA4'
    blank.settings.measurement_id = 'G-1'
    expect(blank.settings.validate).toBe('true')
    expect(trackingDestinationSchema(fields, { isEdit: false }).safeParse(blank).success).toBe(false)
    expect(trackingDestinationSchema(fields, { isEdit: true }).safeParse(blank).success).toBe(true)
  })

  it('shows a replaced secret without its value, and real changes only', () => {
    const before = toTrackingDestinationValues(fields, existing)
    const after = { ...before, settings: { ...before.settings, validate: '' }, secret: 'new-secret-value', useDefaultEvents: false, events: ['purchase', 'refund'] }
    const rows = trackingDestinationDiff(fields, true)(before, after)
    expect(rows.map((r) => r.label)).toEqual(['API secret', 'Validate only', 'Events'])
    expect(JSON.stringify(rows)).not.toContain('new-secret-value')
    expect(rows[2].after).toBe('Purchase, Refund')
    expect(trackingDestinationDiff(fields, true)(before, before)).toEqual([])
  })
})
