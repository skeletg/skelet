import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { countryListText, formatCountryList, parseCountryList } from '@/shared/schemas/countries'

// Shipping zone form - THE field list (docs/conventions.md, "Record forms"). Its methods are CRUD
// rows with their own Save (./method.ts), not part of this form

export type ExistingZone = {
  id: string
  channelId: string
  code: string
  name: string
  sortOrder: number
  isActive: boolean
  countries: string[]
}

export const zoneSchema = z.object({
  // Stable across renames - carrier-integration plugins look zones up by it
  code: z.string().trim().min(1, 'Code is required'),
  name: z.string(),
  countries: countryListText,
  sortOrder: wholeNumber(),
  isActive: z.boolean(),
})

export type ZoneValues = z.infer<typeof zoneSchema>

export const ZONE_LABELS: Record<keyof ZoneValues, string> = {
  code: 'Code',
  name: 'Display name',
  countries: 'Countries',
  sortOrder: 'Sort order',
  isActive: 'Active',
}

export function toZoneValues(zone?: ExistingZone): ZoneValues {
  return {
    code: zone?.code ?? '',
    name: zone?.name ?? '',
    countries: formatCountryList(zone?.countries),
    sortOrder: zone?.sortOrder ?? 0,
    isActive: zone?.isActive ?? true,
  }
}

export function toZoneInput(v: ZoneValues) {
  return {
    code: v.code.trim(),
    name: v.name,
    sortOrder: v.sortOrder,
    isActive: v.isActive,
    countries: parseCountryList(v.countries),
  }
}

export function zoneDiff(original: ZoneValues, draft: ZoneValues): SectionDiffEntry[] {
  return fieldDiff(original, draft, [
    { label: ZONE_LABELS.code, value: (o) => o.code },
    { label: ZONE_LABELS.name, value: (o) => o.name },
    // Compared as the parsed list, so "us,ca" vs "US, CA" isn't a change
    { label: ZONE_LABELS.countries, value: (o) => parseCountryList(o.countries) },
    { label: ZONE_LABELS.sortOrder, value: (o) => o.sortOrder },
    { label: ZONE_LABELS.isActive, value: (o) => o.isActive },
  ])
}
