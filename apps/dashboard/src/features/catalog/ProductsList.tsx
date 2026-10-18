import { useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  type ColumnDef,
  type SortingState,
  type Table,
} from '@/components/data-table/tanstack'
import { ArrowUpDownIcon, CheckIcon, FilterIcon, PlusIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import {
  DataTable,
  DataTableColumnHeader,
  imageColumn,
  rowActionsColumn,
  useListUrlState,
} from '@/components/data-table'
import { formatCents } from '@/lib/format'
import type { AttributeInputType, ProductKind } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  AttributesDocument,
  DeleteProductDocument,
  ProductsPagedDocument,
  ProductTypesDocument,
} from './Catalog.gql'
import { richTextToPlainText, type RichTextValue } from './LexicalRichTextEditor'

type ProductListing = {
  isPublished: boolean
  channel: { id: string; slug: string; name: string }
}

type ProductRow = {
  id: string
  url: string
  name?: string | null
  kind: ProductKind
  shortDescription?: RichTextValue
  sku?: string | null
  content: Array<{ name: string; shortDescription?: RichTextValue }>
  thumbnail?: { id: string; url: string; altText: string; status: string } | null
  displayImage?: { id: string; url: string; altText: string; status: string } | null
  defaultPrice: number
  defaultCurrencyCode: string
  attributeValues: Array<{
    attribute: { id: string; slug: string; name: string; inputType: AttributeInputType }
    selectedValues: Array<{ id: string; slug: string; name: string; value: string }>
    scalarValue?: unknown
  }>
  listings: ProductListing[]
  productType: { id: string; slug: string; name: string }
  createdAt: string
  updatedAt: string
}

type AttributeFilterConfig = {
  attributeId: string
  columnId: string
  title: string
  options: Array<{ label: string; value: string }>
}

const FILTERABLE_ATTRIBUTE_INPUT_TYPES: AttributeInputType[] = [
  'BOOLEAN',
  'DROPDOWN',
  'MULTISELECT',
  'SWATCH',
]

function resolveProductDisplayName(product: ProductRow) {
  const translatedName = product.content.find((entry) => entry.name.trim())?.name.trim()
  if (translatedName) return translatedName

  const fallbackName = product.name?.trim()
  if (fallbackName) return fallbackName

  return product.sku ?? ''
}

function isFilterableAttributeInputType(inputType: AttributeInputType) {
  return FILTERABLE_ATTRIBUTE_INPUT_TYPES.includes(inputType)
}

function getPublishedStoreIds(product: ProductRow) {
  return product.listings.filter((listing) => listing.isPublished).map((listing) => listing.channel.id)
}

function getPublishedCount(product: ProductRow) {
  return getPublishedStoreIds(product).length
}

// Module-level so the reference is stable - it feeds the table's sorting state every render
const DEFAULT_SORTING: SortingState = [{ id: 'updatedAt', desc: true }]

// Per attribute, unsearched - one dropdown in a real catalogue runs to hundreds of values, and rendering them all is unreadable and slow
const OPTIONS_PER_ATTRIBUTE = 6

// Once searching, the cap is over the whole result set rather than per attribute
const MAX_SEARCH_RESULTS = 40

