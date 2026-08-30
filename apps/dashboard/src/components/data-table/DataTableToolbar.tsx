// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import { useEffect, useRef, useState } from 'react'
import type {
  RowData, Table } from '@/components/data-table/tanstack'
import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useOptionalPreference } from '@/lib/admin-prefs'
import { Slot } from '@/plugins/slots'
import { DataTableFacetedFilter, type FacetedFilterOption } from './DataTableFacetedFilter'
import {
  DataTableBooleanFilter,
  DataTableDateRangeFilter,
  DataTableNumberRangeFilter,
  DataTableTextFilter,
} from './filters'
import { findScrollParent, useScrollEdges } from './scrollEdges'

// Gap between the toolbar and whatever it parks under
const STICKY_GAP = 12

// The toolbar can ride along as a floating island like the pagination bar does, but stacked under a
// sticky page header it reads as clutter - off unless the admin turns it on in Preferences
export const STICKY_TOOLBAR_PREF_KEY = 'ui:stickyToolbar'

// hidden declares a filter for label resolution and chips without rendering a button - a page with its
// own combined filter UI still wants its values named rather than shown as raw ids
type FilterBase = { column: string; title: string; hidden?: boolean }

// kind is optional so every existing caller stays an option list without edit
export type DataTableFilter =
  | (FilterBase & { kind?: 'options'; options: FacetedFilterOption[] })
  | (FilterBase & { kind: 'text'; placeholder?: string })
  | (FilterBase & { kind: 'numberRange'; step?: number })
  | (FilterBase & { kind: 'dateRange' })
  | (FilterBase & { kind: 'boolean'; trueLabel?: string; falseLabel?: string })

// One chip per applied filter, so a narrowed list says WHY it is narrowed - a lone Reset button
// leaves you guessing which of five popovers is still holding a value
type ActiveFilter = { id: string; label: string; value: string; clear: () => void }

function describeValue(raw: unknown, filter: DataTableFilter | undefined): string {
  if (Array.isArray(raw)) {
    const options = filter && (!filter.kind || filter.kind === 'options') ? filter.options : undefined
    const labels = raw.map((v) => options?.find((o) => o.value === v)?.label)
    // Unresolvable values would read as raw uuids, so fall back to a count
    return labels.every(Boolean) ? labels.join(', ') : `${raw.length} selected`
  }
  if (typeof raw === 'string') return raw
  if (typeof raw === 'boolean') return raw ? 'yes' : 'no'
  return 'set'
}

