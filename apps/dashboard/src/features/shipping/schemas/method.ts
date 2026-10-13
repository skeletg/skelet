import { z } from 'zod'
import { countryListText, formatCountryList, parseCountryList } from '@/shared/schemas/countries'
import { dollarsToCents, money, wholeNumber } from '@/shared/schemas/fields'

// Shipping method row - an inline CRUD row with its own Save (the conventions' allowed exception),
// validated by the same kind of schema as the record forms

export type CalcType = 'FLAT' | 'FREE' | 'FREE_ABOVE'
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
  }
}
