import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { CategoryOptionsDocument } from './CategoryOptions.gql'

// The pickers' view of the category tree, built on the client from the flat categoryOptions list -
// so it reaches any depth, and every picker shares the one cached fetch

export type CategoryOptionRow = {
  id: string
  parentId?: string | null
  slug: string
  name?: string | null
  position: number
  isPublished: boolean
  productCount: number
}

export type CategoryNode = CategoryOptionRow & {
  label: string
  depth: number
  // Labels from the root down to this node, itself included
  path: string[]
  ancestorIds: string[]
  childIds: string[]
}

export type CategoryIndex = {
  byId: ReadonlyMap<string, CategoryNode>
  // Every node in tree order - parents before children, siblings by position
  ordered: CategoryNode[]
}

export const EMPTY_INDEX: CategoryIndex = { byId: new Map(), ordered: [] }

export const PATH_SEPARATOR = ' › '

export function buildCategoryIndex(rows: readonly CategoryOptionRow[]): CategoryIndex {
  const rowsById = new Map(rows.map((r) => [r.id, r]))
  const children = new Map<string | null, CategoryOptionRow[]>()
  for (const r of rows) {
    // A parent missing from the list (a store-scoped list without it) roots the row instead
    const parent = r.parentId && rowsById.has(r.parentId) ? r.parentId : null
    const list = children.get(parent) ?? []
    list.push(r)
    children.set(parent, list)
  }
  for (const list of children.values()) {
    list.sort((a, b) => a.position - b.position || a.slug.localeCompare(b.slug))
  }

  const byId = new Map<string, CategoryNode>()
  const ordered: CategoryNode[] = []
  const walk = (row: CategoryOptionRow, ancestors: CategoryNode[]) => {
    // A parent cycle never reaches a root, and is picked up below - this guards a row seen twice
    if (byId.has(row.id)) return
    const label = row.name || row.slug
    const node: CategoryNode = {
      ...row,
      label,
      depth: ancestors.length,
      path: [...ancestors.map((a) => a.label), label],
      ancestorIds: ancestors.map((a) => a.id),
      childIds: [],
    }
    byId.set(row.id, node)
    ordered.push(node)
    const kids = children.get(row.id) ?? []
    for (const kid of kids) walk(kid, [...ancestors, node])
    node.childIds = kids.filter((k) => byId.get(k.id)?.ancestorIds.includes(row.id)).map((k) => k.id)
  }
  for (const root of children.get(null) ?? []) walk(root, [])
  // Rows caught in a parent cycle hang off no root - list them as roots rather than lose them
  for (const r of rows) if (!byId.has(r.id)) walk(r, [])
  return { byId, ordered }
}

export function pathOf(index: CategoryIndex, id: string): string[] {
  return index.byId.get(id)?.path ?? []
}

export function formatPath(index: CategoryIndex, id: string, fallback?: string): string {
  const path = pathOf(index, id)
  return path.length ? path.join(PATH_SEPARATOR) : (fallback ?? id)
}

// The node and every category beneath it - what a parent picker must not offer
export function subtreeOf(index: CategoryIndex, id: string): Set<string> {
  const out = new Set<string>()
  const visit = (at: string) => {
    if (out.has(at)) return
    out.add(at)
    for (const kid of index.byId.get(at)?.childIds ?? []) visit(kid)
  }
  visit(id)
  return out
}

export function matchesQuery(node: CategoryNode, q: string): boolean {
  return node.label.toLowerCase().includes(q) || node.slug.toLowerCase().includes(q)
}

// Flat hits in tree order, so results under one parent stay together
export function searchCategories(index: CategoryIndex, query: string): CategoryNode[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  return index.ordered.filter((n) => matchesQuery(n, q))
}

export function useCategoryIndex() {
  const { data, loading, error } = useQuery(CategoryOptionsDocument, { fetchPolicy: 'cache-first' })
  const index = useMemo(() => (data ? buildCategoryIndex(data.categoryOptions) : EMPTY_INDEX), [data])
  return { index, loading: loading && !data, error }
}
