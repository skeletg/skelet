import { useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef, type SortingState } from '@/components/data-table/tanstack'
import { ArrowUpDownIcon, PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  DeleteProductDocument,
  ProductsPagedDocument,
  ProductTypesDocument,
} from './Catalog.gql'
import { richTextToPlainText, type RichTextValue } from './LexicalRichTextEditor'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import {
  AttributeFiltersMenu,
  productFilterColumns,
  productFilterDefs,
  productFilterHiddenColumns,
  productFilterVariables,
  useProductFilterOptions,
} from './product-filters'

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

function resolveProductDisplayName(product: ProductRow) {
  const translatedName = product.content.find((entry) => entry.name.trim())?.name.trim()
  if (translatedName) return translatedName

  const fallbackName = product.name?.trim()
  if (fallbackName) return fallbackName

  return product.sku ?? ''
}

function getPublishedStoreIds(product: ProductRow) {
  return product.listings.filter((listing) => listing.isPublished).map((listing) => listing.channel.id)
}

function getPublishedCount(product: ProductRow) {
  return getPublishedStoreIds(product).length
}

// Module-level so the reference is stable - it feeds the table's sorting state every render
const DEFAULT_SORTING: SortingState = [{ id: 'updatedAt', desc: true }]

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
  const filterOptions = useProductFilterOptions()

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
      ...productFilterVariables(columnFilters),
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

  const [confirmingDelete, setConfirmingDelete] = useState<ProductRow | null>(null)

  const [deleteProduct] = useMutation(DeleteProductDocument, {
    // By operation name, so the active instance refetches with the page and filters
    // the user is actually looking at rather than a bare first page
    refetchQueries: [ProductsPagedDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteProduct({ variables: { id: confirmingDelete.id } })
  }

  const productTypeOptions = useMemo(
    () =>
      (typesData?.productTypes ?? [])
        .map((t) => ({ value: t.slug, label: t.name }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [typesData],
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
      ...productFilterColumns<ProductRow>(filterOptions),
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
              setConfirmingDelete(row)
            },
          },
        ],
      }),
    ],
    [filterOptions, canManage],
  )

  const hiddenColumnVisibility = useMemo(
    () =>
      ({ publishedStatus: false, ...productFilterHiddenColumns(filterOptions) }),
    [filterOptions],
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
            configs={filterOptions.attributes}
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
          ...productFilterDefs(filterOptions),
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

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete ? resolveProductDisplayName(confirmingDelete) : 'product'}?</>}
        description="Permanently removes the product, its attribute assignments, and any variants the backend may cascade."
        confirmLabel="Delete product"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
