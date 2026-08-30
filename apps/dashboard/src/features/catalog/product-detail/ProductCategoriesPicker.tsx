import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ChevronDownIcon,
  ChevronRightIcon,
  FolderIcon,
  FolderOpenIcon,
  SearchIcon,
  StarIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  AddProductToCategoryDocument,
  ProductDocument,
  RemoveProductFromCategoryDocument,
  SetProductPrimaryCategoryDocument,
} from '../Catalog.gql'
import { RootCategoriesDocument, type CategorySummary } from '../Categories.gql'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
import type { ProductCategoryItem } from './shared'

// ---------- Product categories card ----------

type CategoryFlatNode = {
  id: string
  name: string
  slug: string
  depth: number
  hasChildren: boolean
  ancestorIds: string[]
}

// Stable fallback so `roots` (and the memos keyed on it) keeps one identity
// across renders while the query is still loading
const EMPTY_ROOTS: CategorySummary[] = []

function flattenCategoryTree(
  cats: CategorySummary[],
  depth: number,
  ancestorIds: string[],
  out: CategoryFlatNode[],
): void {
  for (const c of cats) {
    out.push({
      id: c.id,
      name: c.name ?? c.slug,
      slug: c.slug,
      depth,
      hasChildren: c.children.length > 0,
      ancestorIds,
    })
    if (c.children.length > 0) {
      flattenCategoryTree(c.children, depth + 1, [...ancestorIds, c.id], out)
    }
  }
}

