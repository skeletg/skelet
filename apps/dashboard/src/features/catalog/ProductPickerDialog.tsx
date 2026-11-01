import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import { AlertCircleIcon, PackageIcon } from 'lucide-react'
import { DataTable, DataTableColumnHeader, imageColumn } from '@/components/data-table'
import type { ColumnDef, ColumnFiltersState, PaginationState } from '@/components/data-table/tanstack'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useDebounced } from '@/shared/hooks/useDebounced'
import type { ProductKind } from '@/graphql/schema.types'
import { ProductPickerPagedDocument, ProductTypesDocument, type ProductPickerPagedQuery } from './Catalog.gql'
import { productLabel } from './category-products'
import {
  AttributeFiltersMenu,
  productFilterColumns,
  productFilterDefs,
  productFilterHiddenColumns,
  productFilterVariables,
  useProductFilterOptions,
} from './product-filters'
import { PickerStateContext, usePickerState, type PickerState } from './product-picker-context'

// The one "pick products" surface in the catalog - a server-paged, filterable table with ticks
// that survive paging and search. Callers state what is already taken, any fixed filters, and
// optional per-pick fields (quantity, default); whether a confirm stages or writes is theirs

export type PickerProduct = ProductPickerPagedQuery['productsPaged']['items'][number]
export type ProductPick<T> = { product: PickerProduct; extra: T }
// Returned from onConfirm to keep the picker open - rowErrors keeps only those picks ticked
export type PickerOutcome = { error?: string; rowErrors?: Record<string, string> } | void

export type PickExtra<T> = {
  initial: (product: PickerProduct) => T
  // Cells for the per-pick fields - they read and write their row through usePickedExtra, and the
  // array must be stable (memoised) or every render remounts the inputs
  columns: ColumnDef<PickerProduct>[]
}

export type ProductPickerProps<T> = {
  // Already linked - shown ticked and locked, with `takenLabel` beside the name
  takenIds?: ReadonlySet<string>
  takenLabel?: string
  // Never offered at all, e.g. the product being edited
  excludeIds?: ReadonlySet<string>
  // Fixed server filters - a fixed one drops its toolbar filter
  kinds?: ProductKind[]
  productTypeSlugs?: string[]
  attributeValueIds?: string[]
  withAttributes?: boolean
  sidebar?: ReactNode
  // Edge-to-edge regions that pad themselves, for a host with no padding of its own
  inset?: boolean
  columns?: ColumnDef<PickerProduct>[]
  extra?: PickExtra<T>
  showPrice?: boolean
  confirmLabel?: (count: number) => string
  emptyMessage?: string
  validate?: (picks: ProductPick<T>[]) => string | null
  onCancel: () => void
  onConfirm: (picks: ProductPick<T>[]) => PickerOutcome | Promise<PickerOutcome>
  onDone: () => void
}

const PAGE_SIZE = 25

const KIND_OPTIONS: { label: string; value: ProductKind }[] = [
  { label: 'Simple', value: 'SIMPLE' },
  { label: 'Configurable', value: 'CONFIGURABLE' },
  { label: 'Bundle', value: 'BUNDLE' },
  { label: 'Grouped', value: 'GROUPED' },
  { label: 'Kit', value: 'KIT' },
  { label: 'Gift card', value: 'GIFTCARD' },
  { label: 'Subscription', value: 'SUBSCRIPTION' },
]

const defaultConfirmLabel = (n: number) =>
  n === 0 ? 'Add products' : `Add ${n} product${n === 1 ? '' : 's'}`

