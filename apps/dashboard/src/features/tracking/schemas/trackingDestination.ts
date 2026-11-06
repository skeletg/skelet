import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import type { TrackingDestinationInput } from '@/graphql/schema.types'
import { eventLabel } from '../platformGuides'

// A tracking destination's form - THE field list (docs/conventions.md, "Record forms"). The settings are the
// platform's own, so the schema is built from its fields. The secret is write-only: never shown back, so on an
// edit an empty value keeps the stored one, and only a new destination has to supply it

export type PlatformField = { key: string; label: string; help: string; required: boolean; secret: boolean }

export type TrackingDestinationValues = {
  name: string
  /** The platform's settings by key, the secret left out */
  settings: Record<string, string>
  /** The secret field's value - its own top-level field so the stored draft can leave it out. Empty keeps the stored one */
  secret: string
  /** True sends the platform's default events; false sends `events` */
  useDefaultEvents: boolean
  events: string[]
}

export type ExistingTrackingDestination = {
  name: string
  events: string[]
  config: { key: string; value: string }[]
}

export const SECRET_SET = 'set (hidden)'

export function trackingDestinationSchema(fields: PlatformField[], opts: { isEdit: boolean }) {
  return z
    .object({
      name: z.string().trim().min(1, 'Name is required').describe('Display name of the destination'),
      settings: z.record(z.string(), z.string()).describe("The platform's settings by key, without the secret"),
      secret: z.string().describe('The write-only secret - empty keeps the stored one'),
      useDefaultEvents: z.boolean().describe("Send the platform's default events"),
      events: z.array(z.string()).describe('Events to send when not using the defaults'),
    })
    .describe('A server-side tracking destination')
    .superRefine((v, ctx) => {
      for (const f of fields) {
        if (!f.required || (f.secret && opts.isEdit)) continue
        if ((f.secret ? v.secret : (v.settings[f.key] ?? '')).trim() === '') {
          ctx.addIssue({ code: 'custom', path: f.secret ? ['secret'] : ['settings', f.key], message: `${f.label} is required` })
        }
      }
      if (!v.useDefaultEvents && v.events.length === 0) {
        ctx.addIssue({ code: 'custom', path: ['events'], message: 'Choose at least one event, or send the defaults' })
      }
    })
}

export function toTrackingDestinationValues(fields: PlatformField[], d?: ExistingTrackingDestination | null, preset: Record<string, string> = {}): TrackingDestinationValues {
  const settings: Record<string, string> = {}
  for (const f of fields) if (!f.secret) settings[f.key] = d?.config.find((c) => c.key === f.key)?.value ?? preset[f.key] ?? ''
  return { name: d?.name ?? '', settings, secret: '', useDefaultEvents: (d?.events.length ?? 0) === 0, events: d?.events ?? [] }
}

export function toTrackingDestinationInput(fields: PlatformField[], v: TrackingDestinationValues, enabled?: boolean): TrackingDestinationInput {
  const config = fields
    .map((f) => ({ key: f.key, value: (f.secret ? v.secret : (v.settings[f.key] ?? '')).trim() }))
    // An empty secret keeps the stored one, so it isn't sent at all
    .filter((c) => !(fields.find((f) => f.key === c.key)?.secret && c.value === ''))
  return { name: v.name.trim(), events: v.useDefaultEvents ? [] : v.events, config, ...(enabled === undefined ? {} : { enabled }) }
}

export function trackingDestinationDiff(fields: PlatformField[], secretSet: boolean) {
  return (original: TrackingDestinationValues, draft: TrackingDestinationValues): SectionDiffEntry[] => {
    const out: SectionDiffEntry[] = fieldDiff(original, draft, [{ label: 'Name', value: (v) => v.name }])
    for (const f of fields) {
      const after = (f.secret ? draft.secret : (draft.settings[f.key] ?? '')).trim()
      if (f.secret) {
        // The value never reaches the dialog - only that it is being replaced
        const entry = after ? changed(f.label, secretSet ? SECRET_SET : '', secretSet ? 'replaced (hidden)' : 'set (hidden)') : null
        if (entry) out.push(entry)
        continue
      }
      const entry = changed(f.label, original.settings[f.key] ?? '', after)
      if (entry) out.push(entry)
    }
    const events = (v: TrackingDestinationValues) => (v.useDefaultEvents ? "The platform's defaults" : v.events.map(eventLabel).join(', '))
    const entry = changed('Events', events(original), events(draft))
    if (entry) out.push(entry)
    return out
  }
}
