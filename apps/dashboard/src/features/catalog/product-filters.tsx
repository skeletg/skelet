import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { CheckIcon, FilterIcon } from 'lucide-react'
import type { ColumnDef, ColumnFiltersState, RowData, Table } from '@/components/data-table/tanstack'
import type { DataTableFilter, NumberRange } from '@/components/data-table'
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
import type { AttributeInputType } from '@/graphql/schema.types'
import { AttributesDocument, ProductFilterOptionsDocument } from './Catalog.gql'

// The Products list and the product picker read the same productsPaged, so they share one set of filters

export type AttributeFilterConfig = {
  attributeId: string
  columnId: string
  title: string
  options: Array<{ label: string; value: string }>
}

type FilterOption = { label: string; value: string }
type CategoryNode = { id: string; name: string; children?: CategoryNode[] | null }

const STOCK_OPTIONS: FilterOption[] = [
  { label: 'In stock', value: 'in' },
  { label: 'Out of stock', value: 'out' },
]

const STATE_COLUMNS = [
  { id: 'category', label: 'Category' },
  { id: 'collection', label: 'Collection' },
  { id: 'price', label: 'Price' },
  { id: 'stock', label: 'Stock' },
]

const OPTIONS_PER_ATTRIBUTE = 6
const MAX_SEARCH_RESULTS = 40

function categoryOptions(nodes: CategoryNode[], trail: string[] = []): FilterOption[] {
  return [...nodes]
    .sort((a, b) => a.name.localeCompare(b.name))
    .flatMap((n) => {
      const path = [...trail, n.name]
      return [{ value: n.id, label: path.join(' › ') }, ...categoryOptions(n.children ?? [], path)]
    })
}

export function useProductFilterOptions() {
  const { data } = useQuery(ProductFilterOptionsDocument)
  const { data: attributesData } = useQuery(AttributesDocument)
  return useMemo(
    () => ({
      categories: categoryOptions((data?.rootCategories ?? []) as CategoryNode[]),
      collections: (data?.collections ?? [])
        .map((c) => ({ value: c.id, label: c.name ?? 'Untitled collection' }))
        .sort((a, b) => a.label.localeCompare(b.label)),
      // Only attributes with value rows: the server matches on value ids, so a derived boolean option would match nothing
      attributes: (attributesData?.attributes ?? [])
        .filter(
          (a) =>
            (['DROPDOWN', 'MULTISELECT', 'SWATCH'] as AttributeInputType[]).includes(a.inputType) &&
            (a.values?.length ?? 0) > 0,
        )
        .map<AttributeFilterConfig>((a) => ({
          attributeId: a.id,
          columnId: `attribute:${a.id}`,
          title: a.name,
          options: (a.values ?? [])
            .map((v) => ({ value: `value:${v.id}`, label: v.name || v.value }))
            .sort((x, y) => x.label.localeCompare(y.label)),
        }))
        .sort((a, b) => a.title.localeCompare(b.title)),
    }),
    [data, attributesData],
  )
}

export type ProductFilterOptions = ReturnType<typeof useProductFilterOptions>

export function productFilterColumns<T extends RowData>(options: ProductFilterOptions): ColumnDef<T>[] {
  return [
    ...STATE_COLUMNS,
    ...options.attributes.map((a) => ({ id: a.columnId, label: a.title })),
  ].map((c) => ({
    id: c.id,
    meta: { label: c.label, noExport: true },
    enableSorting: false,
    enableHiding: false,
    header: () => null,
    cell: () => null,
  }))
}

export function productFilterHiddenColumns(options: ProductFilterOptions): Record<string, false> {
  return Object.fromEntries(
    [...STATE_COLUMNS.map((c) => c.id), ...options.attributes.map((a) => a.columnId)].map((id) => [id, false]),
  )
}

export function productFilterDefs(options: ProductFilterOptions): DataTableFilter[] {
  return [
    { column: 'category', title: 'Category', options: options.categories },
    { column: 'collection', title: 'Collection', options: options.collections },
    { column: 'price', title: 'Price', kind: 'numberRange', step: 0.01 },
    { column: 'stock', title: 'Stock', options: STOCK_OPTIONS },
    ...options.attributes.map((a) => ({ column: a.columnId, title: a.title, options: a.options, hidden: true })),
  ]
}

const bound = (n: number | undefined) => (n !== undefined && Number.isFinite(n) && n >= 0 ? String(n) : null)

export function productFilterVariables(columnFilters: ColumnFiltersState) {
  const values = (id: string) => (columnFilters.find((f) => f.id === id)?.value as string[] | undefined) ?? []
  const price = columnFilters.find((f) => f.id === 'price')?.value as NumberRange | undefined
  const stock = values('stock')
  const attributeValueIds = columnFilters
    .filter((f) => f.id.startsWith('attribute:'))
    .flatMap((f) => (f.value as string[] | undefined) ?? [])
    .filter((v) => v.startsWith('value:'))
    .map((v) => v.slice('value:'.length))
  return {
    categoryIds: values('category').length ? values('category') : null,
    collectionIds: values('collection').length ? values('collection') : null,
    priceMin: bound(price?.min),
    priceMax: bound(price?.max),
    inStock: stock.length === 1 ? stock[0] === 'in' : null,
    attributeValueIds: attributeValueIds.length ? attributeValueIds : null,
  }
}

export function AttributeFiltersMenu<TData extends RowData>({
  table,
  configs,
}: {
  table: Table<TData>
  configs: AttributeFilterConfig[]
}) {
  // `table` is referentially stable and mutated in place, so a memoized read of getFilterValue() never sees a tick
  'use no memo'

  const [query, setQuery] = useState('')
  const needle = query.trim().toLowerCase()

  const activeCount = configs.reduce((count, config) => {
    const values = (table.getColumn(config.columnId)?.getFilterValue() as string[] | undefined) ?? []
    return count + values.length
  }, 0)

  if (configs.length === 0) return null

  const groups: Array<{
    config: AttributeFilterConfig
    column: ReturnType<Table<TData>['getColumn']>
    selected: Set<string>
    shown: AttributeFilterConfig['options']
    hidden: number
  }> = []
  let budget = needle ? MAX_SEARCH_RESULTS : Number.POSITIVE_INFINITY
  for (const config of configs) {
    const column = table.getColumn(config.columnId)
    const selected = new Set((column?.getFilterValue() as string[] | undefined) ?? [])
    const matched = needle ? config.options.filter((o) => o.label.toLowerCase().includes(needle)) : config.options
    const ordered = [...matched].sort((a, b) => Number(selected.has(b.value)) - Number(selected.has(a.value)))
    const cap = Math.min(needle ? budget : OPTIONS_PER_ATTRIBUTE, ordered.length)
    budget -= cap
    groups.push({ config, column, selected, shown: ordered.slice(0, cap), hidden: ordered.length - cap })
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
        <Command shouldFilter={false}>
          <CommandInput placeholder="Search attribute values…" value={query} onValueChange={setQuery} />
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
                            isSelected ? 'bg-primary text-primary-foreground' : 'opacity-50 [&_svg]:invisible',
                          )}
                        >
                          <CheckIcon className="size-3" />
                        </div>
                        <span className="flex-1 truncate">{option.label}</span>
                      </CommandItem>
                    )
                  })}
                  {hidden > 0 && (
                    <p className="px-2 py-1.5 text-xs text-muted-foreground">+{hidden} more - search to narrow</p>
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