export function ProductPickerDialog<T = null>({
  open,
  onOpenChange,
  title,
  description,
  ...picker
}: Omit<ProductPickerProps<T>, 'onCancel' | 'onDone'> & {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* A fixed height, so narrowing to a handful of rows doesn't shrink the dialog under the pointer */}
      <DialogContent
        className={cn(
          'flex h-[min(88vh,56rem)] flex-col gap-0 overflow-hidden p-0',
          picker.sidebar ? 'sm:max-w-6xl' : 'sm:max-w-5xl',
        )}
      >
        <DialogHeader className="gap-1 border-b px-5 py-4 pr-12">
          <DialogTitle className="flex items-center gap-2">
            <PackageIcon className="size-5 text-muted-foreground" />
            {title}
          </DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {/* Content unmounts on close, so the picks and filters start fresh each time */}
        {open ? (
          <ProductPicker<T> {...picker} inset onCancel={() => onOpenChange(false)} onDone={() => onOpenChange(false)} />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

// The body on its own, for a host that already has a sheet or tab around it
export function ProductPicker<T = null>({
  takenIds,
  takenLabel = 'Already added',
  excludeIds,
  kinds,
  productTypeSlugs,
  attributeValueIds,
  withAttributes = false,
  sidebar,
  inset = false,
  columns: infoColumns,
  extra,
  showPrice = false,
  confirmLabel = defaultConfirmLabel,
  emptyMessage = 'No products match.',
  validate,
  onCancel,
  onConfirm,
  onDone,
}: ProductPickerProps<T>) {
  const [search, setSearch] = useState('')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: PAGE_SIZE })
  // Held here, keyed by id, with the product riding along - the table only knows the loaded page,
  // and a pick has to survive paging, a new search and the confirm
  const [picked, setPicked] = useState<Map<string, ProductPick<T>>>(() => new Map())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({})
  const debouncedSearch = useDebounced(search.trim(), 300)

  // A new server filter from the host (the children sidebar) starts again from page one
  const attrKey = (attributeValueIds ?? []).join(',')
  const [prevAttrKey, setPrevAttrKey] = useState(attrKey)
  if (prevAttrKey !== attrKey) {
    setPrevAttrKey(attrKey)
    setPagination((p) => ({ ...p, pageIndex: 0 }))
  }

  const filterValues = (id: string) =>
    (columnFilters.find((f) => f.id === id)?.value as string[] | undefined) ?? []
  const publishedFilter = filterValues('publishedStatus')
  const kindFilter = kinds ?? (filterValues('kind').length ? (filterValues('kind') as ProductKind[]) : null)
  const typeFilter = productTypeSlugs ?? (filterValues('productType').length ? filterValues('productType') : null)
  const filterOptions = useProductFilterOptions()
  const chosen = productFilterVariables(columnFilters)
  // The host's fixed values and the admin's own both have to hold
  const valueIds = [...(attributeValueIds ?? []), ...(chosen.attributeValueIds ?? [])]

  const { data, previousData, loading, error: loadError } = useQuery(ProductPickerPagedDocument, {
    variables: {
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      search: debouncedSearch || null,
      kinds: kindFilter,
      productTypeSlugs: typeFilter,
      // Both ticked is the same as neither
      published: publishedFilter.length === 1 ? publishedFilter[0] === 'published' : null,
      ...chosen,
      attributeValueIds: valueIds.length ? valueIds : null,
      withAttributes,
    },
    notifyOnNetworkStatusChange: true,
  })
  // Hold the last page while the next loads, so the rows don't blank out on every keystroke
  const page = data?.productsPaged ?? previousData?.productsPaged
  const rows = useMemo(
    () => (page?.items ?? []).filter((p) => !excludeIds?.has(p.id)),
    [page, excludeIds],
  )

  const { data: typesData } = useQuery(ProductTypesDocument, { skip: !!productTypeSlugs })
  const productTypeOptions = useMemo(
    () =>
      (typesData?.productTypes ?? [])
        .map((t) => ({ value: t.slug, label: t.name }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [typesData],
  )

  const isTaken = useCallback((id: string) => takenIds?.has(id) ?? false, [takenIds])
  const initial = extra?.initial
  const toggle = useCallback(
    (id: string, on: boolean) =>
      setPicked((prev) => {
        const next = new Map(prev)
        const product = rows.find((r) => r.id === id) ?? prev.get(id)?.product
        if (on && product) next.set(id, { product, extra: initial ? initial(product) : (null as T) })
        else next.delete(id)
        return next
      }),
    [rows, initial],
  )
  const patch = useCallback(
    (id: string, change: Record<string, unknown>) =>
      setPicked((prev) => {
        const current = prev.get(id)
        if (!current) return prev
        const next = new Map(prev)
        next.set(id, { ...current, extra: { ...current.extra, ...change } as T })
        return next
      }),
    [],
  )

  const selectableIds = useMemo(() => rows.filter((r) => !isTaken(r.id)).map((r) => r.id), [rows, isTaken])
  const state = useMemo<PickerState>(
    () => ({
      isPicked: (id) => picked.has(id),
      isTaken,
      extraOf: (id) => picked.get(id)?.extra,
      toggle,
      patch,
      rowError: (id) => rowErrors[id],
      takenLabel,
      selectableIds,
    }),
    [picked, isTaken, toggle, patch, rowErrors, takenLabel, selectableIds],
  )

  const fixedType = !!productTypeSlugs
  const showKind = !kinds || kinds.length > 1
  const extraColumns = extra?.columns
  // Structural inputs only - pick state reaches the cells through PickerStateContext
  const columns = useMemo<ColumnDef<PickerProduct>[]>(
    () => [
      { id: 'pick', size: 32, enableSorting: false, enableHiding: false, header: PickHeaderCell, cell: PickCell },
      imageColumn<PickerProduct>({ image: (r) => r.thumbnail ?? r.displayImage, alt: productLabel, size: 32 }),
      {
        id: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Product" />,
        enableSorting: false,
        cell: NameCell,
      },
      {
        accessorKey: 'sku',
        header: ({ column }) => <DataTableColumnHeader column={column} title="SKU" />,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.sku ?? '-'}</span>
        ),
      },
      ...(fixedType
        ? []
        : [
            {
              id: 'productType',
              header: ({ column }) => <DataTableColumnHeader column={column} title="Type" />,
              enableSorting: false,
              cell: ({ row }) => <span className="text-muted-foreground">{row.original.productType.name}</span>,
            } satisfies ColumnDef<PickerProduct>,
          ]),
      ...(showKind
        ? [
            {
              id: 'kind',
              header: ({ column }) => <DataTableColumnHeader column={column} title="Kind" />,
              enableSorting: false,
              cell: ({ row }) => (
                <Badge variant={row.original.kind === 'SIMPLE' ? 'secondary' : 'outline'} className="font-normal capitalize">
                  {row.original.kind.toLowerCase()}
                </Badge>
              ),
            } satisfies ColumnDef<PickerProduct>,
          ]
        : []),
      ...(infoColumns ?? []),
      ...(showPrice
        ? [
            {
              id: 'price',
              header: () => 'Price',
              enableSorting: false,
              cell: ({ row }) => (
                <span className="font-mono text-xs">{formatCents(row.original.defaultPrice, row.original.defaultCurrencyCode)}</span>
              ),
            } satisfies ColumnDef<PickerProduct>,
          ]
        : []),
      // Per-pick inputs sit inside a click guard so typing a quantity doesn't untick the row
      ...(extraColumns ?? []).map(
        (col): ColumnDef<PickerProduct> => ({
          ...col,
          enableSorting: false,
          cell: (ctx) => (
            <div onClick={(e) => e.stopPropagation()}>{typeof col.cell === 'function' ? col.cell(ctx) : col.cell}</div>
          ),
        }),
      ),
      // Filter-only - the toolbar binds each filter to a column
      { id: 'publishedStatus', enableHiding: false, header: () => null, cell: () => null },
      ...productFilterColumns<PickerProduct>(filterOptions),
    ],
    [fixedType, showKind, infoColumns, showPrice, extraColumns, filterOptions],
  )

  async function confirm() {
    const picks = [...picked.values()]
    if (picks.length === 0) return
    const invalid = validate?.(picks)
    if (invalid) {
      setError(invalid)
      return
    }
    setError(null)
    setRowErrors({})
    setBusy(true)
    try {
      const outcome = await onConfirm(picks)
      const failed = outcome?.rowErrors ?? {}
      if (outcome?.error || Object.keys(failed).length) {
        setError(outcome?.error ?? null)
        setRowErrors(failed)
        if (Object.keys(failed).length) {
          setPicked((prev) => new Map([...prev].filter(([id]) => id in failed)))
        }
        return
      }
      onDone()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <PickerStateContext.Provider value={state}>
    <div className={cn('flex min-h-0 flex-1 flex-col', !inset && 'gap-3')}>
      <div className={cn('grid min-h-0 flex-1', !inset && 'gap-4', sidebar && 'md:grid-cols-[220px_1fr]')}>
        {sidebar ? (
          <aside className={cn('min-h-0 overflow-y-auto text-sm', inset && 'border-r bg-muted/30 p-4')}>{sidebar}</aside>
        ) : null}
        <div className={cn('min-h-0 overflow-y-auto', inset && 'px-5 pb-2')}>
          <DataTable
            columns={columns}
            data={rows}
            loading={loading && !page}
            error={loadError}
            getRowId={(row) => row.id}
            searchPlaceholder="Search by name or SKU…"
            pageSize={PAGE_SIZE}
            initialColumnVisibility={{ publishedStatus: false, ...productFilterHiddenColumns(filterOptions) }}
            onRowClick={(row) => {
              if (!isTaken(row.id)) toggle(row.id, !picked.has(row.id))
            }}
            rowIsActive={(row) => picked.has(row.id)}
            emptyState={<p className="py-6 text-center text-sm text-muted-foreground">{emptyMessage}</p>}
            manual={{
              rowCount: page?.totalCount ?? 0,
              pagination,
              onPaginationChange: setPagination,
              globalFilter: search,
              onGlobalFilterChange: setSearch,
              columnFilters,
              onColumnFiltersChange: setColumnFilters,
            }}
            filters={[
              ...(kinds ? [] : [{ column: 'kind', title: 'Kind', options: KIND_OPTIONS }]),
              ...(productTypeSlugs ? [] : [{ column: 'productType', title: 'Type', options: productTypeOptions }]),
              {
                column: 'publishedStatus',
                title: 'Status',
                options: [
                  { label: 'Published', value: 'published' },
                  { label: 'Unpublished', value: 'unpublished' },
                ],
              },
              ...productFilterDefs(filterOptions),
            ]}
            toolbarSlot={(table) => <AttributeFiltersMenu table={table} configs={filterOptions.attributes} />}
          />
        </div>
      </div>

      {error ? (
        <Alert variant="destructive" className={cn(inset && 'mx-5 mb-3 w-auto')}>
          <AlertCircleIcon className="size-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className={cn('flex flex-wrap items-center justify-between gap-3 border-t', inset ? 'px-5 py-3' : 'pt-3')}>
        <div className="flex min-w-0 items-center gap-3 text-sm text-muted-foreground">
          {picked.size > 0 ? <PickedThumbs picks={[...picked.values()]} /> : null}
          <span>{picked.size === 0 ? 'Nothing selected yet' : `${picked.size} selected`}</span>
          {picked.size > 0 ? (
            <Button type="button" variant="ghost" size="sm" className="h-7 px-2" onClick={() => setPicked(new Map())}>
              Clear
            </Button>
          ) : null}
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" disabled={picked.size === 0 || busy} onClick={() => void confirm()}>
            {busy ? 'Working…' : confirmLabel(picked.size)}
          </Button>
        </div>
      </div>
    </div>
    </PickerStateContext.Provider>
  )
}

const SHOWN_THUMBS = 5

// The picks span pages and searches, so the footer is the one place they all show
function PickedThumbs<T>({ picks }: { picks: ProductPick<T>[] }) {
  const more = picks.length - SHOWN_THUMBS
  return (
    <div className="flex shrink-0 items-center -space-x-2">
      {picks.slice(0, SHOWN_THUMBS).map(({ product }) => {
        const img = product.thumbnail ?? product.displayImage
        const ready = !!img?.url && (!img.status || img.status === 'READY')
        return ready ? (
          <img
            key={product.id}
            src={img.url}
            alt={productLabel(product)}
            title={productLabel(product)}
            className="size-8 rounded-md border object-cover ring-2 ring-background"
          />
        ) : (
          <span
            key={product.id}
            title={productLabel(product)}
            className="size-8 rounded-md border bg-muted ring-2 ring-background"
          />
        )
      })}
      {more > 0 ? (
        <span className="grid size-8 place-items-center rounded-md border bg-muted text-xs font-medium ring-2 ring-background">
          +{more}
        </span>
      ) : null}
    </div>
  )
}

// Cells as components of their own, so the column array can stay stable across picks

function PickHeaderCell() {
  const state = usePickerState()
  const pickedOnPage = state.selectableIds.filter(state.isPicked).length
  const all = state.selectableIds.length > 0 && pickedOnPage === state.selectableIds.length
  return (
    <Checkbox
      checked={all || (pickedOnPage > 0 && 'indeterminate')}
      disabled={state.selectableIds.length === 0}
      onCheckedChange={(v) => {
        for (const id of state.selectableIds) state.toggle(id, !!v)
      }}
      aria-label="Select every product on this page"
    />
  )
}

function PickCell({ row }: { row: { original: PickerProduct } }) {
  const state = usePickerState()
  const taken = state.isTaken(row.original.id)
  return (
    <Checkbox
      checked={taken || state.isPicked(row.original.id)}
      disabled={taken}
      onClick={(e) => e.stopPropagation()}
      onCheckedChange={(v) => state.toggle(row.original.id, !!v)}
      aria-label={`Select ${productLabel(row.original)}`}
    />
  )
}

function NameCell({ row }: { row: { original: PickerProduct } }) {
  const state = usePickerState()
  const error = state.rowError(row.original.id)
  return (
    <div className="grid min-w-0">
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate font-medium">{productLabel(row.original)}</span>
        {state.isTaken(row.original.id) ? (
          <Badge variant="outline" className="shrink-0 font-normal">{state.takenLabel}</Badge>
        ) : null}
      </div>
      {error ? (
        <span className="flex items-center gap-1 text-xs text-destructive">
          <AlertCircleIcon className="size-3" />
          {error}
        </span>
      ) : null}
    </div>
  )
}
