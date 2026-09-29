import { createContext, useContext } from 'react'

// Live pick state for the ProductPicker's cells. The table mounts each column's cell as a
// component, so a column rebuilt on every pick would remount its cells - a quantity input would
// drop focus on each keystroke. Columns stay stable instead, and cells read the state from here

export type PickerState = {
  isPicked: (id: string) => boolean
  isTaken: (id: string) => boolean
  extraOf: (id: string) => unknown
  toggle: (id: string, on: boolean) => void
  patch: (id: string, change: Record<string, unknown>) => void
  rowError: (id: string) => string | undefined
  takenLabel: string
  // The current page's pickable rows, for the header checkbox
  selectableIds: string[]
}

export const PickerStateContext = createContext<PickerState | null>(null)

export function usePickerState(): PickerState {
  const ctx = useContext(PickerStateContext)
  if (!ctx) throw new Error('usePickerState must be used inside a ProductPicker')
  return ctx
}

// For a caller's per-pick column cells: the row's extra values (undefined until the row is
// picked) and a setter that merges into them
export function usePickedExtra<T>(id: string): { value: T | undefined; patch: (change: Partial<T>) => void } {
  const state = usePickerState()
  return {
    value: state.extraOf(id) as T | undefined,
    patch: (change) => state.patch(id, change as Record<string, unknown>),
  }
}
