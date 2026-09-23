import { z } from 'zod'
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

export const countrySchema = z.object({
  // Uppercased as typed; the service keys everything on it, so it has to be exact ISO 3166-1 alpha-2
  code: z.string().regex(/^[A-Z]{2}$/, 'Use ISO 3166-1 alpha-2: exactly 2 uppercase letters (e.g. US, DE).'),
  name: z.string().trim().min(1, 'Name is required'),
  nativeName: z.string(),
  emojiFlag: z.string(),
  phoneCode: z.string(),
  defaultCurrencyCode: z.string(),
  isActive: z.boolean(),
  sortOrder: z.number().int('Whole numbers only'),
  metadata: metadataSchema,
})

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
