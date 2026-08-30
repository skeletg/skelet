import { useCallback, useMemo, useState } from 'react'
import { arrayMove } from '@dnd-kit/sortable'

import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'

// Draft state + diff/payload contract for the storefront-filters editor - split from
// the component file so fast refresh keeps working (hooks and helpers live here)

export type FacetOverrideValue = {
  attributeId: string
  attributeSlug: string
  sortOrder: number
  hidden: boolean
}

export type FacetAttributeCandidate = {
  id: string
  slug: string
  name: string
  sortOrder: number
}

export type FacetRow = {
  attributeId: string
  slug: string
  name: string
  hidden: boolean
}

// Backend default facet order: Attribute.sortOrder ascending, then name
export function sortFacetCandidates(attrs: FacetAttributeCandidate[]): FacetAttributeCandidate[] {
  return [...attrs].sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name))
}

export function useFacetOverridesDraft(
  overrides: FacetOverrideValue[],
  candidates: FacetAttributeCandidate[],
) {
  const buildInitial = useCallback((): { rows: FacetRow[]; customized: boolean } => {
    const overriddenIds = new Set(overrides.map((o) => o.attributeId))
    const overridden = [...overrides]
      .sort((a, b) => a.sortOrder - b.sortOrder)
      .flatMap((o) => {
        const attr = candidates.find((a) => a.id === o.attributeId)
        // A row for an attribute that is no longer filterable/public is stale - drop it
        return attr
          ? [{ attributeId: attr.id, slug: attr.slug, name: attr.name, hidden: o.hidden }]
          : []
      })
    const rest = candidates
      .filter((a) => !overriddenIds.has(a.id))
      .map((a) => ({ attributeId: a.id, slug: a.slug, name: a.name, hidden: false }))
    return { rows: [...overridden, ...rest], customized: overrides.length > 0 }
  }, [overrides, candidates])

  const [rows, setRows] = useState<FacetRow[]>([])
  const [customized, setCustomized] = useState(false)
  // Rebuild when the server data or attribute list changes (state adjusted during render)
  const [prevBuild, setPrevBuild] = useState<typeof buildInitial | null>(null)
  if (prevBuild !== buildInitial) {
    setPrevBuild(() => buildInitial)
    const init = buildInitial()
    setRows(init.rows)
    setCustomized(init.customized)
  }

  const reorder = useCallback((activeId: string, overId: string) => {
    setRows((prev) => {
      const from = prev.findIndex((r) => r.attributeId === activeId)
      const to = prev.findIndex((r) => r.attributeId === overId)
      if (from < 0 || to < 0 || from === to) return prev
      return arrayMove(prev, from, to)
    })
    setCustomized(true)
  }, [])

  const toggleHidden = useCallback((attributeId: string) => {
    setRows((prev) =>
      prev.map((r) => (r.attributeId === attributeId ? { ...r, hidden: !r.hidden } : r)),
    )
    setCustomized(true)
  }, [])

  const reset = useCallback(() => {
    setRows(
      candidates.map((a) => ({ attributeId: a.id, slug: a.slug, name: a.name, hidden: false })),
    )
    setCustomized(false)
  }, [candidates])

  const signature = (rws: FacetRow[], custom: boolean) =>
    custom ? rws.map((r) => `${r.attributeId}${r.hidden ? '!' : ''}`).join(',') : 'default'
  const baseline = useMemo(() => {
    const init = buildInitial()
    return signature(init.rows, init.customized)
  }, [buildInitial])
  const dirty = signature(rows, customized) !== baseline

  const sectionDiff = useMemo<SectionDiffEntry[]>(() => {
    if (!dirty) return []
    const summarize = (rws: FacetRow[], custom: boolean) =>
      custom
        ? rws.map((r) => (r.hidden ? `(${r.name})` : r.name)).join(' → ')
        : 'default order, all visible'
    const init = buildInitial()
    return [
      {
        label: 'Storefront filters',
        before: summarize(init.rows, init.customized),
        after: summarize(rows, customized),
      },
    ]
  }, [dirty, rows, customized, buildInitial])

  // Mutation payload: full list order when customized, empty list = reset to defaults
  const payload = useMemo(
    () =>
      customized ? rows.map((r) => ({ attributeId: r.attributeId, hidden: r.hidden })) : [],
    [rows, customized],
  )

  return { rows, customized, dirty, sectionDiff, payload, reorder, toggleHidden, reset }
}

