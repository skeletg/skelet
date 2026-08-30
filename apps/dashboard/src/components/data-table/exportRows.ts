import type {
  RowData, Table } from '@/components/data-table/tanstack'
import './meta'

export type ExportFormat = 'csv' // a server-paged list holds one page, so one needing everything passes its own onExport

function cell(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (value instanceof Date) return value.toISOString()
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function csvField(raw: string): string { // RFC 4180 - quote on delimiter, quote or newline, and double the inner quotes
  if (!/[",\n\r]/.test(raw)) return raw
  return `"${raw.replace(/"/g, '""')}"`
}

export function toCsv(headers: string[], rows: string[][]): string {
  return [headers, ...rows].map((r) => r.map(csvField).join(',')).join('\r\n')
}

export function exportTableToCsv<TData extends RowData>(table: Table<TData>, filename: string) {
  const columns = table
    .getVisibleLeafColumns()
    .filter((c) => !c.columnDef.meta?.noExport && c.id !== 'select' && c.id !== 'actions')

  const headers = columns.map((c) => String(c.columnDef.meta?.label ?? c.id))
  const rows = table.getRowModel().rows.map((row) => columns.map((c) => cell(row.getValue(c.id))))
  download(toCsv(headers, rows), filename)
}

export function download(content: string, filename: string, mime = 'text/csv;charset=utf-8') {
  const url = URL.createObjectURL(new Blob([content], { type: mime }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  // Revoking immediately cancels the download in Safari - one tick is enough for the click to take
  setTimeout(() => URL.revokeObjectURL(url), 0)
}
