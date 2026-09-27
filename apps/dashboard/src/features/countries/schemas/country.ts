import { z } from 'zod'
import { countryCode, wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import {
  cleanMetadata,
  metadataDiff,
  metadataSchema,
  toMetadataValues,
  type MetadataEntry,
} from '@/shared/schemas/metadata'

// Country form - THE field list (docs/conventions.md, "Record forms")

export type ExistingCountry = {
  code: string
  name: string
  nativeName: string
  emojiFlag: string
  phoneCode: string
  iso3Code: string
  numericCode: string
  defaultCurrencyCode: string
  isActive: boolean
  sortOrder: number
  metadata: MetadataEntry[]
  regions: { code: string; name: string; isActive: boolean; sortOrder: number }[]
}

export const countrySchema = z
  .object({
    // Uppercased as typed; the service keys everything on it, so it has to be exact
    code: countryCode().describe('Upper-case ISO 3166-1 alpha-2 code, such as US - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the country'),
    nativeName: z.string().describe("Name in the country's own language - empty string for none"),
    emojiFlag: z.string().describe('Flag emoji, such as 🇺🇸 - empty string for none'),
    phoneCode: z.string().describe('International dialling code, such as +1 - empty string for none'),
    defaultCurrencyCode: z.string().describe('ISO 4217 code of the local currency, such as USD - empty string for none'),
    isActive: z.boolean().describe('Whether the country is available for use'),
    sortOrder: wholeNumber().describe('Order in country lists - lower first'),
    metadata: metadataSchema,
  })
  .describe('A country')

export type CountryValues = z.infer<typeof countrySchema>

export const COUNTRY_LABELS: Record<keyof CountryValues, string> = {
  code: 'Code',
  name: 'Name',
  nativeName: 'Native name',
  emojiFlag: 'Flag',
  phoneCode: 'Phone code',
  defaultCurrencyCode: 'Default currency',
  isActive: 'Active',
  sortOrder: 'Sort order',
  metadata: 'Metadata',
}

export function toCountryValues(country?: ExistingCountry): CountryValues {
  return {
    code: country?.code ?? '',
    name: country?.name ?? '',
    nativeName: country?.nativeName ?? '',
    emojiFlag: country?.emojiFlag ?? '',
    phoneCode: country?.phoneCode ?? '',
    defaultCurrencyCode: country?.defaultCurrencyCode ?? '',
    isActive: country?.isActive ?? true,
    sortOrder: country?.sortOrder ?? 0,
    metadata: toMetadataValues(country?.metadata),
  }
}

// Sent as-is on update too: the service stores '' as empty (and NULLs an empty currency), so
// clearing a field actually clears it - the old form sent undefined and the clear never landed
export function toCountryInput(v: CountryValues) {
  return {
    code: v.code,
    name: v.name.trim(),
    nativeName: v.nativeName.trim(),
    emojiFlag: v.emojiFlag.trim(),
    phoneCode: v.phoneCode.trim(),
    defaultCurrencyCode: v.defaultCurrencyCode,
    isActive: v.isActive,
    sortOrder: v.sortOrder,
    metadata: cleanMetadata(v.metadata, { trimValues: true }),
  }
}

// Code is immutable after create, so it never appears
const DIFFED = ['name', 'nativeName', 'emojiFlag', 'phoneCode', 'defaultCurrencyCode', 'isActive', 'sortOrder'] as const

export function countryDiff(original: CountryValues, draft: CountryValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      DIFFED.map((k) => ({ label: COUNTRY_LABELS[k], value: (o: CountryValues) => o[k] })),
    ),
    ...metadataDiff(original.metadata, draft.metadata),
  ]
}