export function DataTableToolbar<TData extends RowData>({
  table,
  searchPlaceholder,
  filters = [],
  toolbarSlot,
  viewsSlot,
  rightSlot,
  target,
}: {
  table: Table<TData>
  searchPlaceholder?: string
  filters?: DataTableFilter[]
  toolbarSlot?: React.ReactNode
  viewsSlot?: React.ReactNode
  rightSlot?: React.ReactNode
  // The list's extension target - plugins hang toolbar items off `list-toolbar:<target>`
  target?: string
}) {
  const isFiltered =
    table.state.columnFilters.length > 0 || table.state.globalFilter
  const globalFilter = (table.state.globalFilter ?? '') as string

  const applied: ActiveFilter[] = []
  if (globalFilter) {
    applied.push({ id: '__search', label: 'Search', value: globalFilter, clear: () => table.setGlobalFilter('') })
  }
  for (const f of table.state.columnFilters) {
    const column = table.getColumn(f.id)
    const def = filters.find((x) => x.column === f.id)
    const label = def?.title ?? String(column?.columnDef.meta?.label ?? f.id)
    const options = def && (!def.kind || def.kind === 'options') ? def.options : undefined
    // A multi-value filter gets a chip per value, so one can be dropped without clearing the rest
    if (Array.isArray(f.value)) {
      for (const v of f.value as string[]) {
        applied.push({
          id: `${f.id}:${v}`,
          label,
          value: options?.find((o) => o.value === v)?.label ?? v,
          clear: () => {
            const rest = (f.value as string[]).filter((x) => x !== v)
            column?.setFilterValue(rest.length ? rest : undefined)
          },
        })
      }
      continue
    }
    applied.push({
      id: f.id,
      label,
      value: describeValue(f.value, def),
      clear: () => column?.setFilterValue(undefined),
    })
  }

  const barRef = useRef<HTMLDivElement>(null)
  const sticky = useOptionalPreference<boolean>(STICKY_TOOLBAR_PREF_KEY, false)
  // Plain bar at the top of the list, floating island once the rows have scrolled past it
  const { atStart } = useScrollEdges(barRef, { enabled: sticky })
  const floating = sticky && !atStart

  // A page header is sticky at top-0 in the same scroller, so the toolbar has to clear its height
  // or it pins underneath one - pages without a header resolve to just the gap
  const [stickyTop, setStickyTop] = useState(STICKY_GAP)
  useEffect(() => {
    if (!sticky) return
    const el = barRef.current
    if (!el) return
    const scroller = findScrollParent(el)
    if (!scroller) return
    const header = scroller.querySelector<HTMLElement>('[data-page-header]')
    if (!header) return
    const measure = () => setStickyTop(header.offsetHeight + STICKY_GAP)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(header)
    return () => ro.disconnect()
  }, [sticky])

  return (
    <div
      ref={barRef}
      className={cn(sticky ? 'pointer-events-none sticky z-10 py-2' : 'py-3')}
      style={sticky ? { top: stickyTop } : undefined}
    >
      <div
        className={cn(
          'flex flex-wrap items-center gap-2',
          sticky && 'pointer-events-auto',
          floating &&
            'mx-auto w-fit justify-center rounded-full border bg-background/80 px-4 py-2 shadow-lg backdrop-blur supports-[backdrop-filter]:bg-background/60',
        )}
      >
      {searchPlaceholder && (
        <Input
          placeholder={searchPlaceholder}
          value={globalFilter}
          onChange={(e) => table.setGlobalFilter(e.target.value)}
          className="h-8 w-full sm:w-auto sm:max-w-xs"
        />
      )}
      {viewsSlot}
      {filters.map((f) => {
        if (f.hidden) return null
        const column = table.getColumn(f.column)
        switch (f.kind) {
          case 'text':
            return <DataTableTextFilter key={f.column} column={column} title={f.title} placeholder={f.placeholder} />
          case 'numberRange':
            return <DataTableNumberRangeFilter key={f.column} column={column} title={f.title} step={f.step} />
          case 'dateRange':
            return <DataTableDateRangeFilter key={f.column} column={column} title={f.title} />
          case 'boolean':
            return (
              <DataTableBooleanFilter
                key={f.column}
                column={column}
                title={f.title}
                trueLabel={f.trueLabel}
                falseLabel={f.falseLabel}
              />
            )
          default:
            return <DataTableFacetedFilter key={f.column} column={column} title={f.title} options={f.options} />
        }
      })}
      {toolbarSlot}
      <Slot zone={target && `list-toolbar:${target}`} ctx={{ table }} />
      {isFiltered && (
        <Button
          variant="ghost"
          size="sm"
          className="h-8 px-2 lg:px-3"
          onClick={() => {
            table.resetColumnFilters()
            table.setGlobalFilter('')
          }}
        >
          Reset
          <XIcon className="size-4" />
        </Button>
      )}
        {/* flex-wrap, or the slot's own buttons stay on one line and overflow the viewport - at
            phone width that clipped the last action (Import on /products) out of reach entirely */}
        {rightSlot && (
          <div className={cn('flex flex-wrap items-center gap-2', !floating && 'ml-auto')}>{rightSlot}</div>
        )}
      </div>

      {applied.length > 0 && !floating && (
        <div className="flex flex-wrap items-center gap-1.5 pt-2">
          {applied.map((f) => (
            <span
              key={f.id}
              className="inline-flex max-w-72 items-center gap-1 rounded-full border bg-muted/50 py-0.5 pl-2.5 pr-1 text-xs"
            >
              <span className="text-muted-foreground">{f.label}:</span>
              <span className="truncate font-medium">{f.value}</span>
              <button
                type="button"
                onClick={f.clear}
                aria-label={`Clear ${f.label} filter`}
                className="rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-background hover:text-foreground"
              >
                <XIcon className="size-3" />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
