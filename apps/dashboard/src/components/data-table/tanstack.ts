// Every TanStack Table import goes through here - v9 makes the feature set explicit and threads it
// through every public type, so it's declared once below and the aliases hide it from call sites
import {
  columnFacetingFeature,
  columnFilteringFeature,
  columnOrderingFeature,
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  createExpandedRowModel,
  createFacetedRowModel,
  createFacetedUniqueValues,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFns,
  globalFilteringFeature,
  rowExpandingFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  sortFns,
  tableFeatures,
} from '@tanstack/react-table'
import type {
  Cell as TSCell,
  Column as TSColumn,
  ColumnDef as TSColumnDef,
  FilterFn as TSFilterFn,
  Header as TSHeader,
  HeaderGroup as TSHeaderGroup,
  HeaderContext as TSHeaderContext,
  Row as TSRow,
  RowData,
  TableOptions as TSTableOptions,
} from '@tanstack/react-table'
import type { ReactTable } from '@tanstack/react-table'

export { flexRender, useTable } from '@tanstack/react-table'

const slots = {
  columnFacetingFeature,
  columnFilteringFeature,
  columnOrderingFeature,
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  globalFilteringFeature,
  rowExpandingFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  expandedRowModel: createExpandedRowModel(),
  facetedRowModel: createFacetedRowModel(),
  // Typed optional so the manual set below can drop it and still be the same feature type
  facetedUniqueValues: createFacetedUniqueValues() as
    | ReturnType<typeof createFacetedUniqueValues>
    | undefined,
  filteredRowModel: createFilteredRowModel(),
  paginatedRowModel: createPaginatedRowModel(),
  sortedRowModel: createSortedRowModel(),
  // The built-in registries, so a column can still name its filter/sort as a string
  filterFns,
  sortFns,
}

export const features = tableFeatures(slots)

// Facet counts computed off one server page would be a lie, so a manual table drops that slot
// and the filter dropdowns render their options bare
export const manualFeatures: typeof features = tableFeatures({
  ...slots,
  facetedUniqueValues: undefined,
})

export type Features = typeof features

export type Cell<TData extends RowData, TValue = unknown> = TSCell<Features, TData, TValue>
export type Column<TData extends RowData, TValue = unknown> = TSColumn<Features, TData, TValue>
export type ColumnDef<TData extends RowData, TValue = unknown> = TSColumnDef<Features, TData, TValue>
export type FilterFn<TData extends RowData> = TSFilterFn<Features, TData>
export type Header<TData extends RowData, TValue = unknown> = TSHeader<Features, TData, TValue>
export type HeaderGroup<TData extends RowData> = TSHeaderGroup<Features, TData>
export type HeaderContext<TData extends RowData, TValue> = TSHeaderContext<Features, TData, TValue>
export type Row<TData extends RowData> = TSRow<Features, TData>
export type TableOptions<TData extends RowData> = TSTableOptions<Features, TData>

// The hook's return rather than core's Table - consumers read `state` and call the getX helpers
export type Table<TData extends RowData> = ReactTable<Features, TData>

// State shapes carry no feature param, so these pass through - v9 renamed only the visibility one
export type {
  ColumnFiltersState,
  ColumnOrderState,
  ColumnSizingState,
  ColumnVisibilityState as VisibilityState,
  OnChangeFn,
  PaginationState,
  RowData,
  RowSelectionState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
