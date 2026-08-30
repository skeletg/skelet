// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import type { Column, RowData } from '@/components/data-table/tanstack'
import { CheckIcon, PlusCircleIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'

// Filters beyond the faceted option list - a range or a date window can't be expressed as checkboxes, so every list needing "orders between
// two dates" had to build its own toolbar control Manual-mode lists read these straight off columnFilters and map them to query variables

export type NumberRange = { min?: number; max?: number }
export type DateRange = { from?: string; to?: string } // ISO yyyy-mm-dd, what <input type="date"> emits

function activeLabel(value: unknown): string | null {
  if (value === undefined || value === null || value === '') return null
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (typeof value === 'string') return value
  const r = value as NumberRange & DateRange
  const lo = r.min ?? r.from
  const hi = r.max ?? r.to
  if (lo === undefined && hi === undefined) return null
  if (lo !== undefined && hi !== undefined) return `${lo} - ${hi}`
  return lo !== undefined ? `≥ ${lo}` : `≤ ${hi}`
}

// A shared trigger so every filter kind reads the same in the toolbar
function FilterShell({
  title,
  value,
  onClear,
  children,
}: {
  title: string
  value: unknown
  onClear: () => void
  children: React.ReactNode
}) {
  const label = activeLabel(value)
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 border-dashed">
          <PlusCircleIcon className="size-4" />
          {title}
          {label && (
            <>
              <Separator orientation="vertical" className="mx-2 h-4" />
              <Badge variant="secondary" className="rounded-sm px-1 font-normal">
                {label}
              </Badge>
            </>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-3" align="start">
        <div className="grid gap-2">
          {children}
          {label && (
            <Button variant="ghost" size="sm" className="h-7 justify-center" onClick={onClear}>
              Clear
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function DataTableTextFilter<TData extends RowData, TValue>({
  column,
  title,
  placeholder,
}: {
  column?: Column<TData, TValue>
  title: string
  placeholder?: string
}) {
  const value = (column?.getFilterValue() as string | undefined) ?? ''
  return (
    <FilterShell title={title} value={value} onClear={() => column?.setFilterValue(undefined)}>
      <Input
        autoFocus
        className="h-8"
        placeholder={placeholder ?? title}
        value={value}
        onChange={(e) => column?.setFilterValue(e.target.value || undefined)}
      />
    </FilterShell>
  )
}

export function DataTableNumberRangeFilter<TData extends RowData, TValue>({
  column,
  title,
  step,
}: {
  column?: Column<TData, TValue>
  title: string
  step?: number
}) {
  const value = (column?.getFilterValue() as NumberRange | undefined) ?? {}
  // undefined rather than NaN for a cleared box, or the filter never reports itself inactive
  const set = (patch: NumberRange) => {
    const next = { ...value, ...patch }
    const empty = next.min === undefined && next.max === undefined
    column?.setFilterValue(empty ? undefined : next)
  }
  const num = (raw: string) => (raw === '' ? undefined : Number(raw))
  return (
    <FilterShell title={title} value={value} onClear={() => column?.setFilterValue(undefined)}>
      <div className="grid grid-cols-2 gap-2">
        <div className="grid gap-1">
          <Label className="text-xs text-muted-foreground">Min</Label>
          <Input
            type="number"
            step={step}
            className="h-8"
            value={value.min ?? ''}
            onChange={(e) => set({ min: num(e.target.value) })}
          />
        </div>
        <div className="grid gap-1">
          <Label className="text-xs text-muted-foreground">Max</Label>
          <Input
            type="number"
            step={step}
            className="h-8"
            value={value.max ?? ''}
            onChange={(e) => set({ max: num(e.target.value) })}
          />
        </div>
      </div>
    </FilterShell>
  )
}

export function DataTableDateRangeFilter<TData extends RowData, TValue>({
  column,
  title,
}: {
  column?: Column<TData, TValue>
  title: string
}) {
  const value = (column?.getFilterValue() as DateRange | undefined) ?? {}
  const set = (patch: DateRange) => {
    const next = { ...value, ...patch }
    const empty = !next.from && !next.to
    column?.setFilterValue(empty ? undefined : next)
  }
  return (
    <FilterShell title={title} value={value} onClear={() => column?.setFilterValue(undefined)}>
      <div className="grid gap-2">
        <div className="grid gap-1">
          <Label className="text-xs text-muted-foreground">From</Label>
          <Input
            type="date"
            className="h-8"
            value={value.from ?? ''}
            onChange={(e) => set({ from: e.target.value || undefined })}
          />
        </div>
        <div className="grid gap-1">
          <Label className="text-xs text-muted-foreground">To</Label>
          <Input
            type="date"
            className="h-8"
            value={value.to ?? ''}
            onChange={(e) => set({ to: e.target.value || undefined })}
          />
        </div>
      </div>
    </FilterShell>
  )
}

export function DataTableBooleanFilter<TData extends RowData, TValue>({
  column,
  title,
  trueLabel = 'Yes',
  falseLabel = 'No',
}: {
  column?: Column<TData, TValue>
  title: string
  trueLabel?: string
  falseLabel?: string
}) {
  const value = column?.getFilterValue() as boolean | undefined
  const choose = (next: boolean) => column?.setFilterValue(value === next ? undefined : next)
  return (
    <FilterShell title={title} value={value} onClear={() => column?.setFilterValue(undefined)}>
      <div className="grid gap-1">
        {[
          { v: true, label: trueLabel },
          { v: false, label: falseLabel },
        ].map(({ v, label }) => (
          <Button
            key={String(v)}
            variant="ghost"
            size="sm"
            className="h-8 justify-start"
            onClick={() => choose(v)}
          >
            <CheckIcon className={value === v ? 'size-4' : 'size-4 opacity-0'} />
            {label}
          </Button>
        ))}
      </div>
    </FilterShell>
  )
}
