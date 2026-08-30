// Categorisation and icon mapping for promotion condition/action types, so the rule editor can
// group the type picker instead of showing one flat dropdown
// Categories are inferred from the slug prefix, which covers every shipped core.* type without
// backend changes - plugin slugs off-convention land in "Other", and can mimic the prefix to slot in

import type { LucideIcon } from 'lucide-react'
import {
  BlendIcon,
  CalendarClockIcon,
  GiftIcon,
  HelpCircleIcon,
  LayersIcon,
  MapPinIcon,
  PackageIcon,
  PercentIcon,
  ShoppingCartIcon,
  StoreIcon,
  TagIcon,
  TruckIcon,
  UserIcon,
  WandSparklesIcon,
} from 'lucide-react'

export type CategoryKey =
  | 'cart'
  | 'customer'
  | 'product'
  | 'location'
  | 'time'
  | 'combine'
  | 'discount'
  | 'discount-line'
  | 'shipping'
  | 'gift'
  | 'cap'
  | 'other'

export type CategoryMeta = {
  key: CategoryKey
  label: string
  icon: LucideIcon
}

export const CATEGORIES: Record<CategoryKey, CategoryMeta> = {
  cart:           { key: 'cart',           label: 'Cart',                icon: ShoppingCartIcon },
  customer:       { key: 'customer',       label: 'Customer',            icon: UserIcon },
  product:        { key: 'product',        label: 'Products',            icon: PackageIcon },
  location:       { key: 'location',       label: 'Location',            icon: MapPinIcon },
  time:           { key: 'time',           label: 'Time',                icon: CalendarClockIcon },
  combine:        { key: 'combine',        label: 'Combine (AND/OR/NOT)', icon: BlendIcon },
  discount:       { key: 'discount',       label: 'Cart discount',       icon: PercentIcon },
  'discount-line':{ key: 'discount-line',  label: 'Line discount',       icon: TagIcon },
  shipping:       { key: 'shipping',       label: 'Shipping',            icon: TruckIcon },
  gift:           { key: 'gift',           label: 'Free gift',           icon: GiftIcon },
  cap:            { key: 'cap',            label: 'Cap / limit',         icon: LayersIcon },
  other:          { key: 'other',          label: 'Other',               icon: HelpCircleIcon },
}

// Falls back to "other" for anything unrecognised
export function categorize(slug: string, kind: 'condition' | 'action'): CategoryMeta {
  if (kind === 'condition') {
    if (slug === 'core.all_of' || slug === 'core.any_of' || slug === 'core.not') return CATEGORIES.combine
    if (slug === 'core.schedule') return CATEGORIES.time
    if (slug.startsWith('core.cart_')) return CATEGORIES.cart
    if (slug.startsWith('core.customer_')) return CATEGORIES.customer
    if (slug.startsWith('core.product_')) return CATEGORIES.product
    if (slug.startsWith('core.country_') || slug.startsWith('core.store_')) return CATEGORIES.location
    return CATEGORIES.other
  }
  // actions
  if (slug === 'core.cap_total_discount') return CATEGORIES.cap
  if (slug.startsWith('core.percent_off_matching') || slug.startsWith('core.fixed_off_matching') || slug === 'core.bogo') {
    return CATEGORIES['discount-line']
  }
  if (slug.startsWith('core.percent_off_subtotal') || slug.startsWith('core.fixed_off_subtotal')) {
    return CATEGORIES.discount
  }
  if (slug.startsWith('core.free_shipping')) return CATEGORIES.shipping
  if (slug.startsWith('core.add_free_gift')) return CATEGORIES.gift
  return CATEGORIES.other
}

// Keeps the most commonly used groups near the top of the picker
export const CONDITION_CATEGORY_ORDER: CategoryKey[] = [
  'cart',
  'customer',
  'product',
  'location',
  'time',
  'combine',
  'other',
]

