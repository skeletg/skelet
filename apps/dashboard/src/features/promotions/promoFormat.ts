// Shared formatting and constants for the Promotions hub and the per-promotion page - a plain
// .ts module with no component exports, so fast-refresh stays happy and both format identically

import { formatCents as formatMoney } from '@/lib/format'

export { CHART_ACCENT as ACCENT } from '@/lib/chart-colors'

// shortcut: promotion analytics and rule amounts carry no currency (the backend sums discounts across currencies), so they
// render as plain numbers instead of a guessed "$" - give them a currency once the API groups by one
export function formatCents(c: number): string {
  return formatMoney(c, '')
}

export function fmtDay(d: string): string {
  const dt = new Date(`${d}T00:00:00`)
  return Number.isNaN(dt.getTime())
    ? d
    : dt.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export type DailyPoint = {
  date: string
  redemptionCount: number
  discountTotalCents: number
}
