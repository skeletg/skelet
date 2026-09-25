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
const dollars = money({ optional: true })

export const methodSchema = z.object({
  code: z.string().trim().min(1, 'Code is required'),
  name: z.string(),
  calcType: z.enum(['FLAT', 'FREE', 'FREE_ABOVE']),
  flatDollars: dollars,
  freeAboveDollars: dollars,
  sortOrder: wholeNumber(),
  isActive: z.boolean(),
  countryMode: z.enum(['all', 'specific']),
  allowedCountries: countryListText,
})

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
