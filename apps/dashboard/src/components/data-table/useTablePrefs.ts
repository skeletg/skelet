'use no memo'

import { useState } from 'react'
import type {
  ColumnOrderState,
  ColumnSizingState,
  OnChangeFn,
  Updater,
  VisibilityState,
} from '@/components/data-table/tanstack'
import { usePreference } from '@/lib/admin-prefs'

// Per-admin persistence for the table layout knobs (column visibility/order/widths + density),
// keyed by the table's stable identity - without a key everything falls back to ephemeral
// component state exactly as before. One pref per table means a resize drag debounces into a
// single roaming write
type TableLayoutPref = {
  v?: VisibilityState
  o?: ColumnOrderState
  s?: ColumnSizingState
  d?: boolean
}

export const DENSITY_PREF_KEY = 'ui:density'
// A string enum, so a third density later needs no migration
export type Density = 'comfortable' | 'compact'

// How many rows a list shows before you touch its size picker - the account-wide default that
// tables and URL-backed lists both seed from
export const PAGE_SIZE_PREF_KEY = 'ui:pageSize'
export const DEFAULT_PAGE_SIZE = 20

export function useDefaultPageSize(): number {
  const [size] = usePreference<number>(PAGE_SIZE_PREF_KEY, DEFAULT_PAGE_SIZE)
  // a hand-edited pref must not blank a list
  return Number.isFinite(size) && size > 0 ? Math.min(Math.floor(size), 500) : DEFAULT_PAGE_SIZE
}

const EMPTY_PREF: TableLayoutPref = {}
const EMPTY_ORDER: ColumnOrderState = []
const EMPTY_SIZING: ColumnSizingState = {}

function resolve<T>(updater: Updater<T>, prev: T): T {
  return typeof updater === 'function' ? (updater as (p: T) => T)(prev) : updater
}

export function useTablePrefs(prefsKey: string | undefined, initialVisibility?: VisibilityState) {
  const [localV, setLocalV] = useState<VisibilityState>({})
  const [localO, setLocalO] = useState<ColumnOrderState>([])
  const [localS, setLocalS] = useState<ColumnSizingState>({})
  const [localD, setLocalD] = useState<boolean | null>(null) // null means inherit the account default
  // hooks run unconditionally - keyless tables just write to a throwaway slot they never read
  const [pref, setPref] = usePreference<TableLayoutPref>(
    `table:${prefsKey ?? '__ephemeral'}`,
    EMPTY_PREF,
  )
  const [density] = usePreference<Density>(DENSITY_PREF_KEY, 'comfortable')
  const globalDense = density === 'compact'
  const persisted = !!prefsKey

  // stored choices override the page's defaults column by column
  const columnVisibility: VisibilityState = {
    ...(initialVisibility ?? {}),
    ...(persisted ? (pref.v ?? {}) : localV),
  }
  const columnOrder = persisted ? (pref.o ?? EMPTY_ORDER) : localO
  const columnSizing = persisted ? (pref.s ?? EMPTY_SIZING) : localS
  // a table's own choice wins, otherwise the account default from Preferences
  const dense = persisted ? (pref.d ?? globalDense) : (localD ?? globalDense)

  const setColumnVisibility: OnChangeFn<VisibilityState> = (updater) => {
    const next = resolve(updater, columnVisibility)
    if (persisted) setPref((p) => ({ ...p, v: next }))
    else setLocalV(next)
  }
  const setColumnOrder: OnChangeFn<ColumnOrderState> = (updater) => {
    const next = resolve(updater, columnOrder)
    if (persisted) setPref((p) => ({ ...p, o: next }))
    else setLocalO(next)
  }
  const setColumnSizing: OnChangeFn<ColumnSizingState> = (updater) => {
    const next = resolve(updater, columnSizing)
    if (persisted) setPref((p) => ({ ...p, s: next }))
    else setLocalS(next)
  }
  const setDense = (d: boolean) => {
    if (persisted) setPref((p) => ({ ...p, d }))
    else setLocalD(d)
  }
  // "Reset columns" clears layout but not density - that's a reading preference, not layout
  const resetLayout = () => {
    if (persisted) setPref((p) => (p.d === undefined ? undefined : { d: p.d }))
    else {
      setLocalV({})
      setLocalO([])
      setLocalS({})
    }
  }

  return {
    columnVisibility,
    setColumnVisibility,
    columnOrder,
    setColumnOrder,
    columnSizing,
    setColumnSizing,
    dense,
    setDense,
    resetLayout,
  }
}
