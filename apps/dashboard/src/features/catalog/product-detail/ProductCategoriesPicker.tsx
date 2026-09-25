import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
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
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import {
  PRODUCT_CATEGORY_LABELS,
  productCategoriesDiff,
  productCategoriesSchema,
  productCategoryChanges,
  toggleCategory,
  toProductCategoryValues,
} from '../schemas/product'
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

  const flat = useMemo(() => {
    const out: CategoryFlatNode[] = []
    flattenCategoryTree(roots, 0, [], out)
    return out
  }, [roots])

  const diff = useMemo(() => {
    const names = new Map([...flat.map((n) => [n.id, n.name] as const), ...categories.map((c) => [c.id, c.name ?? c.slug] as const)])
    return productCategoriesDiff((id) => names.get(id) ?? id)
  }, [flat, categories])
  // Clicks only stage; the header Save runs the deltas
  const baseline = useMemo(() => toProductCategoryValues(categories, primaryCategory), [categories, primaryCategory])
  const { form, draft } = useSectionForm({
    id: 'categories',
    title: 'Categories',
    baseline,
    schema: productCategoriesSchema,
    diff,
    routeKey: `product-categories:${productId}`,
    submit: async (v) => {
      const changes = productCategoryChanges(baseline, v)
      // Add new assignments first so a primary swap can target them
      for (const id of changes.add) await addToCategory({ variables: { productId, categoryId: id, isPrimary: false } })
      if (changes.primary) await setPrimary({ variables: { productId, categoryId: changes.primary } })
      // Remove afterwards so we don't temporarily strip the primary if it's being swapped within the same set
      for (const id of changes.remove) await removeFromCategory({ variables: { productId, categoryId: id } })
    },
  })
  const values = useStore(form.store, (s) => s.values)
  const draftAssigned = useMemo(() => new Set(values.assigned), [values.assigned])
  const pending = diff(baseline, values).length

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
    const next = toggleCategory(form.state.values, node.id)
    form.setFieldValue('assigned', next.assigned)
    form.setFieldValue('primaryId', next.primaryId)
  }

  function onSetPrimary(categoryId: string) {
    if (!canManage || !draftAssigned.has(categoryId)) return
    form.setFieldValue('primaryId', categoryId)
  }

  return (
    <div className="grid gap-3">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_CATEGORY_LABELS} />}
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid gap-0.5">
          <Label className="text-sm font-medium">Categories</Label>
          <p className="text-xs text-muted-foreground">
            Tick a box to assign this product to a category. The star marks the primary category used for canonical URLs. Changes are queued and applied when you click Save.
          </p>
        </div>
        <Badge variant="secondary" className="gap-1">
          {draftAssigned.size} assigned
          {pending > 0 ? <span className="text-muted-foreground">· {pending} pending</span> : null}
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
          <div className="scrollbar-accent max-h-104 overflow-y-auto rounded-md border">
            <ul className="divide-y">
              {visibleNodes.map((node) => {
                const isAssigned = draftAssigned.has(node.id)
                const isPrimaryNode = values.primaryId === node.id
                const isExpanded = effectiveExpanded.has(node.id)
                return (
                  <li
                    key={node.id}
                    className={`relative flex items-center gap-2 py-1.5 pr-3 text-sm transition-colors ${
                      isAssigned ? 'bg-muted/60' : 'hover:bg-muted/50'
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
                      <FolderOpenIcon className={`size-4 shrink-0 ${isAssigned ? 'text-foreground' : 'text-muted-foreground'}`} />
                    ) : (
                      <FolderIcon className={`size-4 shrink-0 ${isAssigned ? 'text-foreground' : 'text-muted-foreground'}`} />
                    )}

                    <span className={`flex-1 truncate ${isAssigned ? 'font-medium' : ''}`}>
                      {node.name}
                    </span>

                    {isAssigned ? (
                      <button
                        type="button"
                        className={`shrink-0 transition-colors ${
                          isPrimaryNode
                            ? 'text-warning'
                            : 'text-muted-foreground/40 hover:text-warning'
                        }`}
                        title={isPrimaryNode ? 'Primary category' : 'Set as primary'}
                        onClick={() => onSetPrimary(node.id)}
                        disabled={!canManage || isPrimaryNode}
                      >
                        <StarIcon className={`size-3.5 ${isPrimaryNode ? 'fill-warning' : ''}`} />
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
