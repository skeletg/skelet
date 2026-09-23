import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'

// Page-level staged-save registry - THE dashboard convention for detail pages - Every editable
// section stages edits locally, registers a {diff, save} pair, and the header's single Save
// opens SaveConfirmDialog with the combined list - Sections must NOT render their own save
// buttons - see docs/conventions.md "Dashboard save convention" for the two narrow exceptions

export type SectionDiffEntry = {
  label: string
  before: string
  after: string
  kind?: 'change' | 'add' | 'remove'  // 'change' is before -> after, 'add' a green +, 'remove' a struck-through before - set by shared/saving/diff.ts
}

export type SectionRegistration = {
  id: string
  title: string
  diff: SectionDiffEntry[]
  save: () => Promise<void>
}

export type SaveRegistryValue = {
  register: (reg: SectionRegistration) => void
  unregister: (id: string) => void
}

export const SaveRegistryContext = createContext<SaveRegistryValue | null>(null)

export function normalizeDiff(id: string, diff: SectionDiffEntry[]): SectionDiffEntry[] {  // drops no-op change entries so round-tripped form state can't light up Save or pad the dialog; warns in dev
  const kept = diff.filter((d) => (d.kind ?? 'change') !== 'change' || d.before !== d.after)
  if (import.meta.env.DEV && kept.length !== diff.length) {
    console.warn(
      `[saveRegistry] section "${id}" registered ${diff.length - kept.length} no-op diff ` +
        'entr(y/ies) (before === after) - fix its diff computation',
    )
  }
  return kept
}

// `registry` - pass the page's own contextValue when the section lives in the SAME component that
// calls useSaveRegistry: that component renders the Provider, so it can't read it from context -
// without this the section silently registers nowhere and the header Save never lights up
export function useSaveSection(reg: SectionRegistration, registry?: SaveRegistryValue) {  // re-registers when the diff content changes, and removes its entry on unmount
  const fromContext = useContext(SaveRegistryContext)
  const ctx = registry ?? fromContext
  const regRef = useRef(reg)
  // Declared before the register effect so registration reads a fresh ref
  useEffect(() => {
    regRef.current = reg
  })

  const normalized = normalizeDiff(reg.id, reg.diff)
  const diffKey = JSON.stringify(normalized)

  useEffect(() => {
    if (!ctx) return
    // Re-reads the ref at call time: the effect only re-runs when the diff STRING changes, but a
    // draft can change without changing it (clipped values, rich text), and a closure would save stale
    ctx.register({
      ...regRef.current,
      diff: normalizeDiff(regRef.current.id, regRef.current.diff),
      save: () => regRef.current.save(),
    })
  }, [ctx, diffKey, reg.title])

  useEffect(() => {
    if (!ctx) return
    return () => ctx.unregister(reg.id)
  }, [ctx, reg.id])
}

// `label` names the page in the discard prompts - left off, the workspace tab's own title stands in
// `persisted` - the page keeps its edits in a stored draft (usePersistedForm), so only closing its
// workspace tab prompts
export function useSaveRegistry(label?: string, opts: { persisted?: boolean } = {}) {  // page side - owns the section map, the dirty subset for the dialog, and the save runner
  const [sections, setSections] = useState<Record<string, SectionRegistration>>({})
  const register = useCallback((reg: SectionRegistration) => {
    setSections((prev) => ({ ...prev, [reg.id]: reg }))
  }, [])
  const unregister = useCallback((id: string) => {
    setSections((prev) => {
      if (!(id in prev)) return prev
      const next = { ...prev }
      delete next[id]
      return next
    })
  }, [])
  const contextValue = useMemo<SaveRegistryValue>(
    () => ({ register, unregister }),
    [register, unregister],
  )
  const dirtySections = useMemo(
    () => Object.values(sections).filter((s) => s.diff.length > 0),
    [sections],
  )
  // Sequential so a later failure can't hide an earlier success in the UI - throws the first error
  const saveAll = useCallback(
    () =>
      duringCommit(async () => {
        for (const section of Object.values(sections).filter((s) => s.diff.length > 0)) {
          await section.save()
        }
      }),
    [sections],
  )
  // Every staged-save page is a page that can lose work, so the guard is wired HERE rather than
  // per page - closing, refreshing or switching scope with a dirty section now always prompts
  const isDirty = dirtySections.length > 0
  useUnsavedChangesGuard(isDirty, label, opts)
  return { contextValue, dirtySections, isDirty, saveAll }
}
