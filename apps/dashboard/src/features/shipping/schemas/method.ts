import { z } from 'zod'
import { countryListText, formatCountryList, parseCountryList } from '@/shared/schemas/countries'
import { dollarsToCents, money, wholeNumber } from '@/shared/schemas/fields'

// Shipping method row - an inline CRUD row with its own Save (the conventions' allowed exception),
// validated by the same kind of schema as the record forms

export type CalcType = 'FLAT' | 'FREE' | 'FREE_ABOVE'

export type ExistingMethod = {
  id: string
  code: string
  name: string
  calcType: CalcType
  flatCents: number
  freeAboveCents: number
  sortOrder: number
  isActive: boolean
  countryMode?: string
  allowedCountries?: string[]
}

// Prices are typed in dollars as text and stored in cents
const dollars = (what: string) =>
  money({ optional: true }).describe(`${what} in major currency units written as text, such as "12.50" - empty string for 0`)

export const methodSchema = z
  .object({
    code: z.string().trim().min(1, 'Code is required').describe('Stable identifier of the method'),
    name: z.string().describe('Name shoppers see at checkout - empty string for none'),
    calcType: z
      .enum(['FLAT', 'FREE', 'FREE_ABOVE'])
      .describe('How the rate is worked out: FLAT charges flatDollars, FREE charges nothing, FREE_ABOVE charges flatDollars until the order reaches freeAboveDollars'),
    flatDollars: dollars('Flat rate charged, for FLAT and FREE_ABOVE'),
    freeAboveDollars: dollars('Order subtotal from which shipping is free, for FREE_ABOVE'),
    sortOrder: wholeNumber().describe('Order at checkout - lower first'),
    isActive: z.boolean().describe('Whether shoppers can pick the method'),
    countryMode: z.enum(['all', 'specific']).describe("Offer the method in every country of the zone, or only in allowedCountries"),
    allowedCountries: countryListText.describe(
      'Comma-separated ISO 3166-1 alpha-2 codes the method is limited to, such as "US, CA" - used only when countryMode is specific',
    ),
  })
  .describe('A shipping method offered within a shipping zone')

export type MethodValues = z.infer<typeof methodSchema>

export function toMethodValues(method?: ExistingMethod): MethodValues {
  return {
    code: method?.code ?? '',
    name: method?.name ?? '',
    calcType: method?.calcType ?? 'FLAT',
    flatDollars: method ? (method.flatCents / 100).toFixed(2) : '',
    freeAboveDollars: method ? (method.freeAboveCents / 100).toFixed(2) : '',
    sortOrder: method?.sortOrder ?? 0,
    isActive: method?.isActive ?? true,
    countryMode: method?.countryMode === 'specific' ? 'specific' : 'all',
    allowedCountries: formatCountryList(method?.allowedCountries),
  }
}

export function toMethodInput(v: MethodValues) {
  return {
    code: v.code.trim(),
    name: v.name,
    calcType: v.calcType,
    flatCents: dollarsToCents(v.flatDollars),
    freeAboveCents: dollarsToCents(v.freeAboveDollars),
    sortOrder: v.sortOrder,
    isActive: v.isActive,
    countryMode: v.countryMode,
    allowedCountries: v.countryMode === 'specific' ? parseCountryList(v.allowedCountries) : [],
  }
}
