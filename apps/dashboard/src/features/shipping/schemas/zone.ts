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

export const zoneSchema = z
  .object({
    code: z
      .string()
      .trim()
      .min(1, 'Code is required')
      .describe('Stable identifier carrier-integration plugins look the zone up by - survives renames'),
    name: z.string().describe('Display name of the zone - empty string for none'),
    countries: countryListText.describe('Comma-separated ISO 3166-1 alpha-2 codes of the countries in the zone, such as "US, CA"'),
    sortOrder: wholeNumber().describe('Order in zone lists - lower first'),
    isActive: z.boolean().describe('Whether the zone offers shipping'),
  })
  .describe('A shipping zone: a set of countries with its own shipping methods')

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
