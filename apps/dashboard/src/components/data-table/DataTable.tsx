// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import { Trans } from '@lingui/react/macro'
import { useMemo, useState, type ReactNode, type ReactElement, Fragment } from 'react'
import {
  features,
  flexRender,
  manualFeatures,
  useTable,
  type ColumnDef,
  type ColumnFiltersState,
  type FilterFn,
  type PaginationState,
  type Row,
  type RowSelectionState,
  type SortingState,
  type Table as TableInstance,
  type TableOptions,
  type RowData,
  type Updater,
  type VisibilityState,
} from '@/components/data-table/tanstack'
import { DownloadIcon, Rows2Icon, Rows3Icon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { BulkActionsBar, type BulkAction, type BulkScope } from './BulkActions'
import { DataTablePagination } from './DataTablePagination'
import { DataTableToolbar, type DataTableFilter } from './DataTableToolbar'
import { ColumnControls } from './ColumnControls'
import { exportTableToCsv } from './exportRows'
import { getListColumns, getListFilters, getListMassActions, getListRowActions } from './extensions'
import type { ListUrlState } from './useListUrlState'
import { rowActionsColumn, type RowAction } from './columns'
import { useDefaultPageSize, useTablePrefs } from './useTablePrefs'
import { SavedViews } from './SavedViews'
import { booleanFilterFn, dateRangeFilterFn, numberRangeFilterFn } from './filterFns'

// Opt-in server-driven mode - the caller owns the page/search/filter state and feeds the query
// with it, so the table renders exactly the rows it is handed instead of slicing a full download
export type DataTableManual = {
  rowCount: number
  pagination: PaginationState
  onPaginationChange: (next: PaginationState) => void
  globalFilter: string
  onGlobalFilterChange: (next: string) => void
  columnFilters: ColumnFiltersState
  onColumnFiltersChange: (next: ColumnFiltersState) => void
  // Optional so callers that only page on the server keep their client-side sort. Supply both to move sorting to the server too - without
  // them a header click reorders the current page only, which reads as a broken sort once there is more than one page
  sorting?: SortingState
  onSortingChange?: (next: SortingState) => void
}

const NO_FILTERS: ColumnFiltersState = [] // stable identity - `?? []` re-ran every effect keyed on columnFilters
const NO_SORTING: SortingState = []

function resolve<T>(updater: Updater<T>, prev: T): T {
  return typeof updater === 'function' ? (updater as (old: T) => T)(prev) : updater
}

// Generic data table with search, column filters, row selection, bulk actions, sorting and
// client-side pagination - pass `columns`, `data`, and optionally `filters` and `bulkActions`
// Columns define their own cells - use DataTableColumnHeader for sortable ones, selectColumn()
// for the selection column - pass `manual` when the server does the paging/filtering
export function DataTable<TData extends RowData>({
  columns,
  data,
  loading,
  error,
  emptyState,
  searchPlaceholder,
  filters,
  bulkActions,
  toolbarRightSlot,
  toolbarSlot,
  pageSize,
  getRowId,
  enableRowSelection,
  initialColumnFilters,
  renderSubComponent,
  initialSorting,
  initialColumnVisibility,
  manual,
  urlState,
  target,
  rowActions,
  rowActionLabel,
  enableDensityToggle = false,
  enableColumnControls = false,
  prefsKey,
  viewsKey,
  exportCsv,
  compact = false,
  onRowClick,
  rowIsActive,
  renderRow,
}: {
  columns: ColumnDef<TData, unknown>[]
  data: TData[]
  loading?: boolean
  error?: { message: string } | null
  emptyState?: ReactNode
  searchPlaceholder?: string
  filters?: DataTableFilter[]
  bulkActions?: BulkAction<TData>[]
  toolbarRightSlot?: ReactNode
  toolbarSlot?: (table: TableInstance<TData>) => ReactNode
  pageSize?: number
  getRowId?: (row: TData, index: number) => string
  enableRowSelection?: TableOptions<TData>['enableRowSelection']
  initialColumnFilters?: ColumnFiltersState
  renderSubComponent?: (props: { row: Row<TData> }) => ReactNode
  initialSorting?: SortingState
  initialColumnVisibility?: VisibilityState
  manual?: DataTableManual
  // Opt-in URL-backed state for a CLIENT-side table - the URL owns search/filters/sort/page while the table keeps filtering and paging in
  // the browser, so a view survives navigation and can be pasted Pass `key` to useListUrlState when a route renders more than one table
  urlState?: ListUrlState
  // Names this list for the extension registry - without it nothing can be injected, which is the
  // right default for embedded tables that aren't a page's primary list
  target?: string
  rowActions?: RowAction<TData>[]
  rowActionLabel?: (row: TData) => string
  enableDensityToggle?: boolean
  // stable identity for per-admin layout persistence - defaults to `target`, so plugin-extended
  // tables roam for free; leave both unset for ephemeral embedded tables
  prefsKey?: string
  // opts the saved-views dropdown in explicitly - manual/server-paged pages that keep their list
  // state in the URL themselves (OrdersList) pass this since they never hand DataTable a urlState
  viewsKey?: string
  // Show/hide plus drag-to-reorder, and a reset for both that and any manual resize
  enableColumnControls?: boolean
  // filename exports the visible columns of the loaded rows - onExport takes over for a
  // server-paged list, which only holds one page and must ask the server for the rest
  exportCsv?: { filename: string; onExport?: (scope: BulkScope) => void | Promise<void> }
  compact?: boolean // rows without the list chrome - an 8-row summary panel wants no search row and no "Page 1 of 1" footer
  onRowClick?: (row: TData) => void // click-to-filter lists, distinct from row SELECTION which is the checkbox column
  /** Wrap or replace a data row, preserving default cell rendering and row behavior. */
  renderRow?: (row: Row<TData>, defaultRow: ReactElement) => ReactNode
  rowIsActive?: (row: TData) => boolean
}) {
  // Registered extensions merge in here rather than at each call site, so a plugin reaches a list
  // it cannot import - core's own columns and actions always come first
  const allColumns = useMemo(() => {
    const extra = getListColumns<TData>(target).map((e) => e.column)
    const actions = [...(rowActions ?? []), ...getListRowActions<TData>(target)]
    const base = [...columns, ...extra]
    if (actions.length === 0) return base
    // The action menu stays last even when an extension adds a column after it
    return [...base, rowActionsColumn<TData>({ actions, label: rowActionLabel })]
  }, [columns, target, rowActions, rowActionLabel])

  const allFilters = useMemo(() => [...(filters ?? []), ...getListFilters(target)], [filters, target])

  // A declared filter kind carries its own matcher, so a caller states the kind and nothing else -
  // an explicit filterFn on the column still wins, and manual mode filters on the server anyway
  const columnsWithFilterFns = useMemo(() => {
    const fns: Record<string, FilterFn<TData>> = {
      numberRange: numberRangeFilterFn as FilterFn<TData>,
      dateRange: dateRangeFilterFn as FilterFn<TData>,
      boolean: booleanFilterFn as FilterFn<TData>,
    }
    const byColumn = new Map(allFilters.map((f) => [f.column, f.kind ?? 'options']))
    return allColumns.map((col) => {
      const id = col.id ?? ('accessorKey' in col ? String(col.accessorKey) : undefined)
      const kind = id ? byColumn.get(id) : undefined
      const fn = kind ? fns[kind] : undefined
      return fn && !col.filterFn ? { ...col, filterFn: fn } : col
    })
  }, [allColumns, allFilters])
  const allBulkActions = useMemo(
    () => [...(bulkActions ?? []), ...getListMassActions<TData>(target)],
    [bulkActions, target],
  )

  const isManual = !!manual
  const manualSorting = !!manual?.onSortingChange
  const [localSorting, setLocalSorting] = useState<SortingState>(initialSorting ?? [])
  const [localColumnFilters, setLocalColumnFilters] = useState<ColumnFiltersState>(
    initialColumnFilters ?? [],
  )
  const {
    columnVisibility,
    setColumnVisibility,
    columnOrder,
    setColumnOrder,
    columnSizing,
    setColumnSizing,
    dense,
    setDense,
    resetLayout,
  } = useTablePrefs(prefsKey ?? target, initialColumnVisibility)
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [expanded, setExpanded] = useState({})
  const [localGlobalFilter, setLocalGlobalFilter] = useState('')
  // null until the reader pages or changes size, so a roaming default that lands after mount still
  // takes effect - a seeded useState would freeze the pre-hydrate number
  const [localPagination, setLocalPagination] = useState<PaginationState | null>(null)
  const accountPageSize = useDefaultPageSize()
  const initialPagination = useMemo<PaginationState>(
    () => ({ pageIndex: 0, pageSize: pageSize ?? accountPageSize }),
    [pageSize, accountPageSize],
  )

  // Three possible owners of list state, in precedence order:
  //   manual   - the server pages and filters, so the caller holds the state and feeds the query
  //   urlState - the URL holds it, but the table still filters and pages in the browser
  //   local    - plain component state
  // Everything below reads the effective value, so body, toolbar and footer stay in sync either way
  // Absent state in the URL means "no explicit choice", so the initial* props still apply - else a urlState table silently loses its default sort and filters
  const columnFilters =
    manual?.columnFilters ??
    (urlState
      ? urlState.columnFilters.length
        ? urlState.columnFilters
        : (initialColumnFilters ?? NO_FILTERS)
      : localColumnFilters)
  const globalFilter = manual?.globalFilter ?? urlState?.globalFilter ?? localGlobalFilter
  const pagination = manual?.pagination ?? urlState?.pagination ?? localPagination ?? initialPagination
  const sorting =
    manual?.sorting ??
    (urlState ? (urlState.sorting.length ? urlState.sorting : (initialSorting ?? NO_SORTING)) : localSorting)

  const setSorting = (updater: Updater<SortingState>) => {
    if (manual?.onSortingChange) manual.onSortingChange(resolve(updater, manual.sorting ?? []))
    else if (urlState) urlState.setSorting(resolve(updater, sorting))
    else setLocalSorting((prev) => resolve(updater, prev))
  }

  const setPagination = (updater: Updater<PaginationState>) => {
    if (manual) manual.onPaginationChange(resolve(updater, manual.pagination))
    else if (urlState) urlState.setPagination(resolve(updater, urlState.pagination))
    else setLocalPagination((prev) => resolve(updater, prev ?? initialPagination))
  }
  const setColumnFilters = (updater: Updater<ColumnFiltersState>) => {
    if (manual) manual.onColumnFiltersChange(resolve(updater, manual.columnFilters))
    else if (urlState) urlState.setColumnFilters(resolve(updater, urlState.columnFilters))
    else setLocalColumnFilters((prev) => resolve(updater, prev))
  }
  const setGlobalFilter = (updater: Updater<string>) => {
    if (manual) manual.onGlobalFilterChange(resolve(updater, manual.globalFilter))
    else if (urlState) urlState.setGlobalFilter(resolve(updater, urlState.globalFilter))
    else setLocalGlobalFilter((prev) => resolve(updater, prev))
  }

  const table = useTable({
    // Faceting is the only feature that differs per table - see manualFeatures
    features: isManual ? manualFeatures : features,
    data,
    columns: columnsWithFilterFns,
    state: { sorting, columnFilters, columnVisibility, rowSelection, globalFilter, expanded, pagination, columnOrder, columnSizing },
    onColumnOrderChange: setColumnOrder,
    onColumnSizingChange: setColumnSizing,
    enableColumnResizing: true,
    columnResizeMode: 'onChange',
    // TanStack's autoReset* dispatch state from a microtask queued during render, which under React 19 StrictMode can land on the discarded
    // double-render fiber - reset explicitly: filter and search changes snap to page 0 here, and the effect below clamps on shrink
    autoResetAll: false,
    // A server sort reorders the whole result, so the user belongs back on page 1
    onSortingChange: (updater) => {
      setSorting(updater)
      if (manualSorting) setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    onColumnFiltersChange: (updater) => {
      setColumnFilters(updater)
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    onExpandedChange: setExpanded,
    onGlobalFilterChange: (updater) => {
      setGlobalFilter(updater)
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    onPaginationChange: setPagination,
    manualPagination: isManual,
    manualFiltering: isManual,
    manualSorting,
    rowCount: manual?.rowCount,
    getRowId,
    enableRowSelection,
  })

  const pageCount = table.getPageCount()
  // Clamped with the render-time adjust pattern rather than in an effect - deleting the last row of
  // the last page leaves the index out of range, and an effect would paint that empty page first
  if (pageCount > 0 && pagination.pageIndex >= pageCount) {
    setPagination((prev) => ({ ...prev, pageIndex: pageCount - 1 }))
  }

  const selectedRows = table.getFilteredSelectedRowModel().rows.map((r: Row<TData>) => r.original)

  const hasActiveFilters = columnFilters.length > 0 || !!globalFilter
  const clearAllFilters = () => {
    table.resetColumnFilters()
    table.setGlobalFilter('')
  }

  // Selecting every row the filters match, not just the loaded page - on a server-paged list the
  // browser holds one page, so this is a promise the ACTION has to keep by acting on the criteria
  const totalMatching = manual ? manual.rowCount : table.getFilteredRowModel().rows.length
  const [allMatching, setAllMatching] = useState(false)
  const pageFullySelected = table.getIsAllPageRowsSelected()
  const canOfferAllMatching = pageFullySelected && totalMatching > selectedRows.length
  // A changed filter or an emptied selection invalidates the promise - compared by identity, the
  // same as the effect this replaces, so only a real filter change drops the selection
  const [prevScope, setPrevScope] = useState({
    globalFilter,
    columnFilters,
    pageSize: pagination.pageSize,
  })
  if (
    prevScope.globalFilter !== globalFilter ||
    prevScope.columnFilters !== columnFilters ||
    prevScope.pageSize !== pagination.pageSize
  ) {
    setPrevScope({ globalFilter, columnFilters, pageSize: pagination.pageSize })
    setAllMatching(false)
  }
  if (allMatching && !pageFullySelected) setAllMatching(false)

  const bulkScope: BulkScope = {
    allMatching,
    total: allMatching ? totalMatching : selectedRows.length,
    globalFilter,
    columnFilters,
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn't load</AlertTitle>
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }

  return (
    // min-w-0 lets the table shrink within the page wrapper, so it scrolls horizontally inside
    // its own bordered container instead of pushing the content area sideways
    // Block, not a grid - a grid item's containing block is its own area, leaving the sticky pagination no room to travel
    <div className="min-w-0 space-y-2">
      {!compact && (
      <DataTableToolbar
        table={table}
        target={target}
        searchPlaceholder={searchPlaceholder}
        filters={allFilters}
        toolbarSlot={toolbarSlot?.(table)}
        viewsSlot={(() => {
          const key = viewsKey ?? (urlState ? (prefsKey ?? target) : undefined)
          return key ? <SavedViews viewsKey={key} paramKey={urlState?.paramKey ?? ''} /> : null
        })()}
        rightSlot={
          <>
            {exportCsv && (
              <Button
                variant="outline"
                size="sm"
                className="h-8"
                onClick={() =>
                  exportCsv.onExport
                    ? exportCsv.onExport({ ...bulkScope, allMatching: true, total: totalMatching })
                    : exportTableToCsv(table, exportCsv.filename)
                }
              >
                <DownloadIcon className="size-4" />
                Export
              </Button>
            )}
            {enableColumnControls && (
              <ColumnControls table={table} onReset={resetLayout} />
            )}
            {enableDensityToggle && (
              <Button
                variant="outline"
                size="sm"
                className="h-8"
                aria-pressed={dense}
                onClick={() => setDense(!dense)}
              >
                {dense ? <Rows3Icon className="size-4" /> : <Rows2Icon className="size-4" />}
                {dense ? 'Comfortable' : 'Compact'}
              </Button>
            )}
            {toolbarRightSlot}
          </>
        }
      />
      )}

      {bulkActions && bulkActions.length > 0 && selectedRows.length > 0 && (
        <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/50 px-3 py-2 text-sm">
          <span className="flex flex-wrap items-center gap-2 font-medium">
            {allMatching
              ? `All ${totalMatching.toLocaleString()} matching selected`
              : `${selectedRows.length} selected`}
            {canOfferAllMatching && !allMatching && (
              <Button variant="link" size="sm" className="h-auto p-0" onClick={() => setAllMatching(true)}>
                Select all {totalMatching.toLocaleString()} matching
              </Button>
            )}
            {allMatching && (
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0"
                onClick={() => {
                  setAllMatching(false)
                  table.resetRowSelection()
                }}
              >
                Clear selection
              </Button>
            )}
          </span>
          <BulkActionsBar
            rows={selectedRows}
            actions={allBulkActions}
            scope={bulkScope}
            onAfterRun={() => {
              setAllMatching(false)
              table.resetRowSelection()
            }}
          />
        </div>
      )}

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    className={cn('relative', dense && 'h-8 py-1')}
                    style={{ width: header.getSize() !== 150 ? header.getSize() : undefined }}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                    {header.column.getCanResize() && (
                      // Pointer events rather than mouse, so a resize works on touch and pen too
                      <span
                        role="separator"
                        aria-orientation="vertical"
                        aria-label={`Resize ${header.column.id}`}
                        onDoubleClick={() => header.column.resetSize()}
                        onPointerDown={header.getResizeHandler()}
                        onTouchStart={header.getResizeHandler()}
                        className={cn(
                          'absolute top-0 right-0 h-full w-1 cursor-col-resize touch-none select-none',
                          header.column.getIsResizing() ? 'bg-primary' : 'hover:bg-border',
                        )}
                      />
                    )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  <Trans>Loading…</Trans>
                </TableCell>
              </TableRow>
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {/* "Nothing here yet" is a lie when a filter emptied the list - on a 100k catalogue that reads as data loss */}
                  {hasActiveFilters ? (
                    <div className="grid justify-items-center gap-2">
                      <span><Trans>No results match these filters.</Trans></span>
                      <Button variant="outline" size="sm" onClick={clearAllFilters}>
                        Clear filters
                      </Button>
                    </div>
                  ) : (
                    (emptyState ?? <Trans>No results.</Trans>)
                  )}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => {
                const defaultRow = (
                  <TableRow
                    data-state={row.getIsSelected() && 'selected'}
                    className={cn(
                      (renderSubComponent || onRowClick) && 'cursor-pointer select-none',
                      rowIsActive?.(row.original) && 'bg-muted/60',
                    )}
                    onClick={
                      onRowClick
                        ? () => onRowClick(row.original)
                        : renderSubComponent
                          ? row.getToggleExpandedHandler()
                          : undefined
                    }
                  >
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id} className={dense ? "py-1" : undefined}>
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    ))}
                  </TableRow>
                )
                return (
                  <Fragment key={row.id}>
                    {renderRow ? renderRow(row, defaultRow) : defaultRow}
                    {row.getIsExpanded() && renderSubComponent && (
                      <TableRow className="bg-muted/30">
                        <TableCell colSpan={row.getVisibleCells().length} className="p-0">
                          {renderSubComponent({ row })}
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {!compact && <DataTablePagination table={table} />}
    </div>
  )
}
