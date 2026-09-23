import { useCallback, useContext, useEffect, useRef, useState } from 'react'
import { useStore, type AnyFormApi } from '@tanstack/react-form'
import { PrefsContext } from '@/lib/admin-prefs'
import { TabPanelContext } from '@/lib/tab-panel-context'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import {
  clearDraft,
  draftKey,
  readDraft,
  sameValue,
  touchedFields,
  writeDraft,
} from '@/lib/form-drafts'

// Keeps a TanStack form's unsaved work in a stored draft (lib/form-drafts), so a refresh, a
// crashed tab or a closed browser gives the admin back what they typed - in the same workspace tab
//
// Restores once on mount. A field the server ALSO changed since the draft was written is still
// restored (the admin's value wins, it's what they meant to save) but reported as a clash, so the
// page can say what the server holds now. Nothing is ever written to the server - the draft only
// refills the form, and the normal Save and its confirm dialog still apply
//
// TanStack Form's own listeners only fire from field.handleChange, not from setFieldValue or the
// array helpers, so this watches the value store instead and catches every kind of edit

const WRITE_DEBOUNCE_MS = 400

export type DraftClash = { field: string; server: unknown }

export type PersistedFormOptions<T> = {
  /** Page name plus the record id, or 'new' - e.g. `inventory-location:${id}` */
  routeKey: string
  /** What the stored record holds right now - the same value the form's defaultValues come from */
  baseline: T
  /** Top-level fields never written to storage - secrets, tokens, passwords */
  exclude?: readonly string[]
  /** Names the page in the close-tab prompt */
  label?: string
  /** False keeps no draft - for dialogs, whose work is small and short-lived */
  enabled?: boolean
}

export type PersistedForm = {
  /** When the restored draft was written - null when nothing was restored */
  restoredAt: number | null
  clashes: DraftClash[]
  /** Throws the draft and the edits away, back to the baseline */
  discard: () => void
  /** Call once the edits are saved - drops the draft and treats what's on screen as stored */
  saved: () => void
}

export function usePersistedForm<T extends object>(
  form: AnyFormApi,
  { routeKey, baseline, exclude = [], label, enabled = true }: PersistedFormOptions<T>,
): PersistedForm {
  const adminId = useContext(PrefsContext)
  const { id: tabId } = useContext(TabPanelContext)
  // No admin means a bare render (tests, the login shell) - there's no one to keep a draft for
  const key = enabled && adminId ? draftKey(adminId, tabId, routeKey) : null

  const values = useStore(form.store, (s) => s.values as T)
  const excludeKey = exclude.join('|')
  const excluded = excludeKey ? excludeKey.split('|') : []

  // What the stored draft would put back - planned during render (a storage READ), applied to the
  // form in an effect below. Re-planned with the render-time adjust pattern if the key changes
  const [restored, setRestored] = useState(() => planRestore(key, baseline, excluded))
  const [plannedFor, setPlannedFor] = useState(key)
  if (plannedFor !== key) {
    setPlannedFor(key)
    setRestored(planRestore(key, baseline, excluded))
  }

  // After a save the record refetches a beat later - until it lands, what's on screen IS the stored
  // state, so compare against that instead or the draft would be written straight back
  const [savedOver, setSavedOver] = useState<{ baseline: T; values: T } | null>(null)
  const effectiveBaseline = savedOver && savedOver.baseline === baseline ? savedOver.values : baseline

  const touched = touchedFields(effectiveBaseline, values, excluded)
  const dirty = Object.keys(touched).length > 0
  // Marked persisted: refresh, close and scope switches stop prompting - the draft has it
  useUnsavedChangesGuard(dirty && key !== null, label, { persisted: true })

  // Apply - once per key, and before the first write so the mount's untouched values can't
  // overwrite the draft being restored. A draft with nothing left to apply is dropped
  const restoredFor = useRef<string | null>(null)
  const mountValues = useRef(values)
  useEffect(() => {
    if (!key || restoredFor.current === key) return
    restoredFor.current = key
    if (!restored || restored.key !== key) {
      clearDraft(key)
      return
    }
    for (const [field, value] of Object.entries(restored.apply)) form.setFieldValue(field, value)
  }, [key, form, restored])

  // Write - debounced as a trigger only, the payload is always the latest touched set
  const latestTouched = useRef(touched)
  useEffect(() => {
    latestTouched.current = touched
  })
  const settled = useDebounced(values, WRITE_DEBOUNCE_MS)
  useEffect(() => {
    if (!key || restoredFor.current !== key) return
    // The mount's own values are the baseline (or about to be replaced by a restore) - writing
    // them would clear the very draft the restore is reading
    if (settled === mountValues.current && !savedOver) return
    writeDraft(key, latestTouched.current)
  }, [key, settled, effectiveBaseline, savedOver])

  // pagehide beats the debounce: the last keystroke before a refresh still makes it to storage
  useEffect(() => {
    if (!key) return
    const flush = () => {
      if (restoredFor.current === key) writeDraft(key, latestTouched.current)
    }
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [key])

  const discard = useCallback(() => {
    if (key) clearDraft(key)
    setRestored(null)
    form.reset(baseline)
  }, [key, form, baseline])

  const saved = useCallback(() => {
    if (key) clearDraft(key)
    setRestored(null)
    setSavedOver({ baseline, values: form.state.values as T })
  }, [key, form, baseline])

  return {
    restoredAt: restored?.at ?? null,
    clashes: restored?.clashes ?? [],
    discard,
    saved,
  }
}

type RestorePlan = {
  key: string
  at: number
  clashes: DraftClash[]
  apply: Record<string, unknown>
}

// A field the server already holds at the draft's value has nothing to restore; one the server
// moved off the draft's starting value is still restored, but reported as a clash
function planRestore<T extends object>(
  key: string | null,
  baseline: T,
  exclude: readonly string[],
): RestorePlan | null {
  if (!key) return null
  const draft = readDraft(key)
  if (!draft) return null
  const clashes: DraftClash[] = []
  const apply: Record<string, unknown> = {}
  for (const [field, entry] of Object.entries(draft.touched)) {
    if (!(field in baseline) || exclude.includes(field)) continue
    const server = (baseline as Record<string, unknown>)[field]
    if (sameValue(server, entry.value)) continue
    if (!sameValue(server, entry.base)) clashes.push({ field, server })
    apply[field] = entry.value
  }
  if (Object.keys(apply).length === 0) return null
  return { key, at: draft.savedAt, clashes, apply }
}
