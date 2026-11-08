import { z } from 'zod'
import { countryListText, formatCountryList, parseCountryList } from '@/shared/schemas/countries'
import { dollarsToCents, money, numberText, parseNumberText, wholeNumber } from '@/shared/schemas/fields'

// Shipping method row - an inline CRUD row with its own Save (the conventions' allowed exception),
// validated by the same kind of schema as the record forms

export type CalcType = 'FLAT' | 'FREE' | 'FREE_ABOVE' | 'TABLE'
export type TableBasis = 'WEIGHT' | 'SUBTOTAL' | 'QUANTITY'
export type DeliveryKind = 'ship_to_address' | 'parcel_locker' | 'carrier_point'

export type ExistingTypeRate = {
  productTypeId: string
  productTypeName: string
  priceCents: number
  unitsPerCharge: number
  freeShippingEligible: boolean
}

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
  deliveryKind?: string
  unitsPerCharge?: number
  estimatedDays?: number
  deliversOnWeekends?: boolean
  typeRates?: ExistingTypeRate[]
  tableBasis?: TableBasis
  rateTiers?: { from: number; priceCents: number; notOffered: boolean }[]
}

// A tier's start is typed in the basis' own unit - kilograms, the channel's currency or items - and stored in grams,
// cents or units
export function tierUnit(basis: TableBasis, currency: string): string {
  if (basis === 'WEIGHT') return 'kg'
  if (basis === 'QUANTITY') return 'items'
  return currency || 'amount'
}

function tierFromToText(basis: TableBasis, from: number): string {
  if (basis === 'WEIGHT') return String(from / 1000)
  if (basis === 'SUBTOTAL') return (from / 100).toFixed(2)
  return String(from)
}

export function tierFromTextToValue(basis: TableBasis, text: string): number {
  const n = parseNumberText(text) ?? 0
  if (basis === 'WEIGHT') return Math.round(n * 1000)
  if (basis === 'SUBTOTAL') return Math.round(n * 100)
  return Math.round(n)
}

// Prices are typed in dollars as text and stored in cents
const dollars = (what: string) =>
  money({ optional: true }).describe(`${what} in major currency units written as text, such as "12.50" - empty string for 0`)

