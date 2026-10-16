import type { Mode, Readiness, ServerOption } from './types'

export const TABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'details', label: 'Details' },
  { id: 'delivery', label: 'Delivery' },
  { id: 'points', label: 'Pickup points' },
  { id: 'payment', label: 'Payment' },
] as const
export type TabId = (typeof TABS)[number]['id']

// The sales/checkout settings each tab shows - the shipping tax, risk hold and the rest of the group stay under Settings → Sales
export const DETAILS_PATHS = [
  'sales/checkout/allow_guest',
  'sales/checkout/phone_field',
  'sales/checkout/business_purchase',
  'sales/checkout/tax_id_field',
  'sales/checkout/po_number_field',
  'sales/checkout/terms_consent',
  'sales/checkout/min_order',
  'sales/checkout/threshold_progress',
  'sales/checkout/address_validator',
  'sales/checkout/address_validation_strict',
] as const
export const DELIVERY_PATHS = ['sales/checkout/pickup_by_someone_else', 'sales/checkout/pickup_requires_billing_address'] as const

export function isTab(value: string | null): value is TabId {
  return TABS.some((t) => t.id === value)
}

export const MODE_LABELS: Record<Mode, string> = {
  required: 'Required',
  optional: 'Optional',
  hidden: 'Not asked',
}

export type OptionStatus = 'live' | 'setup' | 'off' | 'unsaved' | 'pending'

export const STATUS_LABELS: Record<OptionStatus, string> = {
  live: 'Live',
  pending: 'Will be offered after saving',
  setup: 'Needs setup',
  off: 'Off',
  unsaved: 'Not saved yet',
}

export const STATUS_VARIANTS = {
  live: 'success',
  pending: 'info',
  setup: 'warning',
  off: 'secondary',
  unsaved: 'info',
} as const

export function optionStatus(server: ServerOption | undefined, enabled: boolean): OptionStatus {
  if (enabled && (!server?.enabled || !server.ready)) return 'pending'
  if (!server) return 'unsaved'
  if (!server.ready) return 'setup'
  return enabled ? 'live' : 'off'
}

// Where a failing check is fixed: a tab on this page, or another dashboard page. The backend tags
// each check with a tab; carrier checks are fixed on the Shipping page, where methods are priced
export type FixTarget = { tab: TabId; optionId?: string } | { href: string; label: string }

export function fixTarget(check: Readiness, optionId?: string): FixTarget {
  if (check.code === 'carrier') return { href: '/shipping', label: 'Open Shipping' }
  if (check.code === 'payment') return { tab: 'payment' }
  if (check.code === 'delivery') return { tab: 'delivery' }
  if (check.code.startsWith('option:')) return { tab: tabFor(check.tab), optionId: check.code.slice('option:'.length) }
  return { tab: tabFor(check.tab), optionId }
}

function tabFor(tab: string): TabId {
  return tab === 'points' ? 'points' : 'delivery'
}

// A saved carrier that no longer quotes the kind stays listed, so the select can show what the option holds
export function carrierChoices(refs: readonly { slug: string; name: string }[], current: string) {
  const out = refs.map((r) => ({ value: r.slug, label: r.name }))
  if (current && !refs.some((r) => r.slug === current)) out.push({ value: current, label: `${current} (not quoting this kind)` })
  return out
}
