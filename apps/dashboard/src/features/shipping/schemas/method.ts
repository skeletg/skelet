import { z } from 'zod'
import { countryListText, formatCountryList, parseCountryList } from '@/shared/schemas/countries'

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
const dollars = z
  .string()
  .refine((v) => v.trim() === '' || (Number.isFinite(Number(v)) && Number(v) >= 0), 'Must be 0 or more')

export const methodSchema = z.object({
  code: z.string().trim().min(1, 'Code is required'),
  name: z.string(),
  calcType: z.enum(['FLAT', 'FREE', 'FREE_ABOVE']),
  flatDollars: dollars,
  freeAboveDollars: dollars,
  sortOrder: z.number().int('Whole numbers only'),
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

const toCents = (v: string) => Math.round((Number(v) || 0) * 100)

export function toMethodInput(v: MethodValues) {
  return {
    code: v.code.trim(),
    name: v.name,
    calcType: v.calcType,
    flatCents: toCents(v.flatDollars),
    freeAboveCents: toCents(v.freeAboveDollars),
    sortOrder: v.sortOrder,
    isActive: v.isActive,
    countryMode: v.countryMode,
    allowedCountries: v.countryMode === 'specific' ? parseCountryList(v.allowedCountries) : [],
  }
}
