import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { ColumnFiltersState, PaginationState, SortingState } from '@/components/data-table/tanstack'
import { useDefaultPageSize } from './useTablePrefs'

// List state lives in the URL so a filtered view survives navigation and can be pasted to someone else
// Namespaced by `key`, so two tables on one route keep their own state

export type ListUrlState = {
  paramKey: string // the namespace this instance reads/writes - saved views snapshot the same params
  globalFilter: string
  setGlobalFilter: (next: string) => void
  columnFilters: ColumnFiltersState
  setColumnFilters: (next: ColumnFiltersState) => void
  sorting: SortingState
  setSorting: (next: SortingState) => void
  pagination: PaginationState
  setPagination: (next: PaginationState) => void
  reset: () => void
}

function decode<T>(raw: string | null, fallback: T): T {
  if (!raw) return fallback
  try {
    return JSON.parse(raw) as T
  } catch {
    return fallback // a hand-edited or truncated URL must not blank the page
  }
}

export function useListUrlState({
  key = '',
  pageSize,
}: { key?: string; pageSize?: number } = {}): ListUrlState {
  // An omitted arg follows the admin's account default, a passed one is the page insisting.
  // The same number is both the fallback and the omit-from-URL sentinel and has to stay that way -
  // split them and picking your own default would delete the param, which then reads back as the
  // other number. So a link with no `size` means "the reader's default", exactly as before when
  // everyone's default was 20
  const accountSize = useDefaultPageSize()
  const size = pageSize ?? accountSize
  const [params, setParams] = useSearchParams()
  const p = (name: string) => (key ? `${key}_${name}` : name)

  // Writes compose through a ref, not react-router's `prev` which still reflects the pre-navigation
  // URL - the table filters and resets the page in one tick, and the second write wiped the first
  const pending = useRef(params)
  useEffect(() => {
    pending.current = params // resync after the navigation commits, and after an external change like Back
  }, [params])

  const set = useCallback(
    (patch: Record<string, string | null>) => {
      const next = new URLSearchParams(pending.current)
      for (const [k, v] of Object.entries(patch)) {
        if (v === null || v === '') next.delete(k)
        else next.set(k, v)
      }
      if (next.toString() === pending.current.toString()) return // no redundant navigation
      pending.current = next
      setParams(next, { replace: true }) // list tweaks are not history steps, or Back walks every keystroke
    },
    [setParams],
  )

  const globalFilter = params.get(p('q')) ?? ''
  const columnFilters = useMemo(
    () => decode<ColumnFiltersState>(params.get(p('f')), []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [params, key],
  )
  const sorting = useMemo(
    () => decode<SortingState>(params.get(p('s')), []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [params, key],
  )
  const pagination = useMemo<PaginationState>(
    () => ({
      pageIndex: Math.max(0, Number(params.get(p('page')) ?? '1') - 1), // 1-based in the URL, humans read it
      pageSize: Number(params.get(p('size')) ?? size),
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [params, key, size],
  )

  return {
    paramKey: key,
    globalFilter,
    setGlobalFilter: (next) => set({ [p('q')]: next || null, [p('page')]: null }),
    columnFilters,
    setColumnFilters: (next) =>
      set({ [p('f')]: next.length ? JSON.stringify(next) : null, [p('page')]: null }),
    sorting,
    setSorting: (next) => set({ [p('s')]: next.length ? JSON.stringify(next) : null }),
    pagination,
    setPagination: (next) =>
      set({
        [p('page')]: next.pageIndex > 0 ? String(next.pageIndex + 1) : null,
        [p('size')]: next.pageSize === size ? null : String(next.pageSize),
      }),
    reset: () => set({ [p('q')]: null, [p('f')]: null, [p('s')]: null, [p('page')]: null, [p('size')]: null }),
  }
}