function AttributeFiltersMenu({
  table,
  configs,
}: {
  table: Table<ProductRow>
  configs: AttributeFilterConfig[]
}) {
  // Same hazard the data-table family opts out for - `table` is referentially stable and mutated in
  // place, so a memoized read of getFilterValue() never sees a tick and the checkmarks freeze
  'use no memo'

  const [query, setQuery] = useState('')
  const needle = query.trim().toLowerCase()

  const activeCount = configs.reduce((count, config) => {
    const values = (table.getColumn(config.columnId)?.getFilterValue() as string[] | undefined) ?? []
    return count + values.length
  }, 0)

  if (configs.length === 0) return null

  // A plain loop, not map - the search budget carries across attributes, and threading a running
  // total through a callback is what the compiler's reassign rule objects to
  const groups: Array<{
    config: AttributeFilterConfig
    column: ReturnType<Table<ProductRow>['getColumn']>
    selected: Set<string>
    shown: AttributeFilterConfig['options']
    hidden: number
  }> = []
  let budget = needle ? MAX_SEARCH_RESULTS : Number.POSITIVE_INFINITY
  for (const config of configs) {
    const column = table.getColumn(config.columnId)
    const selected = new Set((column?.getFilterValue() as string[] | undefined) ?? [])
    const matched = needle
      ? config.options.filter((o) => o.label.toLowerCase().includes(needle))
      : config.options
    // Ticked options sort first so the cap can never hide one and leave it unclearable
    const ordered = [...matched].sort(
      (a, b) => Number(selected.has(b.value)) - Number(selected.has(a.value)),
    )
    const cap = Math.min(needle ? budget : OPTIONS_PER_ATTRIBUTE, ordered.length)
    budget -= cap
    groups.push({
      config,
      column,
      selected,
      shown: ordered.slice(0, cap),
      hidden: ordered.length - cap,
    })
  }

  const anyShown = groups.some((g) => g.shown.length > 0)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 border-dashed">
          <FilterIcon className="size-4" />
          Attributes
          {activeCount > 0 ? ` (${activeCount})` : ''}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        {/* shouldFilter off - the list is filtered and capped here, so cmdk must not also hide rows */}
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search attribute values…"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList className="max-h-80">
            {!anyShown && <CommandEmpty>No matching values.</CommandEmpty>}
            {groups.map(({ config, column, selected, shown, hidden }) =>
              shown.length === 0 ? null : (
                <CommandGroup key={config.columnId} heading={config.title}>
                  {shown.map((option) => {
                    const isSelected = selected.has(option.value)
                    return (
                      <CommandItem
                        key={option.value}
                        value={`${config.columnId}:${option.value}`}
                        onSelect={() => {
                          const next = new Set(selected)
                          if (isSelected) next.delete(option.value)
                          else next.add(option.value)
                          column?.setFilterValue(next.size > 0 ? [...next] : undefined)
                        }}
                      >
                        <div
                          className={cn(
                            'flex size-4 items-center justify-center rounded-sm border border-primary',
                            isSelected
                              ? 'bg-primary text-primary-foreground'
                              : 'opacity-50 [&_svg]:invisible',
                          )}
                        >
                          <CheckIcon className="size-3" />
                        </div>
                        <span className="flex-1 truncate">{option.label}</span>
                      </CommandItem>
                    )
                  })}
                  {hidden > 0 && (
                    <p className="px-2 py-1.5 text-xs text-muted-foreground">
                      +{hidden} more - search to narrow
                    </p>
                  )}
                </CommandGroup>
              ),
            )}
          </CommandList>
          {activeCount > 0 && (
            <>
              <CommandSeparator />
              <div className="p-1">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 w-full"
                  onClick={() => {
                    for (const { column } of groups) column?.setFilterValue(undefined)
                  }}
                >
                  Clear {activeCount} attribute filter{activeCount === 1 ? '' : 's'}
                </Button>
              </div>
            </>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  )
}

export function ProductsList() {
  const { has } = usePermissions()
  const canManage = has('catalog.manage')
  // Paging, sorting and filtering all run on the server. The unpaginated query
  // downloaded the whole catalogue and filtered it in the browser, which is fine
  // at seed scale and a frozen tab at 100k products (a 152MB response, most of the
  // hang spent normalising it into the Apollo cache).
  // URL-backed, so a filtered view survives a detail-page round trip and can be pasted to someone
  // else - on this list in particular, losing page 300 of a filtered catalogue on Back is brutal
  const {
    globalFilter: search,
    setGlobalFilter: setSearch,
    columnFilters,
    setColumnFilters,
    sorting: urlSorting,
    setSorting,
    pagination,
    setPagination,
  } = useListUrlState()
  // An empty URL means the default sort, not "unsorted" - the server falls back to the same thing
  const sorting = urlSorting.length ? urlSorting : DEFAULT_SORTING
  // Typing shouldn't fire a query per keystroke now that each one is a round trip
  const debouncedSearch = useDebounced(search.trim(), 300)

  const filterValues = (id: string) =>
    (columnFilters.find((f) => f.id === id)?.value as string[] | undefined) ?? []

  const publishedFilter = filterValues('publishedStatus')
  const attributeValueIds = columnFilters
    .filter((f) => f.id.startsWith('attribute:'))
    .flatMap((f) => ((f.value as string[] | undefined) ?? []))
    // The menu encodes selectable values as "value:<id>" - anything else (the
    // boolean yes/no options) has no value row to match and is dropped below
    .filter((v) => v.startsWith('value:'))
    .map((v) => v.slice('value:'.length))

  const { data, previousData, loading, error } = useQuery(ProductsPagedDocument, {
    variables: {
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      search: debouncedSearch || null,
      kinds: filterValues('kind').length ? (filterValues('kind') as ProductKind[]) : null,
      productTypeSlugs: filterValues('productType').length ? filterValues('productType') : null,
      // Both ticked is the same as neither - don't send a filter that excludes nothing
      published:
        publishedFilter.length === 1 ? publishedFilter[0] === 'published' : null,
      attributeValueIds: attributeValueIds.length ? attributeValueIds : null,
      sortKey: sorting[0]?.id ?? 'updatedAt',
      sortDesc: sorting[0]?.desc ?? true,
    },
    notifyOnNetworkStatusChange: true,
  })
  // Changing variables empties `data` while the request is in flight, and the table renders a bare "Loading…" in place of every row - so
  // hold the last page until the next arrives, and only show the loading state when there is nothing yet
  const page = data?.productsPaged ?? previousData?.productsPaged
  const rows = useMemo(() => (page?.items ?? []) as ProductRow[], [page])
  const totalCount = page?.totalCount ?? 0

  // Filter options cannot be derived from the loaded rows - one page of 25 would only ever offer the handful of values
  // that page happens to contain. Both are small, cacheable lists
  const { data: typesData } = useQuery(ProductTypesDocument)
  const { data: attributesData } = useQuery(AttributesDocument)

  const [confirmingDelete, setConfirmingDelete] = useState<ProductRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteProduct, { loading: deleting }] = useMutation(DeleteProductDocument, {
    // By operation name, so the active instance refetches with the page and filters
    // the user is actually looking at rather than a bare first page
    refetchQueries: [ProductsPagedDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteProduct({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const productTypeOptions = useMemo(
    () =>
      (typesData?.productTypes ?? [])
        .map((t) => ({ value: t.slug, label: t.name }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [typesData],
  )

  // Only attributes with value rows are offered. Boolean scalars are deliberately left out:
  // deriving them from whichever products happen to be loaded gives the server nothing to match,
  // since it filters on value ids, and an option that silently does nothing is worse than one
  // that isn't there.
  const attributeFilterConfigs = useMemo<AttributeFilterConfig[]>(
    () =>
      (attributesData?.attributes ?? [])
        .filter(
          (a) =>
            isFilterableAttributeInputType(a.inputType) &&
            a.inputType !== 'BOOLEAN' &&
            (a.values?.length ?? 0) > 0,
        )
        .map((a) => ({
          attributeId: a.id,
          columnId: `attribute:${a.id}`,
          title: a.name,
          options: (a.values ?? [])
            .map((v) => ({ value: `value:${v.id}`, label: v.name || v.value }))
            .sort((x, y) => x.label.localeCompare(y.label)),
        }))
        .sort((a, b) => a.title.localeCompare(b.title)),
    [attributesData],
  )

  const columns = useMemo<ColumnDef<ProductRow>[]>(
    () => [
      imageColumn<ProductRow>({
        image: (row) => row.thumbnail ?? row.displayImage, // displayImage already falls back to the first gallery image, which is all most products have
        alt: (row) => resolveProductDisplayName(row),
      }),
      {
        id: 'name',
        accessorFn: (row) => resolveProductDisplayName(row),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => {
          const name = resolveProductDisplayName(row.original)
          const shortDescription = richTextToPlainText(
            row.original.shortDescription ?? row.original.content[0]?.shortDescription,
          )

          return (
            <div className="flex items-center gap-3">
              <div className="grid min-w-0 gap-0.5">
                <Link
                  to={`/products/${row.original.id}`}
                  className="truncate font-medium hover:underline"
                >
                  {name}
                </Link>
                {shortDescription && (
                  <span className="line-clamp-1 text-xs text-muted-foreground">
                    {shortDescription}
                  </span>
                )}
              </div>
            </div>
          )
        },
      },
      {
        accessorKey: 'url',
        header: ({ column }) => <DataTableColumnHeader column={column} title="URL" />,
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{row.original.url}</code>
        ),
      },
      {
        accessorKey: 'sku',
        header: ({ column }) => <DataTableColumnHeader column={column} title="SKU" />,
        cell: ({ row }) =>
          row.original.sku ? (
            <code className="font-mono text-xs">{row.original.sku}</code>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        id: 'kind',
        accessorFn: (row) => row.kind,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Kind" />,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.kind)
        },
        cell: ({ row }) => (
          <Badge
            variant={row.original.kind === 'SIMPLE' ? 'secondary' : 'outline'}
            className="font-normal capitalize"
          >
            {row.original.kind.toLowerCase()}
          </Badge>
        ),
      },
      {
        id: 'productType',
        accessorFn: (row) => row.productType.name,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Type" />,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.productType.slug)
        },
        cell: ({ row }) => (
          <Badge variant="outline" className="font-normal">
            {row.original.productType.name}
          </Badge>
        ),
      },
      {
        accessorKey: 'defaultPrice',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Default price" />,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {formatCents(row.original.defaultPrice, row.original.defaultCurrencyCode)}
          </span>
        ),
      },
      {
        id: 'publishedCount',
        accessorFn: (row) => getPublishedCount(row),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Published on" />,
        cell: ({ row }) => {
          const published = row.original.listings.filter((listing) => listing.isPublished)
          if (published.length === 0) {
            return <span className="text-xs text-muted-foreground">- no stores -</span>
          }
          return (
            <div className="flex flex-wrap gap-1">
              {published.map((l) => (
                <Badge key={l.channel.id} variant="secondary" className="font-normal">
                  {l.channel.name}
                </Badge>
              ))}
            </div>
          )
        },
      },
      {
        id: 'publishedStatus',
        meta: { label: 'Published', noExport: true },
        enableHiding: false,
        accessorFn: (row) => (getPublishedCount(row) > 0 ? 'published' : 'unpublished'),
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(getPublishedCount(row.original) > 0 ? 'published' : 'unpublished')
        },
        cell: () => null,
      },
      // Filter-state holders, not real columns - the server does the matching (manualFiltering), so they carry no accessor or filterFn.
      // meta.label keeps them from showing as a raw uuid in the column controls, and noExport keeps an empty column out of the CSV
      ...attributeFilterConfigs.map<ColumnDef<ProductRow>>((config) => ({
        id: config.columnId,
        meta: { label: config.title, noExport: true },
        enableSorting: false,
        enableHiding: false,
        cell: () => null,
      })),
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Created" />,
        cell: ({ row }) => new Date(row.original.createdAt).toLocaleDateString(),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => new Date(row.original.updatedAt).toLocaleDateString(),
      },
      rowActionsColumn<ProductRow>({
        label: resolveProductDisplayName,
        actions: [
          { id: 'view', label: 'View', to: (row) => `/products/${row.id}` },
          {
            id: 'delete',
            label: 'Delete',
            destructive: true,
            separatorBefore: true,
            hidden: () => !canManage,
            onSelect: (row) => {
              setDeleteError(null)
              setConfirmingDelete(row)
            },
          },
        ],
      }),
    ],
    [attributeFilterConfigs, canManage],
  )

  const hiddenColumnVisibility = useMemo(
    () =>
      Object.fromEntries([
        ['publishedStatus', false],
        ...attributeFilterConfigs.map((config) => [config.columnId, false] as const),
      ]),
    [attributeFilterConfigs],
  )

  return (
    <>
      <DataTable
        columns={columns}
        data={rows}
        loading={loading && !page}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState="No products yet."
        searchPlaceholder="Search name, slug, or SKU…"
        initialColumnVisibility={hiddenColumnVisibility}
        target="catalog.products"
        enableDensityToggle
        enableColumnControls
        manual={{
          rowCount: totalCount,
          pagination,
          onPaginationChange: setPagination,
          globalFilter: search,
          onGlobalFilterChange: setSearch,
          columnFilters,
          onColumnFiltersChange: setColumnFilters,
          sorting,
          onSortingChange: setSorting,
        }}
        toolbarSlot={(table) => (
          <AttributeFiltersMenu
            table={table}
            configs={attributeFilterConfigs}
          />
        )}
        filters={[
          {
            column: 'kind',
            title: 'Kind',
            options: [
              { label: 'Simple', value: 'SIMPLE' },
              { label: 'Configurable', value: 'CONFIGURABLE' },
              { label: 'Bundle', value: 'BUNDLE' },
              { label: 'Grouped', value: 'GROUPED' },
            ],
          },
          {
            column: 'productType',
            title: 'Type',
            options: productTypeOptions.map((o) => ({ label: o.label, value: o.value })),
          },
          {
            column: 'publishedStatus',
            title: 'Status',
            options: [
              { label: 'Published', value: 'published' },
              { label: 'Unpublished', value: 'unpublished' },
            ],
          },
          // hidden - the Attributes popover is the UI for these, but declaring them lets the applied
          // chips print value names instead of `value:<uuid>`
          ...attributeFilterConfigs.map((config) => ({
            column: config.columnId,
            title: config.title,
            options: config.options,
            hidden: true,
          })),
        ]}
        toolbarRightSlot={
          canManage && (
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" asChild>
                <Link to="/data?resource=products">
                  <ArrowUpDownIcon className="size-4" />
                  Import / export
                </Link>
              </Button>
              <Button size="sm" asChild>
                <Link to="/products/new">
                  <PlusIcon className="size-4" />
                  New product
                </Link>
              </Button>
            </div>
          )
        }
      />

      <AlertDialog
        open={!!confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {confirmingDelete ? resolveProductDisplayName(confirmingDelete) : 'product'}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Permanently removes the product, its attribute assignments, and any variants the
              backend may cascade.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                onConfirmDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete product'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
