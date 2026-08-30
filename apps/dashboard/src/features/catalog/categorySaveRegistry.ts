import { createContext, useContext, useEffect, useRef } from 'react'

// Mirror of productSaveRegistry scoped to the category form, so the two feature areas don't share
// state - sections register a (diff, save) pair and confirming runs each dirty save sequentially

export type SectionDiffEntry = {
  label: string
  before: string
  after: string
}

export type SectionRegistration = {
  id: string
  title: string
  diff: SectionDiffEntry[]
  save: () => Promise<void>
}

export type CategorySaveContextValue = {
  register: (reg: SectionRegistration) => void
  unregister: (id: string) => void
}

export const CategorySaveContext = createContext<CategorySaveContextValue | null>(null)

/** Re-registers whenever the diff content changes, and removes its entry on unmount */
export function useCategorySaveSection(reg: SectionRegistration) {
  const ctx = useContext(CategorySaveContext)
  const regRef = useRef(reg)
  useEffect(() => {
    regRef.current = reg
  })

  const diffKey = JSON.stringify(reg.diff)

  useEffect(() => {
    if (!ctx) return
    ctx.register(regRef.current)
  }, [ctx, diffKey, reg.title])

  useEffect(() => {
    if (!ctx) return
    return () => ctx.unregister(reg.id)
  }, [ctx, reg.id])
}
