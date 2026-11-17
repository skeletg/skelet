import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import type { ColumnDef, ColumnFiltersState, RowData } from '@/components/data-table/tanstack'
import type { DataTableFilter } from '@/components/data-table/DataTableToolbar'
import type { PluginList } from '@/graphql/schema.types'
import { PluginListFiltersDocument } from './ListFilters.gql'

const PREFIX = 'plugin:'

// The filters plugins register on one of core's lists, as the list wires them: hidden columns to hold their state,
// toolbar filters beside core's own, and picked, which hands the picks to the server - that narrows the page
export function usePluginListFilters<T extends RowData>(list: PluginList) {
  const { data } = useQuery(PluginListFiltersDocument, { variables: { list } })
  return useMemo(() => {
    const defs = data?.pluginListFilters ?? []
    // Once it's known what the list offers, a pick it doesn't - a plugin switched off since the link was saved, a
    // renamed key - is left out, where the server would refuse the whole list for it
    const offered = data && new Map(defs.map((f) => [f.key, new Set(f.options.map((o) => o.value))]))
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
      // What the list's paged query takes as pluginFilters - null when no plugin filter holds a value
      picked: (columnFilters: ColumnFiltersState) => {
        const picks = columnFilters
          .filter((f) => f.id.startsWith(PREFIX))
          .map((f) => ({ key: f.id.slice(PREFIX.length), values: (f.value as string[] | undefined) ?? [] }))
          .map((p) => (offered ? { key: p.key, values: p.values.filter((v) => offered.get(p.key)?.has(v)) } : p))
          .filter((p) => p.values.length > 0)
        return picks.length > 0 ? picks : null
      },
    }
  }, [data])
}
