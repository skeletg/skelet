import { z } from 'zod'
import { money } from '@/shared/schemas/fields'

export const MAX_PRICE_REASON = 200 // cart.MaxPriceReason

// A price an admin sets on a line of a shopper's own cart. The shopper checks out at it, so it always says why

export const setPriceSchema = z
  .object({
    price: money().describe('The unit price to set, in major units'),
    reason: z
      .string()
      .trim()
      .min(1, 'Say why the price is being set')
      .max(MAX_PRICE_REASON, `At most ${MAX_PRICE_REASON} characters`)
      .describe('Why - it stays with the line and the order it becomes'),
  })
  .describe('A price set on a line of a shopper cart')

export type SetPriceValues = z.infer<typeof setPriceSchema>

// The lowest price the store lets an admin set on the line, in cents - null when any price goes, or the line has no
// catalog price to measure from. Rounded up like the backend's own floor, so a price it shows is one it takes
export function priceFloor(catalogCents: number | null, limits: { maxCutPercent: number }): number | null {
  if (catalogCents === null || limits.maxCutPercent >= 100) return null
  return Math.ceil((catalogCents * (100 - Math.max(limits.maxCutPercent, 0))) / 100)
}
