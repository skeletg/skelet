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

// Locale form - THE field list (docs/conventions.md, "Record forms")

export type ExistingLocale = {
  code: string
  name: string
  nativeName: string
  description: string
  isActive: boolean
  isRTL: boolean
  sortOrder: number
  flagEmoji: string
  dateFormat: string
  timeFormat: string
  decimalSeparator: string
  thousandsSeparator: string
  firstDayOfWeek: number
  defaultCountryCode: string
  hreflangValue: string
  script: string
  pluralRule: string
  emailFromNameOverride: string
  metadata: MetadataEntry[]
}

export const WEEKDAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

export const localeSchema = z.object({
  // Lowercased as typed - BCP 47 style, the service keys on it
  code: z
    .string()
    .min(1, 'Code is required')
    .regex(
      /^[a-z]{2,3}(-[a-z0-9]{2,8})*$/,
      'Use BCP-47 style: lowercase, 2-3 letters then optional -region (e.g. en, es-mx, pt-br).',
    ),
  name: z.string().trim().min(1, 'Name is required'),
  nativeName: z.string(),
  description: z.string(),
  isActive: z.boolean(),
  isRTL: z.boolean(),
  sortOrder: wholeNumber(),
  flagEmoji: z.string(),
  dateFormat: z.string(),
  timeFormat: z.string(),
  decimalSeparator: z.string().max(1, 'One character'),
  thousandsSeparator: z.string().max(1, 'One character'),
  firstDayOfWeek: z.number().int().min(0, 'First day of week must be 0..6.').max(6, 'First day of week must be 0..6.'),
  defaultCountryCode: countryCode({ optional: true }),
  hreflangValue: z.string(),
  script: z.string(),
  pluralRule: z.string(),
  emailFromNameOverride: z.string(),
  metadata: metadataSchema,
})

export type LocaleValues = z.infer<typeof localeSchema>

export const LOCALE_LABELS: Record<keyof LocaleValues, string> = {
  code: 'Code',
  name: 'Name',
  nativeName: 'Native name',
  description: 'Description',
  isActive: 'Active',
  isRTL: 'Right to left',
  sortOrder: 'Sort order',
  flagEmoji: 'Flag',
  dateFormat: 'Date format',
  timeFormat: 'Time format',
  decimalSeparator: 'Decimal separator',
  thousandsSeparator: 'Thousands separator',
  firstDayOfWeek: 'First day of week',
  defaultCountryCode: 'Default country',
  hreflangValue: 'hreflang value',
  script: 'Script',
  pluralRule: 'Plural rule',
  emailFromNameOverride: 'Email from-name override',
  metadata: 'Metadata',
}

export function toLocaleValues(locale?: ExistingLocale): LocaleValues {
  return {
    code: locale?.code ?? '',
    name: locale?.name ?? '',
    nativeName: locale?.nativeName ?? '',
    description: locale?.description ?? '',
    isActive: locale?.isActive ?? true,
    isRTL: locale?.isRTL ?? false,
    sortOrder: locale?.sortOrder ?? 0,
    flagEmoji: locale?.flagEmoji ?? '',
    dateFormat: locale?.dateFormat ?? '',
    timeFormat: locale?.timeFormat ?? '',
    decimalSeparator: locale?.decimalSeparator ?? '.',
    thousandsSeparator: locale?.thousandsSeparator ?? ',',
    firstDayOfWeek: locale?.firstDayOfWeek ?? 1,
    defaultCountryCode: locale?.defaultCountryCode ?? '',
    hreflangValue: locale?.hreflangValue ?? '',
    script: locale?.script ?? '',
    pluralRule: locale?.pluralRule ?? '',
    emailFromNameOverride: locale?.emailFromNameOverride ?? '',
    metadata: toMetadataValues(locale?.metadata),
  }
}

export function toLocaleInput(v: LocaleValues) {
  return {
    code: v.code,
    name: v.name.trim(),
    // Left out when empty - the service fills it from Name on create
    nativeName: v.nativeName || undefined,
    description: v.description,
    isActive: v.isActive,
    isRTL: v.isRTL,
    sortOrder: v.sortOrder,
    flagEmoji: v.flagEmoji,
    dateFormat: v.dateFormat,
    timeFormat: v.timeFormat,
    decimalSeparator: v.decimalSeparator || '.',
    thousandsSeparator: v.thousandsSeparator || ',',
    firstDayOfWeek: v.firstDayOfWeek,
    defaultCountryCode: v.defaultCountryCode,
    hreflangValue: v.hreflangValue,
    script: v.script,
    pluralRule: v.pluralRule,
    emailFromNameOverride: v.emailFromNameOverride,
    metadata: cleanMetadata(v.metadata, { trimValues: true }),
  }
}

// The service's rejection messages, routed back to the field they're about - anything unrecognised
// stays a general message in the dialog
export function localeFieldErrors(message: string): Partial<Record<keyof LocaleValues, string>> {
  const out: Partial<Record<keyof LocaleValues, string>> = {}
  if (/first[_ ]?day[_ ]?of[_ ]?week/i.test(message) && /must be 0\.\.6/i.test(message)) {
    out.firstDayOfWeek = 'First day of week must be 0..6.'
  }
  if (/default[_ ]?country[_ ]?code/i.test(message) && /ISO 3166-1 alpha-2/i.test(message)) {
    out.defaultCountryCode = 'Must be empty or a 2-letter ISO 3166-1 country code.'
  }
  if (/hreflang[_ ]?value/i.test(message) && /BCP[_ ]?47/i.test(message)) {
    out.hreflangValue = "Doesn't look like a BCP 47 code."
  }
  return out
}

// Code is immutable after create, so it never appears
const DIFFED = (Object.keys(LOCALE_LABELS) as (keyof LocaleValues)[]).filter((k) => k !== 'code' && k !== 'metadata')

export function localeDiff(original: LocaleValues, draft: LocaleValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      DIFFED.map((k) => ({
        label: LOCALE_LABELS[k],
        value: (o: LocaleValues) => (k === 'firstDayOfWeek' ? WEEKDAY_LABELS[o.firstDayOfWeek] : o[k]),
      })),
    ),
    ...metadataDiff(original.metadata, draft.metadata),
  ]
}
