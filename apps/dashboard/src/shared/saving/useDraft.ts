import { useCallback, useMemo, useState } from 'react'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'

// Owns a form's values, so "dirty" is complete BY CONSTRUCTION - the alternative, listing the
// fields to watch by hand, silently rots the day someone adds the next one
//
// It also gives an editor that stays put after saving a way to go clean again: re-baseline with
// saved(). A hand-listed watch has no baseline to move, so those pages read dirty until they remount

type Updater<V> = V | ((prev: V) => V)

// setName, setSlug, ... - named exactly like the useState pairs they replace, so field bindings in
// the JSX don't have to change at all
type Setters<T> = {
  [K in keyof T & string as `set${Capitalize<K>}`]: (next: Updater<T[K]>) => void
}

export type Draft<T> = {
  values: T
  setters: Setters<T>
  set: <K extends keyof T>(key: K, next: Updater<T[K]>) => void
  patch: (next: Partial<T>) => void
  dirty: boolean
  /** What's on screen is what's stored now - re-baselines without touching the values */
  saved: () => void
  /** Throws the edits away, back to the mounted values or to a newer baseline */
  reset: (next?: T) => void
}

function apply<V>(next: Updater<V>, prev: V): V {
  return typeof next === 'function' ? (next as (p: V) => V)(prev) : next
}

export function useDraft<T extends object>(initial: T, label?: string): Draft<T> {
  const [mounted] = useState(initial) // first render's values, the baseline until something saves
  const [baseline, setBaseline] = useState(initial)
  const [values, setValues] = useState(initial)

  const set = useCallback(<K extends keyof T>(key: K, next: Updater<T[K]>) => {
    setValues((prev) => ({ ...prev, [key]: apply(next, prev[key]) }))
  }, [])

  // Keys are fixed for the life of the form, so the setters are built once - joined into a string
  // because the key ARRAY would be a new identity every render
  const keyList = Object.keys(mounted).join('|')
  const setters = useMemo(() => {
    const out: Record<string, (next: unknown) => void> = {}
    for (const key of keyList ? keyList.split('|') : []) {
      out[`set${key.charAt(0).toUpperCase()}${key.slice(1)}`] = (next) =>
        setValues((prev) => ({
          ...prev,
          [key]: apply(next, (prev as Record<string, unknown>)[key]),
        }))
    }
    return out as Setters<T>
  }, [keyList])

  const patch = useCallback((next: Partial<T>) => {
    setValues((prev) => ({ ...prev, ...next }))
  }, [])

  const dirty = JSON.stringify(values) !== JSON.stringify(baseline)
  useUnsavedChangesGuard(dirty, label)

  const saved = useCallback(() => setBaseline(values), [values])
  const reset = useCallback(
    (next?: T) => {
      setValues(next ?? mounted)
      setBaseline(next ?? mounted)
    },
    [mounted],
  )

  return { values, setters, set, patch, dirty, saved, reset }
}
