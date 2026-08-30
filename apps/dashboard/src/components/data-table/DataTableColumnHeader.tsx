// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import type { Column, RowData } from '@/components/data-table/tanstack'
import { ArrowDownIcon, ArrowUpIcon, ChevronsUpDownIcon, EyeOffIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

// Sortable/hideable column header - Use inside ColumnDef.header when you want
// sort + hide controls; plain text/labels render as regular <TableHead>.
export function DataTableColumnHeader<TData extends RowData, TValue>({
  column,
  title,
  className,
}: {
  column: Column<TData, TValue>
  title: string
  className?: string
}) {
  if (!column.getCanSort() && !column.getCanHide()) {
    return <span className={cn(className)}>{title}</span>
  }

  return (
    <div className={cn('flex items-center gap-1', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="-ml-2 h-8 data-[state=open]:bg-accent"
          >
            <span>{title}</span>
            {column.getIsSorted() === 'desc' ? (
              <ArrowDownIcon className="ml-1 size-3.5" />
            ) : column.getIsSorted() === 'asc' ? (
              <ArrowUpIcon className="ml-1 size-3.5" />
            ) : (
              <ChevronsUpDownIcon className="ml-1 size-3.5 op-70" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {column.getCanSort() && (
            <>
              <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
                <ArrowUpIcon className="size-4" />
                Ascending
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
                <ArrowDownIcon className="size-4" />
                Descending
              </DropdownMenuItem>
            </>
          )}
          {column.getCanSort() && column.getCanHide() && <DropdownMenuSeparator />}
          {column.getCanHide() && (
            <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
              <EyeOffIcon className="size-4" />
              Hide
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
