import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
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
import { changed } from './diff'
import type { SectionDiffEntry } from './saveRegistry'

// Keeps a TanStack form's unsaved work in a stored draft (lib/form-drafts), so a refresh, a
// crashed tab or a closed browser gives the admin back what they typed - in the same workspace tab
//
// A draft found on mount is OFFERED, never applied behind the admin's back: the page shows what it
// would change (RestoredDraftBanner, with the section's own diff rows) and the admin restores or
// discards it. A field the server ALSO changed since the draft was written is listed as a clash -
// restoring keeps the admin's value, and the page can still say what the server holds now. Nothing
// is ever written to the server - a restore only refills the form, and the normal Save still applies
//
// While a draft is on offer nothing is written, so a reload before deciding offers it again. An
// edit made without deciding drops the offer, and the new work becomes the draft
//
// TanStack Form's own listeners only fire from field.handleChange, not from setFieldValue or the
// array helpers, so this watches the value store instead and catches every kind of edit

const WRITE_DEBOUNCE_MS = 400

export type DraftClash = { field: string; server: unknown }

export type DraftOffer = {
  /** When the draft was written */
  at: number
  /** What restoring would change, as the Save dialog would list it */
  rows: SectionDiffEntry[]
  clashes: DraftClash[]
}

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
  /** The section's diff - lists the offer the way the Save dialog lists changes */
  diff?: (baseline: T, values: T) => SectionDiffEntry[]
}

export type PersistedForm = {
  /** A stored draft waiting for the admin's decision - null when there is none */
  offer: DraftOffer | null
  /** Puts the offered draft into the form - it still needs the page's Save */
  restore: () => void
  /** The restored fields the server moved meanwhile - for the note under each input */
  clashes: DraftClash[]
  /** Throws the draft (offered or restored) and the edits away, back to the baseline */
  discard: () => void
  /** Call once the edits are saved - drops the draft and treats what's on screen as stored */
  saved: () => void
}

export function usePersistedForm<T extends object>(
  form: AnyFormApi,
  { routeKey, baseline, exclude = [], label, enabled = true, diff }: PersistedFormOptions<T>,
): PersistedForm {
  const adminId = useContext(PrefsContext)
  const { id: tabId } = useContext(TabPanelContext)
  // No admin means a bare render (tests, the login shell) - there's no one to keep a draft for
  const key = enabled && adminId ? draftKey(adminId, tabId, routeKey) : null

  const values = useStore(form.store, (s) => s.values as T)
  const excludeKey = exclude.join('|')
  const excluded = excludeKey ? excludeKey.split('|') : []

  // What the stored draft would put back - planned during render (a storage READ) and held as the
  // offer until the admin decides. Re-planned with the render-time adjust pattern if the key changes
  const [plan, setPlan] = useState(() => planRestore(key, baseline, excluded))
  const [plannedFor, setPlannedFor] = useState(key)
  if (plannedFor !== key) {
    setPlannedFor(key)
    setPlan(planRestore(key, baseline, excluded))
  }
  const [clashes, setClashes] = useState<DraftClash[]>([])

  // After a save the record refetches a beat later - until it lands, what's on screen IS the stored
  // state, so compare against that instead or the draft would be written straight back
  const [savedOver, setSavedOver] = useState<{ baseline: T; values: T } | null>(null)
  const effectiveBaseline = savedOver && savedOver.baseline === baseline ? savedOver.values : baseline

  const touched = touchedFields(effectiveBaseline, values, excluded)
  const dirty = Object.keys(touched).length > 0
  // Marked persisted: refresh, close and scope switches stop prompting - the draft has it
  useUnsavedChangesGuard(dirty && key !== null, label, { persisted: true })

  // An edit made while the offer is open is the admin moving on - the offer goes, and the new work
  // is what gets stored. A restore clears the plan itself before its values land
  const offered = plan !== null && plan.key === key
  if (offered && dirty) setPlan(null)

  // Once per key: a draft with nothing left to offer (the server caught up) is dropped
  const checkedFor = useRef<string | null>(null)
  const mountValues = useRef(values)
  useEffect(() => {
    if (!key || checkedFor.current === key) return
    checkedFor.current = key
    if (!offered) clearDraft(key)
  }, [key, offered])

  // Write - debounced as a trigger only, the payload is always the latest touched set. Nothing is
  // written while a draft is on offer, or the mount's untouched values would wipe it
  const latestTouched = useRef(touched)
  const offerOpen = useRef(offered)
  useEffect(() => {
    latestTouched.current = touched
    offerOpen.current = offered
  })
  const settled = useDebounced(values, WRITE_DEBOUNCE_MS)
  useEffect(() => {
    if (!key || checkedFor.current !== key || offerOpen.current) return
    // The mount's own values are the baseline - writing them would only clear storage
    if (settled === mountValues.current && !savedOver) return
    writeDraft(key, latestTouched.current)
  }, [key, settled, effectiveBaseline, savedOver])

  // pagehide beats the debounce: the last keystroke before a refresh still makes it to storage
  useEffect(() => {
    if (!key) return
    const flush = () => {
      if (checkedFor.current === key && !offerOpen.current) writeDraft(key, latestTouched.current)
    }
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [key])

  const restore = useCallback(() => {
    if (!plan) return
    setPlan(null)
    setClashes(plan.clashes)
    for (const [field, value] of Object.entries(plan.apply)) form.setFieldValue(field, value)
  }, [plan, form])

  const discard = useCallback(() => {
    if (key) clearDraft(key)
    setPlan(null)
    setClashes([])
    form.reset(baseline)
  }, [key, form, baseline])

  const saved = useCallback(() => {
    if (key) clearDraft(key)
    setPlan(null)
    setClashes([])
    setSavedOver({ baseline, values: form.state.values as T })
  }, [key, form, baseline])

  const offer = useMemo<DraftOffer | null>(() => {
    if (!offered || !plan) return null
    const listed = diff ? diff(baseline, { ...baseline, ...plan.apply } as T) : []
    // Create pages review nothing (their diff is empty) - one row per restored field, keyed by field
    // name for the banner's labels to name
    const rows = listed.length
      ? listed
      : Object.keys(plan.apply).flatMap((f) => changed(f, (baseline as Record<string, unknown>)[f], plan.apply[f]) ?? [])
    return { at: plan.at, rows, clashes: plan.clashes }
  }, [offered, plan, baseline, diff])

  return { offer, restore, clashes, discard, saved }
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
