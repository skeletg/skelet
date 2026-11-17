import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import type { ColumnDef, ColumnFiltersState, RowData } from '@/components/data-table/tanstack'
import type { DataTableFilter } from '@/components/data-table/DataTableToolbar'
import type { PluginList } from '@/graphql/schema.types'
import { PluginListFiltersDocument } from './ListFilters.gql'

const PREFIX = 'plugin:'

// The filters plugins register on one of core's lists, as the list wires them: hidden columns to hold their state,
// toolbar filters beside core's own, and the variable that hands the picks to the server, which narrows the page
export function usePluginListFilters<T extends RowData>(list: PluginList) {
  const { data } = useQuery(PluginListFiltersDocument, { variables: { list } })
  return useMemo(() => {
    const defs = data?.pluginListFilters ?? []
    return {
      columns: defs.map<ColumnDef<T>>((f) => ({
        id: PREFIX + f.key,
        meta: { label: f.label, noExport: true },
        enableSorting: false,
        enableHiding: false,
        header: () => null,
        cell: () => null,
      })),
      hidden: Object.fromEntries(defs.map((f) => [PREFIX + f.key, false])) as Record<string, false>,
      filters: defs.map<DataTableFilter>((f) => ({
        column: PREFIX + f.key,
        title: f.label,
        options: f.options.map((o) => ({ value: o.value, label: o.label })),
      })),
    }
  }, [data])
}

// What the list's paged query takes as pluginFilters - null when no plugin filter holds a value
export function pluginFilterVariable(columnFilters: ColumnFiltersState) {
  const picked = columnFilters
    .filter((f) => f.id.startsWith(PREFIX))
    .map((f) => ({ key: f.id.slice(PREFIX.length), values: (f.value as string[] | undefined) ?? [] }))
    .filter((p) => p.values.length > 0)
  return picked.length > 0 ? picked : null
}
