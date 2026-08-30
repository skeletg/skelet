// The table/column instance stays referentially stable while its store mutates, so the React Compiler's
// memoization can't see through it - getCanNextPage() and friends go stale (frozen pagination buttons)
// Lifting this needs the reads moved onto table.Subscribe, not another version bump
'use no memo'

import { plural } from '@lingui/core/macro'
import { Trans, useLingui } from '@lingui/react/macro'
import { useRef, useState } from 'react'
import type {
  RowData, Table } from '@/components/data-table/tanstack'
import { ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, ChevronsRightIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { useScrollEdges } from './scrollEdges'

const PRESET_SIZES = [10, 20, 50, 100]

// Ceiling on the custom box - a client-side table renders every row of a page, and a manual-mode one turns this straight into the server limit
const MAX_PAGE_SIZE = 500

// Sentinel, not a size - picking it swaps the select for the number input
const CUSTOM = '__custom__'

// Borderless so the control reads as part of the island rather than a form field dropped into it
const BARE_CONTROL =
  'h-8 w-auto gap-1 border-0 bg-transparent px-2 shadow-none hover:bg-muted dark:bg-transparent dark:hover:bg-muted'

export function DataTablePagination<TData extends RowData>({ table }: { table: Table<TData> }) {
  const { t } = useLingui()
  // getRowCount() is the server total in manual mode and the filtered count otherwise
  const totalRows = table.getRowCount()
  const selectedRows = table.getFilteredSelectedRowModel().rows.length
  const pageIndex = table.state.pagination.pageIndex
  const pageCount = table.getPageCount()
  const pageSize = table.state.pagination.pageSize
  const lastPage = Math.max(pageCount, 1)

  const barRef = useRef<HTMLDivElement>(null)
  // Floating pill while there is list below, plain bar once you reach the end
  const { atEnd: atRest } = useScrollEdges(barRef)

  const [editingCustom, setEditingCustom] = useState(false)
  const [draft, setDraft] = useState('')

  // A committed custom size needs an option backing it or the trigger renders blank
  const sizes = PRESET_SIZES.includes(pageSize)
    ? PRESET_SIZES
    : [...PRESET_SIZES, pageSize].sort((a, b) => a - b)

  function commitCustom() {
    const n = Math.round(Number(draft))
    if (Number.isFinite(n) && n > 0) {
      table.setPageSize(Math.min(n, MAX_PAGE_SIZE))
    }
    setEditingCustom(false)
  }

  // null means "not editing" - an empty string has to survive so the field can be cleared
  const [pageDraft, setPageDraft] = useState<string | null>(null)

  function commitPage() {
    const n = Math.round(Number(pageDraft))
    if (Number.isFinite(n) && n >= 1) {
      table.setPageIndex(Math.min(n, lastPage) - 1)
    }
    setPageDraft(null)
  }

  const rowLabel =
    selectedRows > 0
      ? t`${selectedRows} of ${totalRows} selected`
      : plural(totalRows, { one: '# row', other: '# rows' })

  return (
    // Sticky bottom keeps the controls reachable on a long list - the wrapper spans the width and
    // stays click-through, so only the bar itself swallows clicks on the rows underneath
    <div ref={barRef} className="pointer-events-none sticky bottom-3 z-10 px-2 py-2">
      {/* The floating state is taller, which changes page height on every flip - that is stable, not a
          loop: settling shrinks the content so it stays settled, lifting off grows it so it stays lifted */}
      <div
        className={cn(
          'pointer-events-auto flex flex-wrap items-center gap-2',
          atRest
            ? 'w-full justify-between'
            : 'mx-auto w-fit justify-center rounded-full border bg-background/80 px-4 py-2 shadow-lg backdrop-blur supports-[backdrop-filter]:bg-background/60',
        )}
      >
        {/* The row count is the longest thing in the bar. Settled it has the full width to sit in,
            but in the floating pill on a phone it forced a second line, turning the pill into a
            block tall enough to bury the rows behind it */}
        <span
          className={cn(
            'text-xs whitespace-nowrap text-muted-foreground',
            atRest ? 'flex-1' : 'hidden sm:inline',
          )}
        >
          {rowLabel}
        </span>

        {!atRest && <span className="h-4 w-px bg-border" />}

        <div className="flex items-center gap-1">
          <span className="hidden text-xs text-muted-foreground sm:inline"><Trans>Rows per page</Trans></span>
          {editingCustom ? (
            <Input
              type="number"
              min={1}
              max={MAX_PAGE_SIZE}
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onBlur={commitCustom}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  commitCustom()
                } else if (e.key === 'Escape') {
                  setEditingCustom(false)
                }
              }}
              aria-label="Rows per page"
              className="h-8 w-16 border-0 bg-muted/60 px-2 text-xs shadow-none dark:bg-muted/60"
            />
          ) : (
            <Select
              value={String(pageSize)}
              onValueChange={(value) => {
                if (value === CUSTOM) {
                  setDraft(String(pageSize))
                  setEditingCustom(true)
                  return
                }
                table.setPageSize(Number(value))
              }}
            >
              <SelectTrigger size="sm" className={BARE_CONTROL} aria-label="Rows per page">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {sizes.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size}
                  </SelectItem>
                ))}
                <SelectItem value={CUSTOM}><Trans>Custom…</Trans></SelectItem>
              </SelectContent>
            </Select>
          )}
        </div>

        {!atRest && <span className="h-4 w-px bg-border" />}

        {/* The page number is the jump box - typing in it beats hunting for a separate "go to" field
            Dropped from the floating pill on a phone: with it the pill wraps to two lines and sits
            on top of the rows. The arrows still page, and it comes back once the bar settles */}
        <div
          className={cn(
            'items-center gap-1 text-xs whitespace-nowrap text-muted-foreground',
            atRest ? 'flex' : 'hidden sm:flex',
          )}
        >
          <span><Trans>Page</Trans></span>
          <Input
            type="number"
            min={1}
            max={lastPage}
            value={pageDraft ?? String(pageIndex + 1)}
            onChange={(e) => setPageDraft(e.target.value)}
            onFocus={(e) => e.currentTarget.select()}
            onBlur={commitPage}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                commitPage()
              } else if (e.key === 'Escape') {
                setPageDraft(null)
                e.currentTarget.blur()
              }
            }}
            aria-label="Go to page"
            className="h-8 w-14 border-0 bg-muted/60 px-2 text-center text-xs shadow-none dark:bg-muted/60"
          />
          <span><Trans>of {lastPage}</Trans></span>
        </div>

        {/* Ghost inside the island - the pill's own border is the only one that should read */}
        <div className="flex items-center gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={() => table.setPageIndex(0)}
            aria-label="First page"
            disabled={!table.getCanPreviousPage()}
          >
            <ChevronsLeftIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={() => table.previousPage()}
            aria-label="Previous page"
            disabled={!table.getCanPreviousPage()}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={() => table.nextPage()}
            aria-label="Next page"
            disabled={!table.getCanNextPage()}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={() => table.setPageIndex(pageCount - 1)}
            aria-label="Last page"
            disabled={!table.getCanNextPage()}
          >
            <ChevronsRightIcon className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}