export const ACTION_CATEGORY_ORDER: CategoryKey[] = [
  'discount',
  'discount-line',
  'shipping',
  'gift',
  'cap',
  'other',
]

// Short scannable label for a collapsed row header, so a filled rule reads "Cart subtotal >= $50"
// rather than the bare slug - falls back to the type's label when params are empty
export function summariseType(slug: string, label: string, params: unknown): string {
  const p = (params && typeof params === 'object' && !Array.isArray(params)) ? params as Record<string, unknown> : {}
  switch (slug) {
    case 'core.cart_subtotal_range': {
      const min = num(p.min_cents)
      const max = num(p.max_cents)
      if (min != null && max != null) return `Cart subtotal $${min / 100}–$${max / 100}`
      if (min != null) return `Cart subtotal ≥ $${min / 100}`
      if (max != null) return `Cart subtotal ≤ $${max / 100}`
      return label
    }
    case 'core.cart_qty_range': {
      const min = num(p.min)
      const max = num(p.max)
      if (min != null && max != null) return `Cart qty ${min}–${max}`
      if (min != null) return `Cart qty ≥ ${min}`
      if (max != null) return `Cart qty ≤ ${max}`
      return label
    }
    case 'core.customer_group_in':
      return `Customer in: ${listOrLabel(p.group_slugs, label)}`
    case 'core.customer_in_segment':
      return `Segment: ${listOrLabel(p.slugs, label)}`
    case 'core.country_in':
      return `Ship to: ${listOrLabel(p.codes, label)}`
    case 'core.channel_in':
      return `Store: ${listOrLabel(p.slugs, label)}`
    case 'core.product_kind':
      return `Product kind: ${listOrLabel(p.kinds, label)}`
    case 'core.cart_contains_sku':
      return `Contains SKU: ${listOrLabel(p.skus, label)}`
    case 'core.percent_off_subtotal': {
      const bps = num(p.bps)
      return bps != null ? `${bps / 100}% off cart` : label
    }
    case 'core.fixed_off_subtotal': {
      const cents = num(p.cents)
      return cents != null ? `$${cents / 100} off cart` : label
    }
    case 'core.percent_off_matching_items': {
      const bps = num(p.bps)
      return bps != null ? `${bps / 100}% off matching items` : label
    }
    case 'core.fixed_off_matching_items': {
      const cents = num(p.cents_per_unit)
      return cents != null ? `$${cents / 100} off each matching unit` : label
    }
    case 'core.bogo': {
      const buy = num(p.buy_qty)
      const get = num(p.get_qty)
      const kind = String(p.get_kind ?? '')
      if (buy && get) return `Buy ${buy}, get ${get}${kind ? ` (${kind})` : ''}`
      return label
    }
    case 'core.free_shipping':
      return 'Free shipping'
    case 'core.add_free_gift': {
      const sku = String(p.product_sku ?? '')
      const qty = num(p.qty) ?? 1
      return sku ? `Free gift: ${sku} × ${qty}` : label
    }
    case 'core.add_free_gift_choice':
      return `Free gift choice (max ${num(p.max_picks) ?? '?'})`
    case 'core.cap_total_discount': {
      const cents = num(p.max_cents)
      return cents != null ? `Cap total discount at $${cents / 100}` : label
    }
    default:
      return label
  }
}

function num(v: unknown): number | null {
  if (typeof v === 'number') return v
  if (typeof v === 'string' && v !== '' && !isNaN(Number(v))) return Number(v)
  return null
}

function listOrLabel(v: unknown, fallback: string): string {
  if (!Array.isArray(v) || v.length === 0) return fallback
  const trimmed = v.slice(0, 3).map(String).join(', ')
  return v.length > 3 ? `${trimmed} +${v.length - 3}` : trimmed
}

// Kept so the icon set stays consistent across the form - a fallback "smart" badge for future
// composite condition rendering, not referenced on this module's current path
export const _COMPOSITE_HINT_ICON = WandSparklesIcon
export const _STORE_HINT_ICON = StoreIcon