export const methodSchema = z
  .object({
    code: z.string().trim().min(1, 'Code is required').describe('Stable identifier of the method'),
    name: z.string().describe('Name shoppers see at checkout - empty string for none'),
    calcType: z
      .enum(['FLAT', 'FREE', 'FREE_ABOVE', 'TABLE'])
      .describe(
        'How the rate is worked out: FLAT charges flatDollars, FREE charges nothing, FREE_ABOVE charges flatDollars until the order reaches freeAboveDollars, TABLE charges the rateTiers row the shipment reaches',
      ),
    flatDollars: dollars('Flat rate charged, for FLAT and FREE_ABOVE'),
    freeAboveDollars: dollars('Order subtotal from which shipping is free, for FREE_ABOVE'),
    sortOrder: wholeNumber().describe('Order at checkout - lower first'),
    isActive: z.boolean().describe('Whether shoppers can pick the method'),
    countryMode: z.enum(['all', 'specific']).describe("Offer the method in every country of the zone, or only in allowedCountries"),
    allowedCountries: countryListText.describe(
      'Comma-separated ISO 3166-1 alpha-2 codes the method is limited to, such as "US, CA" - used only when countryMode is specific',
    ),
    deliveryKind: z
      .enum(['ship_to_address', 'parcel_locker', 'carrier_point'])
      .describe(
        'What the method prices: ship_to_address is door delivery, parcel_locker is a pickup point the shopper picks, carrier_point is the pickup point nearest the address, chosen by the carrier',
      ),
    unitsPerCharge: wholeNumber().describe('Charge the price again for every started bundle of this many units - 0 charges once per shipment'),
    estimatedDays: wholeNumber().min(0, 'Zero or more').describe('Delivery days the method takes, counted from the day after the order - 0 shows no arrival date'),
    deliversOnWeekends: z
      .boolean()
      .describe('Whether Saturday and Sunday count toward estimatedDays - off skips them, so a Friday order quoted 1 day arrives Monday'),
    typeRates: z
      .array(
        z.object({
          productTypeId: z.string().min(1, 'Choose a product type').describe('The product type this rate prices'),
          priceDollars: money().describe('Price for this product type in major currency units written as text'),
          unitsPerCharge: wholeNumber().describe('Charge again for every started bundle of this many units of the type - 0 charges once'),
          freeShippingEligible: z.boolean().describe("Whether the method's free-above threshold also waives this type's price"),
        }),
      )
      .refine((rates) => new Set(rates.map((r) => r.productTypeId)).size === rates.length, 'Each product type can have one rate')
      .describe("Prices for particular product types - their items are priced by their row instead of the method's price"),
    tableBasis: z
      .enum(['WEIGHT', 'SUBTOTAL', 'QUANTITY'])
      .describe("What a TABLE method's tiers measure: WEIGHT in kilograms, SUBTOTAL in dollars after discounts, QUANTITY in items"),
    rateTiers: z
      .array(
        z.object({
          from: numberText({ min: 0 }).describe("Where the tier starts, in the basis' unit written as text"),
          priceDollars: money({ optional: true }).describe('Price of a shipment in this tier, in major currency units written as text'),
          notOffered: z.boolean().describe('Whether the method stops being offered from this tier up - its price is then ignored'),
        }),
      )
      .describe(
        'Tiers of a TABLE method - a shipment pays the last tier it reaches, below the first the method is not offered, and a notOffered tier stops it from there up',
      ),
  })
  .superRefine((v, ctx) => {
    if (v.calcType !== 'TABLE') return
    if (v.rateTiers.length === 0) {
      ctx.addIssue({ code: 'custom', path: ['rateTiers'], message: 'Add at least one tier' })
      return
    }
    const starts = v.rateTiers.map((t) => tierFromTextToValue(v.tableBasis, t.from))
    if (new Set(starts).size !== starts.length) {
      ctx.addIssue({ code: 'custom', path: ['rateTiers'], message: 'Each tier starts at a different value' })
    }
    if (v.tableBasis === 'QUANTITY' && v.rateTiers.some((t) => !Number.isInteger(parseNumberText(t.from) ?? 0))) {
      ctx.addIssue({ code: 'custom', path: ['rateTiers'], message: 'Item counts are whole numbers' })
    }
    if (v.rateTiers.some((t) => !t.notOffered && t.priceDollars.trim() === '')) {
      ctx.addIssue({ code: 'custom', path: ['rateTiers'], message: 'Give each offered tier a price - 0 for free' })
    }
    if (v.rateTiers.every((t) => t.notOffered)) {
      ctx.addIssue({ code: 'custom', path: ['rateTiers'], message: 'At least one tier has to be offered' })
    }
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
    deliveryKind: method?.deliveryKind === 'parcel_locker' || method?.deliveryKind === 'carrier_point' ? method.deliveryKind : 'ship_to_address',
    unitsPerCharge: method?.unitsPerCharge ?? 0,
    estimatedDays: method?.estimatedDays ?? 0,
    deliversOnWeekends: method?.deliversOnWeekends ?? false,
    typeRates: (method?.typeRates ?? []).map((r) => ({
      productTypeId: r.productTypeId,
      priceDollars: (r.priceCents / 100).toFixed(2),
      unitsPerCharge: r.unitsPerCharge,
      freeShippingEligible: r.freeShippingEligible,
    })),
    tableBasis: method?.tableBasis ?? 'WEIGHT',
    rateTiers: (method?.rateTiers ?? []).map((t) => ({
      from: tierFromToText(method?.tableBasis ?? 'WEIGHT', t.from),
      priceDollars: t.notOffered ? '' : (t.priceCents / 100).toFixed(2),
      notOffered: t.notOffered,
    })),
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
    deliveryKind: v.deliveryKind,
    unitsPerCharge: v.unitsPerCharge,
    estimatedDays: v.estimatedDays,
    deliversOnWeekends: v.deliversOnWeekends,
    typeRates: v.typeRates.map((r) => ({
      productTypeId: r.productTypeId,
      priceCents: dollarsToCents(r.priceDollars),
      unitsPerCharge: r.unitsPerCharge,
      freeShippingEligible: r.freeShippingEligible,
    })),
    tableBasis: v.tableBasis,
    rateTiers:
      v.calcType === 'TABLE'
        ? v.rateTiers.map((t) => ({
            from: tierFromTextToValue(v.tableBasis, t.from),
            priceCents: t.notOffered ? 0 : dollarsToCents(t.priceDollars),
            notOffered: t.notOffered,
          }))
        : [],
  }
}
