// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import { useState } from 'react'
import type {
  RowData, Table } from '@/components/data-table/tanstack'
import './meta'
import { ChevronDownIcon, ChevronUpIcon, Settings2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

// Show/hide and reorder in one place - reorder is arrow buttons rather than drag, so it works from
// the keyboard and needs no drag library for a control this small

function move(order: string[], id: string, delta: number): string[] {
  const from = order.indexOf(id)
  const to = from + delta
  if (from < 0 || to < 0 || to >= order.length) return order
  const next = order.slice()
  next.splice(to, 0, next.splice(from, 1)[0])
  return next
}

export function ColumnControls<TData extends RowData>({
  table,
  onReset,
}: {
  table: Table<TData>
  onReset: () => void
}) {
  const [open, setOpen] = useState(false)
  // Leaf order as the table currently sees it, so the first reorder starts from the real layout
  const order = table.getAllLeafColumns().map((c) => c.id)
  // Only hideable ones - selection, actions and filter-only holders have no cell worth showing
  const hideable = table.getAllLeafColumns().filter((c) => c.getCanHide())

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          <Settings2Icon className="size-4" />
          Columns
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-2" align="end">
        <div className="grid max-h-80 gap-0.5 overflow-y-auto">
          {hideable.map((column) => {
            const label = String(column.columnDef.meta?.label ?? column.id)
            return (
              <div key={column.id} className="flex items-center gap-2 rounded px-1 py-0.5 hover:bg-accent">
                <Checkbox
                  id={`col-${column.id}`}
                  checked={column.getIsVisible()}
                  disabled={!column.getCanHide()}
                  onCheckedChange={(v) => column.toggleVisibility(!!v)}
                  aria-label={`Show ${label}`}
                />
                <label htmlFor={`col-${column.id}`} className="min-w-0 flex-1 truncate text-sm">
                  {label}
                </label>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  aria-label={`Move ${label} earlier`}
                  onClick={() => table.setColumnOrder(move(order, column.id, -1))}
                >
                  <ChevronUpIcon className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  aria-label={`Move ${label} later`}
                  onClick={() => table.setColumnOrder(move(order, column.id, 1))}
                >
                  <ChevronDownIcon className="size-3.5" />
                </Button>
              </div>
            )
          })}
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="mt-1 h-7 w-full"
          onClick={() => {
            hideable.forEach((c) => c.toggleVisibility(true))
            onReset()
          }}
        >
          Reset columns
        </Button>
      </PopoverContent>
    </Popover>
  )
}
