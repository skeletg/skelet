import type { FilterFn, Row, RowData } from '@/components/data-table/tanstack'
import type { DateRange, NumberRange } from './filters'

// Client-side matchers, registered on the column by DataTable so a caller only declares the kind - a manual-mode list filters on the server and ignores them
export const numberRangeFilterFn: FilterFn<RowData> = (row: Row<RowData>, columnId, value) => {
  const { min, max } = (value ?? {}) as NumberRange
  const raw = row.getValue(columnId)
  const n = typeof raw === 'number' ? raw : Number(raw)
  if (Number.isNaN(n)) return false
  if (min !== undefined && n < min) return false
  if (max !== undefined && n > max) return false
  return true
}

export const dateRangeFilterFn: FilterFn<RowData> = (row: Row<RowData>, columnId, value) => {
  const { from, to } = (value ?? {}) as DateRange
  const raw = row.getValue(columnId)
  const t = new Date(raw as string | number | Date).getTime()
  if (Number.isNaN(t)) return false
  // `to` is inclusive of the whole day, else picking one date matches nothing
  if (from && t < new Date(`${from}T00:00:00`).getTime()) return false
  if (to && t > new Date(`${to}T23:59:59.999`).getTime()) return false
  return true
}

export const booleanFilterFn: FilterFn<RowData> = (row: Row<RowData>, columnId, value) => {
  if (typeof value !== 'boolean') return true
  return !!row.getValue(columnId) === value
}