export function ProductCategoriesPicker({
  productId,
  categories,
  primaryCategory,
  canManage,
  productVariables,
}: {
  productId: string
  categories: ProductCategoryItem[]
  primaryCategory: ProductCategoryItem | null
  canManage: boolean
  productVariables: { id: string }
}) {
  const [search, setSearch] = useState('')
  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set())

  const { data: allCatsData } = useQuery(RootCategoriesDocument, { variables: {} })
  const roots = allCatsData?.rootCategories ?? EMPTY_ROOTS

  const refetchQueries = [{ query: ProductDocument, variables: productVariables }]
  const [addToCategory] = useMutation(AddProductToCategoryDocument, { refetchQueries })
  const [removeFromCategory] = useMutation(RemoveProductFromCategoryDocument, { refetchQueries })
  const [setPrimary] = useMutation(SetProductPrimaryCategoryDocument, { refetchQueries })

  const originalAssignedIds = useMemo(() => new Set(categories.map((c) => c.id)), [categories])
  const originalPrimaryId = primaryCategory?.id ?? null

  // Draft state - clicks only mutate this; the global Save runs the deltas
  const [draftAssigned, setDraftAssigned] = useState<Set<string>>(originalAssignedIds)
  const [draftPrimaryId, setDraftPrimaryId] = useState<string | null>(originalPrimaryId)

  // Re-sync the draft whenever the upstream product refetches (e.g. after save)
  const originalAssignedKey = useMemo(
    () => [...originalAssignedIds].sort().join('|'),
    [originalAssignedIds],
  )
  const [prevAssignedKey, setPrevAssignedKey] = useState(originalAssignedKey)
  const [prevPrimaryId, setPrevPrimaryId] = useState(originalPrimaryId)
  if (prevAssignedKey !== originalAssignedKey || prevPrimaryId !== originalPrimaryId) {
    setPrevAssignedKey(originalAssignedKey)
    setPrevPrimaryId(originalPrimaryId)
    setDraftAssigned(new Set(originalAssignedIds))
    setDraftPrimaryId(originalPrimaryId)
  }

  const flat = useMemo(() => {
    const out: CategoryFlatNode[] = []
    flattenCategoryTree(roots, 0, [], out)
    return out
  }, [roots])

  // Auto-expand ancestors of assigned categories whenever the tree or the draft assignments change (state adjusted
  // during render; expandedIds is still user-editable, so manual collapses stick until the next change)
  const [prevExpandInputs, setPrevExpandInputs] = useState<{
    flat: CategoryFlatNode[]
    assigned: Set<string>
  } | null>(null)
  if (prevExpandInputs?.flat !== flat || prevExpandInputs?.assigned !== draftAssigned) {
    setPrevExpandInputs({ flat, assigned: draftAssigned })
    if (draftAssigned.size > 0) {
      setExpandedIds((prev) => {
        const next = new Set(prev)
        for (const node of flat) {
          if (draftAssigned.has(node.id)) {
            for (const a of node.ancestorIds) next.add(a)
          }
        }
        return next
      })
    }
  }

  const searchExpand = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return null
    const ids = new Set<string>()
    for (const node of flat) {
      if (node.name.toLowerCase().includes(q) || node.slug.toLowerCase().includes(q)) {
        for (const a of node.ancestorIds) ids.add(a)
      }
    }
    return ids
  }, [search, flat])

  const effectiveExpanded = useMemo(
    () => (searchExpand ? new Set([...expandedIds, ...searchExpand]) : expandedIds),
    [searchExpand, expandedIds],
  )

  const visibleNodes = useMemo(() => {
    const q = search.trim().toLowerCase()
    return flat.filter((node) => {
      for (const a of node.ancestorIds) {
        if (!effectiveExpanded.has(a)) return false
      }
      if (!q) return true
      const selfMatch =
        node.name.toLowerCase().includes(q) || node.slug.toLowerCase().includes(q)
      if (selfMatch) return true
      return flat.some(
        (other) =>
          other !== node &&
          other.ancestorIds.includes(node.id) &&
          (other.name.toLowerCase().includes(q) || other.slug.toLowerCase().includes(q)),
      )
    })
  }, [flat, effectiveExpanded, search])

  function toggleExpand(id: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function expandAll() {
    setExpandedIds(new Set(flat.filter((n) => n.hasChildren).map((n) => n.id)))
  }

  function collapseAll() {
    setExpandedIds(new Set())
  }

  function onToggleAssigned(node: CategoryFlatNode) {
    if (!canManage) return
    setDraftAssigned((prev) => {
      const next = new Set(prev)
      if (next.has(node.id)) {
        next.delete(node.id)
        // If the primary is being unassigned, clear it (or pick another)
        if (draftPrimaryId === node.id) {
          const remaining = [...next]
          setDraftPrimaryId(remaining.length > 0 ? remaining[0] : null)
        }
      } else {
        next.add(node.id)
        // First assignment becomes primary by default
        if (next.size === 1 || draftPrimaryId === null) {
          setDraftPrimaryId(node.id)
        }
      }
      return next
    })
  }

  function onSetPrimary(categoryId: string) {
    if (!canManage) return
    if (!draftAssigned.has(categoryId)) return
    setDraftPrimaryId(categoryId)
  }

  // Compute the diff between draft and original
  const toAdd = useMemo(
    () => [...draftAssigned].filter((id) => !originalAssignedIds.has(id)),
    [draftAssigned, originalAssignedIds],
  )
  const toRemove = useMemo(
    () => [...originalAssignedIds].filter((id) => !draftAssigned.has(id)),
    [draftAssigned, originalAssignedIds],
  )
  const primaryChanged = draftPrimaryId !== originalPrimaryId

  const labelById = useMemo(() => {
    const m = new Map<string, string>()
    for (const n of flat) m.set(n.id, n.name)
    for (const c of categories) m.set(c.id, c.name ?? c.slug)
    return m
  }, [flat, categories])

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    for (const id of toAdd) {
      entries.push({ label: 'Assign', before: '-', after: labelById.get(id) ?? id })
    }
    for (const id of toRemove) {
      entries.push({ label: 'Unassign', before: labelById.get(id) ?? id, after: '-' })
    }
    if (primaryChanged) {
      const before = originalPrimaryId ? labelById.get(originalPrimaryId) ?? originalPrimaryId : '-'
      const after = draftPrimaryId ? labelById.get(draftPrimaryId) ?? draftPrimaryId : '-'
      entries.push({ label: 'Primary', before, after })
    }
    return entries
  }, [toAdd, toRemove, primaryChanged, draftPrimaryId, originalPrimaryId, labelById])

  const save = useCallback(async () => {
    // Add new assignments first so a primary swap can target them
    for (const id of toAdd) {
      await addToCategory({
        variables: { productId, categoryId: id, isPrimary: false },
      })
    }
    if (primaryChanged && draftPrimaryId) {
      await setPrimary({ variables: { productId, categoryId: draftPrimaryId } })
    }
    // Remove afterwards so we don't temporarily strip the primary if it's
    // being swapped within the same set
    for (const id of toRemove) {
      await removeFromCategory({ variables: { productId, categoryId: id } })
    }
  }, [toAdd, toRemove, primaryChanged, draftPrimaryId, productId, addToCategory, setPrimary, removeFromCategory])

  useProductSaveSection({ id: 'categories', title: 'Categories', diff, save })

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid gap-0.5">
          <Label className="text-sm font-medium">Categories</Label>
          <p className="text-xs text-muted-foreground">
            Tick a box to assign this product to a category. The star marks the primary category used for canonical URLs. Changes are queued and applied when you click Save.
          </p>
        </div>
        <Badge variant="secondary" className="gap-1">
          {draftAssigned.size} assigned
          {diff.length > 0 ? <span className="text-muted-foreground">· {diff.length} pending</span> : null}
        </Badge>
      </div>

      <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search categories…"
              className="h-8 pl-8 text-xs"
            />
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Button type="button" variant="ghost" size="xs" onClick={expandAll}>
              Expand all
            </Button>
            <Button type="button" variant="ghost" size="xs" onClick={collapseAll}>
              Collapse all
            </Button>
          </div>
        </div>

        {roots.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-xs text-muted-foreground">
            No categories defined yet.
          </div>
        ) : visibleNodes.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-xs text-muted-foreground">
            No matches for &ldquo;{search}&rdquo;
          </div>
        ) : (
          <div className="scrollbar-accent max-h-[26rem] overflow-y-auto rounded-md border">
            <ul className="divide-y">
              {visibleNodes.map((node) => {
                const isAssigned = draftAssigned.has(node.id)
                const isPrimaryNode = draftPrimaryId === node.id
                const isExpanded = effectiveExpanded.has(node.id)
                return (
                  <li
                    key={node.id}
                    className={`relative flex items-center gap-2 py-1.5 pr-3 text-sm transition-colors ${
                      isAssigned ? 'bg-primary/5' : 'hover:bg-muted/50'
                    }`}
                    style={{ paddingLeft: `${node.depth * 1.125 + 0.5}rem` }}
                  >
                    {node.depth > 0
                      ? Array.from({ length: node.depth }).map((_, i) => (
                          <span
                            key={i}
                            aria-hidden
                            className="pointer-events-none absolute top-0 bottom-0 w-px bg-border/60"
                            style={{ left: `${i * 1.125 + 0.875}rem` }}
                          />
                        ))
                      : null}

                    <button
                      type="button"
                      className="grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground disabled:opacity-30"
                      onClick={() => toggleExpand(node.id)}
                      disabled={!node.hasChildren}
                      aria-label={isExpanded ? 'Collapse' : 'Expand'}
                    >
                      {node.hasChildren
                        ? isExpanded
                          ? <ChevronDownIcon className="size-3.5" />
                          : <ChevronRightIcon className="size-3.5" />
                        : null}
                    </button>

                    <Checkbox
                      checked={isAssigned}
                      disabled={!canManage}
                      onCheckedChange={() => onToggleAssigned(node)}
                      aria-label={isAssigned ? `Unassign ${node.name}` : `Assign ${node.name}`}
                    />

                    {isExpanded && node.hasChildren ? (
                      <FolderOpenIcon className={`size-4 shrink-0 ${isAssigned ? 'text-primary' : 'text-muted-foreground'}`} />
                    ) : (
                      <FolderIcon className={`size-4 shrink-0 ${isAssigned ? 'text-primary' : 'text-muted-foreground'}`} />
                    )}

                    <span className={`flex-1 truncate ${isAssigned ? 'font-medium' : ''}`}>
                      {node.name}
                    </span>

                    {isAssigned ? (
                      <button
                        type="button"
                        className={`shrink-0 transition-colors ${
                          isPrimaryNode
                            ? 'text-amber-500'
                            : 'text-muted-foreground/40 hover:text-amber-500'
                        }`}
                        title={isPrimaryNode ? 'Primary category' : 'Set as primary'}
                        onClick={() => onSetPrimary(node.id)}
                        disabled={!canManage || isPrimaryNode}
                      >
                        <StarIcon className={`size-3.5 ${isPrimaryNode ? 'fill-amber-500' : ''}`} />
                      </button>
                    ) : null}
                  </li>
                )
              })}
            </ul>
          </div>
        )}
    </div>
  )
}
