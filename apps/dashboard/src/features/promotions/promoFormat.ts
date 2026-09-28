// Shared formatting and constants for the Promotions hub and the per-promotion page - a plain
// .ts module with no component exports, so fast-refresh stays happy and both format identically

export { CHART_ACCENT as ACCENT } from '@/lib/chart-colors'

export function formatCents(c: number): string {
  return `$${(c / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
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
