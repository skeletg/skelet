import { z } from 'zod'
import { callingCode } from '@/lib/phone'
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
  businessIdFields?: BusinessIdFieldValues[]
  regions: { code: string; name: string; isActive: boolean; sortOrder: number }[]
}

// One company identifier a business purchase from this country asks for at checkout - PIB, Matični broj, VAT number
export const businessIdFieldSchema = z
  .object({
    key: z
      .string()
      .regex(/^([a-z][a-z0-9_]{0,39})?$/, 'Lowercase letters, digits and underscores, starting with a letter')
      .describe('Stable slug the values are stored under - empty derives it from the label'),
    label: z.string().trim().min(1, 'Label is required').describe('What the shopper and the invoice read, such as PIB'),
    required: z.boolean().describe('Whether a business purchase must give it'),
    pattern: z
      .string()
      .refine((p) => {
        try {
          new RegExp(p)
          return true
        } catch {
          return false
        }
      }, "That pattern isn't a valid regular expression")
      .describe('A regular expression the whole value must match, such as \\d{9} - empty accepts anything'),
    hint: z.string().describe('Shown under the field at checkout, such as 9 digits'),
  })
  .describe('A company identifier asked for at checkout')

export type BusinessIdFieldValues = z.infer<typeof businessIdFieldSchema>

export const countrySchema = z
  .object({
    // Uppercased as typed; the service keys everything on it, so it has to be exact
    code: countryCode().describe('Upper-case ISO 3166-1 alpha-2 code, such as US - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the country'),
    nativeName: z.string().describe("Name in the country's own language - empty string for none"),
    emojiFlag: z.string().describe('Flag emoji, such as 🇺🇸 - empty string for none'),
    phoneCode: z.string().describe('International dialling code, such as +1 - derived from the code, read-only'),
    defaultCurrencyCode: z.string().describe('ISO 4217 code of the local currency, such as USD - empty string for none'),
    isActive: z.boolean().describe('Whether the country is available for use'),
    sortOrder: wholeNumber().describe('Order in country lists - lower first'),
    metadata: metadataSchema,
    businessIdFields: z.array(businessIdFieldSchema).describe('Company identifiers a business purchase asks for'),
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
  businessIdFields: 'Company ID fields',
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
    businessIdFields: (country?.businessIdFields ?? []).map((f) => ({
      key: f.key,
      label: f.label,
      required: f.required,
      pattern: f.pattern,
      hint: f.hint,
    })),
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
    // Derived from the code, as the backend does - only a code the metadata doesn't know keeps the stored one
    phoneCode: callingCode(v.code) ?? v.phoneCode.trim(),
    defaultCurrencyCode: v.defaultCurrencyCode,
    isActive: v.isActive,
    sortOrder: v.sortOrder,
    metadata: cleanMetadata(v.metadata, { trimValues: true }),
    businessIdFields: v.businessIdFields.map((f) => ({
      key: f.key.trim() || null,
      label: f.label.trim(),
      required: f.required,
      pattern: f.pattern.trim(),
      hint: f.hint.trim(),
    })),
  }
}

// "PIB (required, \d{9}) - 9 digits; Matični broj" - one readable line for the save dialog's before and after, carrying
// every property so a hint-only edit still shows as a change
function describeFields(fields: BusinessIdFieldValues[]): string {
  return fields
    .map((f) => {
      const extra = [f.key ? `key ${f.key}` : '', f.required ? 'required' : '', f.pattern].filter(Boolean).join(', ')
      return `${f.label.trim()}${extra ? ` (${extra})` : ''}${f.hint.trim() ? ` - ${f.hint.trim()}` : ''}`
    })
    .join('; ')
}

// Code is immutable after create, and the phone code follows it, so neither appears
const DIFFED = ['name', 'nativeName', 'emojiFlag', 'defaultCurrencyCode', 'isActive', 'sortOrder'] as const

export function countryDiff(original: CountryValues, draft: CountryValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      DIFFED.map((k) => ({ label: COUNTRY_LABELS[k], value: (o: CountryValues) => o[k] })),
    ),
    ...fieldDiff(original, draft, [
      { label: COUNTRY_LABELS.businessIdFields, value: (o: CountryValues) => describeFields(o.businessIdFields) },
    ]),
    ...metadataDiff(original.metadata, draft.metadata),
  ]
}
