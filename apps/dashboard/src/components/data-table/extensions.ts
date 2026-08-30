import type {
  RowData, ColumnDef } from '@/components/data-table/tanstack'
import type { BulkAction } from './BulkActions'
import type { DataTableFilter } from './DataTableToolbar'
import type { RowAction } from './columns'

// List-extension registry - registered at import time, reaching a list through its free-form
// `target` so a plugin adds to a list whose component it cannot import - same shape as registerDetailSection
// Sorting rides on the column, and a server-paged list needs its sort key mapped, hence sortKey

export type ListExtensionBase = {
  id: string // conventionally '<plugin-name>:thing' - duplicates throw at registration
  target: string
  order?: number // lower renders earlier, default 100 - core sits at 10-50
}

export type ListColumnEntry<TData extends RowData = RowData> = ListExtensionBase & {
  column: ColumnDef<TData, unknown>
  sortKey?: string // what the server calls this column, for lists running manual sorting
}

export type ListFilterEntry = ListExtensionBase & { filter: DataTableFilter }
export type ListRowActionEntry<TData extends RowData = RowData> = ListExtensionBase & { action: RowAction<TData> }
export type ListMassActionEntry<TData extends RowData = RowData> = ListExtensionBase & { action: BulkAction<TData> }

type AnyEntry = ListExtensionBase

const columns: ListColumnEntry[] = []
const filters: ListFilterEntry[] = []
const rowActions: ListRowActionEntry[] = []
const massActions: ListMassActionEntry[] = []

function add<T extends AnyEntry>(into: T[], entry: T, kind: string) {
  if (into.some((e) => e.id === entry.id)) {
    throw new Error(`list-extensions: duplicate ${kind} id "${entry.id}"`)
  }
  into.push(entry)
}

function forTarget<T extends AnyEntry>(from: T[], target: string | undefined): T[] {
  if (!target) return []
  return from.filter((e) => e.target === target).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
}

export function registerListColumn<TData extends RowData = RowData>(entry: ListColumnEntry<TData>) {
  add(columns, entry as ListColumnEntry, 'column')
}
export function registerListFilter(entry: ListFilterEntry) {
  add(filters, entry, 'filter')
}
export function registerListRowAction<TData extends RowData = RowData>(entry: ListRowActionEntry<TData>) {
  add(rowActions, entry as ListRowActionEntry, 'row action')
}
export function registerListMassAction<TData extends RowData = RowData>(entry: ListMassActionEntry<TData>) {
  add(massActions, entry as ListMassActionEntry, 'mass action')
}

export function getListColumns<TData extends RowData>(target?: string): ListColumnEntry<TData>[] {
  return forTarget(columns, target) as ListColumnEntry<TData>[]
}
export function getListFilters(target?: string): DataTableFilter[] {
  return forTarget(filters, target).map((e) => e.filter)
}
export function getListRowActions<TData extends RowData>(target?: string): RowAction<TData>[] {
  return (forTarget(rowActions, target) as ListRowActionEntry<TData>[]).map((e) => e.action)
}
export function getListMassActions<TData extends RowData>(target?: string): BulkAction<TData>[] {
  return (forTarget(massActions, target) as ListMassActionEntry<TData>[]).map((e) => e.action)
}

export function __resetListExtensionsForTests() {
  columns.length = 0
  filters.length = 0
  rowActions.length = 0
  massActions.length = 0
}
