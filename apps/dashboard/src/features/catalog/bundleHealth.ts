import type { BundlePricing } from '@/graphql/schema.types'
import type { BundleOption, Selection } from './bundleStaging'

export type BundleListing = {
  isPublished: boolean
  effectivePrice?: number | null
  bundlePricing?: BundlePricing
  bundleDiscountPercent?: number
  channel: { id: string; name: string; currencyCode: string }
}

export type StorePricing = { mode: BundlePricing; percent: number }

// The cart's rule: fixed charges the listing's own price, the percent comes off the whole sum rounded half up
export function applyPricing(sum: number, p: StorePricing, fixed: number | null | undefined): number | null {
  if (p.mode === 'FIXED') return fixed ?? null
  if (p.mode === 'SUM_MINUS_PERCENT') return sum - Math.floor((sum * p.percent + 50) / 100)
  return sum
}

export type HealthIssue = { level: 'warning' | 'destructive'; text: string }

export type StoreHealth = {
  channelId: string
  name: string
  currencyCode: string
  issues: HealthIssue[]
  // Minor units in the store's currency, null when no option has anything buyable to price
  price: { min: number; max: number; default: number } | null
  // What the picks add up to before a FIXED or SUM_MINUS_PERCENT mode, null under SUM
  sum: { min: number; max: number } | null
}

const isSingle = (o: BundleOption) => o.inputType === 'RADIO' || o.inputType === 'DROPDOWN'
const label = (s: Selection) => s.product.name ?? s.product.sku ?? 'A selection'

// The add-to-cart rules from the cart service: listed, published, in stock and priced on the store. Unsaved picks carry no listings yet and are left out
function buyablePrice(s: Selection, channelId: string): number | null | 'unknown' {
  if (!s.product.listings) return 'unknown'
  const l = s.product.listings.find((x) => x.channelId === channelId)
  if (!l || !l.isPublished || !l.inStock || l.effectivePrice == null) return null
  return Math.round(l.effectivePrice * s.quantity)
}

function unavailableReason(s: Selection, channelId: string): string | null {
  const l = s.product.listings?.find((x) => x.channelId === channelId)
  if (!s.product.listings) return null
  if (!l || !l.isPublished) return 'isn’t sold on this store'
  if (!l.inStock) return 'is out of stock'
  if (l.effectivePrice == null) return 'has no price in this store’s currency'
  return null
}

// Problems that hold on every store, said once rather than per store
export function structureIssues(options: readonly BundleOption[]): HealthIssue[] {
  const out: HealthIssue[] = []
  for (const o of options) {
    if (o.isRequired && o.selections.length === 0) {
      out.push({ level: 'destructive', text: `“${o.title}” is required but has no selections - the bundle can’t be bought.` })
    } else if (!isSingle(o) && o.minSelections > o.selections.length) {
      out.push({ level: 'destructive', text: `“${o.title}” needs at least ${o.minSelections} picks but has ${o.selections.length} selection(s).` })
    }
  }
  return out
}

export function storeHealth(
  listings: readonly BundleListing[],
  options: readonly BundleOption[],
  // Staged pricing per store, over what the listing has saved
  pricingFor: (l: BundleListing) => StorePricing = (l) => ({ mode: l.bundlePricing ?? 'SUM', percent: l.bundleDiscountPercent ?? 0 }),
): StoreHealth[] {
  return listings.map((listing) => {
    const { isPublished, channel } = listing
    const issues: HealthIssue[] = []
    if (!isPublished) issues.push({ level: 'warning', text: 'The bundle itself is unpublished on this store.' })
    let min = 0
    let max = 0
    let def = 0
    let priced = false
    for (const o of options) {
      if (o.selections.length === 0) continue
      const prices: number[] = []
      let unknown = 0
      for (const s of o.selections) {
        const p = buyablePrice(s, channel.id)
        if (p === 'unknown') { unknown++; continue }
        if (p === null) {
          const why = unavailableReason(s, channel.id)
          issues.push(s.isDefault
            ? { level: 'warning', text: `Default ${label(s)} in “${o.title}” ${why} - customers open the bundle without it.` }
            : { level: 'warning', text: `${label(s)} in “${o.title}” ${why}.` })
          continue
        }
        prices.push(p)
        if (s.isDefault) def += p
      }
      const need = isSingle(o) ? (o.isRequired ? 1 : 0) : Math.max(o.minSelections, o.isRequired ? 1 : 0)
      const allow = isSingle(o) ? 1 : (o.maxSelections ?? prices.length)
      if (need > prices.length + unknown && need <= o.selections.length) {
        issues.push({ level: 'destructive', text: `“${o.title}” needs ${need} pick(s) but only ${prices.length} can be bought here - the bundle can’t be bought.` })
      }
      if (prices.length === 0) continue
      priced = true
      prices.sort((a, b) => a - b)
      min += prices.slice(0, need).reduce((a, b) => a + b, 0)
      max += prices.slice(-Math.min(allow, prices.length)).reduce((a, b) => a + b, 0)
    }
    const p = pricingFor(listing)
    if (p.mode === 'FIXED' && listing.effectivePrice == null) {
      issues.push({ level: 'destructive', text: 'Fixed price needs a store price in this store’s currency - set one under per-store pricing.' })
    }
    const at = (n: number) => applyPricing(n, p, listing.effectivePrice)
    const lo = at(min), hi = at(max), dflt = at(def)
    return {
      channelId: channel.id,
      name: channel.name,
      currencyCode: channel.currencyCode,
      issues,
      price: priced && lo !== null && hi !== null && dflt !== null ? { min: lo, max: hi, default: dflt } : null,
      sum: priced && p.mode !== 'SUM' ? { min, max } : null,
    }
  })
}
